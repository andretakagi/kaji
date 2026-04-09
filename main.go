// Entry point. Starts Caddy, loads config, serves the API + embedded frontend.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/andretakagi/kaji/internal/api"
	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
	"github.com/andretakagi/kaji/internal/system"
)

var version = "1.1.0"

//go:embed dist/*
var frontendFiles embed.FS

// spaHandler serves static files from the given filesystem, falling back to
// index.html for paths that don't match a real file (so client-side routing works).
func spaHandler(fsys http.FileSystem) http.Handler {
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := fsys.Open(r.URL.Path)
		if err != nil {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()
		fileServer.ServeHTTP(w, r)
	})
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("kaji " + version)
		os.Exit(0)
	}

	configExists := config.Exists()
	raw, err := config.Load()
	if err != nil {
		log.Printf("No config found, using defaults: %v", err)
		raw = config.DefaultConfig()
	}

	store := config.NewStore(raw)
	mgr := system.NewCaddyManager(raw.CaddyAdminURL)

	running, _ := mgr.Status()
	if !running {
		if configExists {
			log.Println("Caddy not running, starting...")
		} else {
			log.Println("First run detected, starting Caddy...")
		}
		if err := mgr.Start(); err != nil {
			log.Printf("Failed to start Caddy: %v", err)
		}
	}

	caddyClient := caddy.NewClient(func() string {
		return store.Get().CaddyAdminURL
	})

	snapshotDir := filepath.Join(filepath.Dir(config.Path()), "snapshots")
	snapStore := snapshot.NewStore(snapshotDir)
	if err := snapStore.Load(); err != nil {
		log.Printf("Failed to load snapshot index: %v", err)
	}

	if configExists {
		cfg := store.Get()
		saved, err := os.ReadFile(cfg.CaddyConfigPath)
		switch {
		case err == nil:
			if err := caddyClient.LoadConfig(saved); err != nil {
				log.Printf("Failed to load saved Caddy config: %v", err)
			}
		case !os.IsNotExist(err):
			log.Printf("Failed to read saved Caddy config: %v", err)
		}
	}

	mux := http.NewServeMux()

	distFS, err := fs.Sub(frontendFiles, "dist")
	if err != nil {
		log.Fatalf("Failed to create sub filesystem: %v", err)
	}
	mux.Handle("/", spaHandler(http.FS(distFS)))

	handler := api.RegisterRoutes(mux, store, mgr, caddyClient, snapStore, version)

	addr := os.Getenv("KAJI_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Kaji %s starting on %s", version, addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	if err := mgr.Stop(); err != nil {
		log.Printf("Caddy shutdown error: %v", err)
	}

	log.Println("Shutdown complete")
}
