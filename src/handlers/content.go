package handlers

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml/v2"
	"li-gh-proxy/config"
)

/*
本文件实现「文件路由」式本地内容功能：友情链接与赞助商卡片。

数据约定（存储于服务器本地，不入仓库）：
  - dataDir 下每个顶层 *.toml 文件即一条记录，文件名（去扩展名）作为 slug；
    新增 = 放入文件，删除 = 移除文件，修改保存后经短 TTL 缓存自动生效，无需重启。
  - 解析失败或校验不通过的文件跳过并记录日志，不影响其余条目。
  - 目录不存在视为空内容（功能开启但尚未放置数据）。

友链文件示例（data/friends/example.toml）：
	name = "示例站点"
	url = "https://example.com"
	description = "站点简介（可选）"
	avatar = "https://example.com/avatar.png（可选）"

赞助商文件示例（data/sponsors/awesome.toml）：
	name = "Awesome Inc"
	url = "https://awesome.com"
	logo = "https://awesome.com/logo.svg（可选）"
	description = "赞助商简介（可选）"
	tier = 1  # 排序权重，数字越小越靠前，缺省为 0
*/

// FriendLink 友情链接条目
type FriendLink struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

// Sponsor 赞助商条目（tier 仅用于排序，数字小的展示在前）
type Sponsor struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Logo        string `json:"logo,omitempty"`
	Description string `json:"description,omitempty"`
	Tier        int    `json:"tier"`
}

// contentTTL 与 config.GetConfig 的副本缓存对齐：改动数据文件后约 5s 内自动生效
const contentTTL = 5 * time.Second

// contentMaxFileSize 单文件上限，防止误放入超大文件拖慢加载
const contentMaxFileSize = 1 << 20

// contentLoader 目录型 TOML 内容的通用加载器（短 TTL 缓存 + 目录键失效）。
// 缓存以 dataDir 为键：管理员在配置中切换目录时立即失效，无需等 TTL 过期。
type contentLoader[T any] struct {
	fetchCfg func(*config.AppConfig) config.ContentFeedConfig
	parse    func(data []byte, slug string) (T, error)
	sortFn   func(items []T)

	mu       sync.Mutex
	dir      string
	items    []T
	expireAt time.Time
}

// load 返回当前条目列表；功能未启用或目录未配置时返回 nil。
// 返回值为缓存切片的浅拷贝（元素为值类型字符串/int，调用方只读使用）。
func (l *contentLoader[T]) load() []T {
	cfg := l.fetchCfg(config.GetConfig())
	if !cfg.Enabled || strings.TrimSpace(cfg.DataDir) == "" {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.items != nil && l.dir == cfg.DataDir && time.Now().Before(l.expireAt) {
		out := make([]T, len(l.items))
		copy(out, l.items)
		return out
	}

	items := readContentDir(cfg.DataDir, l.parse)
	if l.sortFn != nil {
		l.sortFn(items)
	}
	l.dir = cfg.DataDir
	l.items = items
	l.expireAt = time.Now().Add(contentTTL)
	out := make([]T, len(items))
	copy(out, items)
	return out
}

// readContentDir 扫描 dir 下的顶层 *.toml 文件并逐条解析；
// 单个文件失败仅告警跳过。目录不存在返回空切片。
func readContentDir[T any](dir string, parse func([]byte, string) (T, error)) []T {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("读取内容目录 %s 失败: %v", dir, err)
		}
		return []T{}
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".toml") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	items := make([]T, 0, len(names))
	for _, name := range names {
		slug := strings.TrimSuffix(name, filepath.Ext(name))
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("读取内容文件 %s 失败，已跳过: %v", path, err)
			continue
		}
		if len(data) > contentMaxFileSize {
			log.Printf("内容文件 %s 超过 %d 字节上限，已跳过", path, contentMaxFileSize)
			continue
		}
		item, err := parse(data, slug)
		if err != nil {
			log.Printf("解析内容文件 %s 失败，已跳过: %v", path, err)
			continue
		}
		items = append(items, item)
	}
	return items
}

// validateContentEntry 校验必填字段 name/url 与 URL 合法性（http/https）
func validateContentEntry(kind, slug, name, rawURL string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%s %q 缺少必填字段 name", kind, slug)
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%s %q 的 url=%q 非法（需 http:// 或 https:// 完整地址）", kind, slug, rawURL)
	}
	return nil
}

func parseFriendLink(data []byte, slug string) (FriendLink, error) {
	var raw struct {
		Name        string `toml:"name"`
		URL         string `toml:"url"`
		Description string `toml:"description"`
		Avatar      string `toml:"avatar"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return FriendLink{}, err
	}
	if err := validateContentEntry("友链", slug, raw.Name, raw.URL); err != nil {
		return FriendLink{}, err
	}
	return FriendLink{
		Slug:        slug,
		Name:        strings.TrimSpace(raw.Name),
		URL:         strings.TrimSpace(raw.URL),
		Description: strings.TrimSpace(raw.Description),
		Avatar:      strings.TrimSpace(raw.Avatar),
	}, nil
}

func parseSponsor(data []byte, slug string) (Sponsor, error) {
	var raw struct {
		Name        string `toml:"name"`
		URL         string `toml:"url"`
		Logo        string `toml:"logo"`
		Description string `toml:"description"`
		Tier        int    `toml:"tier"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return Sponsor{}, err
	}
	if err := validateContentEntry("赞助商", slug, raw.Name, raw.URL); err != nil {
		return Sponsor{}, err
	}
	return Sponsor{
		Slug:        slug,
		Name:        strings.TrimSpace(raw.Name),
		URL:         strings.TrimSpace(raw.URL),
		Logo:        strings.TrimSpace(raw.Logo),
		Description: strings.TrimSpace(raw.Description),
		Tier:        raw.Tier,
	}, nil
}

var (
	friendLoader = &contentLoader[FriendLink]{
		fetchCfg: func(c *config.AppConfig) config.ContentFeedConfig { return c.Friends },
		parse:    parseFriendLink,
		sortFn:   func(items []FriendLink) {}, // 已按文件名排序
	}
	sponsorLoader = &contentLoader[Sponsor]{
		fetchCfg: func(c *config.AppConfig) config.ContentFeedConfig { return c.Sponsors },
		parse:    parseSponsor,
		sortFn: func(items []Sponsor) {
			sort.SliceStable(items, func(i, j int) bool {
				if items[i].Tier != items[j].Tier {
					return items[i].Tier < items[j].Tier
				}
				return items[i].Slug < items[j].Slug
			})
		},
	}
)

// RegisterContentRoutes 注册本地内容 API：
//   - GET /api/features  功能开关状态（供前端决定导航入口与区块显隐）
//   - GET /api/friends   友情链接列表（未启用返回 404）
//   - GET /api/sponsors  赞助商列表（未启用返回 404）
func RegisterContentRoutes(r *gin.Engine) {
	r.GET("/api/features", func(c *gin.Context) {
		cfg := config.GetConfig()
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{
			"friends":  cfg.Friends.Enabled,
			"sponsors": cfg.Sponsors.Enabled,
		})
	})

	featureGone := func(kind string) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Header("Cache-Control", "no-store")
			c.JSON(http.StatusNotFound, gin.H{
				"error": kind + "功能未开启，可在配置文件中启用",
				"code":  "FEATURE_DISABLED",
			})
		}
	}

	r.GET("/api/friends", func(c *gin.Context) {
		items := friendLoader.load()
		if items == nil {
			featureGone("友情链接")(c)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"items": items})
	})

	r.GET("/api/sponsors", func(c *gin.Context) {
		items := sponsorLoader.load()
		if items == nil {
			featureGone("赞助商")(c)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"items": items})
	})
}
