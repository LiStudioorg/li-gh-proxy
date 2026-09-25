package config

// RegistryMapping Registry映射配置
type RegistryMapping struct {
	Upstream string `toml:"upstream"`
	AuthHost string `toml:"authHost"`
	AuthType string `toml:"authType"`
	Enabled  bool   `toml:"enabled"`
}

// ServerConfig 服务监听与基础开关
type ServerConfig struct {
	Host           string   `toml:"host"`
	Port           int      `toml:"port"`
	FileSize       ByteSize `toml:"fileSize"`
	EnableH2C      bool     `toml:"enableH2C"`
	EnableFrontend bool     `toml:"enableFrontend"`
}

// RateLimitConfig IP 令牌桶限流参数
type RateLimitConfig struct {
	RequestLimit int     `toml:"requestLimit"`
	PeriodHours  float64 `toml:"periodHours"`
}

// SecurityConfig IP 层黑白名单（限流豁免 / 封禁）
type SecurityConfig struct {
	WhiteList []string `toml:"whiteList"`
	BlackList []string `toml:"blackList"`
}

// AccessConfig 仓库层黑白名单与出站代理
type AccessConfig struct {
	WhiteList []string `toml:"whiteList"`
	BlackList []string `toml:"blackList"`
	Proxy     string   `toml:"proxy"`
}

// DownloadConfig 离线镜像下载限制
type DownloadConfig struct {
	MaxImages int `toml:"maxImages"`
}

// TokenCacheConfig Token / Manifest 内存缓存
type TokenCacheConfig struct {
	Enabled    bool     `toml:"enabled"`
	DefaultTTL Duration `toml:"defaultTTL"`
}

// NodeConfig 前端展示的加速节点（本服务的其他部署地址）
type NodeConfig struct {
	Name string `toml:"name" json:"name"`
	URL  string `toml:"url" json:"url"`
}

// ContentFeedConfig 本地文件驱动的内容功能（友链 / 赞助商卡片）通用配置。
// 数据以「一条一个 TOML 文件」的形式存放在服务器本地 dataDir 中，不入仓库；
// dataDir 相对路径基于配置文件所在目录解析（见 LoadConfig 的 resolveContentDirs）。
type ContentFeedConfig struct {
	Enabled bool   `toml:"enabled"`
	DataDir string `toml:"dataDir"`
}

// AppConfig 应用配置。
//
// 并发约定：LoadConfig 成功后通过 atomic 快照发布，GetConfig() 返回的快照
// 在下一次 LoadConfig 之前不可变。调用方只读使用，禁止修改返回值
// （包括切片、map 内部元素），否则会造成跨请求数据竞争。
type AppConfig struct {
	Server     ServerConfig               `toml:"server"`
	RateLimit  RateLimitConfig            `toml:"rateLimit"`
	Security   SecurityConfig             `toml:"security"`
	Access     AccessConfig               `toml:"access"`
	Download   DownloadConfig             `toml:"download"`
	Registries map[string]RegistryMapping `toml:"registries"`
	TokenCache TokenCacheConfig           `toml:"tokenCache"`
	Nodes      []NodeConfig               `toml:"nodes"`
	Friends    ContentFeedConfig          `toml:"friends"`
	Sponsors   ContentFeedConfig          `toml:"sponsors"`
}
