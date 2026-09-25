package config

import "time"

// DefaultConfig 返回内置默认配置，是所有配置项默认值的唯一来源。
func DefaultConfig() *AppConfig {
	return &AppConfig{
		Server: ServerConfig{
			Host:           "0.0.0.0",
			Port:           5000,
			FileSize:       2 * 1024 * 1024 * 1024,
			EnableH2C:      false,
			EnableFrontend: true,
		},
		RateLimit: RateLimitConfig{
			RequestLimit: 500,
			PeriodHours:  3.0,
		},
		Security: SecurityConfig{
			WhiteList: []string{},
			BlackList: []string{},
		},
		Access: AccessConfig{
			WhiteList: []string{},
			BlackList: []string{},
			Proxy:     "",
		},
		Download: DownloadConfig{
			MaxImages: 10,
		},
		Registries: map[string]RegistryMapping{
			"ghcr.io": {
				Upstream: "ghcr.io",
				AuthHost: "ghcr.io/token",
				AuthType: "github",
				Enabled:  true,
			},
			"gcr.io": {
				Upstream: "gcr.io",
				AuthHost: "gcr.io/v2/token",
				AuthType: "google",
				Enabled:  true,
			},
			"quay.io": {
				Upstream: "quay.io",
				AuthHost: "quay.io/v2/auth",
				AuthType: "quay",
				Enabled:  true,
			},
			"registry.k8s.io": {
				Upstream: "registry.k8s.io",
				AuthHost: "registry.k8s.io",
				AuthType: "anonymous",
				Enabled:  true,
			},
		},
		TokenCache: TokenCacheConfig{
			Enabled:    true,
			DefaultTTL: Duration(20 * time.Minute),
		},
	}
}
