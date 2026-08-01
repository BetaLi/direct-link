package intercept

import (
	"fmt"
	"sync"
	"time"

	"directlink/internal/logger"
	"directlink/internal/prober"
	"directlink/internal/system"
)

// Switcher manages the automatic switching between hosts mode and proxy mode.
type Switcher struct {
	mu          sync.Mutex
	mode        string // "hosts", "proxy", "off"
	hostsMgr    *HostsMgr
	proxy       *ProxyServer
	prober      *prober.Prober
	domains     []string
	stopCh      chan struct{}
	healthInterval int
	probeInterval  int
}

func NewSwitcher(hostsMgr *HostsMgr, proxy *ProxyServer, prober *prober.Prober) *Switcher {
	return &Switcher{
		mode:           "off",
		hostsMgr:      hostsMgr,
		proxy:         proxy,
		prober:        prober,
		healthInterval: 60,
		probeInterval:  1800,
	}
}

// SetDomains sets the list of domains to manage.
func (s *Switcher) SetDomains(domains []string) {
	s.mu.Lock()
	s.domains = domains
	s.mu.Unlock()
	s.proxy.SetEnabledDomains(domains)
}

// SetIntervals configures health check and probe intervals.
func (s *Switcher) SetIntervals(healthSec, probeSec int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthInterval = healthSec
	s.probeInterval = probeSec
}

// GetMode returns the current mode.
func (s *Switcher) GetMode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// Start activates acceleration: probes IPs, then starts in hosts mode.
func (s *Switcher) Start() error {
	s.mu.Lock()
	if s.mode != "off" {
		s.mu.Unlock()
		return fmt.Errorf("加速已在运行中")
	}
	s.mu.Unlock()

	logger.Info("开始探测最优 IP...")

	// Probe all domains
	results := s.prober.ProbeDomains(s.domains)
	if len(results) == 0 {
		return fmt.Errorf("所有域名探测失败，请检查网络连接")
	}

	// If relay is configured, go straight to proxy mode (all traffic through SOCKS5)
	// Otherwise, try hosts mode first (falls back to proxy internally)
	s.mu.Lock()
	hasRelay := s.proxy.socksDialer != nil
	s.mu.Unlock()

	if hasRelay {
		logger.Info("检测到 SOCKS5 中转，直接使用代理模式（全量中转）")
		s.enableProxyMode()
	} else {
		s.enableHostsMode(results)
	}

	// Verify that a mode was actually engaged
	s.mu.Lock()
	mode := s.mode
	s.mu.Unlock()

	if mode == "off" {
		return fmt.Errorf("加速启动失败：hosts 和代理模式均无法启用，请以管理员身份运行")
	}

	// Hint to user about browser DNS cache
	if mode == "hosts" {
		logger.Info("提示：如页面仍打不开，请重启浏览器或使用无痕模式（浏览器有独立 DNS 缓存）")
	}
	if mode == "proxy" {
		logger.Info("提示：代理模式已启动，所有加速域名通过 SOCKS5 中转")
	}

	// Start health check loop
	s.stopCh = make(chan struct{})
	go s.healthCheckLoop()

	return nil
}

// Stop deactivates acceleration and cleans up.
func (s *Switcher) Stop() {
	s.mu.Lock()
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
	mode := s.mode
	s.mode = "off"
	s.mu.Unlock()

	if mode == "hosts" {
		s.hostsMgr.Clean()
		logger.Info("已停止 hosts 加速")
	} else if mode == "proxy" {
		system.ClearSystemProxy()
		s.proxy.Stop()
		logger.Info("已停止代理加速")
	}
}

// enableHostsMode writes hosts entries and sets mode.
func (s *Switcher) enableHostsMode(results map[string]*prober.DomainResult) {
	entries := make(map[string]string)
	for domain, result := range results {
		if result.BestIP != "" {
			entries[domain] = result.BestIP
		}
	}

	if len(entries) == 0 {
		logger.Warn("无可用 IP 写入 hosts，切换到代理模式")
		s.enableProxyMode()
		return
	}

	if err := s.hostsMgr.WriteEntries(entries); err != nil {
		logger.Error("写入 hosts 失败，切换到代理模式: %v", err)
		s.enableProxyMode()
		return
	}

	s.mu.Lock()
	s.mode = "hosts"
	s.mu.Unlock()
	logger.Info("已切换到 hosts 模式，加速 %d 个域名", len(entries))
}

// enableProxyMode starts the proxy server and sets system proxy.
// Returns error if the proxy or system proxy setup fails.
func (s *Switcher) enableProxyMode() error {
	// Clean hosts first
	s.hostsMgr.Clean()

	// Start proxy server
	if err := s.proxy.Start(); err != nil {
		logger.Error("启动代理服务器失败: %v", err)
		return err
	}

	// Set system proxy
	if err := system.SetSystemProxy(s.proxy.GetAddr()); err != nil {
		logger.Error("设置系统代理失败: %v", err)
		s.proxy.Stop()
		return err
	}

	s.mu.Lock()
	s.mode = "proxy"
	s.mu.Unlock()

	logger.Info("已切换到代理模式: %s", s.proxy.GetAddr())
	return nil
}

// switchToProxy switches from hosts to proxy mode.
func (s *Switcher) switchToProxy() {
	logger.Info("hosts 健康检测失败，切换到代理模式...")
	if err := s.enableProxyMode(); err != nil {
		logger.Error("切换到代理模式失败: %v", err)
	}
}

// switchToHosts switches from proxy to hosts mode.
func (s *Switcher) switchToHosts() {
	logger.Info("尝试切换回 hosts 模式...")

	// Re-probe
	results := s.prober.ProbeDomains(s.domains)
	if len(results) == 0 {
		logger.Warn("重新探测无结果，保持代理模式")
		return
	}

	// Check if all domains have TLS-valid IPs
	allValid := true
	for _, r := range results {
		if r.BestIP == "" {
			allValid = false
			break
		}
	}

	if !allValid {
		logger.Warn("部分域名无有效 IP，保持代理模式")
		return
	}

	// Stop proxy, enable hosts
	system.ClearSystemProxy()
	s.proxy.Stop()
	s.enableHostsMode(results)
}

// healthCheckLoop periodically checks if the current mode is still working.
func (s *Switcher) healthCheckLoop() {
	healthTicker := time.NewTicker(time.Duration(s.healthInterval) * time.Second)
	probeTicker := time.NewTicker(time.Duration(s.probeInterval) * time.Second)
	defer healthTicker.Stop()
	defer probeTicker.Stop()

	for {
		select {
		case <-s.stopCh:
			return

		case <-healthTicker.C:
			s.doHealthCheck()

		case <-probeTicker.C:
			s.doReprobe()
		}
	}
}

// doHealthCheck checks if the current mode is still functional.
func (s *Switcher) doHealthCheck() {
	s.mu.Lock()
	mode := s.mode
	domains := s.domains
	s.mu.Unlock()

	if mode == "hosts" {
		// Check if hosts entries are still working
		failed := 0
		updatedEntries := make(map[string]string)
		for _, domain := range domains {
			result, ok := s.prober.GetResult(domain)
			if !ok || result.BestIP == "" {
				continue
			}
			// Try main IP + backup IPs (returns working IP or empty)
			workingIP := s.prober.CheckHealthWithBackups(domain)
			if workingIP == "" {
				failed++
			} else {
				updatedEntries[domain] = workingIP
			}
		}

		if failed > 0 {
			logger.Warn("hosts 健康检测: %d/%d 域名不可达", failed, len(domains))
			if failed > len(domains)/2 {
				s.switchToProxy()
			} else {
				// Some domains failed but majority OK — update hosts with working IPs
				if len(updatedEntries) > 0 {
					if err := s.hostsMgr.WriteEntries(updatedEntries); err != nil {
						logger.Error("健康检测后更新 hosts 失败: %v", err)
					} else {
						logger.Info("hosts 已更新（部分域名切换到备用 IP）")
					}
				}
			}
		} else {
			logger.Debug("hosts 健康检测: 全部正常")
		}
	} else if mode == "proxy" {
		// Check if proxy is still serving
		// Try switching back to hosts periodically
		if time.Now().Second() < 30 { // roughly every ~4 cycles
			s.switchToHosts()
		}
	}
}

// doReprobe re-probes all domains and updates IPs.
func (s *Switcher) doReprobe() {
	logger.Info("定期重新探测...")
	s.mu.Lock()
	mode := s.mode
	s.mu.Unlock()

	results := s.prober.ProbeDomains(s.domains)
	if len(results) == 0 {
		logger.Warn("重新探测无结果")
		return
	}

	if mode == "hosts" {
		// Update hosts entries
		entries := make(map[string]string)
		for domain, result := range results {
			if result.BestIP != "" {
				entries[domain] = result.BestIP
			}
		}
		if len(entries) > 0 {
			if err := s.hostsMgr.WriteEntries(entries); err != nil {
				logger.Error("更新 hosts 失败: %v", err)
			} else {
				logger.Info("hosts 已刷新 (%d 条)", len(entries))
			}
		}
	}
}

