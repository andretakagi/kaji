package export

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

type Manifest struct {
	Version     int    `json:"version"`
	ExportedAt  string `json:"exported_at"`
	KajiVersion string `json:"kaji_version"`
}

func BuildZIP(w io.Writer, cc *caddy.Client, store *config.ConfigStore, ss *snapshot.Store, kajiVersion string) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	manifest := Manifest{
		Version:     1,
		ExportedAt:  time.Now().UTC().Format(time.RFC3339),
		KajiVersion: kajiVersion,
	}
	if err := writeJSON(zw, "kaji-export/manifest.json", manifest); err != nil {
		return err
	}

	caddyConfig, err := cc.GetConfig()
	if err != nil {
		return fmt.Errorf("reading caddy config: %w", err)
	}
	if err := writeRaw(zw, "kaji-export/caddy.json", caddyConfig); err != nil {
		return err
	}

	cfg := store.Get()
	caddyfileContent, err := caddy.GenerateCaddyfile(caddyConfig, cfg.LogFile)
	if err != nil {
		return fmt.Errorf("generating caddyfile: %w", err)
	}
	if err := writeRaw(zw, "kaji-export/Caddyfile", []byte(caddyfileContent)); err != nil {
		return err
	}

	stripped := *cfg
	stripped.PasswordHash = ""
	stripped.SessionSecret = ""
	stripped.SessionMaxAge = 0
	stripped.SecureCookies = ""
	if err := writeJSON(zw, "kaji-export/config.json", stripped); err != nil {
		return err
	}

	if err := writeSnapshots(zw, ss); err != nil {
		return err
	}

	return nil
}

func writeJSON(zw *zip.Writer, name string, v any) error {
	f, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("creating %s in zip: %w", name, err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeRaw(zw *zip.Writer, name string, data []byte) error {
	f, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("creating %s in zip: %w", name, err)
	}
	_, err = f.Write(data)
	return err
}

func writeSnapshots(zw *zip.Writer, ss *snapshot.Store) error {
	idx := ss.GetIndex()
	if len(idx.Snapshots) == 0 {
		return nil
	}

	if err := writeJSON(zw, "kaji-export/snapshots/index.json", idx); err != nil {
		return err
	}
	for _, snap := range idx.Snapshots {
		data, err := ss.ReadConfig(snap.ID)
		if err != nil {
			continue
		}
		if err := writeRaw(zw, "kaji-export/snapshots/"+snap.ID+".json", data); err != nil {
			return err
		}
	}
	return nil
}
