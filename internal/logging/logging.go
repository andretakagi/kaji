// logging tails a log file in real time, sending new lines to a channel
// with exponential backoff when waiting for new data.
package logging

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

func TailFile(ctx context.Context, path string, lines chan<- string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening log file %s: %w", path, err)
	}
	defer f.Close()

	// Seek to end so we only get new lines
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seeking to end of log file: %w", err)
	}

	const (
		minWait = 250 * time.Millisecond
		maxWait = 4 * time.Second
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	wait := minWait
	for {
		if scanner.Scan() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case lines <- scanner.Text():
			}
			wait = minWait
			continue
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("scanning log file: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		if wait < maxWait {
			wait *= 2
		}
	}
}

func ReadRecent(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", path, err)
	}
	defer f.Close()

	rs, err := newReverseScanner(f)
	if err != nil {
		return nil, fmt.Errorf("initializing reverse scanner: %w", err)
	}

	var lines []string
	for rs.Scan() && len(lines) < n {
		lines = append(lines, string(rs.Bytes()))
	}

	// Reverse to chronological order
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines, nil
}
