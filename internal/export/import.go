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
	Manifest     Manifest         `json:"manifest"`
	CaddyConfig  json.RawMessage  `json:"caddy_config"`
	AppConfig    config.AppConfig `json:"app_config"`
	Snapshots    *SnapshotData    `json:"snapshots,omitempty"`
	MigrationLog []string         `json:"migration_log,omitempty"`
}

type SnapshotData struct {
	Index snapshot.Index             `json:"index"`
	Files map[string]json.RawMessage `json:"files"`
}

const MaxZIPSize = 5 * 1024 * 1024 // 5 MB

func ParseZIP(r io.Reader, size int64, runningVersion string) (*Backup, error) {
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

	if err := CheckVersion(backup.Manifest.KajiVersion, runningVersion); err != nil {
		return nil, err
	}

	var configMap map[string]any
	if err := json.Unmarshal(configData, &configMap); err != nil {
		return nil, fmt.Errorf("parsing config.json: %w", err)
	}

	migrationLog, err := RunMigrations(configMap, backup.Manifest.KajiVersion)
	if err != nil {
		return nil, fmt.Errorf("migrating config: %w", err)
	}

	migratedData, err := json.Marshal(configMap)
	if err != nil {
		return nil, fmt.Errorf("re-encoding migrated config: %w", err)
	}
	if err := json.Unmarshal(migratedData, &backup.AppConfig); err != nil {
		return nil, fmt.Errorf("parsing migrated config: %w", err)
	}

	backup.MigrationLog = migrationLog

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

func Restore(backup *Backup, cc *caddy.Client, store *config.ConfigStore, ss *snapshot.Store, autoSnapshot bool, version string) ([]string, error) {
	currentConfig, err := cc.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("fetching current caddy config for rollback: %w", err)
	}

	if autoSnapshot {
		cfg := store.Get()
		stripped := *cfg
		stripped.StripCredentials()
		appJSON, err := json.Marshal(stripped)
		if err != nil {
			return nil, fmt.Errorf("marshaling app config for snapshot: %w", err)
		}

		data := &snapshot.Data{
			KajiVersion: version,
			CaddyConfig: currentConfig,
			AppConfig:   json.RawMessage(appJSON),
		}
		name := "pre-import-" + backup.Manifest.ExportedAt
		if _, err := ss.Create(name, "Auto-snapshot before full import", "auto", data); err != nil {
			return nil, fmt.Errorf("creating pre-import snapshot: %w", err)
		}
	}

	previousAppJSON, err := json.Marshal(store.Get())
	if err != nil {
		return nil, fmt.Errorf("marshaling app config for rollback: %w", err)
	}

	if err := cc.LoadConfig(backup.CaddyConfig); err != nil {
		return nil, fmt.Errorf("loading caddy config: %w", err)
	}

	var warnings []string

	if err := store.Update(func(current config.AppConfig) (*config.AppConfig, error) {
		imported := backup.AppConfig
		imported.PreserveCredentials(&current)

		warnings = reconcilePaths(&imported, &current)

		return &imported, nil
	}); err != nil {
		cc.LoadConfig(currentConfig)
		return nil, fmt.Errorf("updating app config: %w", err)
	}

	if backup.Snapshots != nil {
		if err := restoreSnapshots(ss, backup.Snapshots); err != nil {
			cc.LoadConfig(currentConfig)
			var rollbackCfg config.AppConfig
			if json.Unmarshal(previousAppJSON, &rollbackCfg) == nil {
				store.Update(func(_ config.AppConfig) (*config.AppConfig, error) {
					return &rollbackCfg, nil
				})
			}
			return nil, fmt.Errorf("restoring snapshots: %w", err)
		}
	}

	return warnings, nil
}

// reconcilePaths validates machine-specific paths from the imported config
// against the current filesystem. If an imported path doesn't exist, it falls
// back to the current config's value (for existing installs) or the platform
// default (for fresh setups). Returns warnings describing any adjustments.
func reconcilePaths(imported *config.AppConfig, current *config.AppConfig) []string {
	defaults := config.DefaultConfig()
	var warnings []string

	// caddy_config_path: check that the directory containing the config file exists
	if imported.CaddyConfigPath != "" {
		dir := filepath.Dir(imported.CaddyConfigPath)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			original := imported.CaddyConfigPath
			if current.CaddyConfigPath != "" && dirExists(filepath.Dir(current.CaddyConfigPath)) {
				imported.CaddyConfigPath = current.CaddyConfigPath
			} else {
				imported.CaddyConfigPath = defaults.CaddyConfigPath
			}
			if imported.CaddyConfigPath != original {
				warnings = append(warnings, fmt.Sprintf(
					"Caddy config path adjusted from %s to %s (original path not found on this machine)",
					original, imported.CaddyConfigPath,
				))
			}
		}
	}

	// caddy_data_dir: only matters if explicitly set (empty uses auto-detection)
	if imported.CaddyDataDir != "" {
		if !dirExists(imported.CaddyDataDir) {
			original := imported.CaddyDataDir
			if current.CaddyDataDir != "" && dirExists(current.CaddyDataDir) {
				imported.CaddyDataDir = current.CaddyDataDir
			} else {
				imported.CaddyDataDir = ""
			}
			adjusted := imported.CaddyDataDir
			if adjusted == "" {
				adjusted = "(auto-detect)"
			}
			warnings = append(warnings, fmt.Sprintf(
				"Caddy data directory adjusted from %s to %s (original path not found on this machine)",
				original, adjusted,
			))
		}
	}

	// log_file
	if imported.LogFile != "" {
		dir := filepath.Dir(imported.LogFile)
		if !dirExists(dir) {
			original := imported.LogFile
			if current.LogFile != "" && dirExists(filepath.Dir(current.LogFile)) {
				imported.LogFile = current.LogFile
			} else {
				imported.LogFile = defaults.LogFile
			}
			if imported.LogFile != original {
				warnings = append(warnings, fmt.Sprintf(
					"Log file path adjusted from %s to %s (original path not found on this machine)",
					original, imported.LogFile,
				))
			}
		}
	}

	// log_dir
	if imported.LogDir != "" {
		if !dirExists(imported.LogDir) {
			original := imported.LogDir
			if current.LogDir != "" && dirExists(current.LogDir) {
				imported.LogDir = current.LogDir
			} else {
				imported.LogDir = defaults.LogDir
			}
			if imported.LogDir != original {
				warnings = append(warnings, fmt.Sprintf(
					"Log directory adjusted from %s to %s (original path not found on this machine)",
					original, imported.LogDir,
				))
			}
		}
	}

	return warnings
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
