package config

import (
	"os"
	"sync"
	"time"

	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

// K8sConfigWatcher watches for ConfigMap changes
type K8sConfigWatcher struct {
	configPath string
	onChange   func(*Config)
	mu         sync.RWMutex
	stopCh     chan struct{}
}

// NewK8sConfigWatcher creates a ConfigMap watcher
// ConfigMap is mounted at configPath (e.g., /etc/config/gateway.yaml)
func NewK8sConfigWatcher(configPath string, onChange func(*Config)) *K8sConfigWatcher {
	return &K8sConfigWatcher{
		configPath: configPath,
		onChange:   onChange,
		stopCh:     make(chan struct{}),
	}
}

// Start starts watching for ConfigMap changes
func (w *K8sConfigWatcher) Start() {
	// In K8s, ConfigMap updates trigger Pod restart by default
	// For hot-reload, we can watch the file modification time
	go w.watch()
}

// Stop stops the watcher
func (w *K8sConfigWatcher) Stop() {
	close(w.stopCh)
}

func (w *K8sConfigWatcher) watch() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastModTime time.Time

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			// Check if ConfigMap file changed
			info, err := os.Stat(w.configPath)
			if err != nil {
				continue // File doesn't exist yet
			}

			if !info.ModTime().IsZero() && info.ModTime().After(lastModTime) {
				xlog.Infof("ConfigMap changed, reloading...")
				cfg := LoadConfigFromFile(w.configPath)
				if cfg != nil {
					w.onChange(cfg)
				}
				lastModTime = info.ModTime()
			}
		}
	}
}

// LoadConfigFromFile loads config from a YAML file
func LoadConfigFromFile(path string) *Config {
	// For MVP, we still use env vars
	// In production, use viper or similar to load YAML
	// This is a placeholder for ConfigMap hot-reload
	return LoadConfig()
}

// LoadConfigFromConfigMap loads config from K8s ConfigMap mount point
func LoadConfigFromConfigMap() *Config {
	// Standard K8s ConfigMap mount paths
	configPaths := []string{
		"/etc/config/gateway.yaml",
		"/etc/gateway/config.yaml",
		"/config/gateway.yaml",
	}

	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			xlog.Infof("Loading config from ConfigMap: %s", path)
			return LoadConfigFromFile(path)
		}
	}

	// Fallback to env vars
	return LoadConfig()
}

