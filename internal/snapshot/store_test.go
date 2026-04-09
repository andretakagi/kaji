package snapshot

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func cfg(s string) json.RawMessage {
	return json.RawMessage(`{"key":"` + s + `"}`)
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

func TestReadConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	data := json.RawMessage(`{"host":"example.com","port":443}`)
	snap, err := s.Create("rt", "", "manual", data)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.ReadConfig(snap.ID)
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("ReadConfig = %s, want %s", got, data)
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
	for _, snap := range idx.Snapshots {
		if snap.ID == manualSnap.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("manual snapshot should not be pruned")
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
}

func TestConcurrentOps(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	const goroutines = 10
	var wg sync.WaitGroup

	// Seed a snapshot so Delete/SetCurrent have something to work with.
	first, _ := s.Create("seed", "", "manual", cfg("seed"))
	_ = first

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			snap, err := s.Create("concurrent", "", "auto", cfg("c"))
			if err != nil {
				return
			}
			s.GetIndex()
			// Try to delete a non-current snapshot if possible.
			idx := s.GetIndex()
			for _, existing := range idx.Snapshots {
				if existing.ID != idx.CurrentID {
					s.Delete(existing.ID)
					break
				}
			}
			_ = snap
		}(i)
	}
	wg.Wait()
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

	data, err := s2.ReadConfig(a.ID)
	if err != nil {
		t.Fatalf("ReadConfig a: %v", err)
	}
	if string(data) != string(cfg("a")) {
		t.Errorf("ReadConfig = %s, want %s", data, cfg("a"))
	}
}
