package logging

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andretakagi/kaji/internal/config"
)

func newTestPipelineConfig(endpoint string, sinks []string) *config.ConfigStore {
	return config.NewStore(&config.AppConfig{
		Loki: config.LokiConfig{
			Enabled:              true,
			Endpoint:             endpoint,
			BatchSize:            1024,
			FlushIntervalSeconds: 1,
			Sinks:                sinks,
		},
	})
}

func TestPipelineNewHasCorrectInitialState(t *testing.T) {
	store := newTestPipelineConfig("http://localhost:3100/loki/api/v1/push", []string{"access"})
	posPath := filepath.Join(t.TempDir(), "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": "/tmp/access.log"} }

	p := NewLokiPipeline(store, posPath, resolver)

	if p.IsRunning() {
		t.Error("expected IsRunning() to be false after construction")
	}

	running, sinks := p.GetStatus()
	if running {
		t.Error("expected GetStatus() running to be false after construction")
	}
	if sinks != nil {
		t.Errorf("expected GetStatus() sinks to be nil after construction, got %v", sinks)
	}

	if p.GetPusher() != nil {
		t.Error("expected GetPusher() to be nil after construction")
	}
}

func TestPipelineStartNoOpWhenDisabled(t *testing.T) {
	store := config.NewStore(&config.AppConfig{
		Loki: config.LokiConfig{
			Enabled:              false,
			Endpoint:             "http://localhost:3100/loki/api/v1/push",
			BatchSize:            1024,
			FlushIntervalSeconds: 1,
			Sinks:                []string{"access"},
		},
	})
	posPath := filepath.Join(t.TempDir(), "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": "/tmp/access.log"} }

	p := NewLokiPipeline(store, posPath, resolver)
	p.Start()

	if p.IsRunning() {
		t.Error("expected IsRunning() to be false when Loki is disabled")
	}
}

func TestPipelineStartNoOpWhenEndpointEmpty(t *testing.T) {
	store := config.NewStore(&config.AppConfig{
		Loki: config.LokiConfig{
			Enabled:              true,
			Endpoint:             "",
			BatchSize:            1024,
			FlushIntervalSeconds: 1,
			Sinks:                []string{"access"},
		},
	})
	posPath := filepath.Join(t.TempDir(), "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": "/tmp/access.log"} }

	p := NewLokiPipeline(store, posPath, resolver)
	p.Start()

	if p.IsRunning() {
		t.Error("expected IsRunning() to be false when endpoint is empty")
	}
}

func TestPipelineStartAndStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	logFile := filepath.Join(dir, "access.log")
	if err := os.WriteFile(logFile, []byte("line one\nline two\n"), 0644); err != nil {
		t.Fatalf("writing log file: %v", err)
	}

	store := newTestPipelineConfig(server.URL, []string{"access"})
	posPath := filepath.Join(dir, "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": logFile} }

	p := NewLokiPipeline(store, posPath, resolver)
	p.Start()

	if !p.IsRunning() {
		t.Fatal("expected IsRunning() to be true after Start()")
	}
	if p.GetPusher() == nil {
		t.Error("expected GetPusher() to be non-nil after Start()")
	}
	running, _ := p.GetStatus()
	if !running {
		t.Error("expected GetStatus() running to be true after Start()")
	}

	time.Sleep(3 * time.Second)

	p.Stop()

	if p.IsRunning() {
		t.Error("expected IsRunning() to be false after Stop()")
	}
	if p.GetPusher() != nil {
		t.Error("expected GetPusher() to be nil after Stop()")
	}
	running, sinks := p.GetStatus()
	if running {
		t.Error("expected GetStatus() running to be false after Stop()")
	}
	if sinks != nil {
		t.Errorf("expected GetStatus() sinks to be nil after Stop(), got %v", sinks)
	}
}

func TestPipelineDoubleStartIsNoOp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	logFile := filepath.Join(dir, "access.log")
	if err := os.WriteFile(logFile, []byte("log line\n"), 0644); err != nil {
		t.Fatalf("writing log file: %v", err)
	}

	store := newTestPipelineConfig(server.URL, []string{"access"})
	posPath := filepath.Join(dir, "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": logFile} }

	p := NewLokiPipeline(store, posPath, resolver)
	defer p.Stop()

	p.Start()
	p.Start()

	if !p.IsRunning() {
		t.Error("expected IsRunning() to be true after double Start()")
	}
}

func TestPipelineRestart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	logFile := filepath.Join(dir, "access.log")
	if err := os.WriteFile(logFile, []byte("log line\n"), 0644); err != nil {
		t.Fatalf("writing log file: %v", err)
	}

	store := newTestPipelineConfig(server.URL, []string{"access"})
	posPath := filepath.Join(dir, "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": logFile} }

	p := NewLokiPipeline(store, posPath, resolver)

	p.Start()
	if !p.IsRunning() {
		t.Fatal("expected IsRunning() to be true after Start()")
	}

	p.Restart()
	if !p.IsRunning() {
		t.Error("expected IsRunning() to be true after Restart()")
	}

	p.Stop()
	if p.IsRunning() {
		t.Error("expected IsRunning() to be false after Stop()")
	}
}

func TestPipelineSkipsUnknownSinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	logFile := filepath.Join(dir, "access.log")
	if err := os.WriteFile(logFile, []byte("log line\n"), 0644); err != nil {
		t.Fatalf("writing log file: %v", err)
	}

	store := newTestPipelineConfig(server.URL, []string{"access", "nonexistent"})
	posPath := filepath.Join(dir, "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": logFile} }

	p := NewLokiPipeline(store, posPath, resolver)
	defer p.Stop()

	p.Start()

	if !p.IsRunning() {
		t.Error("expected IsRunning() to be true even with unknown sinks in config")
	}
}

func TestPipelineStopWhenNotRunning(t *testing.T) {
	store := newTestPipelineConfig("http://localhost:3100/loki/api/v1/push", []string{"access"})
	posPath := filepath.Join(t.TempDir(), "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": "/tmp/access.log"} }

	p := NewLokiPipeline(store, posPath, resolver)

	p.Stop()

	if p.IsRunning() {
		t.Error("expected IsRunning() to be false")
	}
}
