package logging

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPusherSendsGzippedJSON(t *testing.T) {
	var gotBody []byte
	var gotHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Errorf("gzip reader: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer gz.Close()
		gotBody, _ = io.ReadAll(gz)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"job": "kaji", "sink": "test"},
				Entries: []LokiEntry{{Timestamp: "1700000000000000000", Line: "test log"}},
			},
		},
	}

	// Give pusher time to process
	time.Sleep(500 * time.Millisecond)
	cancel()

	if gotHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", gotHeaders.Get("Content-Type"))
	}
	if gotHeaders.Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding: got %q, want gzip", gotHeaders.Get("Content-Encoding"))
	}

	var parsed struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(parsed.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(parsed.Streams))
	}
	if parsed.Streams[0].Values[0][1] != "test log" {
		t.Errorf("log line: got %q, want %q", parsed.Streams[0].Values[0][1], "test log")
	}
}

func TestPusherSendsBearerToken(t *testing.T) {
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "secret-token", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "test"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "test"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization: got %q, want %q", gotAuth, "Bearer secret-token")
	}
}

func TestPusherSendsTenantID(t *testing.T) {
	var gotTenant string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = r.Header.Get("X-Scope-OrgID")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "my-tenant", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "test"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "test"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	if gotTenant != "my-tenant" {
		t.Errorf("X-Scope-OrgID: got %q, want %q", gotTenant, "my-tenant")
	}
}

func TestPusherOmitsAuthHeadersWhenEmpty(t *testing.T) {
	var hasAuth, hasTenant bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasAuth = r.Header.Get("Authorization") != ""
		hasTenant = r.Header.Get("X-Scope-OrgID") != ""
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "test"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "test"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	if hasAuth {
		t.Error("Authorization header should be absent when bearerToken is empty")
	}
	if hasTenant {
		t.Error("X-Scope-OrgID header should be absent when tenantID is empty")
	}
}

func TestPusherRecordsSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "access"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "a"}, {Timestamp: "2", Line: "b"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	status := pusher.GetStatus()
	s, ok := status["access"]
	if !ok {
		t.Fatal("expected status for sink 'access'")
	}
	if s.EntriesPushed != 2 {
		t.Errorf("EntriesPushed: got %d, want 2", s.EntriesPushed)
	}
	if s.LastPushAt.IsZero() {
		t.Error("LastPushAt should be set")
	}
	if s.LastError != "" {
		t.Errorf("LastError should be empty, got %q", s.LastError)
	}
}

func TestPusherRecordsErrorOnBadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "test"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "bad"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	status := pusher.GetStatus()
	s, ok := status["test"]
	if !ok {
		t.Fatal("expected status for sink 'test'")
	}
	if s.LastError == "" {
		t.Error("LastError should be set after 400 response")
	}
	if s.EntriesPushed != 0 {
		t.Errorf("EntriesPushed should be 0, got %d", s.EntriesPushed)
	}
}

func TestPusherDropsBatchOnBadRequestNoRetry(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "test"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "bad"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	count := atomic.LoadInt32(&requestCount)
	if count != 1 {
		t.Errorf("expected exactly 1 request (no retries on 400), got %d", count)
	}
}

func TestPusherAccumulatesEntryCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 10)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	// Send two batches for the same sink
	for i := 0; i < 2; i++ {
		batches <- LokiBatch{
			Streams: []LokiStream{
				{
					Labels:  map[string]string{"sink": "access"},
					Entries: []LokiEntry{{Timestamp: "1", Line: "log"}},
				},
			},
		}
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	status := pusher.GetStatus()
	if status["access"].EntriesPushed != 2 {
		t.Errorf("EntriesPushed: got %d, want 2", status["access"].EntriesPushed)
	}
}

func TestPusherGetStatusReturnsSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "a"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "log"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	snap1 := pusher.GetStatus()
	snap2 := pusher.GetStatus()

	// Modifying snap1 should not affect snap2
	snap1["a"] = SinkStatus{EntriesPushed: 999}
	if snap2["a"].EntriesPushed == 999 {
		t.Error("GetStatus should return independent snapshots")
	}
}

func TestPusherRetryThenSucceed(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "retry-test"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "will succeed"}},
			},
		},
	}

	// Wait for retry loop to complete (500ms + 1s backoff + request time)
	time.Sleep(3 * time.Second)
	cancel()

	count := atomic.LoadInt32(&attempts)
	if count != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", count)
	}

	status := pusher.GetStatus()
	s, ok := status["retry-test"]
	if !ok {
		t.Fatal("expected status for sink 'retry-test'")
	}
	if s.LastError != "" {
		t.Errorf("LastError should be empty after success, got %q", s.LastError)
	}
	if s.EntriesPushed != 1 {
		t.Errorf("EntriesPushed: got %d, want 1", s.EntriesPushed)
	}
}

func TestPusherRetriesOnServerError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		pusher.Run(ctx)
		close(done)
	}()

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "error-test"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "will fail"}},
			},
		},
	}

	// Let it retry for 3 seconds, then cancel and close batches
	time.Sleep(3 * time.Second)
	cancel()
	close(batches)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("pusher did not stop after cancellation")
	}

	count := atomic.LoadInt32(&attempts)
	if count < 3 {
		t.Errorf("expected at least 3 retry attempts, got %d", count)
	}

	status := pusher.GetStatus()
	s := status["error-test"]
	if s.EntriesPushed != 0 {
		t.Errorf("EntriesPushed should be 0, got %d", s.EntriesPushed)
	}
}

func TestPusherRetryCancelledDuringBackoff(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		pusher.Run(ctx)
		close(done)
	}()

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "cancel-test"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "cancel me"}},
			},
		},
	}

	// Wait for the first attempt to complete, then cancel during backoff and close batches
	time.Sleep(1 * time.Second)
	cancel()
	close(batches)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("pusher did not stop within 3 seconds after cancellation")
	}

	count := atomic.LoadInt32(&attempts)
	if count < 1 {
		t.Errorf("expected at least 1 attempt, got %d", count)
	}
}

func TestPusherDropsBatchAfterMaxRetries(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)
	pusher.afterFunc = func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		pusher.Run(ctx)
		close(done)
	}()

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "exhaust-test"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "will exhaust retries"}},
			},
		},
	}

	// Close batches so Run exits after processing the batch
	close(batches)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("pusher did not finish after exhausting retries")
	}
	cancel()

	// 1 initial attempt + 10 retries = 11 total
	count := atomic.LoadInt32(&attempts)
	if count != 11 {
		t.Errorf("expected exactly 11 attempts (1 + 10 retries), got %d", count)
	}

	status := pusher.GetStatus()
	s, ok := status["exhaust-test"]
	if !ok {
		t.Fatal("expected status for sink 'exhaust-test'")
	}
	if s.LastError == "" {
		t.Error("expected LastError to be set after retry exhaustion")
	}
	if s.EntriesPushed != 0 {
		t.Errorf("EntriesPushed should be 0, got %d", s.EntriesPushed)
	}
}

func TestPusherRecordsErrorForAllSinksInBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "a"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "line from a"}},
			},
			{
				Labels:  map[string]string{"sink": "b"},
				Entries: []LokiEntry{{Timestamp: "2", Line: "line from b"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	status := pusher.GetStatus()
	for _, sink := range []string{"a", "b"} {
		s, ok := status[sink]
		if !ok {
			t.Errorf("expected status for sink %q", sink)
			continue
		}
		if s.LastError == "" {
			t.Errorf("sink %q: LastError should be set after 400 response", sink)
		}
		if s.EntriesPushed != 0 {
			t.Errorf("sink %q: EntriesPushed should be 0, got %d", sink, s.EntriesPushed)
		}
	}
}

func TestPusherRecordsSuccessForAllSinksInBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "a"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "a1"}, {Timestamp: "2", Line: "a2"}},
			},
			{
				Labels:  map[string]string{"sink": "b"},
				Entries: []LokiEntry{{Timestamp: "3", Line: "b1"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	status := pusher.GetStatus()
	for _, tc := range []struct {
		sink  string
		count int64
	}{{"a", 2}, {"b", 1}} {
		s, ok := status[tc.sink]
		if !ok {
			t.Errorf("expected status for sink %q", tc.sink)
			continue
		}
		if s.EntriesPushed != tc.count {
			t.Errorf("sink %q: EntriesPushed got %d, want %d", tc.sink, s.EntriesPushed, tc.count)
		}
		if s.LastPushAt.IsZero() {
			t.Errorf("sink %q: LastPushAt should be set", tc.sink)
		}
		if s.LastError != "" {
			t.Errorf("sink %q: LastError should be empty, got %q", tc.sink, s.LastError)
		}
	}
}

func TestBackoffDoublesAndCaps(t *testing.T) {
	b := initialBackoff

	expected := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
		64 * time.Second,
		128 * time.Second,
		256 * time.Second,
		maxBackoff,
	}

	for i, want := range expected {
		b = nextBackoff(b)
		if b != want {
			t.Errorf("step %d: got %v, want %v", i+1, b, want)
		}
	}

	// After hitting the cap, further calls should stay at maxBackoff
	b = nextBackoff(b)
	if b != maxBackoff {
		t.Errorf("beyond cap: got %v, want %v", b, maxBackoff)
	}
}

func TestNormalizeLokiEndpoint(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://localhost:3100", "http://localhost:3100/loki/api/v1/push"},
		{"http://localhost:3100/", "http://localhost:3100/loki/api/v1/push"},
		{"http://localhost:3100///", "http://localhost:3100/loki/api/v1/push"},
		{"http://localhost:3100/loki/api/v1/push", "http://localhost:3100/loki/api/v1/push"},
		{"http://localhost:3100/loki/api/v1/push/", "http://localhost:3100/loki/api/v1/push"},
		{"https://loki.example.com", "https://loki.example.com/loki/api/v1/push"},
		{"https://loki.example.com/custom/prefix", "https://loki.example.com/custom/prefix/loki/api/v1/push"},
	}

	for _, tt := range tests {
		got := normalizeLokiEndpoint(tt.input)
		if got != tt.want {
			t.Errorf("normalizeLokiEndpoint(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPusherRecordSuccessSkipsNoSinkLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"job": "kaji"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "no sink"}},
			},
			{
				Labels:  map[string]string{"sink": "real"},
				Entries: []LokiEntry{{Timestamp: "2", Line: "has sink"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	status := pusher.GetStatus()
	if _, ok := status[""]; ok {
		t.Error("should not create a status entry for empty sink")
	}
	s, ok := status["real"]
	if !ok {
		t.Fatal("expected status for sink 'real'")
	}
	if s.EntriesPushed != 1 {
		t.Errorf("EntriesPushed: got %d, want 1", s.EntriesPushed)
	}
}

func TestPusherRecordErrorSkipsNoSinkLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"job": "kaji"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "no sink"}},
			},
			{
				Labels:  map[string]string{"sink": "real"},
				Entries: []LokiEntry{{Timestamp: "2", Line: "has sink"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	status := pusher.GetStatus()
	if _, ok := status[""]; ok {
		t.Error("should not create a status entry for empty sink")
	}
	s, ok := status["real"]
	if !ok {
		t.Fatal("expected status for sink 'real'")
	}
	if s.LastError == "" {
		t.Error("expected LastError to be set for sink 'real'")
	}
}

func TestSendTestEntrySuccess(t *testing.T) {
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gz, _ := gzip.NewReader(r.Body)
		gotBody, _ = io.ReadAll(gz)
		gz.Close()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := SendLokiTestEntry(server.URL, "", ""); err != nil {
		t.Fatalf("SendLokiTestEntry: %v", err)
	}

	var parsed struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(parsed.Streams))
	}
	if parsed.Streams[0].Stream["sink"] != "test" {
		t.Errorf("sink label: got %q, want %q", parsed.Streams[0].Stream["sink"], "test")
	}
	if parsed.Streams[0].Values[0][1] != "Kaji test entry - Loki connection verified" {
		t.Errorf("test line: got %q", parsed.Streams[0].Values[0][1])
	}
}

func TestSendTestEntryWithAuth(t *testing.T) {
	var gotAuth, gotTenant string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotTenant = r.Header.Get("X-Scope-OrgID")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := SendLokiTestEntry(server.URL, "my-token", "my-tenant"); err != nil {
		t.Fatalf("SendLokiTestEntry: %v", err)
	}

	if gotAuth != "Bearer my-token" {
		t.Errorf("Authorization: got %q, want %q", gotAuth, "Bearer my-token")
	}
	if gotTenant != "my-tenant" {
		t.Errorf("X-Scope-OrgID: got %q, want %q", gotTenant, "my-tenant")
	}
}

func TestSendTestEntryServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	err := SendLokiTestEntry(server.URL, "", "")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestSendTestEntryConnectionRefused(t *testing.T) {
	err := SendLokiTestEntry("http://127.0.0.1:1", "", "")
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}
