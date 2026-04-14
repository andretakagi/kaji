package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type PositionStore struct {
	path      string
	mu        sync.Mutex
	positions map[string]int64
}

func NewPositionStore(path string) *PositionStore {
	return &PositionStore{
		path:      path,
		positions: make(map[string]int64),
	}
}

func (ps *PositionStore) Load() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	data, err := os.ReadFile(ps.path)
	if err != nil {
		if os.IsNotExist(err) {
			ps.positions = make(map[string]int64)
			return nil
		}
		return fmt.Errorf("reading positions file: %w", err)
	}

	var file struct {
		Positions map[string]int64 `json:"positions"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parsing positions file: %w", err)
	}
	if file.Positions == nil {
		file.Positions = make(map[string]int64)
	}
	ps.positions = file.Positions
	return nil
}

func (ps *PositionStore) Save() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	file := struct {
		Positions map[string]int64 `json:"positions"`
	}{Positions: ps.positions}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling positions: %w", err)
	}

	tmp := ps.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("writing positions temp file: %w", err)
	}
	if err := os.Rename(tmp, ps.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming positions file: %w", err)
	}
	return nil
}

func (ps *PositionStore) Get(path string) int64 {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.positions[path]
}

func (ps *PositionStore) Set(path string, offset int64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.positions[path] = offset
}

func (ps *PositionStore) Remove(path string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.positions, path)
}

func (ps *PositionStore) Cleanup(activePaths map[string]bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for path := range ps.positions {
		if !activePaths[path] {
			delete(ps.positions, path)
		}
	}
}
