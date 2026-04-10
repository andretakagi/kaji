// Authentication and API key handlers.
package api

import (
	"log"
	"net/http"

	"github.com/andretakagi/kaji/internal/auth"
	"github.com/andretakagi/kaji/internal/config"
)

func handleAuthStatus(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		authenticated := !cfg.AuthEnabled
		if cfg.AuthEnabled {
			token := auth.GetSessionToken(r)
			authenticated = token != "" && auth.ValidateSignedToken(token, cfg.SessionSecret, cfg.SessionMaxAge)
		}
		writeJSON(w, map[string]any{
			"auth_enabled":  cfg.AuthEnabled,
			"authenticated": authenticated,
			"has_password":  cfg.PasswordHash != "",
		})
	}
}

func handleLogin(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		if !cfg.AuthEnabled {
			writeError(w, "authentication is disabled", http.StatusConflict)
			return
		}

		var req struct {
			Password string `json:"password"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		if !auth.CheckPassword(cfg.PasswordHash, req.Password) {
			writeError(w, "invalid password", http.StatusUnauthorized)
			return
		}

		token, err := auth.GenerateSessionToken()
		if err != nil {
			log.Printf("handleLogin: generate session token: %v", err)
			writeError(w, "failed to create session", http.StatusInternalServerError)
			return
		}
		auth.SetSessionCookie(w, r, auth.SignToken(token, cfg.SessionSecret), sessionMaxAge(cfg), cfg.SecureCookies)
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleLogout(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		auth.ClearSessionCookie(w, r, cfg.SecureCookies)
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handlePasswordChange(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !store.Get().AuthEnabled {
			writeError(w, "authentication is disabled", http.StatusConflict)
			return
		}

		var req struct {
			NewPassword string `json:"new_password"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		if req.NewPassword == "" {
			writeError(w, "new password is required", http.StatusBadRequest)
			return
		}

		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			log.Printf("handlePasswordChange: hash password: %v", err)
			writeError(w, "failed to hash password", http.StatusInternalServerError)
			return
		}

		newSecret, err := auth.GenerateSessionToken()
		if err != nil {
			log.Printf("handlePasswordChange: generate session secret: %v", err)
			writeError(w, "failed to generate session secret", http.StatusInternalServerError)
			return
		}

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.PasswordHash = hash
			c.SessionSecret = newSecret
			return &c, nil
		}); err != nil {
			log.Printf("handlePasswordChange: save config: %v", err)
			writeError(w, "failed to save config", http.StatusInternalServerError)
			return
		}

		token, err := auth.GenerateSessionToken()
		if err != nil {
			log.Printf("handlePasswordChange: generate session token: %v", err)
			writeJSON(w, map[string]string{"status": "ok", "warning": "password changed but session creation failed, please log in manually"})
			return
		}
		cfg := store.Get()
		auth.SetSessionCookie(w, r, auth.SignToken(token, cfg.SessionSecret), sessionMaxAge(cfg), cfg.SecureCookies)

		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleAuthToggle(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AuthEnabled bool   `json:"auth_enabled"`
			Password    string `json:"password"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		if req.AuthEnabled {
			cfg := store.Get()

			newSecret, err := auth.GenerateSessionToken()
			if err != nil {
				log.Printf("handleAuthToggle: generate session secret: %v", err)
				writeError(w, "failed to generate session secret", http.StatusInternalServerError)
				return
			}

			if cfg.PasswordHash == "" {
				if req.Password == "" {
					writeError(w, "password is required to enable auth", http.StatusBadRequest)
					return
				}
				hash, err := auth.HashPassword(req.Password)
				if err != nil {
					log.Printf("handleAuthToggle: hash password: %v", err)
					writeError(w, "failed to hash password", http.StatusInternalServerError)
					return
				}
				if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
					c.AuthEnabled = true
					c.PasswordHash = hash
					c.SessionSecret = newSecret
					return &c, nil
				}); err != nil {
					log.Printf("handleAuthToggle: save config: %v", err)
					writeError(w, "failed to save config", http.StatusInternalServerError)
					return
				}
			} else {
				if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
					c.AuthEnabled = true
					c.SessionSecret = newSecret
					return &c, nil
				}); err != nil {
					log.Printf("handleAuthToggle: save config: %v", err)
					writeError(w, "failed to save config", http.StatusInternalServerError)
					return
				}
			}

			cfg = store.Get()
			token, err := auth.GenerateSessionToken()
			if err != nil {
				log.Printf("handleAuthToggle: generate session token: %v", err)
				writeJSON(w, map[string]string{"status": "ok", "warning": "auth enabled but session creation failed, please log in manually"})
				return
			}
			auth.SetSessionCookie(w, r, auth.SignToken(token, cfg.SessionSecret), sessionMaxAge(cfg), cfg.SecureCookies)
		} else {
			if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
				c.AuthEnabled = false
				c.PasswordHash = ""
				return &c, nil
			}); err != nil {
				log.Printf("handleAuthToggle: save config: %v", err)
				writeError(w, "failed to save config", http.StatusInternalServerError)
				return
			}
		}

		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleAPIKeyStatus(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"has_api_key": store.Get().APIKeyHash != ""})
	}
}

func handleAPIKeyGenerate(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := auth.GenerateAPIKey()
		if err != nil {
			log.Printf("handleAPIKeyGenerate: %v", err)
			writeError(w, "failed to generate API key", http.StatusInternalServerError)
			return
		}
		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.APIKeyHash = auth.HashAPIKey(key)
			return &c, nil
		}); err != nil {
			log.Printf("handleAPIKeyGenerate: save config: %v", err)
			writeError(w, "failed to save config", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"api_key": key})
	}
}

func handleAPIKeyRevoke(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.APIKeyHash = ""
			return &c, nil
		}); err != nil {
			log.Printf("handleAPIKeyRevoke: save config: %v", err)
			writeError(w, "failed to save config", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}
