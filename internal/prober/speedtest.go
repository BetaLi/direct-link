package prober

import (
	"net"
	"sort"
	"sync"
	"time"
)

// tcpResult holds TCP probe result for a single IP.
type tcpResult struct {
	IP  string
	RTT time.Duration
	OK  bool
}

// tcpSpeedTest tests TCP 443 connectivity and measures RTT for all candidate IPs concurrently.
func (p *Prober) tcpSpeedTest(ips []string, domain string) []IPResult {
	var wg sync.WaitGroup
	results := make([]IPResult, len(ips))

	for i, ip := range ips {
		wg.Add(1)
		go func(idx int, candidate string) {
			defer wg.Done()
			rtt, ok := tcpProbe(candidate, "443", 3*time.Second)
			results[idx] = IPResult{
				IP:    candidate,
				RTT:   rtt,
				TLSOK: false,
			}
			if !ok {
				results[idx].RTT = 0 // mark as failed (0 = failed)
			}
		}(i, ip)
	}

	wg.Wait()

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

	// Limit to maxIPs*2
	if len(valid) > p.maxIPs*2 {
		valid = valid[:p.maxIPs*2]
	}

	return valid
}

// tcpProbe dials a TCP connection and measures RTT.
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
