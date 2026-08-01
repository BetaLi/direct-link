package prober

import (
	"crypto/tls"
	"net"
	"time"

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
