package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "snapshots")
	return NewStore(dir)
}

func TestCreateAndList(t *testing.T) {
	s := newTestStore(t)

	cfg := json.RawMessage(`{"apps":{}}`)
	snap, err := s.Create("first", "my first snapshot", "manual", cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	idx := s.GetIndex()
	if len(idx.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(idx.Snapshots))
	}
	if idx.Snapshots[0].ID != snap.ID {
		t.Fatalf("snapshot ID mismatch")
	}
	if idx.CurrentID != snap.ID {
		t.Fatalf("expected current_id %s, got %s", snap.ID, idx.CurrentID)
	}
}

func TestReadConfig(t *testing.T) {
	s := newTestStore(t)

	original := json.RawMessage(`{"apps":{"http":{"servers":{}}}}`)
	snap, err := s.Create("test", "", "manual", original)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	data, err := s.ReadConfig(snap.ID)
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	if string(data) != string(original) {
		t.Fatalf("config mismatch: got %s, want %s", string(data), string(original))
	}
}

func TestParentChain(t *testing.T) {
	s := newTestStore(t)
	cfg := json.RawMessage(`{}`)

	s1, _ := s.Create("s1", "", "manual", cfg)
	s2, _ := s.Create("s2", "", "manual", cfg)
	s3, _ := s.Create("s3", "", "manual", cfg)

	if s1.ParentID != "" {
		t.Fatalf("s1 parent should be empty, got %s", s1.ParentID)
	}
	if s2.ParentID != s1.ID {
		t.Fatalf("s2 parent should be %s, got %s", s1.ID, s2.ParentID)
	}
	if s3.ParentID != s2.ID {
		t.Fatalf("s3 parent should be %s, got %s", s2.ID, s3.ParentID)
	}
}

func TestBranching(t *testing.T) {
	s := newTestStore(t)
	cfg := json.RawMessage(`{}`)

	s1, _ := s.Create("s1", "", "manual", cfg)
	s.Create("s2", "", "manual", cfg)

	// Roll back to s1, then create s3 - it should branch from s1.
	if err := s.SetCurrent(s1.ID); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}

	s3, _ := s.Create("s3", "", "manual", cfg)
	if s3.ParentID != s1.ID {
		t.Fatalf("s3 parent should be %s (branch from s1), got %s", s1.ID, s3.ParentID)
	}
}

func TestDeleteReparentsChildren(t *testing.T) {
	s := newTestStore(t)
	cfg := json.RawMessage(`{}`)

	s1, _ := s.Create("s1", "", "manual", cfg)
	s2, _ := s.Create("s2", "", "manual", cfg)
	s3, _ := s.Create("s3", "", "manual", cfg)

	// Delete s2 - s3's parent should become s1.
	if err := s.Delete(s2.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	idx := s.GetIndex()
	if len(idx.Snapshots) != 2 {
		t.Fatalf("expected 2 snapshots after delete, got %d", len(idx.Snapshots))
	}

	for _, snap := range idx.Snapshots {
		if snap.ID == s3.ID && snap.ParentID != s1.ID {
			t.Fatalf("s3 parent should be %s after deleting s2, got %s", s1.ID, snap.ParentID)
		}
	}
}

func TestDeleteCurrentMovesToParent(t *testing.T) {
	s := newTestStore(t)
	cfg := json.RawMessage(`{}`)

	s1, _ := s.Create("s1", "", "manual", cfg)
	s2, _ := s.Create("s2", "", "manual", cfg)

	if err := s.Delete(s2.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	idx := s.GetIndex()
	if idx.CurrentID != s1.ID {
		t.Fatalf("current should be %s after deleting s2, got %s", s1.ID, idx.CurrentID)
	}
}

func TestPruneAutoOnly(t *testing.T) {
	s := newTestStore(t)
	cfg := json.RawMessage(`{}`)

	if err := s.UpdateSettings(true, 2); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	s.Create("manual1", "", "manual", cfg)
	s.Create("auto1", "", "auto", cfg)
	s.Create("auto2", "", "auto", cfg)
	s.Create("auto3", "", "auto", cfg)

	idx := s.GetIndex()

	manualCount := 0
	autoCount := 0
	for _, snap := range idx.Snapshots {
		switch snap.Type {
		case "manual":
			manualCount++
		case "auto":
			autoCount++
		}
	}

	if manualCount != 1 {
		t.Fatalf("expected 1 manual snapshot, got %d", manualCount)
	}
	if autoCount > 2 {
		t.Fatalf("expected at most 2 auto snapshots, got %d", autoCount)
	}
}

func TestUpdateNameDescription(t *testing.T) {
	s := newTestStore(t)
	cfg := json.RawMessage(`{}`)

	snap, _ := s.Create("old name", "old desc", "manual", cfg)

	if err := s.Update(snap.ID, "new name", "new desc"); err != nil {
		t.Fatalf("Update: %v", err)
	}

	idx := s.GetIndex()
	if idx.Snapshots[0].Name != "new name" {
		t.Fatalf("expected name 'new name', got %s", idx.Snapshots[0].Name)
	}
	if idx.Snapshots[0].Description != "new desc" {
		t.Fatalf("expected description 'new desc', got %s", idx.Snapshots[0].Description)
	}
}

func TestPersistAndReload(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	s := NewStore(dir)

	cfg := json.RawMessage(`{"persist":"yes"}`)
	snap, err := s.Create("persist test", "", "manual", cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// New store on the same directory.
	s2 := NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	idx := s2.GetIndex()
	if len(idx.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot after reload, got %d", len(idx.Snapshots))
	}
	if idx.Snapshots[0].ID != snap.ID {
		t.Fatalf("snapshot ID mismatch after reload")
	}
	if idx.CurrentID != snap.ID {
		t.Fatalf("current_id mismatch after reload")
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	s := newTestStore(t)
	cfg := json.RawMessage(`{"data":true}`)

	snap, _ := s.Create("to delete", "", "manual", cfg)

	path := filepath.Join(s.dir, snap.ID+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("config file should exist after create")
	}

	if err := s.Delete(snap.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file should be removed after delete")
	}
}
