package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

/*
本包提供进程级配置：config.toml → 环境变量覆盖 → 校验 → 不可变快照。

加载管线（LoadConfig 唯一入口）：
  1. DefaultConfig()          内置默认值
  2. decodeTOML               按 CONFIG_PATH（缺省 ./config.toml）解码，未知字段告警
  3. overrideFromEnv          环境变量覆盖，非法值告警并忽略
  4. validate                 启动期校验，致命错误聚合返回
  5. freeze + atomic.Store    切片/map 克隆为只读快照后发布

并发约定：GetConfig() 返回的快照在两次 LoadConfig() 之间不可变，
调用方只读使用，禁止修改返回值。
*/

var configSnapshot atomic.Pointer[AppConfig]

func init() {
	// 未调用 LoadConfig 前提供合法默认快照，保证 GetConfig 永不返回 nil
	configSnapshot.Store(DefaultConfig())
}

// GetConfig 返回当前配置快照（只读，禁止修改返回值及其内部切片/map）。
func GetConfig() *AppConfig {
	return configSnapshot.Load()
}

// LoadConfig 加载配置并发布快照。
// 配置文件缺失时使用默认配置；解析或校验失败返回 error，由调用方决定是否终止启动。
func LoadConfig() error {
	cfg := DefaultConfig()
	path := configFilePath()

	if data, err := os.ReadFile(path); err == nil {
		warnings, err := decodeTOML(cfg, data)
		printWarnings(warnings)
		if err != nil {
			return fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
		}
	} else {
		fmt.Printf("未找到配置文件 %s，使用默认配置\n", path)
	}

	printWarnings(overrideFromEnv(cfg))

	// 内容目录（友链/赞助商）相对路径统一基于配置文件所在目录解析：
	// deb/rpm 部署 → /etc/li-gh-proxy/data/...；Docker → /app/data/...；本地开发 → src/data/...
	// 必须在 validate 之前完成，保证快照中存储的是最终绝对路径
	resolveContentDirs(cfg, filepath.Dir(path))

	warnings, err := validate(cfg)
	printWarnings(warnings)
	if err != nil {
		return err
	}

	freeze(cfg)
	configSnapshot.Store(cfg)
	return nil
}

func printWarnings(warnings []string) {
	for _, w := range warnings {
		fmt.Printf("配置警告: %s\n", w)
	}
}

func configFilePath() string {
	if path := strings.TrimSpace(os.Getenv("CONFIG_PATH")); path != "" {
		return path
	}
	return "config.toml"
}

// resolveContentDirs 将 friends/sponsors 的 dataDir 中的相对路径
// 转换为基于 configDir 的绝对路径；绝对路径与空值原样保留。
func resolveContentDirs(cfg *AppConfig, configDir string) {
	resolve := func(dir *string) {
		val := strings.TrimSpace(*dir)
		if val == "" || filepath.IsAbs(val) {
			*dir = val
			return
		}
		*dir = filepath.Clean(filepath.Join(configDir, val))
	}
	resolve(&cfg.Friends.DataDir)
	resolve(&cfg.Sponsors.DataDir)
}

// freeze 克隆全部可变字段，使快照与解码过程中的临时状态完全隔离
func freeze(cfg *AppConfig) {
	cfg.Security.WhiteList = append([]string(nil), cfg.Security.WhiteList...)
	cfg.Security.BlackList = append([]string(nil), cfg.Security.BlackList...)
	cfg.Access.WhiteList = append([]string(nil), cfg.Access.WhiteList...)
	cfg.Access.BlackList = append([]string(nil), cfg.Access.BlackList...)
	registries := make(map[string]RegistryMapping, len(cfg.Registries))
	for domain, mapping := range cfg.Registries {
		registries[domain] = mapping
	}
	cfg.Registries = registries
	cfg.Nodes = append([]NodeConfig(nil), cfg.Nodes...)
}
