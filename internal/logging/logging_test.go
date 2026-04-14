package logging

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailFileReturnsErrorForMissingFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines := make(chan string, 10)
	err := TailFile(ctx, "/tmp/nonexistent-kaji-test-file.log", lines)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestTailFileFollowsAppendedLines(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("creating log file: %v", err)
	}

	// Write initial content before TailFile starts (should be skipped since it
	// seeks to end).
	if _, err := f.WriteString("pre-existing line\n"); err != nil {
		t.Fatalf("writing initial content: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines := make(chan string, 100)
	errCh := make(chan error, 1)
	go func() {
		errCh <- TailFile(ctx, logFile, lines)
	}()

	// Give TailFile time to open and seek
	time.Sleep(500 * time.Millisecond)

	// Append lines after TailFile has started
	if _, err := f.WriteString("line one\n"); err != nil {
		t.Fatalf("appending line: %v", err)
	}
	if _, err := f.WriteString("line two\n"); err != nil {
		t.Fatalf("appending line: %v", err)
	}

	// Collect lines with a timeout
	var got []string
	timeout := time.After(5 * time.Second)
	for len(got) < 2 {
		select {
		case line := <-lines:
			got = append(got, line)
		case <-timeout:
			t.Fatalf("timed out waiting for lines, got %d: %v", len(got), got)
		}
	}

	if got[0] != "line one" {
		t.Errorf("expected first line to be %q, got %q", "line one", got[0])
	}
	if got[1] != "line two" {
		t.Errorf("expected second line to be %q, got %q", "line two", got[1])
	}

	// Should not have received the pre-existing line
	select {
	case extra := <-lines:
		t.Errorf("unexpected extra line: %q", extra)
	default:
	}

	f.Close()
}

func TestTailFileStopsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	if err := os.WriteFile(logFile, []byte("existing\n"), 0644); err != nil {
		t.Fatalf("creating log file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	lines := make(chan string, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- TailFile(ctx, logFile, lines)
	}()

	// Give it time to start polling
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for TailFile to return after cancel")
	}
}
