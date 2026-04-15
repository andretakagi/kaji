package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func cfg(s string) *Data {
	return &Data{CaddyConfig: json.RawMessage(`{"key":"` + s + `"}`)}
}

func TestCreateSetsCurrentAndParent(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	a, err := s.Create("snap-a", "first", "manual", cfg("a"))
	if err != nil {
		t.Fatalf("Create a: %v", err)
	}
	if s.idx.CurrentID != a.ID {
		t.Errorf("CurrentID = %q, want %q", s.idx.CurrentID, a.ID)
	}
	if a.ParentID != "" {
		t.Errorf("first snapshot ParentID = %q, want empty", a.ParentID)
	}

	b, err := s.Create("snap-b", "second", "manual", cfg("b"))
	if err != nil {
		t.Fatalf("Create b: %v", err)
	}
	if s.idx.CurrentID != b.ID {
		t.Errorf("CurrentID = %q, want %q", s.idx.CurrentID, b.ID)
	}
	if b.ParentID != a.ID {
		t.Errorf("snap-b ParentID = %q, want %q", b.ParentID, a.ID)
	}
}

func TestGetIndexReturnsOrder(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	names := []string{"alpha", "beta", "gamma"}
	for _, name := range names {
		if _, err := s.Create(name, "", "manual", cfg(name)); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	idx := s.GetIndex()
	if len(idx.Snapshots) != len(names) {
		t.Fatalf("len(Snapshots) = %d, want %d", len(idx.Snapshots), len(names))
	}
	for i, name := range names {
		if idx.Snapshots[i].Name != name {
			t.Errorf("Snapshots[%d].Name = %q, want %q", i, idx.Snapshots[i].Name, name)
		}
	}
}

func TestReadDataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	caddyCfg := json.RawMessage(`{"host":"example.com","port":443}`)
	d := &Data{CaddyConfig: caddyCfg, KajiVersion: "1.2.3"}
	snap, err := s.Create("rt", "", "manual", d)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.ReadData(snap.ID)
	if err != nil {
		t.Fatalf("ReadData: %v", err)
	}

	var wantVal, gotVal any
	json.Unmarshal(caddyCfg, &wantVal)
	json.Unmarshal(got.CaddyConfig, &gotVal)
	wantB, _ := json.Marshal(wantVal)
	gotB, _ := json.Marshal(gotVal)
	if string(gotB) != string(wantB) {
		t.Errorf("CaddyConfig = %s, want %s", gotB, wantB)
	}
	if got.KajiVersion != "1.2.3" {
		t.Errorf("KajiVersion = %q, want 1.2.3", got.KajiVersion)
	}
}

func TestSetCurrent(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	a, _ := s.Create("a", "", "manual", cfg("a"))
	b, _ := s.Create("b", "", "manual", cfg("b"))

	if err := s.SetCurrent(a.ID); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}
	if s.idx.CurrentID != a.ID {
		t.Errorf("CurrentID = %q, want %q", s.idx.CurrentID, a.ID)
	}

	if err := s.SetCurrent("nonexistent"); err == nil {
		t.Error("SetCurrent with bad ID should return error")
	}
	_ = b
}

func TestUpdate(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	snap, _ := s.Create("old-name", "old-desc", "manual", cfg("x"))

	if err := s.Update(snap.ID, "new-name", "new-desc"); err != nil {
		t.Fatalf("Update: %v", err)
	}

	idx := s.GetIndex()
	if idx.Snapshots[0].Name != "new-name" {
		t.Errorf("Name = %q, want new-name", idx.Snapshots[0].Name)
	}
	if idx.Snapshots[0].Description != "new-desc" {
		t.Errorf("Description = %q, want new-desc", idx.Snapshots[0].Description)
	}
}

func TestDeleteReparentsChildren(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	a, _ := s.Create("a", "", "manual", cfg("a"))
	b, _ := s.Create("b", "", "manual", cfg("b"))
	// Move current back to a so b is not current, then create c as child of a.
	// Actually: a -> b -> c where b is current.
	// We want to delete b; c should then point to a.
	// To do that: create a, b (current=b), then SetCurrent back to a and create c.
	// But Create calls removeDescendants(current) which would wipe b.
	// Instead: create a (current=a), b (current=b), c (current=c), then SetCurrent(b) and delete b.
	c, _ := s.Create("c", "", "manual", cfg("c"))

	// Move current to c, so b is deletable.
	if err := s.SetCurrent(c.ID); err != nil {
		t.Fatalf("SetCurrent c: %v", err)
	}

	if err := s.Delete(b.ID); err != nil {
		t.Fatalf("Delete b: %v", err)
	}

	idx := s.GetIndex()
	for _, snap := range idx.Snapshots {
		if snap.ID == b.ID {
			t.Error("deleted snapshot still in index")
		}
		if snap.ID == c.ID && snap.ParentID != a.ID {
			t.Errorf("c.ParentID = %q after deleting b, want %q", snap.ParentID, a.ID)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, b.ID+".json")); !os.IsNotExist(err) {
		t.Error("deleted snapshot data file still exists on disk")
	}
}

func TestDeleteRejectsCurrent(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	snap, _ := s.Create("snap", "", "manual", cfg("x"))

	if err := s.Delete(snap.ID); err == nil {
		t.Error("deleting current snapshot should return error")
	}
}

func TestPruneRemovesOldestAuto(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.idx.AutoSnapshotLimit = 3

	// Create 4 auto snapshots. Each Create calls prune after appending, so
	// after the 4th we should have 3 auto snapshots remaining.
	var ids []string
	for i := 0; i < 4; i++ {
		// Small sleep so CreatedAt timestamps differ.
		time.Sleep(2 * time.Millisecond)
		snap, err := s.Create("auto", "", "auto", cfg("x"))
		if err != nil {
			t.Fatalf("Create auto %d: %v", i, err)
		}
		ids = append(ids, snap.ID)
	}

	idx := s.GetIndex()
	if len(idx.Snapshots) != 3 {
		t.Errorf("after pruning len = %d, want 3", len(idx.Snapshots))
	}

	// Oldest (ids[0]) should be gone; newest (ids[3], the current) stays.
	for _, snap := range idx.Snapshots {
		if snap.ID == ids[0] {
			t.Error("oldest auto snapshot should have been pruned")
		}
	}

	if _, err := os.Stat(filepath.Join(dir, ids[0]+".json")); !os.IsNotExist(err) {
		t.Error("pruned snapshot data file still exists on disk")
	}
}

func TestPruneKeepsManual(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.idx.AutoSnapshotLimit = 2

	manualSnap, _ := s.Create("manual-one", "", "manual", cfg("m"))

	// Create 3 auto snapshots on top.
	for i := 0; i < 3; i++ {
		time.Sleep(2 * time.Millisecond)
		if _, err := s.Create("auto", "", "auto", cfg("a")); err != nil {
			t.Fatalf("Create auto %d: %v", i, err)
		}
	}

	idx := s.GetIndex()
	found := false
	autoCount := 0
	for _, snap := range idx.Snapshots {
		if snap.ID == manualSnap.ID {
			found = true
		}
		if snap.Type == "auto" {
			autoCount++
		}
	}
	if !found {
		t.Error("manual snapshot should not be pruned")
	}
	if _, err := os.Stat(filepath.Join(dir, manualSnap.ID+".json")); err != nil {
		t.Error("manual snapshot data file should still exist on disk")
	}

	// One auto snapshot was pruned (3 created, limit 2). Verify its file is gone.
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	// Remaining: index.json + manual + 2 auto = 4 files
	wantFiles := 1 + 1 + autoCount
	if len(files) != wantFiles {
		t.Errorf("disk has %d .json files, want %d (index + manual + %d auto)", len(files), wantFiles, autoCount)
	}
}

func TestRemoveDescendants(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Build chain: a -> b -> c
	a, _ := s.Create("a", "", "manual", cfg("a"))
	b, _ := s.Create("b", "", "manual", cfg("b"))
	c, _ := s.Create("c", "", "manual", cfg("c"))

	// Go back to a and create d. This triggers removeDescendants(a.ID)
	// before creating d, removing b and c.
	if err := s.SetCurrent(a.ID); err != nil {
		t.Fatalf("SetCurrent a: %v", err)
	}
	d, err := s.Create("d", "", "manual", cfg("d"))
	if err != nil {
		t.Fatalf("Create d: %v", err)
	}

	idx := s.GetIndex()
	for _, snap := range idx.Snapshots {
		if snap.ID == b.ID || snap.ID == c.ID {
			t.Errorf("snapshot %s should have been removed as a descendant", snap.ID)
		}
	}

	if d.ParentID != a.ID {
		t.Errorf("d.ParentID = %q, want %q", d.ParentID, a.ID)
	}

	for _, id := range []string{b.ID, c.ID} {
		if _, err := os.Stat(filepath.Join(dir, id+".json")); !os.IsNotExist(err) {
			t.Errorf("descendant %s data file still exists on disk", id)
		}
	}
}

func TestConcurrentOps(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	const goroutines = 10
	var wg sync.WaitGroup

	if _, err := s.Create("seed", "", "manual", cfg("seed")); err != nil {
		t.Fatalf("seeding snapshot: %v", err)
	}

	errs := make(chan error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			snap, err := s.Create("concurrent", "", "auto", cfg("c"))
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: Create: %w", n, err)
				return
			}
			if snap.ID == "" {
				errs <- fmt.Errorf("goroutine %d: Create returned empty ID", n)
				return
			}
			idx := s.GetIndex()
			if len(idx.Snapshots) == 0 {
				errs <- fmt.Errorf("goroutine %d: GetIndex returned zero snapshots", n)
				return
			}
			// Delete can legitimately fail under contention (already deleted,
			// is current, etc.) so we don't check its error here.
			for _, existing := range idx.Snapshots {
				if existing.ID != idx.CurrentID {
					s.Delete(existing.ID)
					break
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	idx := s.GetIndex()
	if len(idx.Snapshots) == 0 {
		t.Error("expected at least one snapshot after concurrent ops")
	}
	if idx.CurrentID == "" {
		t.Error("CurrentID is empty after concurrent ops")
	}
}

func TestUpdateSettingsEnableAutoSnapshot(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	if s.IsAutoEnabled() {
		t.Error("auto snapshots should be disabled by default")
	}

	if err := s.UpdateSettings(true, 10); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	if !s.IsAutoEnabled() {
		t.Error("auto snapshots should be enabled after UpdateSettings(true, ...)")
	}

	idx := s.GetIndex()
	if idx.AutoSnapshotLimit != 10 {
		t.Errorf("AutoSnapshotLimit = %d, want 10", idx.AutoSnapshotLimit)
	}
}

func TestUpdateSettingsDisableAutoSnapshot(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	if err := s.UpdateSettings(true, 5); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := s.UpdateSettings(false, 5); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if s.IsAutoEnabled() {
		t.Error("auto snapshots should be disabled after UpdateSettings(false, ...)")
	}
}

func TestUpdateSettingsIgnoresZeroLimit(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	if err := s.UpdateSettings(true, 20); err != nil {
		t.Fatalf("set limit 20: %v", err)
	}

	// Passing limit=0 should not change the existing limit
	if err := s.UpdateSettings(true, 0); err != nil {
		t.Fatalf("set limit 0: %v", err)
	}

	idx := s.GetIndex()
	if idx.AutoSnapshotLimit != 20 {
		t.Errorf("AutoSnapshotLimit = %d, want 20 (zero should be ignored)", idx.AutoSnapshotLimit)
	}
}

func TestUpdateSettingsPrunesOnEnable(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.idx.AutoSnapshotEnabled = false
	s.idx.AutoSnapshotLimit = 50

	// Create 5 auto snapshots without pruning (auto is disabled, but Create
	// calls prune for auto-type snapshots regardless)
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Millisecond)
		if _, err := s.Create(fmt.Sprintf("auto-%d", i), "", "auto", cfg("x")); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Now enable with a limit of 2, which should prune down
	if err := s.UpdateSettings(true, 2); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	idx := s.GetIndex()
	autoCount := 0
	for _, snap := range idx.Snapshots {
		if snap.Type == "auto" {
			autoCount++
		}
	}
	if autoCount > 2 {
		t.Errorf("auto snapshot count = %d after enabling with limit 2, want <= 2", autoCount)
	}
}

func TestUpdateSettingsNoPruneOnDisable(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.idx.AutoSnapshotEnabled = true
	s.idx.AutoSnapshotLimit = 50

	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Millisecond)
		if _, err := s.Create(fmt.Sprintf("auto-%d", i), "", "auto", cfg("x")); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Disable with limit 2 - should NOT prune because enabled=false
	if err := s.UpdateSettings(false, 2); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	idx := s.GetIndex()
	autoCount := 0
	for _, snap := range idx.Snapshots {
		if snap.Type == "auto" {
			autoCount++
		}
	}
	if autoCount != 5 {
		t.Errorf("auto snapshot count = %d, want 5 (no pruning when disabling)", autoCount)
	}
}

func TestUpdateSettingsPersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	s1 := NewStore(dir)

	if err := s1.UpdateSettings(true, 15); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	s2 := NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	idx := s2.GetIndex()
	if !idx.AutoSnapshotEnabled {
		t.Error("AutoSnapshotEnabled not persisted")
	}
	if idx.AutoSnapshotLimit != 15 {
		t.Errorf("AutoSnapshotLimit = %d, want 15 (not persisted)", idx.AutoSnapshotLimit)
	}
}

func TestLoadEmptyDir(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	if err := s.Load(); err != nil {
		t.Fatalf("Load on empty dir: %v", err)
	}

	idx := s.GetIndex()
	if len(idx.Snapshots) != 0 {
		t.Errorf("expected empty snapshots, got %d", len(idx.Snapshots))
	}
	if idx.CurrentID != "" {
		t.Errorf("expected empty CurrentID, got %q", idx.CurrentID)
	}
}

func TestDeleteNonexistent(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Create("keep", "", "manual", cfg("k"))

	if err := s.Delete("no-such-id"); err == nil {
		t.Error("Delete on nonexistent ID should return error")
	}
}

func TestUpdateNonexistent(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	if err := s.Update("no-such-id", "name", "desc"); err == nil {
		t.Error("Update on nonexistent ID should return error")
	}
}

func TestLoadCorruptIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte("not json{{{"), 0600); err != nil {
		t.Fatal(err)
	}

	s := NewStore(dir)
	if err := s.Load(); err == nil {
		t.Error("Load with corrupt index should return error")
	}
}

func TestLoadNormalizesAutoSnapshotLimit(t *testing.T) {
	dir := t.TempDir()
	idx := Index{AutoSnapshotLimit: 0, Snapshots: []Snapshot{}}
	data, _ := json.Marshal(idx)
	if err := os.WriteFile(filepath.Join(dir, "index.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := s.GetIndex()
	if got.AutoSnapshotLimit != 50 {
		t.Errorf("AutoSnapshotLimit = %d, want 50", got.AutoSnapshotLimit)
	}
}

func TestLoadRestoresState(t *testing.T) {
	dir := t.TempDir()
	s1 := NewStore(dir)

	a, _ := s1.Create("a", "desc-a", "manual", cfg("a"))
	b, _ := s1.Create("b", "desc-b", "manual", cfg("b"))

	// Load into a fresh store from the same directory.
	s2 := NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	idx := s2.GetIndex()
	if idx.CurrentID != b.ID {
		t.Errorf("CurrentID = %q, want %q", idx.CurrentID, b.ID)
	}
	if len(idx.Snapshots) != 2 {
		t.Fatalf("len(Snapshots) = %d, want 2", len(idx.Snapshots))
	}
	if idx.Snapshots[0].ID != a.ID || idx.Snapshots[0].Name != "a" {
		t.Errorf("Snapshots[0] = %+v, want id=%q name=a", idx.Snapshots[0], a.ID)
	}

	got, err := s2.ReadData(a.ID)
	if err != nil {
		t.Fatalf("ReadData a: %v", err)
	}
	want := cfg("a").CaddyConfig
	var wantVal, gotVal any
	json.Unmarshal(want, &wantVal)
	json.Unmarshal(got.CaddyConfig, &gotVal)
	wantB, _ := json.Marshal(wantVal)
	gotB, _ := json.Marshal(gotVal)
	if string(gotB) != string(wantB) {
		t.Errorf("CaddyConfig = %s, want %s", gotB, wantB)
	}
}
