package api

import (
	"slices"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/SkynetNext/unified-access-gateway/internal/config"
	"github.com/SkynetNext/unified-access-gateway/internal/security"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

// AdminAPI provides control plane API for dynamic configuration
type AdminAPI struct {
	cfg      *config.Config
	security *security.Manager
	store    *config.RedisStore
	mu       sync.RWMutex
}

func NewAdminAPI(cfg *config.Config, sec *security.Manager, store *config.RedisStore) *AdminAPI {
	return &AdminAPI{
		cfg:      cfg,
		security: sec,
		store:    store,
	}
}

// RegisterRoutes registers admin API routes
func (a *AdminAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin/config", a.handleConfig)
	mux.HandleFunc("/admin/security/rate-limit", a.handleRateLimit)
	mux.HandleFunc("/admin/security/waf/ips", a.handleWAFIPs)
	mux.HandleFunc("/admin/security/waf/patterns", a.handleWAFPatterns)
	mux.HandleFunc("/admin/health", a.handleHealth)
}

// GET /admin/config - Get current configuration
func (a *AdminAPI) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"security": map[string]any{
			"auth": map[string]any{
				"enabled": a.cfg.Security.Auth.Enabled,
			},
			"rate_limit": map[string]any{
				"enabled":             a.cfg.Security.RateLimit.Enabled,
				"requests_per_second": a.cfg.Security.RateLimit.RequestsPerSecond,
				"burst":               a.cfg.Security.RateLimit.Burst,
			},
			"waf": map[string]any{
				"enabled":          a.cfg.Security.WAF.Enabled,
				"blocked_ips":      a.cfg.Security.WAF.BlockedIPs,
				"blocked_patterns": a.cfg.Security.WAF.BlockedPatterns,
			},
			"audit": map[string]interface{}{
				"enabled": a.cfg.Security.Audit.Enabled,
				"sink":    a.cfg.Security.Audit.Sink,
			},
		},
	})
}

// POST /admin/security/rate-limit - Update rate limit config
func (a *AdminAPI) handleRateLimit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Enabled *bool    `json:"enabled"`
		RPS     *float64 `json:"requests_per_second"`
		Burst   *int     `json:"burst"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	a.mu.Lock()
	if req.Enabled != nil {
		a.cfg.Security.RateLimit.Enabled = *req.Enabled
	}
	if req.RPS != nil {
		a.cfg.Security.RateLimit.RequestsPerSecond = *req.RPS
	}
	if req.Burst != nil {
		a.cfg.Security.RateLimit.Burst = *req.Burst
	}
	enabled := a.cfg.Security.RateLimit.Enabled
	rps := a.cfg.Security.RateLimit.RequestsPerSecond
	burst := a.cfg.Security.RateLimit.Burst
	a.mu.Unlock()

	if a.store != nil {
		if err := a.store.SetRateLimit(enabled, rps, burst); err != nil {
			http.Error(w, "Failed to persist rate limit config", http.StatusInternalServerError)
			return
		}
	}

	// Update runtime limiter
	if enabled && rps > 0 {
		a.security.UpdateRateLimit(rps, burst)
	} else {
		a.security.DisableRateLimit()
	}

	xlog.Infof("Rate limit updated: enabled=%v, rps=%.2f, burst=%d",
		a.cfg.Security.RateLimit.Enabled,
		a.cfg.Security.RateLimit.RequestsPerSecond,
		a.cfg.Security.RateLimit.Burst)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /admin/security/waf/ips - Update blocked IPs
func (a *AdminAPI) handleWAFIPs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Action string   `json:"action"` // "add" or "remove"
		IPs    []string `json:"ips"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	switch req.Action {
	case "add":
		if a.store != nil {
			if err := a.store.AddBlockedIPs(req.IPs); err != nil {
				http.Error(w, "Failed to update WAF IPs", http.StatusInternalServerError)
				return
			}
			ips, err := a.store.GetBlockedIPs()
			if err != nil {
				http.Error(w, "Failed to load WAF IPs", http.StatusInternalServerError)
				return
			}
			a.setBlockedIPs(ips)
		} else {
			for _, ip := range req.IPs {
				found := slices.Contains(a.cfg.Security.WAF.BlockedIPs, ip)
				if !found {
					a.cfg.Security.WAF.BlockedIPs = append(a.cfg.Security.WAF.BlockedIPs, ip)
				}
			}
			a.security.UpdateBlockedIPs(a.cfg.Security.WAF.BlockedIPs)
		}
	case "remove":
		if a.store != nil {
			if err := a.store.RemoveBlockedIPs(req.IPs); err != nil {
				http.Error(w, "Failed to update WAF IPs", http.StatusInternalServerError)
				return
			}
			ips, err := a.store.GetBlockedIPs()
			if err != nil {
				http.Error(w, "Failed to load WAF IPs", http.StatusInternalServerError)
				return
			}
			a.setBlockedIPs(ips)
		} else {
			newIPs := make([]string, 0, len(a.cfg.Security.WAF.BlockedIPs))
			removeSet := make(map[string]struct{}, len(req.IPs))
			for _, ip := range req.IPs {
				removeSet[ip] = struct{}{}
			}
			for _, ip := range a.cfg.Security.WAF.BlockedIPs {
				if _, ok := removeSet[ip]; !ok {
					newIPs = append(newIPs, ip)
				}
			}
			a.cfg.Security.WAF.BlockedIPs = newIPs
			a.security.UpdateBlockedIPs(a.cfg.Security.WAF.BlockedIPs)
		}
	default:
		http.Error(w, "Invalid action, use 'add' or 'remove'", http.StatusBadRequest)
		return
	}

	xlog.Infof("WAF IPs updated: action=%s, count=%d", req.Action, len(req.IPs))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /admin/security/waf/patterns - Update blocked patterns
func (a *AdminAPI) handleWAFPatterns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Action   string   `json:"action"` // "add" or "remove"
		Patterns []string `json:"patterns"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	switch req.Action {
	case "add":
		if a.store != nil {
			if err := a.store.AddBlockedPatterns(req.Patterns); err != nil {
				http.Error(w, "Failed to update WAF patterns", http.StatusInternalServerError)
				return
			}
			pats, err := a.store.GetBlockedPatterns()
			if err != nil {
				http.Error(w, "Failed to load WAF patterns", http.StatusInternalServerError)
				return
			}
			a.setBlockedPatterns(pats)
		} else {
			for _, pat := range req.Patterns {
				found := false
				for _, existing := range a.cfg.Security.WAF.BlockedPatterns {
					if existing == pat {
						found = true
						break
					}
				}
				if !found {
					a.cfg.Security.WAF.BlockedPatterns = append(a.cfg.Security.WAF.BlockedPatterns, pat)
				}
			}
			a.security.UpdateBlockedPatterns(a.cfg.Security.WAF.BlockedPatterns)
		}
	case "remove":
		if a.store != nil {
			if err := a.store.RemoveBlockedPatterns(req.Patterns); err != nil {
				http.Error(w, "Failed to update WAF patterns", http.StatusInternalServerError)
				return
			}
			pats, err := a.store.GetBlockedPatterns()
			if err != nil {
				http.Error(w, "Failed to load WAF patterns", http.StatusInternalServerError)
				return
			}
			a.setBlockedPatterns(pats)
		} else {
			newPatterns := make([]string, 0, len(a.cfg.Security.WAF.BlockedPatterns))
			removeSet := make(map[string]struct{}, len(req.Patterns))
			for _, pat := range req.Patterns {
				removeSet[pat] = struct{}{}
			}
			for _, pat := range a.cfg.Security.WAF.BlockedPatterns {
				if _, ok := removeSet[pat]; !ok {
					newPatterns = append(newPatterns, pat)
				}
			}
			a.cfg.Security.WAF.BlockedPatterns = newPatterns
			a.security.UpdateBlockedPatterns(a.cfg.Security.WAF.BlockedPatterns)
		}
	default:
		http.Error(w, "Invalid action, use 'add' or 'remove'", http.StatusBadRequest)
		return
	}

	xlog.Infof("WAF patterns updated: action=%s, count=%d", req.Action, len(req.Patterns))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GET /admin/health - Admin API health check
func (a *AdminAPI) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (a *AdminAPI) setBlockedIPs(ips []string) {
	a.mu.Lock()
	a.cfg.Security.WAF.BlockedIPs = ips
	a.mu.Unlock()
	a.security.UpdateBlockedIPs(ips)
}

func (a *AdminAPI) setBlockedPatterns(patterns []string) {
	a.mu.Lock()
	a.cfg.Security.WAF.BlockedPatterns = patterns
	a.mu.Unlock()
	a.security.UpdateBlockedPatterns(patterns)
}
