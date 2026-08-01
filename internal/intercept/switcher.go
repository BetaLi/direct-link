package intercept

import (
	"fmt"
	"sync"
	"time"

	"directlink/internal/logger"
	"directlink/internal/prober"
	"directlink/internal/system"
)

// Switcher manages the proxy-based acceleration via SOCKS5 relay.
type Switcher struct {
	mu            sync.Mutex
	mode          string // "proxy" or "off"
	proxy         *ProxyServer
	prober        *prober.Prober
	domains       []string
	stopCh        chan struct{}
	probeInterval int
}

func NewSwitcher(proxy *ProxyServer, prober *prober.Prober) *Switcher {
	return &Switcher{
		mode:          "off",
		proxy:         proxy,
		prober:        prober,
		probeInterval: 1800,
	}
}

// SetDomains sets the list of domains to manage.
func (s *Switcher) SetDomains(domains []string) {
	s.mu.Lock()
	s.domains = domains
	s.mu.Unlock()
	s.proxy.SetEnabledDomains(domains)
}

// SetIntervals configures the probe interval (health check interval is unused in proxy-only mode).
func (s *Switcher) SetIntervals(healthSec, probeSec int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probeInterval = probeSec
}

// GetMode returns the current mode.
func (s *Switcher) GetMode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// Start activates acceleration: starts proxy immediately, probes in background.
func (s *Switcher) Start() error {
	s.mu.Lock()
	if s.mode != "off" {
		s.mu.Unlock()
		return fmt.Errorf("加速已在运行中")
	}
	s.mu.Unlock()

	// Start proxy immediately — no waiting for probing
	if err := s.enableProxyMode(); err != nil {
		return err
	}

	logger.Info("代理已启动，加速域名先试直连，失败则自动走 SOCKS5 中转")

	// Start background probe loop (refreshes known IPs for SOCKS5 fallback)
	s.stopCh = make(chan struct{})
	go s.probeLoop()

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

	if mode == "proxy" {
		system.ClearSystemProxy()
		s.proxy.Stop()
		logger.Info("已停止代理加速")
	}
}

// enableProxyMode starts the proxy server and sets system proxy.
func (s *Switcher) enableProxyMode() error {
	if err := s.proxy.Start(); err != nil {
		logger.Error("启动代理服务器失败: %v", err)
		return err
	}

	if err := system.SetSystemProxy(s.proxy.GetAddr()); err != nil {
		logger.Error("设置系统代理失败: %v", err)
		s.proxy.Stop()
		return err
	}

	s.mu.Lock()
	s.mode = "proxy"
	s.mu.Unlock()

	logger.Info("代理模式已启动: %s", s.proxy.GetAddr())
	return nil
}

// probeLoop runs an initial probe immediately, then periodically re-probes to refresh known IPs.
func (s *Switcher) probeLoop() {
	// Initial probe in background (non-blocking for Start)
	s.doReprobe()

	probeTicker := time.NewTicker(time.Duration(s.probeInterval) * time.Second)
	defer probeTicker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-probeTicker.C:
			s.doReprobe()
		}
	}
}

// doReprobe re-probes all domains and updates cached IPs for SOCKS5 fallback.
func (s *Switcher) doReprobe() {
	logger.Info("后台探测域名 IP...")
	results := s.prober.ProbeDomains(s.domains)
	if len(results) == 0 {
		logger.Warn("后台探测无结果")
		return
	}
	logger.Info("后台探测完成，已更新 %d 个域名的 IP", len(results))
}
