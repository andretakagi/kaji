// Entry point. Starts Caddy, loads config, serves the API + embedded frontend.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/andretakagi/kaji/internal/api"
	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/export"
	"github.com/andretakagi/kaji/internal/logging"
	"github.com/andretakagi/kaji/internal/snapshot"
	"github.com/andretakagi/kaji/internal/system"
)

const (
	serverReadTimeout  = 15 * time.Second
	serverWriteTimeout = 30 * time.Second
	serverIdleTimeout  = 60 * time.Second
	shutdownTimeout    = 10 * time.Second
	caddyReadyTimeout  = 10 * time.Second
)

var version = "1.7.0"

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

	store := config.NewStoreWithPath(raw, config.Path())
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

	resolveSinks := func() map[string]string {
		result := make(map[string]string)
		raw, err := caddyClient.GetLoggingConfig()
		if err != nil {
			log.Printf("loki: failed to get logging config: %v", err)
			return result
		}
		var loggingCfg struct {
			Logs map[string]struct {
				Writer struct {
					Output   string `json:"output"`
					Filename string `json:"filename"`
				} `json:"writer"`
			} `json:"logs"`
		}
		if json.Unmarshal(raw, &loggingCfg) != nil {
			return result
		}
		for name, sink := range loggingCfg.Logs {
			if sink.Writer.Output == "file" && sink.Writer.Filename != "" {
				result[name] = sink.Writer.Filename
			}
		}
		return result
	}

	positionsPath := filepath.Join(filepath.Dir(config.Path()), "positions.json")
	lokiPipeline := logging.NewLokiPipeline(store, positionsPath, resolveSinks)

	if configExists {
		if err := caddyClient.WaitReady(caddyReadyTimeout); err != nil {
			log.Printf("Caddy admin API not reachable, skipping config restore: %v", err)
		} else {
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

			if cfg.KajiVersion != version {
				runStartupMigration(store, caddyClient, version)
			}
		}
	}

	lokiPipeline.Start()

	mux := http.NewServeMux()

	distFS, err := fs.Sub(frontendFiles, "dist")
	if err != nil {
		log.Fatalf("Failed to create sub filesystem: %v", err)
	}
	mux.Handle("/", spaHandler(http.FS(distFS)))

	handler := api.RegisterRoutes(mux, store, mgr, caddyClient, snapStore, lokiPipeline, version)

	addr := os.Getenv("KAJI_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
		IdleTimeout:  serverIdleTimeout,
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

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	lokiPipeline.Stop()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	if err := mgr.Stop(); err != nil {
		log.Printf("Caddy shutdown error: %v", err)
	}

	log.Println("Shutdown complete")
}

func runStartupMigration(store *config.ConfigStore, cc *caddy.Client, ver string) {
	cfg := store.Get()

	configMap := make(map[string]any)
	data, err := json.Marshal(cfg)
	if err != nil {
		log.Printf("Migration: failed to marshal config: %v", err)
		return
	}
	if err := json.Unmarshal(data, &configMap); err != nil {
		log.Printf("Migration: failed to unmarshal config map: %v", err)
		return
	}

	fromVersion := cfg.KajiVersion
	if fromVersion == "" {
		fromVersion = "0.0.0"
	}

	changes, err := export.RunMigrations(configMap, fromVersion)
	if err != nil {
		log.Printf("Migration: %v", err)
		return
	}
	for _, c := range changes {
		log.Printf("Migration: %s", c)
	}

	// Apply migration changes back to the config. RunMigrations mutates
	// configMap in place (removes disabled_routes, route_settings, converts
	// disabled routes to domain entries), so we need to unmarshal it back.
	var migrated config.AppConfig
	if len(changes) > 0 {
		migratedData, err := json.Marshal(configMap)
		if err != nil {
			log.Printf("Migration: failed to re-marshal migrated config: %v", err)
			return
		}
		if err := json.Unmarshal(migratedData, &migrated); err != nil {
			log.Printf("Migration: failed to parse migrated config: %v", err)
			return
		}
	} else {
		migrated = *cfg
	}

	// If the migration didn't produce any domains, convert active Caddy
	// routes (old kaji_ prefix, not kaji_rule_) into domain entries.
	if len(migrated.Domains) == 0 {
		active := migrateActiveCaddyRoutes(cc)
		if len(active) > 0 {
			migrated.Domains = active
			log.Printf("Migration: converted %d active Caddy routes to domains", len(active))
		}
	} else if len(changes) > 0 {
		// Migration produced domains (from disabled_routes). Also pull in
		// active Caddy routes that aren't already covered.
		active := migrateActiveCaddyRoutes(cc)
		if len(active) > 0 {
			existing := make(map[string]bool, len(migrated.Domains))
			for _, d := range migrated.Domains {
				existing[d.Name] = true
			}
			for _, d := range active {
				if !existing[d.Name] {
					migrated.Domains = append(migrated.Domains, d)
				}
			}
			log.Printf("Migration: converted %d active Caddy routes to domains", len(active))
		}
	}

	migrated.KajiVersion = ver

	if err := store.Update(func(current config.AppConfig) (*config.AppConfig, error) {
		// Preserve credentials and paths from current config
		migrated.PreserveCredentials(&current)
		return &migrated, nil
	}); err != nil {
		log.Printf("Migration: failed to save migrated config: %v", err)
		return
	}

	cfg = store.Get()
	syncDomains := export.ToSyncDomains(cfg.Domains)
	if _, err := caddy.SyncDomains(cc, syncDomains, export.ResolveIPsFromConfig(cfg)); err != nil {
		log.Printf("Migration: failed to sync domains to Caddy: %v", err)
	}
}

func migrateActiveCaddyRoutes(cc *caddy.Client) []config.Domain {
	raw, err := cc.GetConfig()
	if err != nil || raw == nil {
		return nil
	}

	var cfg struct {
		Apps struct {
			HTTP struct {
				Servers map[string]struct {
					Routes []json.RawMessage `json:"routes"`
				} `json:"servers"`
			} `json:"http"`
		} `json:"apps"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return nil
	}

	var domains []config.Domain
	for _, srv := range cfg.Apps.HTTP.Servers {
		for _, routeRaw := range srv.Routes {
			var r struct {
				ID string `json:"@id"`
			}
			if json.Unmarshal(routeRaw, &r) != nil {
				continue
			}
			if !strings.HasPrefix(r.ID, "kaji_") {
				continue
			}
			if strings.HasPrefix(r.ID, "kaji_rule_") {
				continue
			}

			params, err := caddy.ParseRouteParams(routeRaw)
			if err != nil || params.Domain == "" {
				continue
			}

			handlerType := params.HandlerType
			if handlerType == "" {
				handlerType = "reverse_proxy"
			}

			var hcData json.RawMessage
			switch handlerType {
			case "redirect", "static_response":
				hcData = params.HandlerConfig
			default:
				rpCfg := caddy.ReverseProxyConfig{
					Upstream:          params.Upstream,
					TLSSkipVerify:     params.Toggles.TLSSkipVerify,
					WebSocketPassthru: params.Toggles.WebSocketPassthru,
				}
				hcData, err = caddy.MarshalReverseProxyConfig(rpCfg)
				if err != nil {
					continue
				}
			}

			ruleID := caddy.GenerateRuleID()
			domain := config.Domain{
				ID:      caddy.GenerateDomainID(),
				Name:    params.Domain,
				Enabled: true,
				Rules: []config.Rule{
					{
						ID:            ruleID,
						Enabled:       true,
						MatchType:     "",
						HandlerType:   handlerType,
						HandlerConfig: hcData,
					},
				},
			}
			domains = append(domains, domain)
		}
	}

	return domains
}
