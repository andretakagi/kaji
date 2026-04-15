package logging

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"syscall"
	"time"
)

const tailerRetryInterval = 2 * time.Second

type TaggedLine struct {
	Sink string
	Line string
}

type LokiTailer struct {
	sink           string
	path           string
	pos            *PositionStore
	lines          chan<- TaggedLine
	offset         int64
	afterFunc      func(time.Duration) <-chan time.Time
	onScanComplete func() // test hook: called after scanner finishes, before inode check
}

func NewLokiTailer(sink, path string, pos *PositionStore, lines chan<- TaggedLine) *LokiTailer {
	return &LokiTailer{
		sink:      sink,
		path:      path,
		pos:       pos,
		lines:     lines,
		afterFunc: time.After,
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
		case <-t.afterFunc(tailerRetryInterval):
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
		case <-t.afterFunc(tailerRetryInterval):
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
		t.pos.Set(t.path, 0)
	}

	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
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
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if t.onScanComplete != nil {
		t.onScanComplete()
	}

	fdInfo, err := f.Stat()
	if err != nil {
		return err
	}
	if fdInfo.Size() < t.offset {
		log.Printf("loki tailer [%s]: file truncated during tail, resetting to start", t.sink)
		t.offset = 0
		t.pos.Set(t.path, 0)
		return nil
	}

	pathInfo, err := os.Stat(t.path)
	if err != nil {
		return nil
	}
	if fileInode(fdInfo) != fileInode(pathInfo) {
		log.Printf("loki tailer [%s]: file rotated (inode changed), resetting to start", t.sink)
		t.offset = 0
		t.pos.Set(t.path, 0)
	}

	return nil
}

func fileInode(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}
