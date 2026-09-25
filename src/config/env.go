package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// overrideFromEnv 按固定顺序应用环境变量覆盖，返回警告列表。
//
// 语义约定（与历史版本的差异见各变量注释）：
//   - 标量字段：设置即替换，值非法时警告并忽略该变量（保持宽容，适配容器编排残留变量场景）
//   - IP_WHITELIST / IP_BLACKLIST：设置即整体替换文件中的名单（历史版本为追加合并）
//   - ACCESS_PROXY：使用 LookupEnv，设为空串表示显式清除
func overrideFromEnv(cfg *AppConfig) []string {
	var warnings []string
	warn := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	if val := strings.TrimSpace(os.Getenv("SERVER_HOST")); val != "" {
		cfg.Server.Host = val
	}

	if val := os.Getenv("SERVER_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil && port > 0 && port <= 65535 {
			cfg.Server.Port = port
		} else {
			warn("环境变量 SERVER_PORT=%q 非法（需 1-65535 的整数），已忽略", val)
		}
	}

	setBool := func(name string, target *bool) {
		if val := os.Getenv(name); val != "" {
			if v, err := strconv.ParseBool(val); err == nil {
				*target = v
			} else {
				warn("环境变量 %s=%q 非法（需布尔值 true/false/1/0），已忽略", name, val)
			}
		}
	}
	setBool("ENABLE_H2C", &cfg.Server.EnableH2C)
	setBool("ENABLE_FRONTEND", &cfg.Server.EnableFrontend)

	if val := os.Getenv("MAX_FILE_SIZE"); val != "" {
		var size ByteSize
		if err := size.UnmarshalText([]byte(val)); err == nil && size > 0 {
			cfg.Server.FileSize = size
		} else {
			warn("环境变量 MAX_FILE_SIZE=%q 非法（需正整数字节或 \"2GB\" 格式），已忽略", val)
		}
	}

	if val := os.Getenv("RATE_LIMIT"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil && limit > 0 {
			cfg.RateLimit.RequestLimit = limit
		} else {
			warn("环境变量 RATE_LIMIT=%q 非法（需正整数），已忽略", val)
		}
	}

	if val := os.Getenv("RATE_PERIOD_HOURS"); val != "" {
		if period, err := strconv.ParseFloat(val, 64); err == nil && period > 0 {
			cfg.RateLimit.PeriodHours = period
		} else {
			warn("环境变量 RATE_PERIOD_HOURS=%q 非法（需正数），已忽略", val)
		}
	}

	replaceList := func(name string, target *[]string) {
		if val, ok := os.LookupEnv(name); ok {
			parts := strings.Split(val, ",")
			list := make([]string, 0, len(parts))
			for _, part := range parts {
				if part = strings.TrimSpace(part); part != "" {
					list = append(list, part)
				}
			}
			*target = list
		}
	}
	replaceList("IP_WHITELIST", &cfg.Security.WhiteList)
	replaceList("IP_BLACKLIST", &cfg.Security.BlackList)

	// NODES：逗号分隔的节点 URL 列表，设置即整体替换文件中的节点（name 自动取域名）
	if val, ok := os.LookupEnv("NODES"); ok {
		parts := strings.Split(val, ",")
		nodes := make([]NodeConfig, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if validNodeURL(part) {
				nodes = append(nodes, NodeConfig{Name: nodeHost(part), URL: part})
			} else {
				warn("环境变量 NODES 中的条目 %q 非法（需 http:// 或 https:// 完整地址），已忽略", part)
			}
		}
		cfg.Nodes = nodes
	}

	if val, ok := os.LookupEnv("ACCESS_PROXY"); ok {
		val = strings.TrimSpace(val)
		switch {
		case val == "":
			cfg.Access.Proxy = ""
		case validProxyURL(val):
			cfg.Access.Proxy = val
		default:
			warn("环境变量 ACCESS_PROXY=%q 非法（支持 socks5://、http://、https://），已忽略", val)
		}
	}

	if val := os.Getenv("MAX_IMAGES"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.Download.MaxImages = n
		} else {
			warn("环境变量 MAX_IMAGES=%q 非法（需正整数），已忽略", val)
		}
	}

	return warnings
}
