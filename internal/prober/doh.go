package prober

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"directlink/internal/config"
	"directlink/internal/logger"
)

// DoHResponse represents a DNS-over-HTTPS JSON response (Cloudflare/Google format).
type DoHResponse struct {
	Status   int         `json:"Status"`
	Answer   []DoHAnswer `json:"Answer"`
	Authoritative bool   `json:"Authoritative,omitempty"`
}

type DoHAnswer struct {
	Name string `json:"name"`
	Type int    `json:"type"`
	TTL  int    `json:"TTL"`
	Data string `json:"data"`
}

// DoH providers — Chinese providers first (reachable in China), international as fallback.
// We use IP-direct URLs to avoid DNS resolution of the DoH server itself.
var dohEndpoints = map[string]string{
	"alidns":     "https://223.5.5.5/dns-query",
	"dnspod":     "https://1.12.12.12/dns-query",
	"cloudflare": "https://1.1.1.1/dns-query",
	"google":     "https://8.8.8.8/resolve",
}

// dohQuery queries a single DoH provider for A records of the given domain.
func (p *Prober) dohQuery(domain string, provider string) []string {
	url, ok := dohEndpoints[provider]
	if !ok {
		return nil
	}

	reqURL := url + "?name=" + domain + "&type=A"
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/dns-json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Debug("DoH 查询 %s @ %s 失败: %v", domain, provider, err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var dohResp DoHResponse
	if err := json.Unmarshal(body, &dohResp); err != nil {
		return nil
	}

	if dohResp.Status != 0 {
		return nil
	}

	var ips []string
	for _, ans := range dohResp.Answer {
		if ans.Type == 1 { // Type 1 = A record
			if isValidPublicIP(ans.Data) {
				ips = append(ips, ans.Data)
			}
		}
	}
	return ips
}

// dohQueryAll queries all configured DoH providers concurrently.
// Each provider is queried 2 times to get more candidate IPs from CDN rotation.
// Also merges known-good IPs from config rules, and tries system DNS as fallback.
func (p *Prober) dohQueryAll(domain string) []string {
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := make(map[string]bool)

	// Add known-good IPs first (from domain rules — bypasses DNS entirely)
	for _, ip := range config.GetKnownIPs(domain) {
		if isValidPublicIP(ip) {
			seen[ip] = true
		}
	}

	for _, provider := range p.dohProviders {
		// Query each provider 2 times (CDN load balancing returns different IPs)
		for round := 0; round < 2; round++ {
			wg.Add(1)
			go func(prov string, r int) {
				defer wg.Done()
				// Small stagger between rounds
				time.Sleep(time.Duration(r) * 200 * time.Millisecond)
				ips := p.dohQuery(domain, prov)
				mu.Lock()
				for _, ip := range ips {
					if !seen[ip] {
						seen[ip] = true
					}
				}
				mu.Unlock()
			}(provider, round)
		}
	}

	// System DNS fallback
	wg.Add(1)
	go func() {
		defer wg.Done()
		systemIPs := systemDNSLookup(domain)
		mu.Lock()
		for _, ip := range systemIPs {
			if !seen[ip] && isValidPublicIP(ip) {
				seen[ip] = true
			}
		}
		mu.Unlock()
	}()

	wg.Wait()

	// If we got very few IPs, try expanding by scanning /24 subnet neighbors
	result := make([]string, 0, len(seen))
	for ip := range seen {
		result = append(result, ip)
	}

	if len(result) <= 2 {
		expanded := p.expandBySubnet(result)
		logger.Info("候选 IP 少(%d)，扩展 /24 子网扫描 → %d 个", len(result), len(expanded))
		result = expanded
	}

	return result
}

// expandBySubnet takes a list of IPs and adds neighbors in the same /24 subnet.
// This helps with CDN domains where nearby IPs serve the same content.
func (p *Prober) expandBySubnet(ips []string) []string {
	seen := make(map[string]bool)
	for _, ip := range ips {
		seen[ip] = true
	}

	for _, ip := range ips {
		parts := strings.Split(ip, ".")
		if len(parts) != 4 {
			continue
		}
		base := parts[0] + "." + parts[1] + "." + parts[2] + "."
		// Add a few neighbors in the same /24 (not too many — each adds probe time)
		for _, lastOctet := range []int{1, 2, 3} {
			neighbor := fmt.Sprintf("%s%d", base, lastOctet)
			if !seen[neighbor] && isValidPublicIP(neighbor) {
				seen[neighbor] = true
			}
		}
	}

	result := make([]string, 0, len(seen))
	for ip := range seen {
		result = append(result, ip)
	}
	return result
}

// isValidPublicIP checks if a string is a valid public IPv4 address.
func isValidPublicIP(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}

	octets := make([]int, 4)
	for i, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return false
		}
		octets[i] = n
	}

	first := octets[0]

	if first == 127 { return false }           // loopback
	if first == 10 { return false }             // private
	if first == 172 && octets[1] >= 16 && octets[1] <= 31 { return false } // private
	if first == 192 && octets[1] == 168 { return false } // private
	if first == 169 && octets[1] == 254 { return false } // link-local
	if first == 0 { return false }              // 0.x reserved
	if first == 100 && octets[1] >= 64 && octets[1] <= 127 { return false } // CGNAT
	if first >= 224 { return false }            // multicast/reserved

	return true
}

// isValidIP checks if a string is a valid IPv4 address (any range).
func isValidIP(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}
