package logging

import (
	"context"
	"os"
	"path/filepath"
	"sync"
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

func TestTailFileBackoffDoublesAndCaps(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	if err := os.WriteFile(logFile, []byte("existing\n"), 0644); err != nil {
		t.Fatalf("creating log file: %v", err)
	}

	var mu sync.Mutex
	var waits []time.Duration

	afterFunc := func(d time.Duration) <-chan time.Time {
		mu.Lock()
		waits = append(waits, d)
		mu.Unlock()
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}

	ctx, cancel := context.WithCancel(context.Background())
	lines := make(chan string, 100)

	errCh := make(chan error, 1)
	go func() {
		errCh <- tailFile(ctx, logFile, lines, afterFunc)
	}()

	// Let it poll enough times to hit the cap and stay there
	for {
		mu.Lock()
		n := len(waits)
		mu.Unlock()
		if n >= 8 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	<-errCh

	mu.Lock()
	defer mu.Unlock()

	expected := []time.Duration{
		250 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		4 * time.Second, // cap
		4 * time.Second,
		4 * time.Second,
		4 * time.Second,
	}

	for i, want := range expected {
		if i >= len(waits) {
			t.Fatalf("only got %d waits, expected at least %d", len(waits), len(expected))
		}
		if waits[i] != want {
			t.Errorf("wait[%d]: got %v, want %v", i, waits[i], want)
		}
	}
}

func TestTailFileBackoffResetsOnNewData(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("creating log file: %v", err)
	}

	// Gate each poll: afterFunc records the wait, then blocks until released.
	// This gives the test control over exactly when each poll proceeds.
	var mu sync.Mutex
	var waits []time.Duration
	gate := make(chan struct{}, 1)

	afterFunc := func(d time.Duration) <-chan time.Time {
		mu.Lock()
		waits = append(waits, d)
		mu.Unlock()
		// Wait for the test to release this poll
		<-gate
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lines := make(chan string, 100)

	errCh := make(chan error, 1)
	go func() {
		errCh <- tailFile(ctx, logFile, lines, afterFunc)
	}()

	// Release 3 empty polls so backoff grows: 250ms, 500ms, 1s
	for i := 0; i < 3; i++ {
		gate <- struct{}{}
		// Wait for the poll to complete and the next wait to be recorded
		for {
			mu.Lock()
			n := len(waits)
			mu.Unlock()
			if n > i+1 {
				break
			}
			if n == i+1 {
				// The i-th gate released, next afterFunc call may not have happened yet
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	// At this point the tailer is blocked in afterFunc waiting for gate.
	// waits[0]=250ms, waits[1]=500ms, waits[2]=1s, and a 4th afterFunc call
	// is blocked. Write data, then release the gate so the scanner picks it up.
	if _, err := f.WriteString("new data\n"); err != nil {
		t.Fatalf("writing line: %v", err)
	}
	gate <- struct{}{}

	select {
	case line := <-lines:
		if line != "new data" {
			t.Errorf("got %q, want %q", line, "new data")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for line")
	}

	// Now let one more empty poll through so we can check the reset value
	mu.Lock()
	resetIdx := len(waits)
	mu.Unlock()

	// Wait for the next afterFunc call to be recorded (it's blocked on gate)
	for {
		mu.Lock()
		n := len(waits)
		mu.Unlock()
		if n > resetIdx {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	// Unblock any pending afterFunc so tailFile can exit
	select {
	case gate <- struct{}{}:
	default:
	}
	<-errCh

	mu.Lock()
	defer mu.Unlock()

	if waits[resetIdx] != tailMinWait {
		t.Errorf("wait after data: got %v, want %v (backoff should reset)", waits[resetIdx], tailMinWait)
	}

	f.Close()
}
