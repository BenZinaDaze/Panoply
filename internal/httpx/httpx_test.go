package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// shortRetryAfterCap 测试内缩短 Retry-After 封顶，避免真实等待。
func shortRetryAfterCap(t *testing.T, d time.Duration) {
	t.Helper()
	old := maxRetryAfterWait
	maxRetryAfterWait = d
	t.Cleanup(func() { maxRetryAfterWait = old })
}

func TestFetchDeterministicClientErrorNoRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	start := time.Now()
	_, err := Fetch(server.Client(), server.URL, FetchOptions{RetryDelay: 10 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("err = %v, want HTTP 404", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("确定性 4xx 不应重试：calls = %d, want 1", n)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("应立即失败，实际耗时 %v", elapsed)
	}
}

func TestFetchRetryOnServerError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	body, err := Fetch(server.Client(), server.URL, FetchOptions{RetryDelay: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" || calls.Load() != 3 {
		t.Errorf("body = %q, calls = %d, want ok/3", body, calls.Load())
	}
}

func TestFetchRetryAfterCapped(t *testing.T) {
	// Retry-After: 3600 应被封顶缩短，否则测试早已超时
	shortRetryAfterCap(t, 20*time.Millisecond)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	body, err := Fetch(server.Client(), server.URL, FetchOptions{RetryDelay: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" || calls.Load() != 2 {
		t.Errorf("body = %q, calls = %d, want ok/2", body, calls.Load())
	}
}

func TestFetchRetryAfterInvalidFallsBack(t *testing.T) {
	// 非秒数形式（HTTP-date）不解析，回退默认间隔
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "Wed, 21 Oct 2015 07:28:00 GMT")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	body, err := Fetch(server.Client(), server.URL, FetchOptions{RetryDelay: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" || calls.Load() != 2 {
		t.Errorf("body = %q, calls = %d, want ok/2", body, calls.Load())
	}
}

func TestRetryAfterDelayParse(t *testing.T) {
	shortRetryAfterCap(t, 10*time.Second)
	for _, tc := range []struct {
		header string
		want   time.Duration
		ok     bool
	}{
		{"30", 10 * time.Second, true},              // 封顶
		{"1", time.Second, true},                    // 正常秒数
		{" 2 ", 2 * time.Second, true},              // 容忍空白
		{"0", 0, true},                              // 0 秒合法
		{"-1", 0, false},                            // 负数拒绝
		{"abc", 0, false},                           // 非数字拒绝
		{"", 0, false},                              // 缺失拒绝
		{"Wed, 21 Oct 2015 07:28:00 GMT", 0, false}, // HTTP-date 不支持
	} {
		got, ok := retryAfterDelay(tc.header)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("retryAfterDelay(%q) = %v,%v want %v,%v", tc.header, got, ok, tc.want, tc.ok)
		}
	}
}
