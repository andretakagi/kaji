package logging

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	pos := NewPositionStore(filepath.Join(t.TempDir(), "positions.json"))
	pusher := NewLokiPusher(server.URL, "", "", batches, pos)

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
	pusher := NewLokiPusher(server.URL, "secret-token", "", batches, nil)

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
	pusher := NewLokiPusher(server.URL, "", "my-tenant", batches, nil)

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
	pusher := NewLokiPusher(server.URL, "", "", batches, nil)

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
	pos := NewPositionStore(filepath.Join(t.TempDir(), "positions.json"))
	pusher := NewLokiPusher(server.URL, "", "", batches, pos)

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
	pusher := NewLokiPusher(server.URL, "", "", batches, nil)

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
	pusher := NewLokiPusher(server.URL, "", "", batches, nil)

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

func TestPusherSavesPositionsOnSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	posPath := filepath.Join(dir, "positions.json")
	pos := NewPositionStore(posPath)
	pos.Set("/var/log/caddy/access.log", 9999)

	batches := make(chan LokiBatch, 1)
	pusher := NewLokiPusher(server.URL, "", "", batches, pos)

	ctx, cancel := context.WithCancel(context.Background())
	go pusher.Run(ctx)

	batches <- LokiBatch{
		Streams: []LokiStream{
			{
				Labels:  map[string]string{"sink": "access"},
				Entries: []LokiEntry{{Timestamp: "1", Line: "log"}},
			},
		},
	}

	time.Sleep(500 * time.Millisecond)
	cancel()

	// Verify positions were saved to disk
	pos2 := NewPositionStore(posPath)
	if err := pos2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := pos2.Get("/var/log/caddy/access.log"); got != 9999 {
		t.Errorf("saved position: got %d, want 9999", got)
	}
}

func TestPusherAccumulatesEntryCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	batches := make(chan LokiBatch, 10)
	pusher := NewLokiPusher(server.URL, "", "", batches, nil)

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
	pusher := NewLokiPusher(server.URL, "", "", batches, nil)

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

func TestSendTestEntrySuccess(t *testing.T) {
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gz, _ := gzip.NewReader(r.Body)
		gotBody, _ = io.ReadAll(gz)
		gz.Close()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	pusher := NewLokiPusher("", "", "", nil, nil)
	if err := pusher.SendTestEntry(server.URL, "", ""); err != nil {
		t.Fatalf("SendTestEntry: %v", err)
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

	pusher := NewLokiPusher("", "", "", nil, nil)
	if err := pusher.SendTestEntry(server.URL, "my-token", "my-tenant"); err != nil {
		t.Fatalf("SendTestEntry: %v", err)
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

	pusher := NewLokiPusher("", "", "", nil, nil)
	err := pusher.SendTestEntry(server.URL, "", "")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestSendTestEntryConnectionRefused(t *testing.T) {
	pusher := NewLokiPusher("", "", "", nil, nil)
	err := pusher.SendTestEntry("http://127.0.0.1:1", "", "")
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}
