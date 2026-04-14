package logging

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPositionStoreGetSetBasic(t *testing.T) {
	ps := NewPositionStore(filepath.Join(t.TempDir(), "positions.json"))

	if got := ps.Get("/var/log/caddy/access.log"); got != 0 {
		t.Errorf("Get on empty store: got %d, want 0", got)
	}

	ps.Set("/var/log/caddy/access.log", 4096)
	if got := ps.Get("/var/log/caddy/access.log"); got != 4096 {
		t.Errorf("Get after Set: got %d, want 4096", got)
	}
}

func TestPositionStoreRemove(t *testing.T) {
	ps := NewPositionStore(filepath.Join(t.TempDir(), "positions.json"))

	ps.Set("/var/log/caddy/access.log", 100)
	ps.Remove("/var/log/caddy/access.log")

	if got := ps.Get("/var/log/caddy/access.log"); got != 0 {
		t.Errorf("Get after Remove: got %d, want 0", got)
	}
}

func TestPositionStoreSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "positions.json")
	ps := NewPositionStore(path)

	ps.Set("/var/log/caddy/access.log", 12345)
	ps.Set("/var/log/caddy/errors.log", 67890)

	if err := ps.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ps2 := NewPositionStore(path)
	if err := ps2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := ps2.Get("/var/log/caddy/access.log"); got != 12345 {
		t.Errorf("access.log offset: got %d, want 12345", got)
	}
	if got := ps2.Get("/var/log/caddy/errors.log"); got != 67890 {
		t.Errorf("errors.log offset: got %d, want 67890", got)
	}
}

func TestPositionStoreLoadMissingFile(t *testing.T) {
	ps := NewPositionStore(filepath.Join(t.TempDir(), "nonexistent.json"))

	if err := ps.Load(); err != nil {
		t.Fatalf("Load on missing file should not error, got: %v", err)
	}
	if got := ps.Get("/any"); got != 0 {
		t.Errorf("Get after loading missing file: got %d, want 0", got)
	}
}

func TestPositionStoreLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "positions.json")
	os.WriteFile(path, []byte("not json"), 0600)

	ps := NewPositionStore(path)
	if err := ps.Load(); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestPositionStoreLoadNullPositions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "positions.json")
	os.WriteFile(path, []byte(`{"positions": null}`), 0600)

	ps := NewPositionStore(path)
	if err := ps.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := ps.Get("/any"); got != 0 {
		t.Errorf("Get after loading null positions: got %d, want 0", got)
	}
}

func TestPositionStoreCleanup(t *testing.T) {
	ps := NewPositionStore(filepath.Join(t.TempDir(), "positions.json"))

	ps.Set("/active/one.log", 100)
	ps.Set("/active/two.log", 200)
	ps.Set("/stale/old.log", 300)

	active := map[string]bool{
		"/active/one.log": true,
		"/active/two.log": true,
	}
	ps.Cleanup(active)

	if got := ps.Get("/active/one.log"); got != 100 {
		t.Errorf("active one: got %d, want 100", got)
	}
	if got := ps.Get("/active/two.log"); got != 200 {
		t.Errorf("active two: got %d, want 200", got)
	}
	if got := ps.Get("/stale/old.log"); got != 0 {
		t.Errorf("stale entry should be removed: got %d, want 0", got)
	}
}

func TestPositionStoreSaveAtomicOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "positions.json")
	ps := NewPositionStore(path)

	ps.Set("/log", 42)
	if err := ps.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Temp file should not linger
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful save")
	}

	// Actual file should exist
	if _, err := os.Stat(path); err != nil {
		t.Errorf("positions file should exist: %v", err)
	}
}

func TestPositionStoreConcurrentAccess(t *testing.T) {
	ps := NewPositionStore(filepath.Join(t.TempDir(), "positions.json"))

	const goroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ps.Set("/log", int64(n))
			ps.Get("/log")
		}(i)
	}
	wg.Wait()

	got := ps.Get("/log")
	if got < 0 || got >= int64(goroutines) {
		t.Errorf("unexpected offset %d after concurrent writes", got)
	}
}
