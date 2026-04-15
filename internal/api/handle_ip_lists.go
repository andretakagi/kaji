// IP list CRUD handlers.
package api

import (
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type listWithCount struct {
	config.IPList
	ResolvedCount int `json:"resolved_count"`
}

func handleListIPLists(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		lists := cfg.IPLists
		if lists == nil {
			lists = []config.IPList{}
		}

		result := make([]listWithCount, len(lists))
		for i, l := range lists {
			resolved, err := caddy.ResolveIPList(l.ID, lists)
			count := 0
			if err == nil {
				count = len(resolved)
			}
			result[i] = listWithCount{
				IPList:        l,
				ResolvedCount: count,
			}
		}
		writeJSON(w, result)
	}
}

func handleCreateIPList(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Type        string   `json:"type"`
			IPs         []string `json:"ips"`
			Children    []string `json:"children"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		if msg := validateIPListName(req.Name); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if msg := validateIPListType(req.Type); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		for _, ip := range req.IPs {
			if msg := validateIPOrCIDR(ip); msg != "" {
				writeError(w, msg, http.StatusBadRequest)
				return
			}
		}

		cfg := store.Get()

		if msg := validateIPListChildren(req.Children, req.Type, "", cfg.IPLists); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		newList := config.IPList{
			ID:          newUUID(),
			Name:        req.Name,
			Description: req.Description,
			Type:        req.Type,
			IPs:         req.IPs,
			Children:    req.Children,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if newList.IPs == nil {
			newList.IPs = []string{}
		}
		if newList.Children == nil {
			newList.Children = []string{}
		}

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.IPLists = append(c.IPLists, newList)
			return &c, nil
		}); err != nil {
			log.Printf("handleCreateIPList: %v", err)
			writeError(w, "failed to save IP list", http.StatusInternalServerError)
			return
		}

		writeJSON(w, newList)
	}
}

func handleUpdateIPList(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		listID := r.PathValue("id")
		if listID == "" {
			writeError(w, "list id is required", http.StatusBadRequest)
			return
		}

		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			IPs         []string `json:"ips"`
			Children    []string `json:"children"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		if msg := validateIPListName(req.Name); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		for _, ip := range req.IPs {
			if msg := validateIPOrCIDR(ip); msg != "" {
				writeError(w, msg, http.StatusBadRequest)
				return
			}
		}

		cfg := store.Get()

		// Verify the list exists and get its type (type is immutable)
		var existing *config.IPList
		for i := range cfg.IPLists {
			if cfg.IPLists[i].ID == listID {
				existing = &cfg.IPLists[i]
				break
			}
		}
		if existing == nil {
			writeError(w, "IP list not found", http.StatusNotFound)
			return
		}

		if msg := validateIPListChildren(req.Children, existing.Type, listID, cfg.IPLists); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		var updated config.IPList
		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			for i := range c.IPLists {
				if c.IPLists[i].ID == listID {
					c.IPLists[i].Name = req.Name
					c.IPLists[i].Description = req.Description
					c.IPLists[i].IPs = req.IPs
					if c.IPLists[i].IPs == nil {
						c.IPLists[i].IPs = []string{}
					}
					c.IPLists[i].Children = req.Children
					if c.IPLists[i].Children == nil {
						c.IPLists[i].Children = []string{}
					}
					c.IPLists[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
					updated = c.IPLists[i]
					break
				}
			}
			return &c, nil
		}); err != nil {
			log.Printf("handleUpdateIPList: %v", err)
			writeError(w, "failed to update IP list", http.StatusInternalServerError)
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "IP list updated: "+updated.Name)
		if err := cascadeIPListChange(listID, store, cc); err != nil {
			log.Printf("handleUpdateIPList: %v", err)
			writeError(w, "IP list saved but some routes failed to update: "+err.Error(), http.StatusInternalServerError)
			return
		}
		persistCaddyConfig(cc, store)

		writeJSON(w, updated)
	}
}

func handleDeleteIPList(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		listID := r.PathValue("id")
		if listID == "" {
			writeError(w, "list id is required", http.StatusBadRequest)
			return
		}

		cfg := store.Get()

		// Verify the list exists
		var listName string
		for _, l := range cfg.IPLists {
			if l.ID == listID {
				listName = l.Name
				break
			}
		}
		if listName == "" {
			writeError(w, "IP list not found", http.StatusNotFound)
			return
		}

		// Find routes that use this list (directly or through composites)
		affectedRoutes := findRoutesUsingList(listID, cfg.IPLists, store, cc)

		maybeAutoSnapshot(cc, ss, store, version, "IP list deleted: "+listName)

		// Remove list from config, clean up parent composites, remove route bindings
		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			// Remove the list itself
			fresh := make([]config.IPList, 0, len(c.IPLists))
			for _, l := range c.IPLists {
				if l.ID != listID {
					fresh = append(fresh, l)
				}
			}
			c.IPLists = fresh

			// Remove this list from any parent composite's children
			for i := range c.IPLists {
				cleaned := make([]string, 0, len(c.IPLists[i].Children))
				for _, childID := range c.IPLists[i].Children {
					if childID != listID {
						cleaned = append(cleaned, childID)
					}
				}
				c.IPLists[i].Children = cleaned
			}

			// Remove route bindings that reference this list
			if c.RouteIPLists != nil {
				for routeID, boundListID := range c.RouteIPLists {
					if boundListID == listID {
						delete(c.RouteIPLists, routeID)
					}
				}
			}

			return &c, nil
		}); err != nil {
			log.Printf("handleDeleteIPList: %v", err)
			writeError(w, "failed to delete IP list", http.StatusInternalServerError)
			return
		}

		// Rebuild affected routes (they'll lose their IP filtering since the list is gone)
		var rebuildErrs []string
		for _, ar := range affectedRoutes {
			if err := rebuildRoute(ar.RouteID, store, cc); err != nil {
				rebuildErrs = append(rebuildErrs, fmt.Sprintf("route %s: %v", ar.RouteID, err))
			}
		}
		if len(rebuildErrs) > 0 {
			log.Printf("handleDeleteIPList: %d route(s) failed to rebuild: %v", len(rebuildErrs), rebuildErrs)
			writeError(w, "IP list deleted but some routes failed to update: "+strings.Join(rebuildErrs, "; "), http.StatusInternalServerError)
			return
		}
		if len(affectedRoutes) > 0 {
			persistCaddyConfig(cc, store)
		}

		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleIPListUsage(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		listID := r.PathValue("id")
		if listID == "" {
			writeError(w, "list id is required", http.StatusBadRequest)
			return
		}

		cfg := store.Get()

		// Find routes using this list
		routes := findRoutesUsingList(listID, cfg.IPLists, store, cc)

		// Find composite lists that include this list as a child
		type compositeRef struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		var composites []compositeRef
		for _, l := range cfg.IPLists {
			for _, childID := range l.Children {
				if childID == listID {
					composites = append(composites, compositeRef{ID: l.ID, Name: l.Name})
					break
				}
			}
		}
		if composites == nil {
			composites = []compositeRef{}
		}

		routeResults := make([]map[string]string, 0, len(routes))
		for _, ar := range routes {
			routeResults = append(routeResults, map[string]string{
				"id":     ar.RouteID,
				"domain": ar.Domain,
			})
		}

		writeJSON(w, map[string]any{
			"routes":          routeResults,
			"composite_lists": composites,
		})
	}
}

func handleRouteIPListBindings(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		bindings := cfg.RouteIPLists
		if bindings == nil {
			bindings = map[string]string{}
		}
		writeJSON(w, bindings)
	}
}
