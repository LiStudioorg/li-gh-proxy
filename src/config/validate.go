package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// validate 启动期校验，致命问题聚合为单个 error（调用方终止启动），
// 非致命问题以警告列表返回。
//
// 严重性划分原则：数值/枚举类错误在旧版会以隐晦方式运行异常，属致命；
// 历史上可被静默容忍的写法（如 IP 名单中的非法条目、非标准 authType）
// 仅警告，避免升级后存量部署无法启动。
func validate(cfg *AppConfig) ([]string, error) {
	var errs, warns []string

	if cfg.Server.Host == "" {
		errs = append(errs, "server.host 不能为空")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		errs = append(errs, fmt.Sprintf("server.port=%d 超出范围（1-65535）", cfg.Server.Port))
	}
	if cfg.Server.FileSize <= 0 {
		errs = append(errs, fmt.Sprintf("server.fileSize=%s 必须为正数", cfg.Server.FileSize))
	}
	if cfg.RateLimit.RequestLimit <= 0 {
		errs = append(errs, fmt.Sprintf("rateLimit.requestLimit=%d 必须为正数", cfg.RateLimit.RequestLimit))
	}
	if cfg.RateLimit.PeriodHours <= 0 {
		errs = append(errs, fmt.Sprintf("rateLimit.periodHours=%g 必须为正数", cfg.RateLimit.PeriodHours))
	}
	if cfg.Download.MaxImages <= 0 {
		errs = append(errs, fmt.Sprintf("download.maxImages=%d 必须为正数", cfg.Download.MaxImages))
	}
	if cfg.TokenCache.Enabled && cfg.TokenCache.DefaultTTL <= 0 {
		errs = append(errs, fmt.Sprintf("tokenCache.defaultTTL=%s 必须为正时长", time.Duration(cfg.TokenCache.DefaultTTL)))
	}
	if cfg.Access.Proxy != "" && !validProxyURL(cfg.Access.Proxy) {
		errs = append(errs, fmt.Sprintf("access.proxy=%q 非法（支持 socks5://、socks5h://、http://、https://）", cfg.Access.Proxy))
	}

	for domain, mapping := range cfg.Registries {
		if strings.TrimSpace(mapping.Upstream) == "" {
			errs = append(errs, fmt.Sprintf("registries.%s.upstream 不能为空", domain))
		}
		switch mapping.AuthType {
		case "github", "google", "quay", "anonymous":
		default:
			warns = append(warns, fmt.Sprintf(
				"registries.%s.authType=%q 非标准取值（通常为 github/google/quay/anonymous），当前版本不参与认证逻辑",
				domain, mapping.AuthType))
		}
	}

	seenNodeURLs := make(map[string]bool, len(cfg.Nodes))
	for i := range cfg.Nodes {
		node := &cfg.Nodes[i]
		node.Name = strings.TrimSpace(node.Name)
		node.URL = strings.TrimSpace(node.URL)
		if !validNodeURL(node.URL) {
			errs = append(errs, fmt.Sprintf("nodes[%d].url=%q 非法（需 http:// 或 https:// 完整地址）", i, node.URL))
			continue
		}
		if node.Name == "" {
			node.Name = nodeHost(node.URL)
			warns = append(warns, fmt.Sprintf("nodes[%d].name 为空，已自动使用域名 %q", i, node.Name))
		}
		if seenNodeURLs[node.URL] {
			warns = append(warns, fmt.Sprintf("nodes[%d].url=%q 重复，前端将展示重复条目", i, node.URL))
		}
		seenNodeURLs[node.URL] = true
	}

	checkIPList := func(name string, list []string) {
		for _, entry := range list {
			if !validIPOrCIDR(entry) {
				warns = append(warns, fmt.Sprintf("%s 条目 %q 不是合法的 IP 或 CIDR，运行时将被忽略", name, entry))
			}
		}
	}
	checkIPList("security.whiteList", cfg.Security.WhiteList)
	checkIPList("security.blackList", cfg.Security.BlackList)

	if len(errs) > 0 {
		return warns, fmt.Errorf("配置校验失败:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return warns, nil
}

func validIPOrCIDR(s string) bool {
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		return err == nil
	}
	return net.ParseIP(s) != nil
}

var validProxySchemes = map[string]bool{
	"http": true, "https": true, "socks5": true, "socks5h": true,
}

func validProxyURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	return validProxySchemes[strings.ToLower(u.Scheme)]
}

var validNodeSchemes = map[string]bool{
	"http": true, "https": true,
}

func validNodeURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Fragment != "" || u.RawQuery != "" {
		return false
	}
	return validNodeSchemes[strings.ToLower(u.Scheme)]
}

func nodeHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Host
}
