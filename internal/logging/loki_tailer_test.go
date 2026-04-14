package logging

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailerReadsExistingLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	os.WriteFile(logPath, []byte("line one\nline two\nline three\n"), 0644)

	pos := NewPositionStore(filepath.Join(dir, "positions.json"))
	lines := make(chan TaggedLine, 100)
	tailer := NewLokiTailer("test_sink", logPath, pos, lines)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go tailer.Run(ctx)

	var got []string
	for i := 0; i < 3; i++ {
		select {
		case tl := <-lines:
			got = append(got, tl.Line)
			if tl.Sink != "test_sink" {
				t.Errorf("sink: got %q, want %q", tl.Sink, "test_sink")
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for line %d, got %d lines so far", i+1, len(got))
		}
	}

	cancel()

	want := []string{"line one", "line two", "line three"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line %d: got %q, want %q", i, got[i], w)
		}
	}
}

func TestTailerSkipsEmptyLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	os.WriteFile(logPath, []byte("first\n\n\nsecond\n"), 0644)

	pos := NewPositionStore(filepath.Join(dir, "positions.json"))
	lines := make(chan TaggedLine, 100)
	tailer := NewLokiTailer("test", logPath, pos, lines)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go tailer.Run(ctx)

	var got []string
	for i := 0; i < 2; i++ {
		select {
		case tl := <-lines:
			got = append(got, tl.Line)
		case <-ctx.Done():
			t.Fatalf("timed out, got %d lines", len(got))
		}
	}

	cancel()

	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if got[0] != "first" || got[1] != "second" {
		t.Errorf("unexpected lines: %v", got)
	}
}

func TestTailerResumesFromSavedOffset(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	content := "line one\nline two\nline three\n"
	os.WriteFile(logPath, []byte(content), 0644)

	// Pre-set position past "line one\n" (9 bytes)
	pos := NewPositionStore(filepath.Join(dir, "positions.json"))
	pos.Set(logPath, 9)

	lines := make(chan TaggedLine, 100)
	tailer := NewLokiTailer("test", logPath, pos, lines)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go tailer.Run(ctx)

	var got []string
	for i := 0; i < 2; i++ {
		select {
		case tl := <-lines:
			got = append(got, tl.Line)
		case <-ctx.Done():
			t.Fatalf("timed out, got %d lines", len(got))
		}
	}

	cancel()

	want := []string{"line two", "line three"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line %d: got %q, want %q", i, got[i], w)
		}
	}
}

func TestTailerUpdatesPositionStore(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	os.WriteFile(logPath, []byte("hello world\n"), 0644)

	pos := NewPositionStore(filepath.Join(dir, "positions.json"))
	lines := make(chan TaggedLine, 100)
	tailer := NewLokiTailer("test", logPath, pos, lines)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tailer.Run(ctx)
		close(done)
	}()

	select {
	case <-lines:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for line")
	}

	cancel()
	<-done

	offset := pos.Get(logPath)
	if offset <= 0 {
		t.Errorf("expected positive offset after reading, got %d", offset)
	}
}

func TestTailerDetectsFileRotation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	os.WriteFile(logPath, []byte("old line one\nold line two\n"), 0644)

	// Set offset beyond what a new file would have
	pos := NewPositionStore(filepath.Join(dir, "positions.json"))
	pos.Set(logPath, 1000)

	lines := make(chan TaggedLine, 100)
	tailer := NewLokiTailer("test", logPath, pos, lines)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go tailer.Run(ctx)

	// Should re-read from beginning since file is smaller than saved offset
	var got []string
	for i := 0; i < 2; i++ {
		select {
		case tl := <-lines:
			got = append(got, tl.Line)
		case <-ctx.Done():
			t.Fatalf("timed out, got %d lines", len(got))
		}
	}

	cancel()

	if len(got) != 2 {
		t.Fatalf("expected 2 lines after rotation detection, got %d", len(got))
	}
	if got[0] != "old line one" {
		t.Errorf("expected to re-read from start, got %q", got[0])
	}
}

func TestTailerWaitsForFileCreation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "delayed.log")

	pos := NewPositionStore(filepath.Join(dir, "positions.json"))
	lines := make(chan TaggedLine, 100)
	tailer := NewLokiTailer("test", logPath, pos, lines)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go tailer.Run(ctx)

	// File doesn't exist yet, tailer should be polling
	time.Sleep(500 * time.Millisecond)

	// Now create the file
	os.WriteFile(logPath, []byte("delayed line\n"), 0644)

	select {
	case tl := <-lines:
		if tl.Line != "delayed line" {
			t.Errorf("got %q, want %q", tl.Line, "delayed line")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for line after file creation")
	}

	cancel()
}

func TestTailerDetectsTruncationDuringTail(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	os.WriteFile(logPath, []byte("first line that is long enough\n"), 0644)

	pos := NewPositionStore(filepath.Join(dir, "positions.json"))
	lines := make(chan TaggedLine, 100)
	tailer := NewLokiTailer("test", logPath, pos, lines)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	go tailer.Run(ctx)

	select {
	case tl := <-lines:
		if tl.Line != "first line that is long enough" {
			t.Fatalf("got %q, want %q", tl.Line, "first line that is long enough")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for first line")
	}

	os.WriteFile(logPath, []byte("after truncate\n"), 0644)

	select {
	case tl := <-lines:
		if tl.Line != "after truncate" {
			t.Fatalf("got %q, want %q", tl.Line, "after truncate")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for line after truncation")
	}

	cancel()
}

func TestTailerAppendsAfterIdlePeriod(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	os.WriteFile(logPath, []byte("initial\n"), 0644)

	pos := NewPositionStore(filepath.Join(dir, "positions.json"))
	lines := make(chan TaggedLine, 100)
	tailer := NewLokiTailer("test", logPath, pos, lines)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	go tailer.Run(ctx)

	select {
	case tl := <-lines:
		if tl.Line != "initial" {
			t.Fatalf("got %q, want %q", tl.Line, "initial")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for initial line")
	}

	time.Sleep(5 * time.Second)

	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("failed to open file for append: %v", err)
	}
	f.WriteString("late arrival\n")
	f.Close()

	select {
	case tl := <-lines:
		if tl.Line != "late arrival" {
			t.Fatalf("got %q, want %q", tl.Line, "late arrival")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for late arrival")
	}

	cancel()
}

func TestTailerStopsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	os.WriteFile(logPath, []byte("line\n"), 0644)

	pos := NewPositionStore(filepath.Join(dir, "positions.json"))
	lines := make(chan TaggedLine, 100)
	tailer := NewLokiTailer("test", logPath, pos, lines)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		tailer.Run(ctx)
		close(done)
	}()

	// Drain the line
	select {
	case <-lines:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out reading line")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tailer did not stop after context cancel")
	}
}
