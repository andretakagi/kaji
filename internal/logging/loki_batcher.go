package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type LokiEntry struct {
	Timestamp string
	Line      string
}

type LokiStream struct {
	Labels  map[string]string
	Entries []LokiEntry
}

type LokiBatch struct {
	Streams []LokiStream
}

type LokiBatcher struct {
	lines         <-chan TaggedLine
	batches       chan<- LokiBatch
	batchSize     int
	flushInterval time.Duration
	staticLabels  map[string]string
}

func NewLokiBatcher(
	lines <-chan TaggedLine,
	batches chan<- LokiBatch,
	batchSize int,
	flushInterval time.Duration,
	staticLabels map[string]string,
) *LokiBatcher {
	return &LokiBatcher{
		lines:         lines,
		batches:       batches,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		staticLabels:  staticLabels,
	}
}

func (b *LokiBatcher) Run(ctx context.Context) {
	streams := make(map[string]*LokiStream)
	currentSize := 0
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(streams) == 0 {
			return
		}
		batch := b.buildBatch(streams)
		select {
		case b.batches <- batch:
		case <-ctx.Done():
			return
		}
		streams = make(map[string]*LokiStream)
		currentSize = 0
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case line, ok := <-b.lines:
			if !ok {
				flush()
				return
			}
			stream, exists := streams[line.Sink]
			if !exists {
				labels := make(map[string]string, len(b.staticLabels)+1)
				for k, v := range b.staticLabels {
					labels[k] = v
				}
				labels["sink"] = line.Sink
				stream = &LokiStream{Labels: labels}
				streams[line.Sink] = stream
			}
			entry := LokiEntry{
				Timestamp: strconv.FormatInt(time.Now().UnixNano(), 10),
				Line:      line.Line,
			}
			stream.Entries = append(stream.Entries, entry)
			currentSize += len(line.Line) + 20
			if currentSize >= b.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (b *LokiBatcher) buildBatch(streams map[string]*LokiStream) LokiBatch {
	batch := LokiBatch{Streams: make([]LokiStream, 0, len(streams))}
	for _, stream := range streams {
		batch.Streams = append(batch.Streams, *stream)
	}
	return batch
}

func MarshalLokiBatch(batch LokiBatch) ([]byte, error) {
	type jsonEntry [2]string
	type jsonStream struct {
		Stream map[string]string `json:"stream"`
		Values []jsonEntry       `json:"values"`
	}
	type jsonPush struct {
		Streams []jsonStream `json:"streams"`
	}

	push := jsonPush{Streams: make([]jsonStream, 0, len(batch.Streams))}
	for _, s := range batch.Streams {
		js := jsonStream{
			Stream: s.Labels,
			Values: make([]jsonEntry, 0, len(s.Entries)),
		}
		for _, e := range s.Entries {
			js.Values = append(js.Values, jsonEntry{e.Timestamp, e.Line})
		}
		push.Streams = append(push.Streams, js)
	}

	data, err := json.Marshal(push)
	if err != nil {
		return nil, fmt.Errorf("marshaling loki batch: %w", err)
	}
	return data, nil
}
