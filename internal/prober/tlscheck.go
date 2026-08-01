package prober

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"directlink/internal/config"
	"directlink/internal/logger"
)

// checkTLS attempts a TLS handshake to the IP with the given domain as SNI.
// Retries up to 2 times with a longer timeout (8s per attempt).
// Returns true if the handshake succeeds and the certificate is valid for the domain.
func (p *Prober) checkTLS(domain string, ip string) bool {
	return p.checkTLSWithRetry(domain, ip, 2)
}

// checkTLSWithRetry attempts TLS handshake with configurable retries.
func (p *Prober) checkTLSWithRetry(domain string, ip string, maxRetries int) bool {
	var lastErr string

	for attempt := 0; attempt < maxRetries; attempt++ {
		ok, err := p.tryTLS(domain, ip, 8*time.Second)
		if ok {
			return true
		}
		if err != "" {
			lastErr = err
		}

		// If the error is a connection reset (SNI blocking), retrying won't help
		// But if it's a timeout, retry might succeed
		if attempt < maxRetries-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	if lastErr != "" {
		logger.Debug("TLS 握手失败 %s @ %s (重试 %d 次): %s", domain, ip, maxRetries, lastErr)
	}
	return false
}

// tryTLS does a single TLS handshake attempt.
func (p *Prober) tryTLS(domain string, ip string, timeout time.Duration) (bool, string) {
	// Use a combined dial+TLS timeout
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp",
		net.JoinHostPort(ip, "443"),
		&tls.Config{
			ServerName:         domain,
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		})
	if err != nil {
		return false, err.Error()
	}
	defer conn.Close()

	// Verify the certificate chain
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return false, "no peer certificates"
	}

	// Verify the leaf certificate is valid for this domain
	cert := state.PeerCertificates[0]
	if err := cert.VerifyHostname(domain); err != nil {
		return false, "证书域名不匹配: " + err.Error()
	}

	return true, ""
}

// CheckHealth checks if a domain is still reachable at the given IP via TLS.
// For domains that need port 22, also verifies SSH port.
func (p *Prober) CheckHealth(domain string, ip string) bool {
	if !p.checkTLSWithRetry(domain, ip, 2) {
		return false
	}
	if config.NeedsPort22(domain) {
		if !p.checkPort22(ip) {
			return false
		}
	}
	return true
}

// CheckHealthWithBackups checks the best IP (and port 22 if needed),
// and if it fails, tries backup IPs. Returns the working IP or empty string.
func (p *Prober) CheckHealthWithBackups(domain string) string {
	p.mu.RLock()
	result, ok := p.results[domain]
	p.mu.RUnlock()
	if !ok || result.BestIP == "" {
		return ""
	}

	// Try best IP first
	if p.CheckHealth(domain, result.BestIP) {
		return result.BestIP
	}

	// Try backup IPs
	for _, backupIP := range result.BackupIPs {
		logger.Debug("主 IP 失败，尝试备用 IP %s @ %s", backupIP, domain)
		if p.CheckHealth(domain, backupIP) {
			// Promote backup to best
			p.mu.Lock()
			if r, ok := p.results[domain]; ok {
				oldBest := r.BestIP
				r.BestIP = backupIP
				// Move old best to backups
				newBackups := []string{oldBest}
				for _, b := range r.BackupIPs {
					if b != backupIP {
						newBackups = append(newBackups, b)
					}
				}
				r.BackupIPs = newBackups
			}
			p.mu.Unlock()
			logger.Info("域名 %s 已切换到备用 IP: %s", domain, backupIP)
			return backupIP
		}
	}

	return ""
}

// checkHTTP sends a HEAD request to verify the IP actually serves the domain correctly.
func (p *Prober) checkHTTP(domain string, ip string) bool {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName: domain,
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip, "443"))
		},
		ResponseHeaderTimeout: 10 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	url := "https://" + domain + "/"
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false
	}
	req.Host = domain

	resp, err := client.Do(req)
	if err != nil {
		logger.Debug("HTTP 检测失败 %s @ %s: %v", domain, ip, err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode > 0
}
