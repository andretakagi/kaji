package logging

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const lokiPushPath = "/loki/api/v1/push"

const (
	initialBackoff = 500 * time.Millisecond
	maxBackoff     = 5 * time.Minute
)

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > maxBackoff {
		next = maxBackoff
	}
	return next
}

func normalizeLokiEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(endpoint, "/")
	if !strings.HasSuffix(endpoint, lokiPushPath) {
		endpoint += lokiPushPath
	}
	return endpoint
}

type SinkStatus struct {
	Tailing       bool      `json:"tailing"`
	LastPushAt    time.Time `json:"last_push_at"`
	EntriesPushed int64     `json:"entries_pushed"`
	LastError     string    `json:"last_error"`
}

type LokiPusher struct {
	endpoint    string
	bearerToken string
	tenantID    string
	batches     <-chan LokiBatch
	client      *http.Client
	afterFunc   func(time.Duration) <-chan time.Time

	mu     sync.RWMutex
	status map[string]*SinkStatus
}

func NewLokiPusher(
	endpoint, bearerToken, tenantID string,
	batches <-chan LokiBatch,
) *LokiPusher {
	return &LokiPusher{
		endpoint:    normalizeLokiEndpoint(endpoint),
		bearerToken: bearerToken,
		tenantID:    tenantID,
		batches:     batches,
		client:      &http.Client{Timeout: 30 * time.Second},
		afterFunc:   time.After,
		status:      make(map[string]*SinkStatus),
	}
}

func (p *LokiPusher) Run(ctx context.Context) {
	for {
		select {
		case batch, ok := <-p.batches:
			if !ok {
				return
			}
			p.pushWithRetry(ctx, batch)
		case <-ctx.Done():
			// Drain any batches already in the channel before exiting.
			for {
				select {
				case batch, ok := <-p.batches:
					if !ok {
						return
					}
					p.pushWithRetry(ctx, batch)
				default:
					return
				}
			}
		}
	}
}

func (p *LokiPusher) pushWithRetry(ctx context.Context, batch LokiBatch) {
	const maxRetries = 10

	body, err := p.encodeBatch(batch)
	if err != nil {
		log.Printf("loki pusher: encode error: %v", err)
		p.recordError(batch, err.Error())
		return
	}

	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return
		}

		statusCode, err := p.sendRequest(ctx, body)
		if err == nil && statusCode == http.StatusNoContent {
			p.recordSuccess(batch)
			return
		}

		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		} else {
			errMsg = fmt.Sprintf("loki returned status %d", statusCode)
		}

		if statusCode == http.StatusBadRequest {
			log.Printf("loki pusher: dropping batch, bad request: %s", errMsg)
			p.recordError(batch, errMsg)
			return
		}

		if attempt == maxRetries {
			log.Printf("loki pusher: dropping batch after %d retries: %s", maxRetries, errMsg)
			p.recordError(batch, errMsg)
			return
		}

		log.Printf("loki pusher: attempt %d failed (%s), retrying in %v", attempt+1, errMsg, backoff)
		select {
		case <-ctx.Done():
			return
		case <-p.afterFunc(backoff):
		}
		backoff = nextBackoff(backoff)
	}
}

func (p *LokiPusher) encodeBatch(batch LokiBatch) ([]byte, error) {
	jsonData, err := MarshalLokiBatch(batch)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(jsonData); err != nil {
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

func (p *LokiPusher) sendRequest(ctx context.Context, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	if p.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.bearerToken)
	}
	if p.tenantID != "" {
		req.Header.Set("X-Scope-OrgID", p.tenantID)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func (p *LokiPusher) recordSuccess(batch LokiBatch) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for _, stream := range batch.Streams {
		sink := stream.Labels["sink"]
		if sink == "" {
			continue
		}
		s := p.getOrCreateStatus(sink)
		s.LastPushAt = now
		s.EntriesPushed += int64(len(stream.Entries))
		s.LastError = ""
	}
}

func (p *LokiPusher) recordError(batch LokiBatch, errMsg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, stream := range batch.Streams {
		sink := stream.Labels["sink"]
		if sink == "" {
			continue
		}
		s := p.getOrCreateStatus(sink)
		s.LastError = errMsg
	}
}

func (p *LokiPusher) getOrCreateStatus(sink string) *SinkStatus {
	s, ok := p.status[sink]
	if !ok {
		s = &SinkStatus{}
		p.status[sink] = s
	}
	return s
}

func (p *LokiPusher) GetStatus() map[string]SinkStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make(map[string]SinkStatus, len(p.status))
	for k, v := range p.status {
		result[k] = *v
	}
	return result
}

func SendLokiTestEntry(endpoint, bearerToken, tenantID string) error {
	type jsonEntry [2]string
	type jsonStream struct {
		Stream map[string]string `json:"stream"`
		Values []jsonEntry       `json:"values"`
	}
	type jsonPush struct {
		Streams []jsonStream `json:"streams"`
	}

	push := jsonPush{
		Streams: []jsonStream{{
			Stream: map[string]string{"job": "kaji", "sink": "test"},
			Values: []jsonEntry{{
				fmt.Sprintf("%d", time.Now().UnixNano()),
				"Kaji test entry - Loki connection verified",
			}},
		}},
	}

	jsonData, err := json.Marshal(push)
	if err != nil {
		return fmt.Errorf("marshaling test entry: %w", err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(jsonData); err != nil {
		return fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("gzip close: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeLokiEndpoint(endpoint), &buf)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	if tenantID != "" {
		req.Header.Set("X-Scope-OrgID", tenantID)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("loki returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
