package cloudpool

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"directlink/internal/logger"
)

// CloudPool represents the cloud IP pool JSON structure.
type CloudPool struct {
	Version int               `json:"version"`
	Updated string            `json:"updated"`
	Pools   map[string][]string `json:"pools"`
}

// Manager handles fetching, caching, and serving the cloud IP pool.
type Manager struct {
	mu          sync.RWMutex
	pool        *CloudPool
	cachePath   string
	sources     []string // fetch URLs in priority order
	lastFetch   time.Time
}

// New creates a new cloud pool manager.
func New(cacheDir string) *Manager {
	if cacheDir == "" {
		cacheDir = filepath.Join(os.Getenv("APPDATA"), "DirectLink")
	}
	os.MkdirAll(cacheDir, 0755)

	return &Manager{
		cachePath: filepath.Join(cacheDir, "cloud-ip-pool.json"),
		sources: []string{
			// jsDelivr CDN (accessible in China, mirrors GitHub)
			"https://cdn.jsdelivr.net/gh/BetaLi/direct-link@main/cloud-ip-pool.json",
			// GitHub raw (fallback)
			"https://raw.githubusercontent.com/BetaLi/direct-link/main/cloud-ip-pool.json",
		},
	}
}

// LoadLocal loads the cached cloud pool from disk. Called on startup.
func (m *Manager) LoadLocal() {
	data, err := os.ReadFile(m.cachePath)
	if err != nil {
		return
	}
	var pool CloudPool
	if err := json.Unmarshal(data, &pool); err != nil {
		return
	}
	m.mu.Lock()
	m.pool = &pool
	m.mu.Unlock()
	logger.Info("已加载本地云 IP 池缓存: %d 个域名", len(pool.Pools))
}

// Fetch fetches the cloud pool from remote sources. Non-blocking — run in goroutine.
// Tries each source in order, falls back to local cache on failure.
func (m *Manager) Fetch() {
	client := &http.Client{Timeout: 10 * time.Second}

	for _, url := range m.sources {
		resp, err := client.Get(url)
		if err != nil {
			logger.Debug("云 IP 池拉取失败 %s: %v", url, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			logger.Debug("云 IP 池 HTTP %d %s", resp.StatusCode, url)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		var pool CloudPool
		if err := json.Unmarshal(body, &pool); err != nil {
			continue
		}

		// Save to local cache
		os.WriteFile(m.cachePath, body, 0644)

		m.mu.Lock()
		m.pool = &pool
		m.lastFetch = time.Now()
		m.mu.Unlock()

		logger.Info("云 IP 池已更新: %d 个域名 (来源: %s, 更新时间: %s)",
			len(pool.Pools), url, pool.Updated)
		return
	}

	logger.Debug("云 IP 池拉取失败（所有源均不可达），使用本地缓存")
}

// StartFetchLoop starts a background goroutine that periodically fetches the pool.
// Initial fetch happens immediately, then every 1 hour.
func (m *Manager) StartFetchLoop() {
	go func() {
		// Initial fetch
		m.Fetch()

		// Periodic refresh every hour
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			m.Fetch()
		}
	}()
}

// GetIPs returns the cloud IPs for a domain, or nil if not available.
func (m *Manager) GetIPs(domain string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.pool == nil {
		return nil
	}
	return m.pool.Pools[domain]
}

// HasPool returns true if the cloud pool is loaded.
func (m *Manager) HasPool() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pool != nil
}
