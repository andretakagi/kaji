package snapshot

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Snapshot struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	ParentID    string `json:"parent_id"`
	CreatedAt   string `json:"created_at"`
}

type Index struct {
	CurrentID           string     `json:"current_id"`
	AutoSnapshotEnabled bool       `json:"auto_snapshot_enabled"`
	AutoSnapshotLimit   int        `json:"auto_snapshot_limit"`
	Snapshots           []Snapshot `json:"snapshots"`
}

type Store struct {
	dir string
	mu  sync.Mutex
	idx Index
}

func NewStore(dir string) *Store {
	return &Store{
		dir: dir,
		idx: Index{
			AutoSnapshotLimit: 50,
		},
	}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.indexPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading snapshot index: %w", err)
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return fmt.Errorf("parsing snapshot index: %w", err)
	}
	if idx.AutoSnapshotLimit <= 0 {
		idx.AutoSnapshotLimit = 50
	}
	s.idx = idx
	return nil
}

func (s *Store) GetIndex() Index {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := s.idx
	cp.Snapshots = make([]Snapshot, len(s.idx.Snapshots))
	copy(cp.Snapshots, s.idx.Snapshots)
	return cp
}

func (s *Store) Create(name, description, snapType string, configData json.RawMessage) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generating snapshot ID: %w", err)
	}

	snap := Snapshot{
		ID:          id,
		Name:        name,
		Description: description,
		Type:        snapType,
		ParentID:    s.idx.CurrentID,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return nil, fmt.Errorf("creating snapshots directory: %w", err)
	}

	if err := atomicWrite(s.configPath(id), configData); err != nil {
		return nil, fmt.Errorf("writing snapshot config: %w", err)
	}

	prevSnapshots := make([]Snapshot, len(s.idx.Snapshots))
	copy(prevSnapshots, s.idx.Snapshots)
	prevCurrentID := s.idx.CurrentID

	// Remove any snapshots that descend from the current one (the old
	// forward path). This keeps history as a flat linear chain.
	s.removeDescendants(s.idx.CurrentID)

	s.idx.Snapshots = append(s.idx.Snapshots, snap)
	s.idx.CurrentID = id

	if snapType == "auto" {
		s.prune()
	}

	if err := s.saveIndex(); err != nil {
		s.idx.Snapshots = prevSnapshots
		s.idx.CurrentID = prevCurrentID
		os.Remove(s.configPath(id))
		return nil, err
	}

	return &snap, nil
}

func (s *Store) ReadConfig(id string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.exists(id) {
		return nil, fmt.Errorf("snapshot %s not found", id)
	}

	data, err := os.ReadFile(s.configPath(id))
	if err != nil {
		return nil, fmt.Errorf("reading snapshot config: %w", err)
	}
	return json.RawMessage(data), nil
}

func (s *Store) SetCurrent(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.exists(id) {
		return fmt.Errorf("snapshot %s not found", id)
	}

	s.idx.CurrentID = id
	return s.saveIndex()
}

func (s *Store) Update(id, name, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.idx.Snapshots {
		if s.idx.Snapshots[i].ID == id {
			s.idx.Snapshots[i].Name = name
			s.idx.Snapshots[i].Description = description
			return s.saveIndex()
		}
	}
	return fmt.Errorf("snapshot %s not found", id)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapIdx := -1
	for i, snap := range s.idx.Snapshots {
		if snap.ID == id {
			snapIdx = i
			break
		}
	}
	if snapIdx == -1 {
		return fmt.Errorf("snapshot %s not found", id)
	}
	if s.idx.CurrentID == id {
		return fmt.Errorf("cannot delete the current snapshot")
	}

	parentID := s.idx.Snapshots[snapIdx].ParentID

	// Re-parent children to the deleted node's parent.
	for i := range s.idx.Snapshots {
		if s.idx.Snapshots[i].ParentID == id {
			s.idx.Snapshots[i].ParentID = parentID
		}
	}

	s.idx.Snapshots = append(s.idx.Snapshots[:snapIdx], s.idx.Snapshots[snapIdx+1:]...)

	os.Remove(s.configPath(id))

	return s.saveIndex()
}

func (s *Store) UpdateSettings(enabled bool, limit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.idx.AutoSnapshotEnabled = enabled
	if limit > 0 {
		s.idx.AutoSnapshotLimit = limit
	}

	if enabled {
		s.prune()
	}

	return s.saveIndex()
}

func (s *Store) IsAutoEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idx.AutoSnapshotEnabled
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) ReplaceIndex(idx Index) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idx = idx
	return s.saveIndex()
}

// prune removes the oldest auto snapshots when the count exceeds the limit.
// Never prunes manual snapshots or the current snapshot. Must be called with
// mu held.
func (s *Store) prune() {
	totalAutos := 0
	var removable []int
	for i, snap := range s.idx.Snapshots {
		if snap.Type == "auto" {
			totalAutos++
			if snap.ID != s.idx.CurrentID {
				removable = append(removable, i)
			}
		}
	}

	excess := totalAutos - s.idx.AutoSnapshotLimit
	if excess <= 0 {
		return
	}

	// Sort by creation time ascending so oldest come first.
	sort.Slice(removable, func(a, b int) bool {
		return s.idx.Snapshots[removable[a]].CreatedAt < s.idx.Snapshots[removable[b]].CreatedAt
	})

	if excess > len(removable) {
		excess = len(removable)
	}

	toRemove := make(map[int]bool)
	for i := 0; i < excess; i++ {
		idx := removable[i]
		snap := s.idx.Snapshots[idx]

		// Re-parent children of pruned snapshots.
		for j := range s.idx.Snapshots {
			if s.idx.Snapshots[j].ParentID == snap.ID {
				s.idx.Snapshots[j].ParentID = snap.ParentID
			}
		}

		os.Remove(s.configPath(snap.ID))
		toRemove[idx] = true
	}

	filtered := s.idx.Snapshots[:0]
	for i, snap := range s.idx.Snapshots {
		if !toRemove[i] {
			filtered = append(filtered, snap)
		}
	}
	s.idx.Snapshots = filtered
}

// removeDescendants removes all snapshots that descend from the given id.
// Must be called with mu held.
func (s *Store) removeDescendants(id string) {
	if id == "" {
		return
	}
	descendants := make(map[string]bool)
	// Iteratively find all descendants.
	changed := true
	for changed {
		changed = false
		for _, snap := range s.idx.Snapshots {
			if descendants[snap.ID] {
				continue
			}
			if snap.ParentID == id || descendants[snap.ParentID] {
				descendants[snap.ID] = true
				changed = true
			}
		}
	}
	if len(descendants) == 0 {
		return
	}
	filtered := s.idx.Snapshots[:0]
	for _, snap := range s.idx.Snapshots {
		if descendants[snap.ID] {
			os.Remove(s.configPath(snap.ID))
		} else {
			filtered = append(filtered, snap)
		}
	}
	s.idx.Snapshots = filtered
}

func (s *Store) exists(id string) bool {
	for _, snap := range s.idx.Snapshots {
		if snap.ID == id {
			return true
		}
	}
	return false
}

func (s *Store) indexPath() string {
	return filepath.Join(s.dir, "index.json")
}

func (s *Store) configPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *Store) saveIndex() error {
	data, err := json.MarshalIndent(s.idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling snapshot index: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("creating snapshots directory: %w", err)
	}
	return atomicWrite(s.indexPath(), data)
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming %s to %s: %w", tmp, path, err)
	}
	return nil
}

func generateID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b)), nil
}
