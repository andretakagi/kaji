package logging

import (
	"context"
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

func TestPipelineAllSinksUnresolvable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	store := newTestPipelineConfig(server.URL, []string{"ghost", "phantom"})
	posPath := filepath.Join(t.TempDir(), "positions.json")
	resolver := func() map[string]string { return map[string]string{} }

	p := NewLokiPipeline(store, posPath, resolver)
	p.Start()

	if p.IsRunning() {
		p.Stop()
		t.Fatal("expected pipeline to not be running when all sinks are unresolvable")
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

func TestPipelineDrainTimeout(t *testing.T) {
	// Server blocks long enough to exceed the 5s drain timeout, so the
	// pusher will be stuck in sendRequest and Stop must force shutdown.
	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-reqCtx.Done()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer func() {
		reqCancel()
		server.Close()
	}()

	dir := t.TempDir()
	logFile := filepath.Join(dir, "access.log")
	if err := os.WriteFile(logFile, []byte("line one\n"), 0644); err != nil {
		t.Fatalf("writing log file: %v", err)
	}

	store := newTestPipelineConfig(server.URL, []string{"access"})
	posPath := filepath.Join(dir, "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": logFile} }

	p := NewLokiPipeline(store, posPath, resolver)
	p.Start()
	if !p.IsRunning() {
		t.Fatal("expected pipeline to be running")
	}

	// Wait for the tailer to read and batcher to flush to the pusher
	time.Sleep(3 * time.Second)

	// Stop should complete within ~drainTimeout (5s) + some margin, not hang forever
	done := make(chan struct{})
	go func() {
		p.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Stop() did not complete, drain timeout may not be working")
	}

	if p.IsRunning() {
		t.Error("expected pipeline to be stopped after drain timeout")
	}
}

func TestPipelinePositionsSavedOnStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	logFile := filepath.Join(dir, "access.log")
	content := "line one\nline two\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("writing log file: %v", err)
	}

	posPath := filepath.Join(dir, "positions.json")
	store := newTestPipelineConfig(server.URL, []string{"access"})
	resolver := func() map[string]string { return map[string]string{"access": logFile} }

	p := NewLokiPipeline(store, posPath, resolver)
	p.Start()
	if !p.IsRunning() {
		t.Fatal("expected pipeline to be running")
	}

	// Wait for tailer to read both lines
	time.Sleep(3 * time.Second)

	p.Stop()

	// Load the positions file written on Stop and verify the offset
	saved := NewPositionStore(posPath)
	if err := saved.Load(); err != nil {
		t.Fatalf("loading saved positions: %v", err)
	}
	offset := saved.Get(logFile)
	if offset != int64(len(content)) {
		t.Errorf("saved offset: got %d, want %d", offset, len(content))
	}
}

func TestReconfigureWhenNotRunningCallsRestart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	logFile := filepath.Join(dir, "access.log")
	if err := os.WriteFile(logFile, []byte("line\n"), 0644); err != nil {
		t.Fatalf("writing log file: %v", err)
	}

	store := newTestPipelineConfig(server.URL, []string{"access"})
	posPath := filepath.Join(dir, "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": logFile} }

	p := NewLokiPipeline(store, posPath, resolver)

	// Pipeline is not running, so Reconfigure should fall through to Restart
	// which calls Stop (no-op) then Start.
	p.Reconfigure()

	if !p.IsRunning() {
		t.Error("expected pipeline to be running after Reconfigure on stopped pipeline")
	}
	p.Stop()
}

func TestReconfigureWhenDisabledCallsRestart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	logFile := filepath.Join(dir, "access.log")
	if err := os.WriteFile(logFile, []byte("line\n"), 0644); err != nil {
		t.Fatalf("writing log file: %v", err)
	}

	store := newTestPipelineConfig(server.URL, []string{"access"})
	posPath := filepath.Join(dir, "positions.json")
	resolver := func() map[string]string { return map[string]string{"access": logFile} }

	p := NewLokiPipeline(store, posPath, resolver)
	p.Start()
	if !p.IsRunning() {
		t.Fatal("expected pipeline to be running after Start()")
	}

	// Disable Loki in config, then Reconfigure should stop the pipeline
	if err := store.Update(func(_ config.AppConfig) (*config.AppConfig, error) {
		return &config.AppConfig{
			CaddyAdminURL: "http://localhost:2019",
			Loki: config.LokiConfig{
				Enabled:              false,
				Endpoint:             server.URL,
				BatchSize:            1024,
				FlushIntervalSeconds: 1,
				Sinks:                []string{"access"},
			},
		}, nil
	}); err != nil {
		t.Fatalf("updating config: %v", err)
	}

	p.Reconfigure()

	if p.IsRunning() {
		t.Error("expected pipeline to be stopped after Reconfigure with Loki disabled")
	}
}

func TestReconfigureAddsAndRemovesTailers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	accessLog := filepath.Join(dir, "access.log")
	errorLog := filepath.Join(dir, "error.log")
	if err := os.WriteFile(accessLog, []byte("access line\n"), 0644); err != nil {
		t.Fatalf("writing access log: %v", err)
	}
	if err := os.WriteFile(errorLog, []byte("error line\n"), 0644); err != nil {
		t.Fatalf("writing error log: %v", err)
	}

	store := newTestPipelineConfig(server.URL, []string{"access"})
	posPath := filepath.Join(dir, "positions.json")
	resolver := func() map[string]string {
		return map[string]string{"access": accessLog, "error": errorLog}
	}

	p := NewLokiPipeline(store, posPath, resolver)
	p.Start()
	defer p.Stop()

	if !p.IsRunning() {
		t.Fatal("expected pipeline to be running after Start()")
	}

	// Should have one tailer (access)
	p.mu.Lock()
	if len(p.tailers) != 1 {
		t.Errorf("expected 1 tailer, got %d", len(p.tailers))
	}
	if _, ok := p.tailers["access"]; !ok {
		t.Error("expected 'access' tailer to exist")
	}
	p.mu.Unlock()

	// Reconfigure to have both access and error sinks
	if err := store.Update(func(_ config.AppConfig) (*config.AppConfig, error) {
		return &config.AppConfig{
			CaddyAdminURL: "http://localhost:2019",
			Loki: config.LokiConfig{
				Enabled:              true,
				Endpoint:             server.URL,
				BatchSize:            1024,
				FlushIntervalSeconds: 1,
				Sinks:                []string{"access", "error"},
			},
		}, nil
	}); err != nil {
		t.Fatalf("updating config: %v", err)
	}
	p.Reconfigure()

	p.mu.Lock()
	if len(p.tailers) != 2 {
		t.Errorf("expected 2 tailers after adding error sink, got %d", len(p.tailers))
	}
	if _, ok := p.tailers["error"]; !ok {
		t.Error("expected 'error' tailer to exist after reconfigure")
	}
	p.mu.Unlock()

	// Reconfigure to remove access, keep only error
	if err := store.Update(func(_ config.AppConfig) (*config.AppConfig, error) {
		return &config.AppConfig{
			CaddyAdminURL: "http://localhost:2019",
			Loki: config.LokiConfig{
				Enabled:              true,
				Endpoint:             server.URL,
				BatchSize:            1024,
				FlushIntervalSeconds: 1,
				Sinks:                []string{"error"},
			},
		}, nil
	}); err != nil {
		t.Fatalf("updating config: %v", err)
	}
	p.Reconfigure()

	p.mu.Lock()
	if len(p.tailers) != 1 {
		t.Errorf("expected 1 tailer after removing access sink, got %d", len(p.tailers))
	}
	if _, ok := p.tailers["access"]; ok {
		t.Error("expected 'access' tailer to be removed after reconfigure")
	}
	if _, ok := p.tailers["error"]; !ok {
		t.Error("expected 'error' tailer to still exist after reconfigure")
	}
	p.mu.Unlock()
}
