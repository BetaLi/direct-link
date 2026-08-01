package prober

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"directlink/internal/logger"
)

// checkTLS attempts a TLS handshake to the IP with the given domain as SNI.
// Returns true if the handshake succeeds and the certificate is valid for the domain.
func (p *Prober) checkTLS(domain string, ip string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "443"), 3*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         domain,
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS12,
	})

	if err := tlsConn.Handshake(); err != nil {
		logger.Debug("TLS 握手失败 %s @ %s: %v", domain, ip, err)
		return false
	}

	// Verify the certificate chain
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return false
	}

	// Verify the leaf certificate is valid for this domain
	cert := state.PeerCertificates[0]
	err = cert.VerifyHostname(domain)
	if err != nil {
		logger.Debug("证书域名不匹配 %s @ %s: %v", domain, ip, err)
		return false
	}

	tlsConn.Close()
	return true
}

// checkHTTP sends a HEAD request to verify the IP actually serves the domain correctly.
func (p *Prober) checkHTTP(domain string, ip string) bool {
	// Create a custom transport that connects to the specific IP
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName: domain,
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Replace the address with our target IP
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip, "443"))
		},
		ResponseHeaderTimeout: 8 * time.Second,
	}

	// Use a client that doesn't follow redirects
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
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

	// Accept any HTTP response (including redirects, 4xx, 5xx) as "working"
	// The main concern is that we get a real response, not a timeout or connection reset
	return resp.StatusCode > 0
}
