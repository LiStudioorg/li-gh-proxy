package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"li-gh-proxy/config"
	"li-gh-proxy/handlers"
	"li-gh-proxy/utils"
)

func newTestRouter(t *testing.T, configBody string) *gin.Engine {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(configBody), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG_PATH", path)
	if err := config.LoadConfig(); err != nil {
		t.Fatal(err)
	}

	utils.InitHTTPClients()
	globalLimiter = utils.InitGlobalLimiter()
	handlers.InitDockerProxy()
	handlers.InitImageStreamer()
	handlers.InitDebouncer()

	return buildRouter(config.GetConfig())
}

func performRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "li-gh-proxy-test")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestReadyRoute(t *testing.T) {
	router := newTestRouter(t, "")

	w := performRequest(router, http.MethodGet, "/ready", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ready"] != true || got["service"] != "li-gh-proxy" {
		t.Fatalf("unexpected ready response: %#v", got)
	}
}

func TestFrontendDisabledRoutesReturnNotFound(t *testing.T) {
	router := newTestRouter(t, `
[server]
enableFrontend = false
`)

	for _, path := range []string{"/", "/images", "/search", "/links", "/favicon.ico"} {
		w := performRequest(router, http.MethodGet, path, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, w.Code)
		}
	}
}

func TestSingleImageDownloadPrepareReturnsURL(t *testing.T) {
	router := newTestRouter(t, "")

	w := performRequest(router, http.MethodGet, "/api/image/download?image=nginx&mode=prepare", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got struct {
		DownloadURL string `json:"download_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.DownloadURL, "image=nginx") || !strings.Contains(got.DownloadURL, "token=") {
		t.Fatalf("download_url = %q", got.DownloadURL)
	}
	if !strings.HasPrefix(got.DownloadURL, "/api/image/download?") {
		t.Fatalf("download_url = %q", got.DownloadURL)
	}
}

func TestBatchImageDownloadPrepareReturnsURL(t *testing.T) {
	router := newTestRouter(t, "")

	body := `{"images":["nginx"],"useCompressedLayers":true}`
	w := performRequest(router, http.MethodPost, "/api/image/batch?mode=prepare", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got struct {
		DownloadURL string `json:"download_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.DownloadURL, "/api/image/batch?token=") {
		t.Fatalf("download_url = %q", got.DownloadURL)
	}
}

func TestBatchImageDownloadRejectsTooManyImages(t *testing.T) {
	router := newTestRouter(t, `
[download]
maxImages = 1
`)

	body := `{"images":["nginx","redis"],"useCompressedLayers":true}`
	w := performRequest(router, http.MethodPost, "/api/image/batch?mode=prepare", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestGitHubNoRouteRejectsUnsupportedHost(t *testing.T) {
	router := newTestRouter(t, "")

	w := performRequest(router, http.MethodGet, "/https://example.com/file.zip", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

func TestDockerV2PingAndInvalidPath(t *testing.T) {
	router := newTestRouter(t, "")

	w := performRequest(router, http.MethodGet, "/v2/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("/v2/ status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	w = performRequest(router, http.MethodGet, "/v2/library/nginx/unknown/latest", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid v2 status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestSearchAPIRejectsMissingQuery(t *testing.T) {
	router := newTestRouter(t, `
[server]
enableFrontend = false
`)

	w := performRequest(router, http.MethodGet, "/api/search", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}

	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] == "" {
		t.Fatalf("missing error response: %#v", got)
	}
}

func TestSearchServesSPAWhenFrontendEnabled(t *testing.T) {
	router := newTestRouter(t, `
[server]
enableFrontend = true
`)

	w := performRequest(router, http.MethodGet, "/search?q=nginx", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("content-type = %q, want text/html", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), `<div id="__nuxt">`) {
		t.Fatalf("SPA shell missing: %s", w.Body.String())
	}

	w = performRequest(router, http.MethodGet, "/links", "")
	if w.Code != http.StatusOK {
		t.Fatalf("/links status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("/links content-type = %q, want text/html", w.Header().Get("Content-Type"))
	}
}

// writeContentFile 在 dir 下写入内容数据文件（友链/赞助商的「文件路由」测试辅助）
func writeContentFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFeaturesAPIReflectsConfig(t *testing.T) {
	router := newTestRouter(t, `
[friends]
enabled = true
`)

	w := performRequest(router, http.MethodGet, "/api/features", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got struct {
		Friends  bool `json:"friends"`
		Sponsors bool `json:"sponsors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Friends || got.Sponsors {
		t.Fatalf("features = %+v, want friends=true sponsors=false", got)
	}
}

func TestFriendsAPILoadsFromLocalFiles(t *testing.T) {
	dataDir := filepath.ToSlash(filepath.Join(t.TempDir(), "data", "friends"))
	writeContentFile(t, dataDir, "b-site.toml", `
name = "B 站点"
url = "https://b.example.com"
description = "按文件名排序在后面"
`)
	writeContentFile(t, dataDir, "a-site.toml", `
name = "A 站点"
url = "https://a.example.com"
avatar = "https://a.example.com/a.png"
`)
	// 非法条目（缺 url）应被跳过且不影响其他条目
	writeContentFile(t, dataDir, "broken.toml", `name = "Broken"`)
	// 非 .toml 文件应被忽略
	writeContentFile(t, dataDir, "readme.txt", "ignore me")

	router := newTestRouter(t, fmt.Sprintf(`
[friends]
enabled = true
dataDir = "%s"
`, dataDir))

	w := performRequest(router, http.MethodGet, "/api/friends", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got struct {
		Items []struct {
			Slug        string `json:"slug"`
			Name        string `json:"name"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Avatar      string `json:"avatar"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items = %+v, want 2 entries (broken/non-toml files skipped)", got.Items)
	}
	if got.Items[0].Slug != "a-site" || got.Items[0].Name != "A 站点" || got.Items[0].Avatar == "" {
		t.Fatalf("items[0] = %+v", got.Items[0])
	}
	if got.Items[1].Slug != "b-site" || got.Items[1].URL != "https://b.example.com" {
		t.Fatalf("items[1] = %+v", got.Items[1])
	}
}

func TestFriendsAPIDisabledReturns404(t *testing.T) {
	router := newTestRouter(t, "")

	w := performRequest(router, http.MethodGet, "/api/friends", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}

	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["code"] != "FEATURE_DISABLED" {
		t.Fatalf("response = %#v, want code=FEATURE_DISABLED", got)
	}
}

func TestSponsorsAPISortedByTier(t *testing.T) {
	dataDir := filepath.ToSlash(filepath.Join(t.TempDir(), "data", "sponsors"))
	writeContentFile(t, dataDir, "x.toml", `
name = "X Inc"
url = "https://x.example.com"
tier = 2
`)
	writeContentFile(t, dataDir, "a.toml", `
name = "A Inc"
url = "https://a.example.com"
tier = 1
`)
	writeContentFile(t, dataDir, "b.toml", `
name = "B Inc"
url = "https://b.example.com"
tier = 1
`)
	// 未填写 tier 时默认 0，应排在最前
	writeContentFile(t, dataDir, "m.toml", `
name = "M Inc"
url = "https://m.example.com"
`)

	router := newTestRouter(t, fmt.Sprintf(`
[sponsors]
enabled = true
dataDir = "%s"
`, dataDir))

	w := performRequest(router, http.MethodGet, "/api/sponsors", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got struct {
		Items []struct {
			Slug string `json:"slug"`
			Tier int    `json:"tier"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{"m", "a", "b", "x"}
	if len(got.Items) != len(wantOrder) {
		t.Fatalf("items = %+v, want %d entries", got.Items, len(wantOrder))
	}
	for i, slug := range wantOrder {
		if got.Items[i].Slug != slug {
			t.Fatalf("items[%d].slug = %q, want %q (order: tier asc, tie by slug)", i, got.Items[i].Slug, slug)
		}
	}
}

func TestNodesAPIReturnsConfiguredNodes(t *testing.T) {
	router := newTestRouter(t, `
[[nodes]]
name = "节点 A"
url = "https://a.example.com"

[[nodes]]
url = "https://b.example.com"
`)

	w := performRequest(router, http.MethodGet, "/api/nodes", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got struct {
		Current string `json:"current"`
		Nodes   []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	// httptest.NewRequest 默认 Host 为 example.com
	if got.Current != "example.com" {
		t.Fatalf("current = %q, want example.com", got.Current)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("nodes = %+v, want 2 entries", got.Nodes)
	}
	if got.Nodes[0].Name != "节点 A" || got.Nodes[0].URL != "https://a.example.com" {
		t.Fatalf("nodes[0] = %+v", got.Nodes[0])
	}
	// name 缺失时后端自动补全为域名
	if got.Nodes[1].Name != "b.example.com" {
		t.Fatalf("nodes[1].name = %q, want auto-filled host", got.Nodes[1].Name)
	}
}

func TestNodesAPIEmptyWhenNotConfigured(t *testing.T) {
	router := newTestRouter(t, "")

	w := performRequest(router, http.MethodGet, "/api/nodes", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got struct {
		Current string            `json:"current"`
		Nodes   []json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 0 {
		t.Fatalf("nodes = %v, want empty list", got.Nodes)
	}
	if got.Current == "" {
		t.Fatal("current 不应为空")
	}
}
