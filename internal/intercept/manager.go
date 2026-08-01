package intercept

import (
	"fmt"
	"sync"

	"directlink/internal/cloudpool"
	"directlink/internal/config"
	"directlink/internal/logger"
	"directlink/internal/prober"
)

// Manager is the top-level orchestrator for the acceleration system.
// It ties together the prober, hosts manager, proxy server, and switcher.
type Manager struct {
	mu        sync.Mutex
	cfg       *config.AppConfig
	prober    *prober.Prober
	hostsMgr  *HostsMgr
	proxy     *ProxyServer
	switcher  *Switcher
	cloudPool *cloudpool.Manager
	running   bool
}

// NewManager creates a new Manager from the given config.
func NewManager(cfg *config.AppConfig) *Manager {
	p := prober.New(cfg.Advanced.MaxIPsPerDomain, cfg.Advanced.DohProviders)

	// Initialize cloud IP pool (fetches from GitHub, caches locally)
	cp := cloudpool.New("")
	cp.LoadLocal()
	cp.StartFetchLoop()
	p.SetCloudPool(cp)

	hostsMgr := NewHostsMgr()
	proxySrv := NewProxyServer(cfg.Advanced.ProxyPort, p)
	proxySrv.SetRelay(&cfg.Relay)
	switcher := NewSwitcher(hostsMgr, proxySrv, p)
	switcher.SetIntervals(cfg.Advanced.HealthCheckInterval, cfg.Advanced.ProbeInterval)

	// Set domains from enabled sites
	domains := domainListFromConfig(cfg)
	switcher.SetDomains(domains)

	return &Manager{
		cfg:       cfg,
		prober:    p,
		hostsMgr:  hostsMgr,
		proxy:     proxySrv,
		switcher:  switcher,
		cloudPool: cp,
	}
}

// Start activates the acceleration system.
func (m *Manager) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("加速已在运行中")
	}
	m.running = true
	m.mu.Unlock()

	logger.Info("DirectLink 加速器启动中...")

	if err := m.switcher.Start(); err != nil {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		return err
	}

	logger.Info("DirectLink 加速器已启动，模式: %s", m.switcher.GetMode())
	return nil
}

// Stop deactivates the acceleration system and cleans up.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.mu.Unlock()

	m.switcher.Stop()
	logger.Info("DirectLink 加速器已停止")
}

// IsRunning returns whether acceleration is active.
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// GetMode returns the current interception mode.
func (m *Manager) GetMode() string {
	return m.switcher.GetMode()
}

// GetProber returns the prober instance.
func (m *Manager) GetProber() *prober.Prober {
	return m.prober
}

// Reprobe forces an immediate re-probe of all domains.
func (m *Manager) Reprobe() error {
	if !m.IsRunning() {
		return fmt.Errorf("加速未运行")
	}
	domains := domainListFromConfig(m.cfg)
	results := m.prober.ProbeDomains(domains)

	// Update hosts if in hosts mode
	if m.switcher.GetMode() == "hosts" {
		entries := make(map[string]string)
		for domain, result := range results {
			if result.BestIP != "" {
				entries[domain] = result.BestIP
			}
		}
		if len(entries) > 0 {
			if err := m.hostsMgr.WriteEntries(entries); err != nil {
				logger.Error("重新探测后更新 hosts 失败: %v", err)
			}
		}
	}

	logger.Info("手动重新探测完成")
	return nil
}

// UpdateConfig reloads the configuration and updates domains.
func (m *Manager) UpdateConfig(cfg *config.AppConfig) {
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()

	domains := domainListFromConfig(cfg)
	m.switcher.SetDomains(domains)
	m.switcher.SetIntervals(cfg.Advanced.HealthCheckInterval, cfg.Advanced.ProbeInterval)
}

// GetStatus returns a summary of the current state.
func (m *Manager) GetStatus() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	results := m.prober.GetAllResults()
	siteStatuses := make(map[string]SiteStatus)

	rules := config.BuiltinRules()
	for _, rule := range rules {
		siteCfg, ok := m.cfg.Sites[rule.ID]
		enabled := ok && siteCfg.Enabled

		var bestIP string
		var latency int64
		domainCount := 0
		for _, d := range rule.Domains {
			if r, ok := results[d.Domain]; ok && r.BestIP != "" {
				domainCount++
				if bestIP == "" {
					bestIP = r.BestIP
					latency = r.Latency
				}
			}
		}

		siteStatuses[rule.ID] = SiteStatus{
			Name:      rule.Name,
			Icon:      rule.Icon,
			Enabled:   enabled,
			BestIP:    bestIP,
			Latency:   latency,
			Domains:   len(rule.Domains),
			Connected: domainCount,
		}
	}

	return Status{
		Running: m.running,
		Mode:    m.switcher.GetMode(),
		Sites:   siteStatuses,
	}
}

type SiteStatus struct {
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Enabled   bool   `json:"enabled"`
	BestIP    string `json:"bestIP"`
	Latency   int64  `json:"latency"`
	Domains   int    `json:"domains"`
	Connected int    `json:"connected"`
}

type Status struct {
	Running bool                   `json:"running"`
	Mode    string                 `json:"mode"`
	Sites   map[string]SiteStatus  `json:"sites"`
}

func domainListFromConfig(cfg *config.AppConfig) []string {
	rules := config.GetEnabledDomains(cfg)
	domains := make([]string, len(rules))
	for i, r := range rules {
		domains[i] = r.Domain
	}
	return domains
}
