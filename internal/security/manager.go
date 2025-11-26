package security

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/SkynetNext/unified-access-gateway/internal/config"
	"github.com/SkynetNext/unified-access-gateway/internal/middleware"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
	"golang.org/x/time/rate"
)

// Manager coordinates auth, rate limiting, WAF, and audit logging.
type Manager struct {
	cfg *config.Config

	stateMu         sync.RWMutex
	allowedSubjects map[string]struct{}
	blockedIPs      map[string]struct{}
	blockedPatterns []*regexp.Regexp
	limiter         *rate.Limiter

	auditEnabled bool
	auditSink    io.Writer
	auditMu      sync.Mutex

	redisStore *config.RedisStore
}

func NewManager(cfg *config.Config, store *config.RedisStore) *Manager {
	m := &Manager{
		cfg:        cfg,
		redisStore: store,
	}

	m.loadStaticConfig()

	// Load security config from Redis (READ-ONLY, no sync back)
	if store != nil {
		if snapshot, err := store.LoadSecurityConfig(); err == nil && snapshot != nil {
			m.applySnapshot(snapshot)
			xlog.Infof("Loaded security configuration from Redis (READ-ONLY)")
		} else if err != nil {
			xlog.Warnf("Failed to load security config from Redis: %v (using defaults)", err)
		}
		// Listen for config updates via pub/sub
		go m.consumeRedisUpdates()
	}

	// Audit sink
	if cfg.Security.Audit.Enabled {
		m.auditEnabled = true
		switch {
		case cfg.Security.Audit.Sink == "" || strings.EqualFold(cfg.Security.Audit.Sink, "stdout"):
			m.auditSink = os.Stdout
		case strings.EqualFold(cfg.Security.Audit.Sink, "stderr"):
			m.auditSink = os.Stderr
		case strings.HasPrefix(cfg.Security.Audit.Sink, "file://"):
			path := strings.TrimPrefix(cfg.Security.Audit.Sink, "file://")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				xlog.Warnf("Failed to create audit log dir %s: %v", path, err)
				m.auditSink = os.Stdout
			} else {
				f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
				if err != nil {
					xlog.Warnf("Failed to open audit log file %s: %v", path, err)
					m.auditSink = os.Stdout
				} else {
					m.auditSink = f
				}
			}
		default:
			m.auditSink = os.Stdout
		}
	}

	return m
}

func (m *Manager) loadStaticConfig() {
	if m.cfg.Security.Auth.Enabled {
		m.UpdateAllowedSubjects(m.cfg.Security.Auth.AllowedSubjects)
	}
	if m.cfg.Security.RateLimit.Enabled && m.cfg.Security.RateLimit.RequestsPerSecond > 0 {
		m.UpdateRateLimit(m.cfg.Security.RateLimit.RequestsPerSecond, m.cfg.Security.RateLimit.Burst)
	}
	if m.cfg.Security.WAF.Enabled {
		m.UpdateBlockedIPs(m.cfg.Security.WAF.BlockedIPs)
		m.UpdateBlockedPatterns(m.cfg.Security.WAF.BlockedPatterns)
	}
}

func (m *Manager) applySnapshot(sec *config.SecurityConfig) {
	if sec == nil {
		return
	}
	if sec.RateLimit.Enabled {
		if sec.RateLimit.RequestsPerSecond > 0 {
			m.UpdateRateLimit(sec.RateLimit.RequestsPerSecond, sec.RateLimit.Burst)
		} else {
			m.DisableRateLimit()
		}
	}
	if len(sec.WAF.BlockedIPs) > 0 {
		m.UpdateBlockedIPs(sec.WAF.BlockedIPs)
	}
	if len(sec.WAF.BlockedPatterns) > 0 {
		m.UpdateBlockedPatterns(sec.WAF.BlockedPatterns)
	}
	if len(sec.Auth.AllowedSubjects) > 0 {
		m.UpdateAllowedSubjects(sec.Auth.AllowedSubjects)
	}
}

func (m *Manager) consumeRedisUpdates() {
	ch := m.redisStore.Updates()
	if ch == nil {
		return
	}
	for update := range ch {
		xlog.Infof("Received config update from Redis: type=%s", update.Type)
		// Reload all security config from Redis on any change
		// This is simpler and ensures consistency
		if snapshot, err := m.redisStore.LoadSecurityConfig(); err == nil && snapshot != nil {
			m.applySnapshot(snapshot)
			xlog.Infof("Reloaded security configuration from Redis")
		} else if err != nil {
			xlog.Warnf("Failed to reload security config from Redis: %v", err)
		}
	}
}

// CheckConnection performs per-connection checks before accepting traffic.
func (m *Manager) CheckConnection(addr net.Addr) error {
	if addr == nil {
		return nil
	}
	ip := extractIP(addr.String())

	if m.cfg.Security.WAF.Enabled && m.isBlockedIP(ip) {
		middleware.RecordSecurityBlock("waf_blocked_ip")
		return fmt.Errorf("blocked IP: %s", ip)
	}

	limiter := m.getLimiter()
	if limiter != nil && !limiter.Allow() {
		middleware.RecordSecurityBlock("rate_limit")
		return errors.New("rate limit exceeded")
	}

	return nil
}

// AuthorizeHTTP validates client identity using TLS certificate subject or headers.
func (m *Manager) AuthorizeHTTP(r *http.Request) error {
	if !m.cfg.Security.Auth.Enabled {
		return nil
	}

	subject := ""
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		subject = r.TLS.PeerCertificates[0].Subject.String()
	}
	if subject == "" && m.cfg.Security.Auth.HeaderSubject != "" {
		subject = r.Header.Get(m.cfg.Security.Auth.HeaderSubject)
	}
	if subject == "" {
		middleware.RecordSecurityBlock("auth_missing_subject")
		return errors.New("client certificate subject missing")
	}

	m.stateMu.RLock()
	allowed := m.allowedSubjects
	m.stateMu.RUnlock()
	if len(allowed) == 0 {
		return nil
	}
	if _, ok := allowed[subject]; !ok {
		middleware.RecordSecurityBlock("auth_unauthorized")
		return fmt.Errorf("subject %s not allowed", subject)
	}
	return nil
}

// ApplyWAF enforces HTTP-level WAF rules.
func (m *Manager) ApplyWAF(r *http.Request) error {
	if !m.cfg.Security.WAF.Enabled {
		return nil
	}
	ip := extractIP(r.RemoteAddr)
	if m.isBlockedIP(ip) {
		middleware.RecordSecurityBlock("waf_blocked_ip")
		return fmt.Errorf("blocked IP: %s", ip)
	}
	patterns := m.getBlockedPatterns()
	if len(patterns) == 0 {
		return nil
	}
	payload := r.URL.Path
	if r.URL.RawQuery != "" {
		payload += "?" + r.URL.RawQuery
	}
	for _, re := range patterns {
		if re.MatchString(payload) {
			middleware.RecordSecurityBlock("waf_pattern_match")
			return fmt.Errorf("blocked by pattern %s", re.String())
		}
	}
	return nil
}

func (m *Manager) getLimiter() *rate.Limiter {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.limiter
}

func (m *Manager) getBlockedPatterns() []*regexp.Regexp {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return append([]*regexp.Regexp(nil), m.blockedPatterns...)
}

func (m *Manager) AuditHTTP(r *http.Request, status int, duration time.Duration, err error) {
	if !m.auditEnabled || m.auditSink == nil {
		return
	}
	action := "allow"
	detail := ""
	if err != nil {
		action = "deny"
		detail = err.Error()
	}
	entry := fmt.Sprintf(
		`{"ts":"%s","protocol":"http","remote_addr":"%s","method":"%s","path":"%s","status":%d,"action":"%s","duration_ms":%d,"detail":"%s"}`+"\n",
		time.Now().Format(time.RFC3339Nano),
		r.RemoteAddr,
		r.Method,
		r.URL.Path,
		status,
		action,
		duration.Milliseconds(),
		escapeQuotes(detail),
	)
	m.writeAudit(entry)
}

func (m *Manager) AuditTCP(remoteAddr, backend string, allowed bool, detail string) {
	if !m.auditEnabled || m.auditSink == nil {
		return
	}
	action := "allow"
	if !allowed {
		action = "deny"
	}
	entry := fmt.Sprintf(
		`{"ts":"%s","protocol":"tcp","remote_addr":"%s","backend":"%s","action":"%s","detail":"%s"}`+"\n",
		time.Now().Format(time.RFC3339Nano),
		remoteAddr,
		backend,
		action,
		escapeQuotes(detail),
	)
	m.writeAudit(entry)
}

func (m *Manager) writeAudit(payload string) {
	m.auditMu.Lock()
	defer m.auditMu.Unlock()
	if _, err := m.auditSink.Write([]byte(payload)); err != nil {
		xlog.Warnf("Failed to write audit log: %v", err)
	}
}

func (m *Manager) isBlockedIP(ip string) bool {
	if ip == "" {
		return false
	}
	m.stateMu.RLock()
	_, blocked := m.blockedIPs[ip]
	m.stateMu.RUnlock()
	return blocked
}

func extractIP(addr string) string {
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `'`)
}

// UpdateRateLimit updates rate limiter configuration at runtime
func (m *Manager) UpdateRateLimit(rps float64, burst int) {
	if rps <= 0 || burst <= 0 {
		m.DisableRateLimit()
		return
	}
	m.stateMu.Lock()
	m.cfg.Security.RateLimit.Enabled = true
	m.cfg.Security.RateLimit.RequestsPerSecond = rps
	m.cfg.Security.RateLimit.Burst = burst
	m.limiter = rate.NewLimiter(rate.Limit(rps), burst)
	m.stateMu.Unlock()
	xlog.Infof("Rate limiter updated: rps=%.2f, burst=%d", rps, burst)
}

// DisableRateLimit disables rate limiting
func (m *Manager) DisableRateLimit() {
	m.stateMu.Lock()
	m.cfg.Security.RateLimit.Enabled = false
	m.cfg.Security.RateLimit.RequestsPerSecond = 0
	m.cfg.Security.RateLimit.Burst = 0
	m.limiter = nil
	m.stateMu.Unlock()
	xlog.Infof("Rate limiting disabled")
}

// UpdateBlockedIPs updates the blocked IP list at runtime
func (m *Manager) UpdateBlockedIPs(ips []string) {
	m.stateMu.Lock()
	m.blockedIPs = make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		if ip == "" {
			continue
		}
		m.blockedIPs[ip] = struct{}{}
	}
	m.cfg.Security.WAF.BlockedIPs = append([]string(nil), ips...)
	m.stateMu.Unlock()
	xlog.Infof("Blocked IPs updated: count=%d", len(ips))
}

// UpdateBlockedPatterns updates the blocked pattern list at runtime
func (m *Manager) UpdateBlockedPatterns(patterns []string) {
	m.stateMu.Lock()
	m.blockedPatterns = make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			xlog.Warnf("Invalid WAF pattern %q: %v", pattern, err)
			continue
		}
		m.blockedPatterns = append(m.blockedPatterns, re)
	}
	m.cfg.Security.WAF.BlockedPatterns = append([]string(nil), patterns...)
	m.stateMu.Unlock()
	xlog.Infof("Blocked patterns updated: count=%d", len(m.blockedPatterns))
}

// UpdateAllowedSubjects updates the allowed subject list at runtime
func (m *Manager) UpdateAllowedSubjects(subjects []string) {
	m.stateMu.Lock()
	m.allowedSubjects = make(map[string]struct{}, len(subjects))
	for _, subj := range subjects {
		if subj == "" {
			continue
		}
		m.allowedSubjects[subj] = struct{}{}
	}
	m.cfg.Security.Auth.AllowedSubjects = append([]string(nil), subjects...)
	m.stateMu.Unlock()
	xlog.Infof("Allowed subjects updated: count=%d", len(subjects))
}
