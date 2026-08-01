package intercept

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"directlink/internal/config"
	"directlink/internal/logger"
	"directlink/internal/prober"
	"golang.org/x/net/proxy"
)

// ProxyServer is a local HTTP CONNECT tunnel proxy that routes target domains to optimal IPs.
type ProxyServer struct {
	mu             sync.RWMutex
	listener       net.Listener
	port           int
	prober         *prober.Prober
	enabledDomains map[string]bool
	relayConfig    *config.RelayConfig
	socksDialer    proxy.Dialer
	closed         bool
}

func NewProxyServer(port int, prober *prober.Prober) *ProxyServer {
	return &ProxyServer{
		port:           port,
		prober:         prober,
		enabledDomains: make(map[string]bool),
	}
}

// SetRelay configures the SOCKS5 relay for blocked domains.
func (p *ProxyServer) SetRelay(cfg *config.RelayConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.relayConfig = cfg
	if cfg != nil && cfg.Enabled && cfg.Host != "" {
		addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		var auth *proxy.Auth
		if cfg.Username != "" {
			auth = &proxy.Auth{User: cfg.Username, Password: cfg.Password}
		}
		dialer, err := proxy.SOCKS5("tcp", addr, auth, &net.Dialer{Timeout: 10 * time.Second})
		if err != nil {
			logger.Error("SOCKS5 中转初始化失败: %v", err)
			return
		}
		p.socksDialer = dialer
		logger.Info("SOCKS5 中转已配置: %s", addr)
	} else {
		p.socksDialer = nil
	}
}

// SetEnabledDomains updates which domains should be accelerated.
func (p *ProxyServer) SetEnabledDomains(domains []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabledDomains = make(map[string]bool, len(domains))
	for _, d := range domains {
		p.enabledDomains[d] = true
	}
}

// Start begins listening for proxy connections.
func (p *ProxyServer) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", p.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Try to find an available port
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("无法启动代理服务器: %w", err)
		}
		p.port = ln.Addr().(*net.TCPAddr).Port
		logger.Info("代理端口 %d 被占用，改用 %d", p.port, p.port)
	}
	p.mu.Lock()
	p.closed = false
	p.mu.Unlock()

	logger.Info("代理服务器已启动: 127.0.0.1:%d", p.port)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				p.mu.RLock()
				closed := p.closed
				p.mu.RUnlock()
				if closed {
					return
				}
				logger.Error("代理接受连接失败: %v", err)
				continue
			}
			go p.handleConnection(conn)
		}
	}()

	return nil
}

// Stop closes the proxy server.
func (p *ProxyServer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	if p.listener != nil {
		p.listener.Close()
	}
	logger.Info("代理服务器已停止")
}

// GetPort returns the actual listening port (may differ from configured if port was in use).
func (p *ProxyServer) GetPort() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.port
}

// GetAddr returns the proxy address string.
func (p *ProxyServer) GetAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", p.GetPort())
}

// handleConnection handles a single proxy connection.
func (p *ProxyServer) handleConnection(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	reader := bufio.NewReader(conn)

	// Read the first line of the HTTP request
	line, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	line = strings.TrimSpace(line)

	// Parse the request
	if strings.HasPrefix(line, "CONNECT ") {
		p.handleConnect(conn, reader, line)
	} else {
		p.handleHTTP(conn, reader, line)
	}
}

// handleConnect handles an HTTP CONNECT method (HTTPS tunneling).
func (p *ProxyServer) handleConnect(conn net.Conn, reader *bufio.Reader, requestLine string) {
	parts := strings.Fields(requestLine)
	if len(parts) < 2 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	hostPort := parts[1]
	domain, port := splitHostPort(hostPort)

	p.mu.RLock()
	isAccelerated := p.enabledDomains[domain]
	socksDialer := p.socksDialer
	p.mu.RUnlock()

	var upstream net.Conn
	var err error

	if !isAccelerated {
		// Non-accelerated domain: direct dial only, no relay
		upstream, err = net.DialTimeout("tcp", hostPort, 10*time.Second)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
	} else if socksDialer != nil {
		// Accelerated domain with relay: try SOCKS5 with known-good IP first
		// Use known IPs from config rules to bypass VPS DNS pollution
		// (VPS DNS may resolve github.com to blocked Azure IPs too)
		knownIPs := config.GetKnownIPs(domain)
		var upstreamConnected bool

		for _, knownIP := range knownIPs {
			relayTarget := fmt.Sprintf("%s:%s", knownIP, port)
			logger.Debug("SOCKS5 中转 (已知IP): %s → %s", domain, relayTarget)
			upstream, err = socksDialer.Dial("tcp", relayTarget)
			if err == nil {
				logger.Debug("SOCKS5 中转成功: %s @ %s", domain, knownIP)
				upstreamConnected = true
				break
			}
			logger.Debug("SOCKS5 中转失败 %s @ %s: %v", domain, knownIP, err)
		}

		if !upstreamConnected {
			// All known IPs failed — try domain name as last resort
			logger.Debug("已知IP全部失败，尝试域名中转 %s...", hostPort)
			upstream, err = socksDialer.Dial("tcp", hostPort)
			if err == nil {
				upstreamConnected = true
				logger.Debug("域名中转成功: %s", domain)
			}
		}

		if !upstreamConnected {
			// SOCKS5 fully failed — try direct with prober's best IP
			logger.Debug("SOCKS5 全部失败，尝试直连 %s...", domain)
			bestIP := ""
			if p.prober != nil {
				bestIP = p.prober.GetBestIP(domain)
			}
			if bestIP != "" {
				upstream, err = net.DialTimeout("tcp", fmt.Sprintf("%s:%s", bestIP, port), 10*time.Second)
			} else {
				upstream, err = net.DialTimeout("tcp", hostPort, 10*time.Second)
			}
			if err != nil {
				logger.Debug("直连也失败 %s: %v", domain, err)
				conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
				return
			}
			logger.Debug("直连成功: %s", domain)
		}
	} else {
		// Accelerated domain without relay: direct dial with best IP
		targetAddr := hostPort
		if p.prober != nil {
			bestIP := p.prober.GetBestIP(domain)
			if bestIP != "" {
				targetAddr = fmt.Sprintf("%s:%s", bestIP, port)
			}
		}
		upstream, err = net.DialTimeout("tcp", targetAddr, 10*time.Second)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
	}
	defer upstream.Close()

	// Respond to client that the tunnel is established
	conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// Reset deadline — tunnel is established, no timeout
	conn.SetDeadline(time.Time{})

	// Bidirectional copy
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(upstream, conn)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(conn, upstream)
		done <- struct{}{}
	}()

	<-done
}

// handleHTTP handles a plain HTTP request (non-CONNECT).
func (p *ProxyServer) handleHTTP(conn net.Conn, reader *bufio.Reader, requestLine string) {
	// Parse HTTP request to get Host header
	// Read remaining headers
	var headers string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		headers += line
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	// Extract Host from headers
	host := ""
	for _, hline := range strings.Split(headers, "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(hline)), "host:") {
			host = strings.TrimSpace(strings.SplitN(hline, ":", 2)[1])
			break
		}
	}

	if host == "" {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	domain, port := splitHostPort(host)
	if port == "" {
		port = "80"
	}

	targetAddr := fmt.Sprintf("%s:%s", host, port)
	p.mu.RLock()
	isAccelerated := p.enabledDomains[domain]
	p.mu.RUnlock()

	if isAccelerated && p.prober != nil {
		bestIP := p.prober.GetBestIP(domain)
		if bestIP != "" {
			targetAddr = fmt.Sprintf("%s:%s", bestIP, port)
		}
	}

	upstream, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer upstream.Close()

	// Forward the original request
	upstream.Write([]byte(requestLine + "\r\n"))
	upstream.Write([]byte(headers))

	// Bidirectional copy
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(upstream, conn)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(conn, upstream)
		done <- struct{}{}
	}()

	<-done
}

// splitHostPort splits "host:port" into host and port.
// Handles IPv6 addresses.
func splitHostPort(hostPort string) (string, string) {
	// Handle [IPv6]:port format
	if strings.HasPrefix(hostPort, "[") {
		idx := strings.LastIndex(hostPort, "]")
		if idx > 0 {
			host := hostPort[1:idx]
			if idx+1 < len(hostPort) && hostPort[idx+1] == ':' {
				return host, hostPort[idx+2:]
			}
			return host, ""
		}
	}

	// Regular host:port
	idx := strings.LastIndex(hostPort, ":")
	if idx < 0 {
		return hostPort, ""
	}
	return hostPort[:idx], hostPort[idx+1:]
}
