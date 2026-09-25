package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- 测试辅助 ----------

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// loadFromString 以指定 toml 内容走完整 LoadConfig 管线，返回加载后的快照
func loadFromString(t *testing.T, content string) *AppConfig {
	t.Helper()
	t.Setenv("CONFIG_PATH", writeConfigFile(t, content))
	if err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return GetConfig()
}

// ---------- 默认值 ----------

func TestDefaultConfigIsValid(t *testing.T) {
	if _, err := validate(DefaultConfig()); err != nil {
		t.Fatalf("内置默认配置未通过校验: %v", err)
	}
}

// ---------- TOML 解码 ----------

const fullConfigTOML = `
[server]
host = "127.0.0.1"
port = 6001
fileSize = "1.5GB"
enableH2C = true
enableFrontend = false

[rateLimit]
requestLimit = 100
periodHours = 1.5

[security]
whiteList = ["10.0.0.1", "10.0.0.0/8"]
blackList = ["192.168.100.0/24"]

[access]
whiteList = []
blackList = ["baduser/*"]
proxy = "socks5://127.0.0.1:1080"

[download]
maxImages = 5

[registries."ghcr.io"]
upstream = "ghcr.io"
authHost = "ghcr.io/token"
authType = "github"
enabled = true

[tokenCache]
enabled = false
defaultTTL = "1h30m"
`

func TestDecodeTOMLFullDocument(t *testing.T) {
	t.Setenv("CONFIG_PATH", writeConfigFile(t, fullConfigTOML))
	if err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg := GetConfig()

	if cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != 6001 {
		t.Fatalf("server = %+v", cfg.Server)
	}
	if cfg.Server.FileSize != ByteSize(1500000000) {
		t.Fatalf("fileSize = %s, want 1.50 GB", cfg.Server.FileSize)
	}
	if !cfg.Server.EnableH2C || cfg.Server.EnableFrontend {
		t.Fatalf("server switches = %+v", cfg.Server)
	}
	if cfg.RateLimit.RequestLimit != 100 || cfg.RateLimit.PeriodHours != 1.5 {
		t.Fatalf("rateLimit = %+v", cfg.RateLimit)
	}
	if len(cfg.Security.WhiteList) != 2 || len(cfg.Security.BlackList) != 1 {
		t.Fatalf("security lists = %+v", cfg.Security)
	}
	if len(cfg.Access.BlackList) != 1 || cfg.Access.Proxy != "socks5://127.0.0.1:1080" {
		t.Fatalf("access = %+v", cfg.Access)
	}
	if cfg.Download.MaxImages != 5 {
		t.Fatalf("maxImages = %d", cfg.Download.MaxImages)
	}
	if m, ok := cfg.Registries["ghcr.io"]; !ok || m.Upstream != "ghcr.io" || !m.Enabled {
		t.Fatalf("registries = %+v", cfg.Registries)
	}
	if cfg.TokenCache.Enabled || cfg.TokenCache.DefaultTTL != Duration(90*time.Minute) {
		t.Fatalf("tokenCache = %+v", cfg.TokenCache)
	}
}

func TestDecodeTOMLUnknownFieldsWarnButLoad(t *testing.T) {
	content := `
[server]
port = 6001
enableH2c = true
unknownKey = 1
`
	cfg := DefaultConfig()
	warnings, err := decodeTOML(cfg, []byte(content))
	if err != nil {
		t.Fatalf("decodeTOML: %v", err)
	}
	// go-toml 字段名大小写不敏感：enableH2c 可匹配 EnableH2C；
	// 完全未知的 unknownKey 产生 1 条警告
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want 1 entry", warnings)
	}
	if cfg.Server.Port != 6001 {
		t.Fatalf("port = %d, 已知字段应正常生效", cfg.Server.Port)
	}
	if !cfg.Server.EnableH2C {
		t.Fatalf("enableH2c 应按大小写不敏感匹配到 EnableH2C")
	}
}

func TestDecodeTOMLSyntaxError(t *testing.T) {
	cfg := DefaultConfig()
	if _, err := decodeTOML(cfg, []byte("port = [broken")); err == nil {
		t.Fatal("非法 TOML 应返回错误")
	}
}

func TestDecodeTOMLInvalidTypedValue(t *testing.T) {
	content := `
[tokenCache]
defaultTTL = "20x"
`
	cfg := DefaultConfig()
	if _, err := decodeTOML(cfg, []byte(content)); err == nil {
		t.Fatal("非法时长应返回错误而非静默回退")
	}
}

// ---------- 环境变量覆盖 ----------

func TestOverrideFromEnvScalars(t *testing.T) {
	t.Setenv("SERVER_HOST", "0.0.0.0")
	t.Setenv("SERVER_PORT", "6001")
	t.Setenv("ENABLE_H2C", "true")
	t.Setenv("ENABLE_FRONTEND", "false")
	t.Setenv("MAX_FILE_SIZE", "2GB")
	t.Setenv("RATE_LIMIT", "100")
	t.Setenv("RATE_PERIOD_HOURS", "2.5")
	t.Setenv("MAX_IMAGES", "3")

	cfg := DefaultConfig()
	if warnings := overrideFromEnv(cfg); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	if cfg.Server.Host != "0.0.0.0" || cfg.Server.Port != 6001 {
		t.Fatalf("server = %+v", cfg.Server)
	}
	if !cfg.Server.EnableH2C || cfg.Server.EnableFrontend {
		t.Fatalf("server switches = %+v", cfg.Server)
	}
	if cfg.Server.FileSize != ByteSize(2000000000) {
		t.Fatalf("fileSize = %s", cfg.Server.FileSize)
	}
	if cfg.RateLimit.RequestLimit != 100 || cfg.RateLimit.PeriodHours != 2.5 {
		t.Fatalf("rateLimit = %+v", cfg.RateLimit)
	}
	if cfg.Download.MaxImages != 3 {
		t.Fatalf("maxImages = %d", cfg.Download.MaxImages)
	}
}

func TestOverrideFromEnvInvalidValuesWarnAndIgnore(t *testing.T) {
	t.Setenv("SERVER_PORT", "abc")
	t.Setenv("ENABLE_H2C", "yes")
	t.Setenv("MAX_FILE_SIZE", "-5GB")
	t.Setenv("RATE_LIMIT", "0")
	t.Setenv("RATE_PERIOD_HOURS", "-1")
	t.Setenv("MAX_IMAGES", "x")

	cfg := DefaultConfig()
	warnings := overrideFromEnv(cfg)
	if len(warnings) != 6 {
		t.Fatalf("warnings = %d 项, want 6: %v", len(warnings), warnings)
	}
	if cfg.Server.Port != 5000 || cfg.Server.EnableH2C ||
		cfg.RateLimit.RequestLimit != 500 || cfg.RateLimit.PeriodHours != 3.0 ||
		cfg.Download.MaxImages != 10 {
		t.Fatalf("非法值应被忽略并保留默认: %+v", cfg)
	}
	if cfg.Server.FileSize != 2*1024*1024*1024 {
		t.Fatalf("fileSize = %s, 默认值应保留", cfg.Server.FileSize)
	}
}

func TestOverrideFromEnvIPListsReplace(t *testing.T) {
	t.Run("设置即替换", func(t *testing.T) {
		t.Setenv("IP_WHITELIST", "1.1.1.1,  2.2.2.2")
		t.Setenv("IP_BLACKLIST", "3.3.3.0/24")
		cfg := DefaultConfig()
		if warnings := overrideFromEnv(cfg); len(warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", warnings)
		}
		if got := strings.Join(cfg.Security.WhiteList, ","); got != "1.1.1.1,2.2.2.2" {
			t.Fatalf("whiteList = %v", cfg.Security.WhiteList)
		}
		if got := strings.Join(cfg.Security.BlackList, ","); got != "3.3.3.0/24" {
			t.Fatalf("blackList = %v", cfg.Security.BlackList)
		}
	})

	t.Run("设空即清空", func(t *testing.T) {
		t.Setenv("IP_WHITELIST", "")
		cfg := DefaultConfig()
		overrideFromEnv(cfg)
		if len(cfg.Security.WhiteList) != 0 {
			t.Fatalf("whiteList = %v, want empty", cfg.Security.WhiteList)
		}
	})

	t.Run("未设置保留文件值", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Security.WhiteList = []string{"10.0.0.1"}
		overrideFromEnv(cfg)
		if len(cfg.Security.WhiteList) != 1 || cfg.Security.WhiteList[0] != "10.0.0.1" {
			t.Fatalf("whiteList = %v", cfg.Security.WhiteList)
		}
	})
}

func TestOverrideFromEnvAccessProxy(t *testing.T) {
	t.Run("覆盖", func(t *testing.T) {
		t.Setenv("ACCESS_PROXY", "socks5://127.0.0.1:1080")
		cfg := DefaultConfig()
		if warnings := overrideFromEnv(cfg); len(warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", warnings)
		}
		if cfg.Access.Proxy != "socks5://127.0.0.1:1080" {
			t.Fatalf("proxy = %q", cfg.Access.Proxy)
		}
	})
	t.Run("设空清除", func(t *testing.T) {
		t.Setenv("ACCESS_PROXY", "")
		cfg := DefaultConfig()
		cfg.Access.Proxy = "socks5://127.0.0.1:1080"
		overrideFromEnv(cfg)
		if cfg.Access.Proxy != "" {
			t.Fatalf("proxy = %q, want cleared", cfg.Access.Proxy)
		}
	})
	t.Run("非法告警忽略", func(t *testing.T) {
		t.Setenv("ACCESS_PROXY", "ftp://127.0.0.1:21")
		cfg := DefaultConfig()
		if warnings := overrideFromEnv(cfg); len(warnings) != 1 {
			t.Fatalf("warnings = %v, want 1", warnings)
		}
		if cfg.Access.Proxy != "" {
			t.Fatalf("proxy = %q", cfg.Access.Proxy)
		}
	})
}

// ---------- 校验 ----------

func TestValidateAggregatesFatalErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Port = 70000
	cfg.RateLimit.RequestLimit = -1
	cfg.Download.MaxImages = 0
	cfg.TokenCache.DefaultTTL = 0
	cfg.Access.Proxy = "ftp://bad"
	cfg.Registries["example.com"] = RegistryMapping{Upstream: " ", AuthType: "github"}

	_, err := validate(cfg)
	if err == nil {
		t.Fatal("应返回校验错误")
	}
	for _, want := range []string{
		"server.port=70000",
		"rateLimit.requestLimit=-1",
		"download.maxImages=0",
		"tokenCache.defaultTTL",
		"access.proxy",
		"registries.example.com.upstream",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("错误信息缺少 %q:\n%s", want, err)
		}
	}
}

func TestValidateWarnsForTolerableIssues(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.WhiteList = []string{"not-an-ip", "10.0.0.0/8"}
	cfg.Registries["example.com"] = RegistryMapping{Upstream: "example.com", AuthType: "custom"}

	warnings, err := validate(cfg)
	if err != nil {
		t.Fatalf("可容忍问题不应致命: %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want 2", warnings)
	}
}

func TestValidateAcceptsValidIPOrCIDR(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1":      true,
		"::1":            true,
		"192.168.0.0/24": true,
		"10.0.0.0/8":     true,
		"not-an-ip":      false,
		"10.0.0.0/33":    false,
	}
	for input, want := range cases {
		if got := validIPOrCIDR(input); got != want {
			t.Fatalf("validIPOrCIDR(%q) = %v, want %v", input, got, want)
		}
	}
}

// ---------- 节点配置 ----------

func TestValidNodeURL(t *testing.T) {
	cases := map[string]bool{
		"https://a.example.com":           true,
		"https://a.example.com/":          true, // 尾斜杠视为根路径
		"http://127.0.0.1:5000":           true,
		"HTTPS://a.example.com":           true, // scheme 大小写不敏感
		"https://a.example.com/base":      false, // 路径会被前端 origin 丢弃，拒绝
		"https://a.example.com/base/":     false,
		"https://a.example.com/?x=1":      false,
		"https://a.example.com/#frag":     false,
		"https://user:pass@a.example.com": false, // 凭据会被前端 origin 丢弃，拒绝
		"ftp://a.example.com":             false,
		"a.example.com":                   false,
		"":                                false,
	}
	for input, want := range cases {
		if got := validNodeURL(input); got != want {
			t.Fatalf("validNodeURL(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestLoadConfigNodesEndToEnd(t *testing.T) {
	cfg := loadFromString(t, `
[[nodes]]
name = "节点 A"
url = "https://a.example.com"

[[nodes]]
url = "https://b.example.com"
`)

	if len(cfg.Nodes) != 2 {
		t.Fatalf("nodes = %+v, want 2 entries", cfg.Nodes)
	}
	if cfg.Nodes[0].Name != "节点 A" || cfg.Nodes[0].URL != "https://a.example.com" {
		t.Fatalf("nodes[0] = %+v", cfg.Nodes[0])
	}
	// name 缺失时由 validate 自动补全为域名
	if cfg.Nodes[1].Name != "b.example.com" || cfg.Nodes[1].URL != "https://b.example.com" {
		t.Fatalf("nodes[1] = %+v", cfg.Nodes[1])
	}
}

func TestValidateNodesFatalInvalidURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Nodes = []NodeConfig{
		{Name: "bad", URL: "ftp://a.example.com"},
		{Name: "empty", URL: ""},
	}

	_, err := validate(cfg)
	if err == nil {
		t.Fatal("非法节点 URL 应导致校验失败")
	}
	for _, want := range []string{"nodes[0].url", "nodes[1].url"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("错误信息缺少 %q:\n%s", want, err)
		}
	}
}

func TestValidateNodesWarnings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Nodes = []NodeConfig{
		{Name: "", URL: "https://a.example.com"},
		{Name: "重复", URL: "https://a.example.com"},
	}

	warnings, err := validate(cfg)
	if err != nil {
		t.Fatalf("可容忍问题不应致命: %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want 2", warnings)
	}
	if cfg.Nodes[0].Name != "a.example.com" {
		t.Fatalf("name 应自动补全为域名, got %q", cfg.Nodes[0].Name)
	}
}

func TestValidateNodesNormalizesWhitespace(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Nodes = []NodeConfig{
		{Name: "  节点  ", URL: "  https://a.example.com  "},
	}

	if _, err := validate(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cfg.Nodes[0].Name != "节点" || cfg.Nodes[0].URL != "https://a.example.com" {
		t.Fatalf("nodes[0] = %+v, 应去除首尾空白", cfg.Nodes[0])
	}
}

func TestOverrideFromEnvNodesReplace(t *testing.T) {
	t.Run("设置即替换", func(t *testing.T) {
		t.Setenv("NODES", "https://a.example.com, https://b.example.com")
		cfg := DefaultConfig()
		cfg.Nodes = []NodeConfig{{Name: "old", URL: "https://old.example.com"}}
		if warnings := overrideFromEnv(cfg); len(warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", warnings)
		}
		if len(cfg.Nodes) != 2 ||
			cfg.Nodes[0].Name != "a.example.com" || cfg.Nodes[0].URL != "https://a.example.com" ||
			cfg.Nodes[1].Name != "b.example.com" || cfg.Nodes[1].URL != "https://b.example.com" {
			t.Fatalf("nodes = %+v", cfg.Nodes)
		}
	})

	t.Run("设空即清空", func(t *testing.T) {
		t.Setenv("NODES", "")
		cfg := DefaultConfig()
		cfg.Nodes = []NodeConfig{{Name: "old", URL: "https://old.example.com"}}
		overrideFromEnv(cfg)
		if len(cfg.Nodes) != 0 {
			t.Fatalf("nodes = %+v, want empty", cfg.Nodes)
		}
	})

	t.Run("非法条目告警忽略", func(t *testing.T) {
		t.Setenv("NODES", "https://good.example.com, not-a-url")
		cfg := DefaultConfig()
		if warnings := overrideFromEnv(cfg); len(warnings) != 1 {
			t.Fatalf("warnings = %v, want 1", warnings)
		}
		if len(cfg.Nodes) != 1 || cfg.Nodes[0].URL != "https://good.example.com" {
			t.Fatalf("nodes = %+v", cfg.Nodes)
		}
	})

	t.Run("未设置保留文件值", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Nodes = []NodeConfig{{Name: "file", URL: "https://file.example.com"}}
		overrideFromEnv(cfg)
		if len(cfg.Nodes) != 1 || cfg.Nodes[0].Name != "file" {
			t.Fatalf("nodes = %+v", cfg.Nodes)
		}
	})
}

// ---------- LoadConfig / GetConfig 端到端 ----------

// 与旧版行为保持兼容的回归用例
func TestLoadConfigUsesConfigPathAndEnvOverrides(t *testing.T) {
	path := writeConfigFile(t, `
[server]
host = "127.0.0.1"
port = 5999

[access]
proxy = "socks5://127.0.0.1:1080"
`)
	t.Setenv("CONFIG_PATH", path)
	t.Setenv("SERVER_PORT", "6001")
	t.Setenv("ACCESS_PROXY", "")

	if err := LoadConfig(); err != nil {
		t.Fatal(err)
	}

	cfg := GetConfig()
	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("Server.Host = %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 6001 {
		t.Fatalf("Server.Port = %d, want 6001", cfg.Server.Port)
	}
	if cfg.Access.Proxy != "" {
		t.Fatalf("Access.Proxy = %q, want empty override", cfg.Access.Proxy)
	}
}

func TestLoadConfigMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "not-exist.toml"))
	if err := LoadConfig(); err != nil {
		t.Fatalf("文件缺失不应报错: %v", err)
	}
	cfg := GetConfig()
	if cfg.Server.Port != 5000 {
		t.Fatalf("port = %d, want default 5000", cfg.Server.Port)
	}
}

func TestLoadConfigValidationFailure(t *testing.T) {
	path := writeConfigFile(t, `
[server]
port = -1
`)
	t.Setenv("CONFIG_PATH", path)
	if err := LoadConfig(); err == nil {
		t.Fatal("非法端口应导致 LoadConfig 失败")
	}
	// 失败后快照应保持上一个合法状态（init 默认值）
	if cfg := GetConfig(); cfg == nil || cfg.Server.Port < 1 {
		t.Fatalf("失败后 GetConfig 应返回合法快照, got %+v", cfg)
	}
}

// ---------- 并发安全（配合 -race 运行） ----------

func TestGetConfigConcurrentWithReload(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.toml")
	pathB := filepath.Join(dir, "b.toml")
	if err := os.WriteFile(pathA, []byte("[server]\nport = 6001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte("[server]\nport = 6002\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					cfg := GetConfig()
					if cfg.Server.Port != 6001 && cfg.Server.Port != 6002 && cfg.Server.Port != 5000 {
						t.Errorf("读到非法端口 %d", cfg.Server.Port)
						return
					}
				}
			}
		}()
	}

	for i := 0; i < 50; i++ {
		if i%2 == 0 {
			t.Setenv("CONFIG_PATH", pathA)
		} else {
			t.Setenv("CONFIG_PATH", pathB)
		}
		if err := LoadConfig(); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}

// ---------- 自定义类型 ----------

func TestByteSizeParsing(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"2147483648", 2147483648, true},
		{"2GB", 2000000000, true},
		{"1.5GB", 1500000000, true},
		{"500MB", 500000000, true},
		{"1KiB", 1024, true},
		{"1.5GiB", 1610612736, true},
		{"1 TB", 1000000000000, true},
		{"0B", 0, true},
		{"", 0, false},
		{"abc", 0, false},
		{"GB", 0, false},
		{"2XB", 0, false},
		{"-1GB", 0, false},
	}
	for _, c := range cases {
		var bs ByteSize
		err := bs.UnmarshalText([]byte(c.in))
		if c.ok && (err != nil || int64(bs) != c.want) {
			t.Fatalf("ByteSize(%q) = %d, %v; want %d", c.in, int64(bs), err, c.want)
		}
		if !c.ok && err == nil {
			t.Fatalf("ByteSize(%q) 应解析失败", c.in)
		}
	}
}

func TestByteSizeString(t *testing.T) {
	cases := map[ByteSize]string{
		ByteSize(512):          "512 B",
		ByteSize(1024):         "1.00 KiB",
		ByteSize(2 << 20):      "2.00 MiB",
		ByteSize(2 << 30):      "2.00 GiB",
		ByteSize(3 << 40):      "3.00 TiB",
		ByteSize(2 << 50):      "2.00 PiB",
	}
	for in, want := range cases {
		if got := in.String(); got != want {
			t.Fatalf("ByteSize(%d).String() = %q, want %q", int64(in), got, want)
		}
	}
}

func TestDurationTOMLIntegration(t *testing.T) {
	content := `
[tokenCache]
defaultTTL = "20m"
`
	cfg := DefaultConfig()
	if _, err := decodeTOML(cfg, []byte(content)); err != nil {
		t.Fatalf("decodeTOML: %v", err)
	}
	if cfg.TokenCache.DefaultTTL != Duration(20*time.Minute) {
		t.Fatalf("defaultTTL = %s", time.Duration(cfg.TokenCache.DefaultTTL))
	}
}
