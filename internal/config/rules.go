package config

// DomainRule defines a single domain to accelerate.
type DomainRule struct {
	Domain    string `json:"domain"`    // e.g. "store.steampowered.com"
	QueryName string `json:"queryName"` // DoH query name, empty = same as Domain
}

// SiteRule defines a group of domains for a site (e.g. Steam, GitHub).
type SiteRule struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Icon    string       `json:"icon"`
	Domains []DomainRule `json:"domains"`
}

// BuiltinRules returns the built-in domain rules for Steam and GitHub.
func BuiltinRules() []SiteRule {
	return []SiteRule{
		{
			ID:   "steam",
			Name: "Steam",
			Icon: "steam",
			Domains: []DomainRule{
				{Domain: "store.steampowered.com"},
				{Domain: "steamcommunity.com"},
				{Domain: "api.steampowered.com"},
				{Domain: "steamstatic.com"},
				{Domain: "steamcdn-a.akamaihd.net"},
				{Domain: "community.akamai.steamstatic.com"},
				{Domain: "cdn.akamai.steamstatic.com"},
				{Domain: "cdn.cloudflare.steamstatic.com"},
				{Domain: "login.steampowered.com"},
				{Domain: "checkout.steampowered.com"},
			},
		},
		{
			ID:   "github",
			Name: "GitHub",
			Icon: "github",
			Domains: []DomainRule{
				{Domain: "github.com"},
				{Domain: "api.github.com"},
				{Domain: "raw.githubusercontent.com"},
				{Domain: "assets-cdn.github.com"},
				{Domain: "github.global.ssl.fastly.net"},
				{Domain: "codeload.github.com"},
				{Domain: "objects.githubusercontent.com"},
				{Domain: "github.githubassets.com"},
				{Domain: "collector.github.com"},
				{Domain: "live.github.com"},
				{Domain: "github.io"},
			},
		},
	}
}

// GetEnabledDomains returns all domains for enabled sites.
func GetEnabledDomains(cfg *AppConfig) []DomainRule {
	rules := BuiltinRules()
	var result []DomainRule
	for _, rule := range rules {
		if siteCfg, ok := cfg.Sites[rule.ID]; ok && siteCfg.Enabled {
			result = append(result, rule.Domains...)
		}
	}
	return result
}

// GetAllDomains returns all domains for a given site ID.
func GetAllDomains(siteID string) []string {
	rules := BuiltinRules()
	for _, rule := range rules {
		if rule.ID == siteID {
			domains := make([]string, len(rule.Domains))
			for i, d := range rule.Domains {
				domains[i] = d.Domain
			}
			return domains
		}
	}
	return nil
}
