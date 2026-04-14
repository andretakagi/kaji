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

	wait := minWait
	for {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case lines <- scanner.Text():
			}
			wait = minWait
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

