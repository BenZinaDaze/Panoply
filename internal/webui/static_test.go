package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newStaticTestServer 构造带 frontend/dist 的测试服务。
func newStaticTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.MkdirAll(filepath.Join(rootDir, "frontend", "dist", "assets"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "frontend", "dist", "index.html"),
		[]byte("<!doctype html><html><body>app</body></html>"), 0o644)
	os.WriteFile(filepath.Join(rootDir, "frontend", "dist", "assets", "app-abc123.js"),
		[]byte("console.log('app')"), 0o644)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\n"), 0o644)
	rt := InitRuntime(rootDir)
	rt.ResetJWTSecretCache()
	server := NewServer(rt)
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// 重建后旧 hash chunk 已不存在：缺失的静态资源必须 404，
// 回退 index.html 会以 text/html 响应 JS 请求导致浏览器 MIME 校验失败（切页全黑）
func TestRootMissingAssetReturns404(t *testing.T) {
	ts := newStaticTestServer(t)
	resp, err := http.Get(ts.URL + "/assets/FileManagerPage-gone.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失资源 status = %d, want 404", resp.StatusCode)
	}
	if strings.Contains(string(body), "app</body>") {
		t.Error("缺失资源被回退成 index.html")
	}
}

// SPA 路由（无扩展名）仍回退 index.html，且必须 no-cache（重建后浏览器立刻拿到新资源引用）
func TestRootSPAFallbackAndCacheHeaders(t *testing.T) {
	ts := newStaticTestServer(t)

	// SPA 回退路径
	resp, err := http.Get(ts.URL + "/files")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(string(body), "app</body>") ||
		resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("SPA 路由未正确回退 index.html: status=%d content-type=%s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("SPA 回退 Cache-Control = %q, want no-cache", cc)
	}

	// 入口 index.html
	resp2, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if cc := resp2.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("index.html Cache-Control = %q, want no-cache", cc)
	}

	// 已存在的内容哈希资源长缓存
	resp3, err := http.Get(ts.URL + "/assets/app-abc123.js")
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if cc := resp3.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("/assets/ Cache-Control = %q, want immutable", cc)
	}
}
