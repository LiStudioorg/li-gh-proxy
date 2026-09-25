# li-gh-proxy 开发文档

> 本文面向开发者，介绍 li-gh-proxy 的整体架构、模块划分、核心设计、本地开发与构建发布流程。

## 1. 项目概述

li-gh-proxy 是一个轻量级、高性能的多功能加速代理服务，核心能力：

| 能力 | 说明 |
|---|---|
| Docker 镜像加速 | 兼容 Registry API v2，支持 Docker Hub、GHCR、GCR、Quay、registry.k8s.io |
| GitHub 文件加速 | Release、Raw、Git Clone、`api.github.com`，脚本内 URL 自动改写 |
| Hugging Face 加速 | 模型文件与 LFS 大文件下载 |
| 离线镜像包 | 无需本地 Docker，在线打包单镜像或批量 tar（`docker load` 兼容格式） |
| 镜像搜索 | Web 界面 + API 搜索 Docker Hub 镜像、浏览标签 |
| 安全防护 | IP 令牌桶限流（IPv6 按 /64）、IP 黑白名单、仓库/镜像黑白名单（通配符） |
| 出站代理 | 可选 SOCKS5/HTTP 上游代理 |

**总体形态**：Go 单二进制（内嵌 Nuxt 4 SPA 前端），支持 Docker 多架构镜像与 `deb` / `rpm` / `apk` 系统包分发。

---

## 2. 技术栈

| 层 | 技术 | 版本 |
|---|---|---|
| 后端 | Go + Gin | Go 1.26 / Gin v1.12 |
| 容器操作 | google/go-containerregistry | v0.21.5 |
| 配置 | pelletier/go-toml/v2 | v2.3.1 |
| 限流 | golang.org/x/time（令牌桶） | v0.15 |
| 传输 | golang.org/x/net（h2c、HTTP2） | — |
| 前端 | Nuxt 4（Vue 3 + TypeScript，纯 SPA 模式） | ^4.5 |
| UI 库 | fuxsto-design（zinc 单色 Tailwind v4 组件库）+ lucide-vue-next | ^1.0.4 |
| 前端构建 | Nuxt generate（Vite 8 + Tailwind CSS v4） | Vite ^8.3 |
| 文档站 | Astro + Starlight（中英双语） | Astro ^7.0 |
| 打包 | nfpm（deb/rpm/apk）、UPX、Docker buildx | — |
| 运行环境 | Node >= 24 | — |

---

## 3. 目录结构

```
li-gh-proxy/
├── src/                        # Go 后端（module: li-gh-proxy）
│   ├── main.go                 # 入口：路由注册、embed 静态资源、HTTP Server
│   ├── main_test.go            # 全栈集成测试
│   ├── config.toml             # 默认配置模板（也是 Docker 镜像内配置）
│   ├── go.mod / go.sum
│   ├── config/
│   │   └── config.go           # 配置加载（toml + 环境变量覆盖）+ 5s TTL 缓存
│   ├── handlers/
│   │   ├── docker.go           # Docker Registry v2 代理（/v2/*、/token*）
│   │   ├── github.go           # GitHub / HuggingFace 代理（NoRoute 兜底）
│   │   ├── imagetar.go         # 离线镜像流式打包下载（/api/image/*）
│   │   └── search.go           # Docker Hub 搜索 / 标签 API（/api/search、/api/tags）
│   ├── utils/
│   │   ├── access_control.go   # 仓库/镜像黑白名单 + 通配符匹配
│   │   ├── cache.go            # 通用内存缓存（manifest / token 共用）
│   │   ├── http_client.go      # 全局 HTTP 客户端（含 SOCKS5 上游代理）
│   │   ├── ratelimiter.go      # IP 令牌桶限流 + IP 黑白名单
│   │   └── proxy_shell.go      # .sh/.ps1 脚本内 GitHub URL 改写
│   └── dist/                   # 前端构建产物（gitignore，构建 web/ 后生成，被 go:embed）
├── web/                        # Nuxt 4 前端（SPA，fuxsto-design UI）
│   ├── nuxt.config.ts          # ssr:false、buildAssetsDir 'assets/'、产物输出 ../src/dist、devProxy /api
│   └── app/
│       ├── app.vue             # 根组件（标题 + AppShell 外壳）
│       ├── assets/css/main.css # tailwindcss + fuxsto-design/styles + Manrope 字体
│       ├── components/         # AppShell（导航/主题切换）、PageHero
│       ├── composables/        # useTheme（暗色模式）
│       ├── utils/              # api.ts（唯一后端 API 封装层）、format.ts（格式化工具）
│       └── pages/              # index.vue（GitHub 加速）、images.vue（离线镜像）、search.vue（搜索）
├── docs/                       # Astro Starlight 文档站（部署 GitHub Pages）
│   └── src/content/docs/       # 中文为根，en/ 为英文镜像
├── packaging/                  # nfpm 打包配置 + systemd/OpenRC service + logrotate
├── install.sh                  # 一键安装脚本（识别包管理器下载 Release 资产）
├── Dockerfile                  # 三阶段构建（前端 → Go → alpine 运行时）
├── docker-compose.yml
└── .github/workflows/          # release.yml / docker-ghcr.yml / docs.yml
```

---

## 4. 架构设计

### 4.1 前后端集成方式（Go embed）

```
web/ (Nuxt 4 SPA)
  npm run build (nuxt generate) ──→ 输出到 ../src/dist   (nuxt.config.ts: nitro.output.publicDir)
                         │
                         ▼
src/main.go  //go:embed all:dist        ← 编译期将整个 SPA 嵌入二进制
                         │
                         ▼
registerFrontendRoutes(router, cfg.Server.EnableFrontend)
  GET / 、/images、/search  → serveSPA（dist/index.html，Nuxt 客户端路由接管；Cache-Control: no-cache）
  GET /assets/*filepath     → dist/assets（含 ".." 路径穿越防护；Cache-Control: public, max-age=31536000, immutable）
  GET /favicon.ico          → dist/favicon.ico（Cache-Control: public, max-age=604800）
  EnableFrontend=false      → 上述路由全部 404（纯代理模式）
```

**静态资源加速**：启动时 `precompressStaticAssets()` 对内嵌文本资源（html/js/css/svg/json 等，≥1KB）做一次性 gzip 最高压缩，请求期按 `Accept-Encoding` 协商回放压缩副本（可压缩类型恒定返回 `Vary: Accept-Encoding`）；woff2/图片本身已压缩，直接透传。

**关键约束**：
- Go 编译前必须先完成 `web/` 构建，否则 embed 失败。这是 CI 与 Dockerfile 都把前端构建放在 Go 构建之前的原因。
- Nuxt 侧必须保持 `ssr: false`（Go 只回源 `index.html`，无 Node 运行时）与 `buildAssetsDir: 'assets/'`（Go 只映射 `/assets/*`）。

### 4.2 请求路由总表

| 路由 | Handler | 说明 |
|---|---|---|
| `GET /ready` | main.go 内置 | 健康检查（version、uptime） |
| `GET /api/search`、`GET /api/tags/:ns/:name` | handlers/search.go | Docker Hub 搜索与标签 |
| `GET /api/image/download`、`/batch`、`/info` | handlers/imagetar.go | 离线镜像打包下载 |
| `ANY /token`、`/token/*path` | handlers/docker.go | Docker 认证 token 代理（含缓存） |
| `ANY /v2/*path` | handlers/docker.go | Docker Registry v2 代理 |
| `NoRoute`（其余全部） | handlers/github.go | GitHub / HuggingFace 代理兜底 |
| `/`、`/images`、`/search`、`/assets/*` | main.go | 内嵌 SPA（可关闭） |

> 前端为 Nuxt 文件式路由，但 Go 侧 SPA 路由是**显式枚举**的——新增页面必须同步 `registerFrontendRoutes`（见 6.4）。

### 4.3 全局中间件链

```
gin.Default()
 ├─ utils.ConfigureTrustedProxies   # 信任私网段（127/8、10/8、172.16/12、192.168/16），保证 ClientIP 真实
 ├─ gin.CustomRecovery              # panic → 500 JSON
 └─ utils.RateLimitMiddleware       # IP 令牌桶限流（前端静态路径跳过）
```

### 4.4 启动流程（main.go）

```
main()
 ├─ config.LoadConfig()            # 默认值 → toml → 环境变量覆盖
 ├─ utils.InitHTTPClients()        # 读 Access.Proxy → 设置 HTTP(S)_PROXY 环境变量（原生支持 socks5://）
 ├─ utils.InitGlobalLimiter()      # 初始化 IP 限流器
 ├─ handlers.InitDockerProxy() / InitImageStreamer() / InitDebouncer()
 ├─ buildRouter(cfg)               # 注册全部路由与中间件
 └─ http.Server.ListenAndServe()   # EnableH2C 时用 h2c.NewHandler 包裹
```

超时设计：`ReadTimeout 60s`、`WriteTimeout 30min`（为大镜像流式传输留时间）、`IdleTimeout 120s`。

### 4.5 模块依赖关系

```
main.go
 ├─→ config     所有模块按需 GetConfig()（5s 副本缓存 + 切片深拷贝）
 ├─→ utils      InitHTTPClients / InitGlobalLimiter
 ├─→ handlers   InitDockerProxy / InitImageStreamer / InitDebouncer
 └─→ 路由层：imagetar、search、docker、github 四个 handler
       全部经 utils.GlobalAccessController 做访问控制
       docker.go 经 utils.GlobalCache 做 manifest/token 缓存
       github.go 调 utils.ProcessSmart 改写脚本
       出站 HTTP 统一复用 utils 全局 Transport（共享代理配置）
```

---

## 5. 核心模块详解

### 5.1 config/ — 配置

**文件划分**：`config.go`（Load/Get 门面 + atomic 快照）、`model.go`（命名结构体）、`types.go`（`ByteSize`/`Duration` 强类型）、`defaults.go`（默认值唯一来源）、`toml.go`（解码 + 未知字段告警）、`env.go`（环境变量覆盖）、`validate.go`（启动校验）。

- **加载管线**：`DefaultConfig()` 内置默认值 → 按 `CONFIG_PATH` 环境变量（缺省 `./config.toml`）读 toml（文件不存在仅提示不报错；未知字段**告警不阻断**）→ `overrideFromEnv` 环境变量覆盖（非法值告警并忽略）→ `validate` 启动校验（致命错误聚合返回，启动终止）→ 切片/map 克隆后 `atomic.Pointer` 发布。
- **环境变量**：`SERVER_HOST`、`SERVER_PORT`、`ENABLE_H2C`、`ENABLE_FRONTEND`、`MAX_FILE_SIZE`、`RATE_LIMIT`、`RATE_PERIOD_HOURS`、`IP_WHITELIST`/`IP_BLACKLIST`（逗号分隔，**设置即整体替换**文件名单，不再追加）、`ACCESS_PROXY`（允许设空清除）、`MAX_IMAGES`。
- **强类型**：`server.fileSize` 为 `ByteSize`（裸整数或 `"2GB"`/`"1.5GiB"` 字符串），`tokenCache.defaultTTL` 为 `Duration`（时长字符串，加载期校验，不再运行期静默回退）。
- **校验策略**：数值/枚举类错误（端口越界、非正数、非法 proxy scheme、registries.upstream 为空）致命；历史上可静默容忍的问题（IP 名单非法条目、非标准 authType）仅告警——避免存量部署升级后无法启动。
- **并发模型**：快照在两次 `LoadConfig()` 之间不可变，`GetConfig()` 为单次原子读，无锁无深拷贝。**调用方禁止修改返回值**（含切片/map 元素）。
- 配置段：`[server]`、`[rateLimit]`、`[security]`（IP 层黑白名单）、`[access]`（仓库层黑白名单 + proxy）、`[download]`、`[registries.*]`（多 Registry 映射）、`[tokenCache]`。

### 5.2 handlers/docker.go — Registry v2 代理

**请求流程**（`/v2/*path`）：

1. `/v2/` 直接返回 `200 {}`（API ping）。
2. `detectRegistryDomain`：优先 `?ns=` 查询参数（兼容 containerd），否则按路径前缀匹配 `Registries` 配置；命中且 `Enabled` 走多 Registry 分支（上游引用 = `Upstream + "/" + image`）。
3. 未命中按 Docker Hub 处理：`parseRegistryPath` 拆 `manifests`/`blobs`/`tags/list`；无 `/` 的镜像自动补 `library/`。
4. `GlobalAccessController.CheckDockerAccess(imageName)` → 拒绝返回 403。
5. 按 API 类型分派：
   - **Manifest**：缓存开启时查 `BuildManifestCacheKey` 命中直接回放；HEAD → `remote.Head`；GET → `remote.Get` 后按 TTL 写缓存。TTL 策略：digest 引用 24h、`latest/main/master/dev/develop` 10min、其他用配置 TTL。
   - **Blob**：`remote.Layer` → `layer.Compressed()` → `io.Copy` **纯流式转发**（不落盘不缓存）。
   - **Tags**：`remote.List` → JSON。
6. **Token 代理**（`/token*`）：命中缓存直接回放；未命中用 `ResponseRecorder` 截获原始代理响应，解析 `expires_in`（减 300s 余量、下限 5min）后写缓存。关键改写：`rewriteAuthHeader` 把响应 `Www-Authenticate` 的 realm 统一替换为 `http://<本机>/token`，使后续 token 请求回流本代理。

### 5.3 handlers/github.go — GitHub / HF 代理

- **入口**：`NoRoute` 兜底 `GitHubProxyHandler`。9 条正则覆盖 `github.com`（releases/archive、blob/raw、git 协议）、`raw.githubusercontent.com`、`gist`、`api.github.com/repos`、`huggingface.co`、`cdn-lfs.hf.co`、`githubassets.com`。不匹配 → 403。
- **重定向处理**：递归跟随上游重定向上限 20 次（超过 508）；`Location` 若仍匹配 GitHub 模式则改写为 `"/" + location` 让客户端回流本代理。
- **防护**：
  - GET 响应 Content-Type 为 `text/html`、`xml` 等 → 403（不支持网页加速）；
  - `Content-Length` 超过 `server.fileSize`（默认 2 GiB）→ 413；
  - 清理 CSP / Referrer-Policy / HSTS 头。
- **脚本改写**：`.sh`/`.ps1` 结尾的 URL 调 `utils.ProcessSmart` 把内容中的 GitHub URL 改写为指向本代理；改写后删 `Content-Length`、置 chunked。

### 5.4 handlers/imagetar.go — 离线镜像打包

最大的模块（约 1090 行），三层防护叠加：

1. **防抖 `DownloadDebouncer`**：key = 用户标识 + 内容指纹（镜像列表排序后取 MD5），单镜像窗口 5s / 批量 60s 只放行一次；用户标识取 `session_id` cookie，否则 `md5(ip+UA)`。
2. **一次性下载令牌 `tokenStore`**（泛型）：32 字节 crypto/rand，TTL 2 分钟，容量上限 2000；**消费即删除**（防重放），且与 IP/UA 绑定。
3. **流程**：`?mode=prepare` → 防抖检查（429 带 retry_after）+ 签发 token + 返回 `download_url`；正式下载必须携带有效 token。

**流式打包**：`name.ParseReference` → `remote.Get` → 多架构 index 按 `os/arch` 选平台（默认 `linux/amd64`）→ `tar.Writer` 逐层写 `layer.Compressed()`（原样透传）或 `layer.Uncompressed()` → 生成 `docker load` 兼容的 `manifest.json` + `repositories`。批量模式每镜像独立 15 分钟超时，任一失败立即中止整批。

**错误处理约定**：`writeDownloadError` — 若响应已开始输出（tar 流进行中）则只记日志不再写响应，避免污染已发出的 tar 数据。

### 5.5 handlers/search.go — 搜索 API

- 本地缓存 `Cache`：`map + RWMutex`，maxSize 1000、TTL 30min，`init()` 起 5 分钟定期清理。
- 搜索降级：query 含 `/` 时先试 `/repositories/{ns}/` 端点，404/为空则递归降级到全局 `/search/repositories/`。
- 标签接口最多 3 次重试、线性退避 500ms 递增；4xx（除 429）不重试。
- 使用独立的 10s 超时搜索客户端（与全局流式客户端隔离）。

### 5.6 utils/ — 基础设施

| 文件 | 职责 | 关键点 |
|---|---|---|
| `ratelimiter.go` | IP 令牌桶限流 | 速率 = RequestLimit/(PeriodHours*3600)，burst = RequestLimit；**IPv6 截断 /64** 防海量地址绕过；条目 2h 未访问清理，超 10000 整表重置；IP 白名单共享 `rate.Inf`；前端静态路径跳过 |
| `access_control.go` | 仓库黑白名单 | 白名单非空必须命中、黑名单命中即拒；通配符支持 `ns/*`、`*/repo`、前缀 `*`；统一小写比较；镜像解析识别 registry 端口（最后一个冒号且后面无 `/`） |
| `cache.go` | 通用内存缓存 | `sync.Map`，惰性过期 + 20min 定时清理；`[tokenCache].enabled` 单一开关同时控制 manifest 与 token 缓存 |
| `http_client.go` | HTTP 客户端 | `globalHTTPClient` 无整体超时（大文件流式必需）、连接池 1000；`searchHTTPClient` 10s 超时；SOCKS5 经 `os.Setenv` + `ProxyFromEnvironment` 实现 |
| `proxy_shell.go` | 脚本 URL 改写 | gzip 魔数探测、10MB 上限防滥用、防递归改写（URL 已含本机 host 则跳过） |

---

## 6. 前端（web/）

### 6.1 技术与结构

- **Nuxt 4**（`ssr: false` 纯 SPA，Nuxt 4 默认 `app/` 目录结构）+ **fuxsto-design**（zinc 单色、Tailwind CSS v4 组件库，样式自包含、暗色模式开箱即用）+ lucide-vue-next 图标，无状态管理库，`app/utils/api.ts` 为唯一 API 封装（fetch + JSON 错误解析 + `ApiError`）。
- 3 条文件式路由：`/`（GitHub 加速）、`/images`（离线镜像下载）、`/search`（镜像搜索/标签浏览），页面内 `useHead` 设置标题。
- 样式入口 `app/assets/css/main.css`：`@import "tailwindcss"; @import "fuxsto-design/styles";`（库自带 `@theme` 桥接与 `dark:` 变体，消费方无需为其配置 Tailwind 扫描）。
- 暗色模式：`useTheme` 组合式函数切换 `html.dark` + `localStorage`（key: `theme`），`app.head` 内联脚本在挂载前应用主题避免闪白。
- 开发期 `nitro.devProxy`：`/api` → `http://127.0.0.1:5000`；生产期同源由 Gin 直接服务。
- 构建产物输出 `../src/dist`（`nuxt generate`：index.html + assets/ + favicon.ico），被 Go embed。

### 6.2 常用命令

| 命令 | 说明 |
|---|---|
| `npm run dev` | 开发服务器（`/api` 自动代理到 `127.0.0.1:5000`） |
| `npm run build` | `nuxt generate`，静态产物 → `../src/dist`（与 Go embed 约定绑定） |
| `npm run typecheck` | `nuxt typecheck`（vue-tsc，strict 模式） |
| `postinstall: nuxt prepare` | 安装依赖后自动生成 `.nuxt` 类型 |

### 6.3 编码约定

- **UI 组件**：统一使用 fuxsto-design，按子路径导入以获得最优 tree-shaking：`import Button from 'fuxsto-design/button'`。全局反馈用 `Message`（`fuxsto-design/message`），错误提示用 `Alert`，加载态优先用组件自带 `loading` prop（如 Button/Input）。
- **自动导入**：`app/components/*`（页面模板直接用）、`app/composables/*`、`app/utils/*` 的具名导出（Nuxt 约定）；但跨文件引用 `app/utils/api.ts` 建议显式 `import ... from '~/utils/api'`（类型必须显式导入）。
- **格式化工具**：`app/utils/format.ts`（`formatNumber` / `formatSize` / `formatArchs` / `formatTimeAgo` / `copyText` / `errorMessage`）。
- **浏览器 API**：`window` / `document` 仅在事件回调或 `onMounted` 中使用（`ssr: false` 下组件不会在服务端执行，但 prerender 的 shell 阶段也要避免顶层副作用）。

### 6.4 ⚠️ 新增/修改前端路由必须同步 Go 侧

Go 只显式注册了 `/`、`/images`、`/search` 三条 SPA 路由（`src/main.go` 的 `registerFrontendRoutes`）。新增页面（如 `/about` → `app/pages/about.vue`）后，其余未注册路径会落入 NoRoute 的 GitHub 代理兜底（403）。**必须同步修改 `registerFrontendRoutes` 并补充 `main_test.go` 用例**；同理，前端资源约定 `buildAssetsDir: 'assets/'` 不可随意更改。

---

## 7. API 接口速查（前端消费，封装于 `web/app/utils/api.ts`）

| 函数 | 端点 | 说明 |
|---|---|---|
| `searchImages(q, page, pageSize)` | `GET /api/search?q=&page=&page_size=` | 镜像搜索 |
| `fetchTags(ns, name, page, pageSize)` | `GET /api/tags/{ns}/{name}?page=&page_size=` | 标签分页（含架构/os/size） |
| `prepareSingleDownload(...)` | `GET /api/image/download?mode=prepare&...` | 返回一次性 `download_url` |
| `prepareBatchDownload(...)` | `POST /api/image/batch?mode=prepare` | 批量离线包准备 |
| `fetchImageInfo(image)` | `GET /api/image/info?image=` | digest、mediaType、平台列表 |

---

## 8. 本地开发

### 8.1 环境要求

- Go 1.26+、Node >= 24

### 8.2 后端开发（使用 Nuxt devProxy，无需构建前端）

```text
# 终端 1：启动后端（首次需创建空的 src/dist 目录以通过 embed 校验）
cd src
mkdir dist 2>nul & type nul > dist\.keep     # Windows (cmd)
# mkdir -p dist && touch dist/.keep          # Linux / macOS
go run .

# 终端 2：启动前端 dev server（/api 自动代理到 127.0.0.1:5000）
cd web
npm ci
npm run dev
```

### 8.3 完整构建（单二进制）

```bash
# 必须先构建前端，否则 go:embed 失败
cd web && npm ci && npm run typecheck && npm run build   # nuxt generate，产物 → src/dist
cd ../src
go build -ldflags="-s -w -X main.Version=dev" .
```

### 8.4 运行测试

```bash
cd src
go test ./...     # 单测 + main_test.go 集成测试（httptest 全栈）
```

测试覆盖 8 个集成场景：`/ready`、前端禁用 404、单/批量下载 prepare 签发 token、批量数量上限、GitHub 域名白名单校验、`/v2/` ping、搜索缺参 400、SPA 回退。各模块另有配套单测。

> 前端暂无测试框架与 lint 配置，质量门禁为 `npm run typecheck`（vue-tsc strict）+ `npm run build`（构建失败即回退）。

### 8.5 常见问题排查

| 现象 | 原因与处理 |
|---|---|
| `go:embed all:dist` 编译失败 | `src/dist` 不存在或为空。先执行 `web/` 构建，或临时创建 `dist/.keep` 占位 |
| CI `npm ci` 报 lockfile 不同步（missing xxx from lock file） | 本地增量安装可能遗漏部分平台 optional 依赖条目。删除 `node_modules` + `package-lock.json` 重新 `npm install`，并本地跑一次 `npm ci` 验证后再提交 |
| 前端 dev 页面 `/api/*` 全部失败 | 后端未启动或未监听 `127.0.0.1:5000`（devProxy 目标） |
| 访问新增前端页面返回 403 | 路径未在 `registerFrontendRoutes` 注册，落入 GitHub 代理兜底，见 6.4 |
| Docker 镜像内配置不生效 | 挂载路径必须覆盖 `/app/config.toml`；镜像内缺省配置是 `src/config.toml` 模板 |

---

## 9. 构建与发布

### 9.1 Docker（三阶段多架构）

1. **frontend**：`node:24-alpine` → `npm ci && npm run build`（nuxt generate）
2. **builder**：`golang:1.26-alpine` → `COPY src/` + 前端 dist → `CGO_ENABLED=0` 按 `TARGETARCH` 构建，ldflags 注入版本 → **UPX -9 压缩**
3. **runtime**：`alpine`，仅二进制 + config.toml

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t li-gh-proxy:local .
```

### 9.2 系统包（nfpm）

`packaging/nfpm.{deb-rpm,apk}.yaml`，通过 `NFPM_ARCH` / `NFPM_VERSION` 注入：

- 二进制 → `/usr/bin/li-gh-proxy`；配置 → `/etc/li-gh-proxy/config.toml`（noreplace，升级不覆盖）
- deb/rpm：systemd unit（`CONFIG_PATH=/etc/li-gh-proxy/config.toml`，`Restart=always`）
- apk：OpenRC + logrotate

### 9.3 CI/CD（.github/workflows）

| Workflow | 触发 | 流程 |
|---|---|---|
| `release.yml` | 手动（输入版本） | 构建前端 → Go 交叉编译 amd64/arm64 → UPX → nfpm 打 6 个系统包 → tar.gz → 生成 changelog → GitHub Release |
| `docker-ghcr.yml` | 手动（输入版本） | buildx 多架构推送 `ghcr.io/{repo}:{版本,latest}` |
| `docs.yml` | push main（`docs/src/**` 变更）或手动 | Astro 构建 → GitHub Pages 部署 |

手动触发示例：

```bash
gh workflow run "ghcr镜像构建" -f version=v1.2.6
gh run watch --exit-status     # 跟踪进度
```

> **注意**：本仓库为 fork（`LiStudioorg/li-gh-proxy`）。仓库相关引用（`install.sh`、README、workflow 产物地址、文档站、nfpm 元数据）已统一指向本仓库；GHCR 镜像名使用小写 `ghcr.io/listudioorg/li-gh-proxy`（与 `docker-ghcr.yml` 中 `github.repository` 自动小写的结果一致）。LICENSE 版权声明与 FAQ 中引用的上游 issue 链接保持原样（fork 不继承 issue）。文档站域名 `docs.52013120.xyz` 仍为上游域名，如需自有域名需另行替换。

---

## 10. 设计要点与注意事项

1. **流式优先**：blob、GitHub 文件、镜像 tar 全部 `io.Copy` 直通不落盘；`WriteTimeout 30min` 与之配套；缓存命中的响应统一经 `WriteCachedResponse` / `WriteTokenResponse` 回放。
2. **全局单例 + init 初始化**：`dockerProxy`、`globalImageStreamer`、`GlobalCache`、`GlobalAccessController`、`globalLimiter` 等，无生命周期管理（无优雅关闭）。
3. **配置热读**：Handler 每请求调 `config.GetConfig()`，靠 5s TTL 副本缓存降低开销，切片深拷贝保证隔离。
4. **多层防护叠加**：IP 令牌桶（全局限流）→ 下载防抖（内容指纹 + 用户标识）→ 一次性令牌（防重放、IP/UA 绑定）。
5. **错误处理**：以"日志 + 状态码"为主，无重试（搜索模块除外，3 次重试 + 退避）；tar 流开始输出后禁止再写响应体。
6. **gitignore**：`src/dist/`、`web/.nuxt/`、`web/.output/` 不入库，仅构建时生成。
7. **lockfile 即契约**：Docker 与 CI 均用 `npm ci` 安装（严格校验 `package-lock.json`），升级依赖必须在本地完整跑通 `npm ci && npm run build` 后再提交锁文件。
8. **前端产物契约**：`nuxt.config.ts` 中 `ssr: false`、`buildAssetsDir: 'assets/'`、`nitro.output.publicDir → ../src/dist` 三项与 Go embed 一一对应，改动任一项都会破坏单二进制交付。
9. **静态资源加速**：gzip 预压缩在启动时完成（见 4.1）；`/assets/*` 文件名含内容 hash，可 `immutable` 长缓存，因此 `index.html` 必须保持 `no-cache`，否则发版后客户端会拿旧 hash 引用而 404。
