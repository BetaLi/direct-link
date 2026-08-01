package prober

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"directlink/internal/logger"
)

// tcpResult holds TCP probe result for a single IP.
type tcpResult struct {
	IP   string
	RTT  time.Duration
	OK   bool
	Err  string // error description if failed
}

// tcpSpeedTest tests TCP 443 connectivity and measures RTT for all candidate IPs concurrently.
// Each IP is probed with a retry — if the first attempt fails, a second attempt is made.
func (p *Prober) tcpSpeedTest(ips []string, domain string) []IPResult {
	var wg sync.WaitGroup
	results := make([]IPResult, len(ips))

	for i, ip := range ips {
		wg.Add(1)
		go func(idx int, candidate string) {
			defer wg.Done()
			rtt, errStr, ok := tcpProbeWithRetry(candidate, "443", 4*time.Second, 1)
			results[idx] = IPResult{
				IP:    candidate,
				RTT:   rtt,
				TLSOK: false,
			}
			if !ok {
				results[idx].RTT = 0
				logger.Debug("TCP %s @ %s 失败: %s", domain, candidate, errStr)
			}
		}(i, ip)
	}

	wg.Wait()

	// Log summary
	succeeded := 0
	failed := 0
	for _, r := range results {
		if r.RTT > 0 {
			succeeded++
		} else {
			failed++
		}
	}
	logger.Info("TCP 探测 %s: %d 成功, %d 失败 (共 %d 个候选)", domain, succeeded, failed, len(results))

	// Filter out failed connections
	var valid []IPResult
	for _, r := range results {
		if r.RTT > 0 {
			valid = append(valid, r)
		}
	}

	// Sort by RTT ascending
	sort.Slice(valid, func(i, j int) bool {
		return valid[i].RTT < valid[j].RTT
	})

	// Limit to maxIPs*3 (allow more candidates)
	limit := p.maxIPs * 3
	if limit < 10 {
		limit = 10
	}
	if len(valid) > limit {
		valid = valid[:limit]
	}

	return valid
}

// tcpProbeWithRetry dials a TCP connection with retry.
// Returns RTT, error string, and success boolean.
func tcpProbeWithRetry(ip string, port string, timeout time.Duration, maxRetries int) (time.Duration, string, bool) {
	var lastErr string

	for attempt := 0; attempt < maxRetries; attempt++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), timeout)
		if err == nil {
			rtt := time.Since(start)
			conn.Close()
			return rtt, "", true
		}
		lastErr = err.Error()

		// If connection was refused (not timeout), no point retrying
		// If it was a timeout, retry might get through
		if !isTimeout(err) && attempt > 0 {
			break
		}
	}

	return 0, lastErr, false
}

// isTimeout checks if an error is a timeout.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return len(s) > 0 && (contains(s, "timeout") || contains(s, "i/o timeout") || contains(s, "deadline"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > len(sub) && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// tcpProbe dials a TCP connection and measures RTT (no retry).
func tcpProbe(ip string, port string, timeout time.Duration) (time.Duration, bool) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), timeout)
	if err != nil {
		return 0, false
	}
	rtt := time.Since(start)
	conn.Close()
	return rtt, true
}

// systemDNSLookup does a standard system DNS lookup as fallback.
func systemDNSLookup(domain string) []string {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return nil
	}
	var result []string
	for _, ip := range ips {
		if ip.To4() != nil {
			result = append(result, ip.String())
		}
	}
	return result
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%dms", d.Milliseconds())
}
