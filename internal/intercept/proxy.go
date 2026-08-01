package intercept

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"directlink/internal/logger"
	"directlink/internal/prober"
)

// ProxyServer is a local HTTP CONNECT tunnel proxy that routes target domains to optimal IPs.
type ProxyServer struct {
	mu       sync.RWMutex
	listener net.Listener
	port     int
	prober   *prober.Prober
	enabledDomains map[string]bool // domain -> accelerated?
	closed   bool
}

func NewProxyServer(port int, prober *prober.Prober) *ProxyServer {
	return &ProxyServer{
		port:           port,
		prober:         prober,
		enabledDomains: make(map[string]bool),
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
	p.listener = ln

	logger.Info("代理服务器已启动: 127.0.0.1:%d", p.port)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if p.closed {
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
	// Parse: CONNECT host:port HTTP/1.1
	parts := strings.Fields(requestLine)
	if len(parts) < 2 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	hostPort := parts[1]
	domain, port := splitHostPort(hostPort)

	// Determine target: use optimal IP if this is an accelerated domain
	targetAddr := hostPort
	p.mu.RLock()
	isAccelerated := p.enabledDomains[domain]
	p.mu.RUnlock()

	if isAccelerated && p.prober != nil {
		bestIP := p.prober.GetBestIP(domain)
		if bestIP != "" {
			targetAddr = fmt.Sprintf("%s:%s", bestIP, port)
			logger.Debug("代理加速: %s → %s", domain, targetAddr)
		}
	}

	// Connect to target
	upstream, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		logger.Debug("代理连接目标失败 %s: %v", targetAddr, err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
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
