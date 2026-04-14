package logging

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestBatcherFlushesOnInterval(t *testing.T) {
	lines := make(chan TaggedLine, 100)
	batches := make(chan LokiBatch, 10)

	batcher := NewLokiBatcher(lines, batches, 1024*1024, 200*time.Millisecond, map[string]string{"job": "kaji"})

	ctx, cancel := context.WithCancel(context.Background())
	go batcher.Run(ctx)

	lines <- TaggedLine{Sink: "default", Line: `{"msg":"hello"}`}

	select {
	case batch := <-batches:
		if len(batch.Streams) != 1 {
			t.Fatalf("expected 1 stream, got %d", len(batch.Streams))
		}
		if len(batch.Streams[0].Entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(batch.Streams[0].Entries))
		}
		if batch.Streams[0].Entries[0].Line != `{"msg":"hello"}` {
			t.Errorf("unexpected line: %q", batch.Streams[0].Entries[0].Line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for batch flush")
	}

	cancel()
}

func TestBatcherFlushesOnSizeThreshold(t *testing.T) {
	lines := make(chan TaggedLine, 100)
	batches := make(chan LokiBatch, 10)

	// Tiny batch size so a single line triggers a flush
	batcher := NewLokiBatcher(lines, batches, 1, 10*time.Second, map[string]string{"job": "kaji"})

	ctx, cancel := context.WithCancel(context.Background())
	go batcher.Run(ctx)

	lines <- TaggedLine{Sink: "default", Line: "some log line"}

	select {
	case batch := <-batches:
		if len(batch.Streams) != 1 {
			t.Fatalf("expected 1 stream, got %d", len(batch.Streams))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for size-triggered flush")
	}

	cancel()
}

func TestBatcherGroupsBySink(t *testing.T) {
	lines := make(chan TaggedLine, 100)
	batches := make(chan LokiBatch, 10)

	batcher := NewLokiBatcher(lines, batches, 1024*1024, 200*time.Millisecond, map[string]string{"job": "kaji"})

	ctx, cancel := context.WithCancel(context.Background())
	go batcher.Run(ctx)

	lines <- TaggedLine{Sink: "access", Line: "access log 1"}
	lines <- TaggedLine{Sink: "errors", Line: "error log 1"}
	lines <- TaggedLine{Sink: "access", Line: "access log 2"}

	select {
	case batch := <-batches:
		if len(batch.Streams) != 2 {
			t.Fatalf("expected 2 streams, got %d", len(batch.Streams))
		}

		sinkEntries := make(map[string]int)
		for _, stream := range batch.Streams {
			sink := stream.Labels["sink"]
			sinkEntries[sink] = len(stream.Entries)
		}

		if sinkEntries["access"] != 2 {
			t.Errorf("access stream: expected 2 entries, got %d", sinkEntries["access"])
		}
		if sinkEntries["errors"] != 1 {
			t.Errorf("errors stream: expected 1 entry, got %d", sinkEntries["errors"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for batch")
	}

	cancel()
}

func TestBatcherAppliesStaticLabels(t *testing.T) {
	lines := make(chan TaggedLine, 100)
	batches := make(chan LokiBatch, 10)

	labels := map[string]string{"job": "kaji", "env": "test"}
	batcher := NewLokiBatcher(lines, batches, 1024*1024, 200*time.Millisecond, labels)

	ctx, cancel := context.WithCancel(context.Background())
	go batcher.Run(ctx)

	lines <- TaggedLine{Sink: "default", Line: "test"}

	select {
	case batch := <-batches:
		stream := batch.Streams[0]
		if stream.Labels["job"] != "kaji" {
			t.Errorf("job label: got %q, want %q", stream.Labels["job"], "kaji")
		}
		if stream.Labels["env"] != "test" {
			t.Errorf("env label: got %q, want %q", stream.Labels["env"], "test")
		}
		if stream.Labels["sink"] != "default" {
			t.Errorf("sink label: got %q, want %q", stream.Labels["sink"], "default")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}

	cancel()
}

func TestBatcherAssignsTimestamps(t *testing.T) {
	lines := make(chan TaggedLine, 100)
	batches := make(chan LokiBatch, 10)

	batcher := NewLokiBatcher(lines, batches, 1024*1024, 200*time.Millisecond, map[string]string{"job": "kaji"})

	ctx, cancel := context.WithCancel(context.Background())
	go batcher.Run(ctx)

	before := time.Now().UnixNano()
	lines <- TaggedLine{Sink: "default", Line: "test"}

	select {
	case batch := <-batches:
		ts := batch.Streams[0].Entries[0].Timestamp
		if ts == "" {
			t.Fatal("timestamp should not be empty")
		}
		// Timestamp should be a valid nanosecond epoch string
		var parsed int64
		for _, c := range ts {
			if c < '0' || c > '9' {
				t.Fatalf("timestamp %q contains non-digit characters", ts)
			}
			parsed = parsed*10 + int64(c-'0')
		}
		after := time.Now().UnixNano()
		if parsed < before || parsed > after {
			t.Errorf("timestamp %d outside expected range [%d, %d]", parsed, before, after)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}

	cancel()
}

func TestBatcherFlushesPendingOnCancel(t *testing.T) {
	lines := make(chan TaggedLine, 100)
	batches := make(chan LokiBatch, 10)

	batcher := NewLokiBatcher(lines, batches, 1024*1024, 10*time.Minute, map[string]string{"job": "kaji"})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		batcher.Run(ctx)
		close(done)
	}()

	lines <- TaggedLine{Sink: "default", Line: "pending line"}

	// Give batcher time to receive the line before cancelling
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("batcher did not stop")
	}

	// Batcher has exited. Check if a batch was flushed.
	select {
	case batch := <-batches:
		if len(batch.Streams) == 0 || len(batch.Streams[0].Entries) == 0 {
			t.Error("expected pending entries to be flushed on cancel")
		}
	default:
		t.Error("expected a batch to be flushed on context cancel")
	}
}

func TestBatcherFlushesPendingOnChannelClose(t *testing.T) {
	lines := make(chan TaggedLine, 100)
	batches := make(chan LokiBatch, 10)

	batcher := NewLokiBatcher(lines, batches, 1024*1024, 10*time.Minute, map[string]string{"job": "kaji"})

	done := make(chan struct{})
	go func() {
		batcher.Run(context.Background())
		close(done)
	}()

	lines <- TaggedLine{Sink: "default", Line: "pending line"}

	time.Sleep(50 * time.Millisecond)
	close(lines)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("batcher did not stop after lines channel closed")
	}

	select {
	case batch := <-batches:
		if len(batch.Streams) == 0 || len(batch.Streams[0].Entries) == 0 {
			t.Error("expected pending entries to be flushed on channel close")
		}
		if batch.Streams[0].Entries[0].Line != "pending line" {
			t.Errorf("unexpected line: %q", batch.Streams[0].Entries[0].Line)
		}
	default:
		t.Error("expected a batch to be flushed when lines channel is closed")
	}

	// batches channel should be closed since Run exited
	select {
	case _, ok := <-batches:
		if ok {
			t.Error("expected batches channel to be closed")
		}
	default:
		t.Error("batches channel should be closed after Run returns")
	}
}

func TestBatcherEmptyFlush(t *testing.T) {
	lines := make(chan TaggedLine, 100)
	batches := make(chan LokiBatch, 10)

	batcher := NewLokiBatcher(lines, batches, 1024*1024, 100*time.Millisecond, map[string]string{"job": "kaji"})

	ctx, cancel := context.WithCancel(context.Background())
	go batcher.Run(ctx)

	// Wait longer than flush interval with no lines
	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case <-batches:
		t.Error("should not produce a batch when no lines were received")
	default:
	}
}

func TestMarshalLokiBatchFormat(t *testing.T) {
	batch := LokiBatch{
		Streams: []LokiStream{
			{
				Labels: map[string]string{"job": "kaji", "sink": "access"},
				Entries: []LokiEntry{
					{Timestamp: "1700000000000000000", Line: `{"level":"info"}`},
					{Timestamp: "1700000000000000001", Line: `{"level":"warn"}`},
				},
			},
		},
	}

	data, err := MarshalLokiBatch(batch)
	if err != nil {
		t.Fatalf("MarshalLokiBatch: %v", err)
	}

	var parsed struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(parsed.Streams))
	}

	stream := parsed.Streams[0]
	if stream.Stream["job"] != "kaji" {
		t.Errorf("stream job label: got %q, want %q", stream.Stream["job"], "kaji")
	}
	if stream.Stream["sink"] != "access" {
		t.Errorf("stream sink label: got %q, want %q", stream.Stream["sink"], "access")
	}
	if len(stream.Values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(stream.Values))
	}
	if stream.Values[0][0] != "1700000000000000000" {
		t.Errorf("first timestamp: got %q", stream.Values[0][0])
	}
	if stream.Values[0][1] != `{"level":"info"}` {
		t.Errorf("first line: got %q", stream.Values[0][1])
	}
}

func TestMarshalLokiBatchMultipleStreams(t *testing.T) {
	batch := LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "a"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "a1"}},
			},
			{
				Labels:  map[string]string{"sink": "b"},
				Entries: []LokiEntry{{Timestamp: "2", Line: "b1"}},
			},
		},
	}

	data, err := MarshalLokiBatch(batch)
	if err != nil {
		t.Fatalf("MarshalLokiBatch: %v", err)
	}

	var parsed struct {
		Streams []json.RawMessage `json:"streams"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed.Streams) != 2 {
		t.Errorf("expected 2 streams, got %d", len(parsed.Streams))
	}
}

func TestMarshalLokiBatchEmpty(t *testing.T) {
	batch := LokiBatch{Streams: []LokiStream{}}

	data, err := MarshalLokiBatch(batch)
	if err != nil {
		t.Fatalf("MarshalLokiBatch: %v", err)
	}

	var parsed struct {
		Streams []json.RawMessage `json:"streams"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Streams) != 0 {
		t.Errorf("expected 0 streams, got %d", len(parsed.Streams))
	}
}
