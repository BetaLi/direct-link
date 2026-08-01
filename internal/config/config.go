package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type SiteConfig struct {
	Enabled bool `json:"enabled"`
}

type AdvancedConfig struct {
	ProxyPort             int      `json:"proxyPort"`
	ProbeInterval         int      `json:"probeInterval"`         // seconds
	HealthCheckInterval   int      `json:"healthCheckInterval"`  // seconds
	DohProviders          []string `json:"dohProviders"`
	MaxIPsPerDomain       int      `json:"maxIPsPerDomain"`
	PreferredMode         string   `json:"preferredMode"` // "auto", "hosts", "proxy"
}

type AppConfig struct {
	Version         string                  `json:"version"`
	Sites           map[string]SiteConfig   `json:"sites"`
	Advanced        AdvancedConfig          `json:"advanced"`
	CustomSites     []string                `json:"customSites"`
	Autostart       bool                    `json:"autostart"`
	MinimizeToTray  bool                    `json:"minimizeToTray"`
}

func DefaultConfig() *AppConfig {
	return &AppConfig{
		Version: "1.0.0",
		Sites: map[string]SiteConfig{
			"steam":  {Enabled: true},
			"github": {Enabled: true},
		},
		Advanced: AdvancedConfig{
			ProxyPort:           8848,
			ProbeInterval:       1800,
			HealthCheckInterval: 60,
			DohProviders:        []string{"alidns", "dnspod"},
			MaxIPsPerDomain:     5,
			PreferredMode:       "auto",
		},
		Autostart:      true,
		MinimizeToTray: true,
	}
}

func configDir() string {
	dir := filepath.Join(os.Getenv("APPDATA"), "DirectLink")
	os.MkdirAll(dir, 0755)
	return dir
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

func Load() (*AppConfig, error) {
	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			Save(cfg)
			return cfg, nil
		}
		return nil, err
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), nil
	}

	// Fill defaults for missing fields
	defaults := DefaultConfig()
	if cfg.Sites == nil {
		cfg.Sites = defaults.Sites
	}
	if cfg.Advanced.ProxyPort == 0 {
		cfg.Advanced.ProxyPort = defaults.Advanced.ProxyPort
	}
	if cfg.Advanced.ProbeInterval == 0 {
		cfg.Advanced.ProbeInterval = defaults.Advanced.ProbeInterval
	}
	if cfg.Advanced.HealthCheckInterval == 0 {
		cfg.Advanced.HealthCheckInterval = defaults.Advanced.HealthCheckInterval
	}
	if cfg.Advanced.DohProviders == nil || len(cfg.Advanced.DohProviders) == 0 {
		cfg.Advanced.DohProviders = defaults.Advanced.DohProviders
	}
	if cfg.Advanced.MaxIPsPerDomain == 0 {
		cfg.Advanced.MaxIPsPerDomain = defaults.Advanced.MaxIPsPerDomain
	}
	if cfg.Advanced.PreferredMode == "" {
		cfg.Advanced.PreferredMode = defaults.Advanced.PreferredMode
	}

	return &cfg, nil
}

func Save(cfg *AppConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0644)
}
