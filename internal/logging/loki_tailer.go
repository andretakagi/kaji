package logging

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"time"
)

type TaggedLine struct {
	Sink string
	Line string
}

type LokiTailer struct {
	sink   string
	path   string
	pos    *PositionStore
	lines  chan<- TaggedLine
	offset int64
}

func NewLokiTailer(sink, path string, pos *PositionStore, lines chan<- TaggedLine) *LokiTailer {
	return &LokiTailer{
		sink:  sink,
		path:  path,
		pos:   pos,
		lines: lines,
	}
}

func (t *LokiTailer) Run(ctx context.Context) {
	t.offset = t.pos.Get(t.path)

	for {
		if err := t.tailFile(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("loki tailer [%s]: %v, retrying...", t.sink, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (t *LokiTailer) tailFile(ctx context.Context) error {
	for {
		_, err := os.Stat(t.path)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	f, err := os.Open(t.path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < t.offset {
		log.Printf("loki tailer [%s]: file truncated, resetting to start", t.sink)
		t.offset = 0
	}

	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return err
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
			line := scanner.Text()
			if line == "" {
				continue
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case t.lines <- TaggedLine{Sink: t.sink, Line: line}:
			}
			pos, err := f.Seek(0, io.SeekCurrent)
			if err != nil {
				return err
			}
			t.offset = pos
			t.pos.Set(t.path, pos)
			wait = minWait
			continue
		}
		if err := scanner.Err(); err != nil {
			return err
		}

		info, err := f.Stat()
		if err != nil {
			return err
		}
		if info.Size() < t.offset {
			log.Printf("loki tailer [%s]: file truncated during tail, restarting", t.sink)
			return nil
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
