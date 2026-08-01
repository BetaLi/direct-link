package prober

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"directlink/internal/logger"
)

// IPResult holds a probed IP with its metrics.
type IPResult struct {
	IP       string        `json:"ip"`
	RTT      time.Duration `json:"rtt"`
	TLSOK    bool          `json:"tlsOk"`
	HTTPOK   bool          `json:"httpOk"`
}

// DomainResult holds the best IPs for a domain.
type DomainResult struct {
	Domain    string     `json:"domain"`
	BestIP    string     `json:"bestIP"`
	BackupIPs []string   `json:"backupIPs"`
	Latency   int64      `json:"latency"` // ms
	ProbedAt  time.Time  `json:"probedAt"`
	AllIPs    []IPResult `json:"allIPs,omitempty"`
}

// Prober is the main probing engine.
type Prober struct {
	mu          sync.RWMutex
	results     map[string]*DomainResult // domain -> result
	dohClient   *http.Client
	httpClient  *http.Client
	maxIPs      int
	dohProviders []string
}

func New(maxIPs int, dohProviders []string) *Prober {
	if maxIPs <= 0 {
		maxIPs = 5
	}
	if len(dohProviders) == 0 {
		dohProviders = []string{"cloudflare", "google", "alidns"}
	}
	return &Prober{
		results:     make(map[string]*DomainResult),
		dohClient:   &http.Client{Timeout: 10 * time.Second},
		httpClient:  &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }},
		maxIPs:      maxIPs,
		dohProviders: dohProviders,
	}
}

// ProbeDomain probes a single domain and returns the result.
func (p *Prober) ProbeDomain(domain string) (*DomainResult, error) {
	logger.Info("开始探测域名: %s", domain)

	// Step 1: DoH query — collect candidate IPs from all providers (multi-round)
	candidates := p.dohQueryAll(domain)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("DoH 查询无结果: %s", domain)
	}
	logger.Info("DoH 查询 %s 得到 %d 个候选IP: %v", domain, len(candidates), candidates)

	// Step 2: TCP speed test — measure RTT, filter failures (with retry)
	tcpResults := p.tcpSpeedTest(candidates, domain)
	if len(tcpResults) == 0 {
		// Log the IPs that were tried
		var ipList []string
		for _, c := range candidates {
			ipList = append(ipList, c)
		}
		return nil, fmt.Errorf("所有候选 IP TCP 连接失败: %s (试过: %v)", domain, ipList)
	}

	// Step 3: TLS verification — verify certificate matches domain
	for i := range tcpResults {
		tcpResults[i].TLSOK = p.checkTLS(domain, tcpResults[i].IP)
	}

	// Log TLS results
	tlsOK := 0
	for _, r := range tcpResults {
		if r.TLSOK {
			tlsOK++
		}
	}
	logger.Info("TLS 验证 %s: %d/%d 通过", domain, tlsOK, len(tcpResults))

	// Step 4: HTTP availability check on TLS-OK IPs
	for i := range tcpResults {
		if tcpResults[i].TLSOK {
			tcpResults[i].HTTPOK = p.checkHTTP(domain, tcpResults[i].IP)
		}
	}

	// Sort: TLS OK + HTTP OK first, then by RTT
	sort.Slice(tcpResults, func(i, j int) bool {
		// Prioritize: TLSOK && HTTPOK > TLSOK > others
		scoreI := 0
		if tcpResults[i].TLSOK {
			scoreI += 10
		}
		if tcpResults[i].HTTPOK {
			scoreI += 5
		}
		scoreJ := 0
		if tcpResults[j].TLSOK {
			scoreJ += 10
		}
		if tcpResults[j].HTTPOK {
			scoreJ += 5
		}
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return tcpResults[i].RTT < tcpResults[j].RTT
	})

	// Filter out IPs that failed TLS
	var validIPs []IPResult
	for _, r := range tcpResults {
		if r.TLSOK {
			validIPs = append(validIPs, r)
		}
	}
	if len(validIPs) == 0 {
		// No TLS-valid IPs — return with empty BestIP so callers know it failed
		logger.Warn("域名 %s 无 TLS 验证通过的 IP", domain)
		result := &DomainResult{
			Domain:    domain,
			BestIP:    "",
			Latency:   0,
			ProbedAt:  time.Now(),
			AllIPs:    tcpResults,
		}
		p.mu.Lock()
		p.results[domain] = result
		p.mu.Unlock()
		return result, nil
	}

	best := validIPs[0]
	backupIPs := make([]string, 0)
	for i := 1; i < len(validIPs) && i < p.maxIPs; i++ {
		backupIPs = append(backupIPs, validIPs[i].IP)
	}

	result := &DomainResult{
		Domain:    domain,
		BestIP:    best.IP,
		BackupIPs: backupIPs,
		Latency:   best.RTT.Milliseconds(),
		ProbedAt:  time.Now(),
		AllIPs:    tcpResults,
	}

	logger.Info("域名 %s 最优IP: %s (延迟: %dms, TLS: %v)", domain, best.IP, best.RTT.Milliseconds(), best.TLSOK)

	p.mu.Lock()
	p.results[domain] = result
	p.mu.Unlock()

	return result, nil
}

// ProbeDomains probes multiple domains concurrently.
func (p *Prober) ProbeDomains(domains []string) map[string]*DomainResult {
	var wg sync.WaitGroup
	results := make(map[string]*DomainResult)
	var mu sync.Mutex

	for _, domain := range domains {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			result, err := p.ProbeDomain(d)
			if err != nil {
				logger.Error("探测域名 %s 失败: %v", d, err)
				return
			}
			mu.Lock()
			results[d] = result
			mu.Unlock()
		}(domain)
	}

	wg.Wait()
	return results
}

// GetResult returns the cached result for a domain.
func (p *Prober) GetResult(domain string) (*DomainResult, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	r, ok := p.results[domain]
	return r, ok
}

// GetAllResults returns all cached results.
func (p *Prober) GetAllResults() map[string]*DomainResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	cp := make(map[string]*DomainResult, len(p.results))
	for k, v := range p.results {
		cp[k] = v
	}
	return cp
}

// GetBestIP returns the best IP for a domain, or empty string if not probed.
func (p *Prober) GetBestIP(domain string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if r, ok := p.results[domain]; ok {
		return r.BestIP
	}
	return ""
}
