package main

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"li-gh-proxy/config"
	"li-gh-proxy/handlers"
	"li-gh-proxy/utils"
)

//go:embed all:dist
var staticFiles embed.FS

var (
	globalLimiter    *utils.IPRateLimiter
	serviceStartTime = time.Now()
)

var Version = "dev"

func init() {
	for ext, typ := range map[string]string{
		".js":    "application/javascript; charset=utf-8",
		".mjs":   "application/javascript; charset=utf-8",
		".woff":  "font/woff",
		".woff2": "font/woff2",
		".map":   "application/json",
	} {
		_ = mime.AddExtensionType(ext, typ)
	}
}

func contentTypeFor(filename string) string {
	if ct := mime.TypeByExtension(path.Ext(filename)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

var (
	// precompressedAssets 缓存内嵌静态资源的 gzip 副本（key: embed 路径）
	precompressedAssets sync.Map
	// compressibleAssetExts 可安全 gzip 的文本类资源（woff2/图片等本身已压缩，跳过）
	compressibleAssetExts = map[string]bool{
		".html": true, ".js": true, ".mjs": true, ".css": true,
		".svg": true, ".json": true, ".map": true, ".txt": true, ".xml": true,
	}
)

func isCompressibleAsset(filename string) bool {
	return compressibleAssetExts[strings.ToLower(path.Ext(filename))]
}

// precompressStaticAssets 启动时对内嵌静态资源做一次性 gzip（最高压缩比），
// 请求期直接回放压缩结果，避免每个请求重复压缩消耗 CPU。
func precompressStaticAssets() {
	var files, rawTotal, gzTotal int
	_ = fs.WalkDir(staticFiles, "dist", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !isCompressibleAsset(p) {
			return nil
		}
		data, err := staticFiles.ReadFile(p)
		if err != nil || len(data) < 1024 {
			return nil
		}
		var buf bytes.Buffer
		w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return nil
		}
		if _, err := w.Write(data); err != nil {
			return nil
		}
		if err := w.Close(); err != nil {
			return nil
		}
		precompressedAssets.Store(p, buf.Bytes())
		files++
		rawTotal += len(data)
		gzTotal += buf.Len()
		return nil
	})
	if files > 0 {
		fmt.Printf("静态资源预压缩: %d 个文件, %.2f MB -> %.2f MB\n",
			files, float64(rawTotal)/(1<<20), float64(gzTotal)/(1<<20))
	}
}

// gzipAccepted 解析 Accept-Encoding，判断客户端是否接受 gzip（含 q 值处理）
func gzipAccepted(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(enc), "gzip") {
			continue
		}
		if params == "" {
			return true
		}
		if q, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(params), "q=")), 64); err == nil && q <= 0 {
			continue
		}
		return true
	}
	return false
}

func serveEmbedFile(c *gin.Context, filename string) {
	data, err := staticFiles.ReadFile(filename)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	contentType := contentTypeFor(filename)
	if isCompressibleAsset(filename) {
		// 无论本请求是否压缩都声明 Vary，避免中间缓存层混淆两种变体
		c.Header("Vary", "Accept-Encoding")
		if gz, ok := precompressedAssets.Load(filename); ok && gzipAccepted(c.Request) {
			c.Header("Content-Encoding", "gzip")
			c.Data(http.StatusOK, contentType, gz.([]byte))
			return
		}
	}
	c.Data(http.StatusOK, contentType, data)
}

func serveSPA(c *gin.Context) {
	// index.html 引用带内容 hash 的静态资源，自身不缓存，发版后立即生效
	c.Header("Cache-Control", "no-cache")
	serveEmbedFile(c, "dist/index.html")
}

func registerFrontendRoutes(router *gin.Engine, enabled bool) {
	if !enabled {
		notFound := func(c *gin.Context) { c.Status(http.StatusNotFound) }
		router.GET("/", notFound)
		router.GET("/images", notFound)
		router.GET("/search", notFound)
		router.GET("/assets/*filepath", notFound)
		router.GET("/favicon.ico", notFound)
		return
	}

	router.GET("/", serveSPA)
	router.GET("/images", serveSPA)
	router.GET("/search", serveSPA)
	router.GET("/favicon.ico", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=604800")
		serveEmbedFile(c, "dist/favicon.ico")
	})
	router.GET("/assets/*filepath", func(c *gin.Context) {
		filepath := strings.TrimPrefix(c.Param("filepath"), "/")
		if filepath == "" || strings.Contains(filepath, "..") {
			c.Status(http.StatusNotFound)
			return
		}
		// Vite 产物文件名含内容 hash，可长缓存（immutable 跳过重验证）
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		serveEmbedFile(c, path.Join("dist/assets", filepath))
	})
}

func buildRouter(cfg *config.AppConfig) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	utils.ConfigureTrustedProxies(router)

	router.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		log.Printf("Panic 已恢复: %v", recovered)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
			"code":  "INTERNAL_ERROR",
		})
	}))

	router.Use(utils.RateLimitMiddleware(globalLimiter))

	initHealthRoutes(router)
	handlers.InitImageTarRoutes(router)
	registerFrontendRoutes(router, cfg.Server.EnableFrontend)
	handlers.RegisterSearchRoute(router)
	handlers.RegisterNodesRoute(router)

	router.Any("/token", handlers.ProxyDockerAuthGin)
	router.Any("/token/*path", handlers.ProxyDockerAuthGin)
	router.Any("/v2/*path", handlers.ProxyDockerRegistryGin)
	router.NoRoute(handlers.GitHubProxyHandler)

	return router
}

func main() {
	if err := config.LoadConfig(); err != nil {
		fmt.Printf("配置加载失败: %v\n", err)
		return
	}

	utils.InitHTTPClients()
	globalLimiter = utils.InitGlobalLimiter()
	handlers.InitDockerProxy()
	handlers.InitImageStreamer()
	handlers.InitDebouncer()

	cfg := config.GetConfig()
	router := buildRouter(cfg)

	precompressStaticAssets()

	fmt.Printf("li-gh-proxy 启动成功\n")
	fmt.Printf("监听地址: %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("限流配置: %d请求/%g小时\n", cfg.RateLimit.RequestLimit, cfg.RateLimit.PeriodHours)
	if cfg.Server.EnableH2C {
		fmt.Printf("H2c: 已启用\n")
	}
	fmt.Printf("版本号: %s\n", Version)
	fmt.Printf("项目地址: https://github.com/LiStudioorg/li-gh-proxy\n")

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 30 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	if cfg.Server.EnableH2C {
		server.Handler = h2c.NewHandler(router, &http2.Server{
			MaxConcurrentStreams:         250,
			IdleTimeout:                  300 * time.Second,
			MaxReadFrameSize:             4 << 20,
			MaxUploadBufferPerConnection: 8 << 20,
			MaxUploadBufferPerStream:     2 << 20,
		})
	} else {
		server.Handler = router
	}

	if err := server.ListenAndServe(); err != nil {
		fmt.Printf("启动服务失败: %v\n", err)
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d秒", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d分钟%d秒", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d小时%d分钟", int(d.Hours()), int(d.Minutes())%60)
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%d天%d小时", days, hours)
}

func getUptimeInfo() (time.Duration, float64, string) {
	uptime := time.Since(serviceStartTime)
	return uptime, uptime.Seconds(), formatDuration(uptime)
}

func initHealthRoutes(router *gin.Engine) {
	router.GET("/ready", func(c *gin.Context) {
		_, uptimeSec, uptimeHuman := getUptimeInfo()
		c.JSON(http.StatusOK, gin.H{
			"ready":           true,
			"service":         "li-gh-proxy",
			"version":         Version,
			"start_time_unix": serviceStartTime.Unix(),
			"uptime_sec":      uptimeSec,
			"uptime_human":    uptimeHuman,
		})
	})
}
