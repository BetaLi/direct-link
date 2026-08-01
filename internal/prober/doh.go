package prober

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
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

// DoH providers — we use IP-direct URLs to avoid DNS resolution of the DoH server itself.
var dohEndpoints = map[string]string{
	"cloudflare": "https://1.1.1.1/dns-query",
	"google":     "https://8.8.8.8/resolve",
	"alidns":     "https://223.5.5.5/dns-query",
}

// dohQuery queries a single DoH provider for A records of the given domain.
func (p *Prober) dohQuery(domain string, provider string) []string {
	url, ok := dohEndpoints[provider]
	if !ok {
		return nil
	}

	// Build request with type=A query
	reqURL := url + "?name=" + domain + "&type=A"
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/dns-json")

	// Use a shorter timeout client
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
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
		// Type 1 = A record
		if ans.Type == 1 {
			// Filter out private/loopback/link-local IPs
			if isValidPublicIP(ans.Data) {
				ips = append(ips, ans.Data)
			}
		}
	}
	return ips
}

// dohQueryAll queries all configured DoH providers concurrently and returns deduplicated candidate IPs.
func (p *Prober) dohQueryAll(domain string) []string {
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := make(map[string]bool)

	for _, provider := range p.dohProviders {
		wg.Add(1)
		go func(prov string) {
			defer wg.Done()
			ips := p.dohQuery(domain, prov)
			mu.Lock()
			for _, ip := range ips {
				if !seen[ip] {
					seen[ip] = true
				}
			}
			mu.Unlock()
		}(provider)
	}

	// Also try system DNS concurrently as a fallback
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

	// Convert to slice
	result := make([]string, 0, len(seen))
	for ip := range seen {
		result = append(result, ip)
	}
	return result
}

// isValidPublicIP checks if a string is a valid public IPv4 address.
// Filters out loopback, private, link-local, and other non-public IPs.
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

	// Filter out loopback (127.x.x.x)
	if first == 127 {
		return false
	}
	// Filter out private ranges (10.x, 172.16-31.x, 192.168.x)
	if first == 10 {
		return false
	}
	if first == 172 && octets[1] >= 16 && octets[1] <= 31 {
		return false
	}
	if first == 192 && octets[1] == 168 {
		return false
	}
	// Filter out link-local (169.254.x.x)
	if first == 169 && octets[1] == 254 {
		return false
	}
	// Filter out 0.x.x.x
	if first == 0 {
		return false
	}

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
