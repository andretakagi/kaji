package export

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

type Backup struct {
	Manifest    Manifest         `json:"manifest"`
	CaddyConfig json.RawMessage  `json:"caddy_config"`
	AppConfig   config.AppConfig `json:"app_config"`
	Snapshots   *SnapshotData    `json:"snapshots,omitempty"`
}

type SnapshotData struct {
	Index snapshot.Index             `json:"index"`
	Files map[string]json.RawMessage `json:"files"`
}

const MaxZIPSize = 100 * 1024 * 1024 // 100 MB

func ParseZIP(r io.Reader, size int64) (*Backup, error) {
	if size > MaxZIPSize {
		return nil, fmt.Errorf("zip file too large (max %d MB)", MaxZIPSize/(1024*1024))
	}

	data, err := io.ReadAll(io.LimitReader(r, MaxZIPSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading zip: %w", err)
	}
	if int64(len(data)) > MaxZIPSize {
		return nil, fmt.Errorf("zip file too large (max %d MB)", MaxZIPSize/(1024*1024))
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("opening zip: %w", err)
	}

	files := make(map[string][]byte)
	for _, f := range zr.File {
		name := stripPrefix(f.Name)
		if name == "" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f.Name, err)
		}
		files[name] = content
	}

	var backup Backup

	manifestData, ok := files["manifest.json"]
	if !ok {
		return nil, fmt.Errorf("missing manifest.json")
	}
	if err := json.Unmarshal(manifestData, &backup.Manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest.json: %w", err)
	}
	if backup.Manifest.Version < 1 {
		return nil, fmt.Errorf("unsupported manifest version: %d", backup.Manifest.Version)
	}

	caddyData, ok := files["caddy.json"]
	if !ok {
		return nil, fmt.Errorf("missing caddy.json")
	}
	backup.CaddyConfig = json.RawMessage(caddyData)

	configData, ok := files["config.json"]
	if !ok {
		return nil, fmt.Errorf("missing config.json")
	}
	if err := json.Unmarshal(configData, &backup.AppConfig); err != nil {
		return nil, fmt.Errorf("parsing config.json: %w", err)
	}

	if indexData, ok := files["snapshots/index.json"]; ok {
		backup.Snapshots = &SnapshotData{
			Files: make(map[string]json.RawMessage),
		}
		if err := json.Unmarshal(indexData, &backup.Snapshots.Index); err != nil {
			return nil, fmt.Errorf("parsing snapshots/index.json: %w", err)
		}
		for name, content := range files {
			if strings.HasPrefix(name, "snapshots/") && name != "snapshots/index.json" {
				id := strings.TrimSuffix(strings.TrimPrefix(name, "snapshots/"), ".json")
				backup.Snapshots.Files[id] = json.RawMessage(content)
			}
		}
	}

	return &backup, nil
}

func Restore(backup *Backup, cc *caddy.Client, store *config.ConfigStore, ss *snapshot.Store, autoSnapshot bool) error {
	if autoSnapshot {
		currentConfig, err := cc.GetConfig()
		if err == nil {
			name := "pre-import-" + backup.Manifest.ExportedAt
			ss.Create(name, "Auto-snapshot before full import", "auto", currentConfig)
		}
	}

	if err := cc.LoadConfig(backup.CaddyConfig); err != nil {
		return fmt.Errorf("loading caddy config: %w", err)
	}

	if err := store.Update(func(current config.AppConfig) (*config.AppConfig, error) {
		imported := backup.AppConfig
		imported.PasswordHash = current.PasswordHash
		imported.SessionSecret = current.SessionSecret
		return &imported, nil
	}); err != nil {
		return fmt.Errorf("updating app config: %w", err)
	}

	if backup.Snapshots != nil {
		if err := restoreSnapshots(ss, backup.Snapshots); err != nil {
			return fmt.Errorf("restoring snapshots: %w", err)
		}
	}

	return nil
}

func restoreSnapshots(ss *snapshot.Store, data *SnapshotData) error {
	idx := ss.GetIndex()
	dir := ss.Dir()

	for id, content := range data.Files {
		path := filepath.Join(dir, id+".json")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating snapshots dir: %w", err)
		}
		if err := os.WriteFile(path, content, 0600); err != nil {
			return fmt.Errorf("writing snapshot %s: %w", id, err)
		}
	}

	data.Index.AutoSnapshotEnabled = idx.AutoSnapshotEnabled
	data.Index.AutoSnapshotLimit = idx.AutoSnapshotLimit

	existing := make(map[string]bool, len(idx.Snapshots))
	for _, s := range idx.Snapshots {
		existing[s.ID] = true
	}
	combined := idx.Snapshots
	for _, s := range data.Index.Snapshots {
		if !existing[s.ID] {
			combined = append(combined, s)
		}
	}
	data.Index.Snapshots = combined

	return ss.ReplaceIndex(data.Index)
}

func stripPrefix(name string) string {
	if i := strings.Index(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return ""
}
