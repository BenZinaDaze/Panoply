// Package httpx 共享的 HTTP 客户端基础设施：请求节流（最小间隔 + 429 冷却）
// 与带重试的 GET 抓取（确定性 4xx 短路、429/5xx 按 Retry-After 等待）。
// 收敛此前散落在 scraper（两套逐行重复的 requestWithRetry）
// 与 u115/trakt（同构的单桶预约限速器）的手写实现，让外部请求行为有单一参照。
// 定制语义较深的客户端（如 tmdb 的 JSON 解析重试）保留自有循环，不强迁。
package httpx

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limiter 请求节流器：相邻请求按最小间隔单桶预约排队，并支持 429 冷却。
// 冷却期内 Reserve 返回 ErrCooldown，由调用方转成各自的业务限流错误。
// 经 NewLimiter 构造，方法并发安全。
type Limiter struct {
	mu            sync.Mutex
	minInterval   time.Duration
	nextAt        time.Time
	cooldownUntil time.Time
	now           func() time.Time // 测试可替换
	sleep         func(time.Duration)
}

// ErrCooldown 冷却期内请求被拒绝。
var ErrCooldown = errors.New("httpx: rate limit cooldown")

// NewLimiter 构造节流器；minInterval<=0 表示不限间隔（仅冷却生效）。
func NewLimiter(minInterval time.Duration) *Limiter {
	return &Limiter{minInterval: minInterval, now: time.Now, sleep: time.Sleep}
}

// SetClockForTest 替换时钟与睡眠（仅测试使用）。
func (l *Limiter) SetClockForTest(now func() time.Time, sleep func(time.Duration)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now, l.sleep = now, sleep
}

// CooldownRemaining 冷却剩余时长（0 = 未冷却）。
func (l *Limiter) CooldownRemaining() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if wait := l.cooldownUntil.Sub(l.now()); wait > 0 {
		return wait
	}
	return 0
}

// EnterCooldown 进入冷却：冷却期内请求被拒绝，时隙同时推后至冷却结束。
func (l *Limiter) EnterCooldown(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	until := l.now().Add(d)
	if until.After(l.cooldownUntil) {
		l.cooldownUntil = until
	}
	if until.After(l.nextAt) {
		l.nextAt = until
	}
}

// Reserve 阻塞等待并获得下一个请求时隙；冷却期内返回 ErrCooldown（不等待）。
func (l *Limiter) Reserve() error {
	l.mu.Lock()
	now := l.now()
	if l.cooldownUntil.After(now) {
		l.mu.Unlock()
		return ErrCooldown
	}
	reserveAt := now
	if l.nextAt.After(reserveAt) {
		reserveAt = l.nextAt
	}
	wait := reserveAt.Sub(now)
	if l.minInterval > 0 {
		l.nextAt = reserveAt.Add(l.minInterval)
	} else {
		l.nextAt = reserveAt
	}
	l.mu.Unlock()
	if wait > 0 {
		l.sleep(wait)
	}
	return nil
}

// FetchOptions 带重试的 GET 抓取参数。
type FetchOptions struct {
	// Headers 附加请求头（如 User-Agent / Accept）。
	Headers map[string]string
	// MaxAttempts 总尝试次数（含首次），<=0 时为 3。
	MaxAttempts int
	// RetryDelay 重试固定间隔，<=0 时为 2s。
	RetryDelay time.Duration
	// MaxBodyBytes 响应体大小上限（0 = 不限），超限按失败重试。
	MaxBodyBytes int64
}

// maxRetryAfterWait Retry-After 指示的等待上限：尊重服务端限流要求的同时，
// 避免异常大的值长时间阻塞调用方。var 仅为测试可缩短。
var maxRetryAfterWait = 10 * time.Second

// retryableStatus 判断失败状态码是否值得重试：408/429 与全部 5xx 重试；
// 其余 4xx 属确定性失败（参数错误、未授权、不存在等），重试不可能自愈。
func retryableStatus(code int) bool {
	if code == http.StatusRequestTimeout || code == http.StatusTooManyRequests {
		return true
	}
	return code >= 500
}

// retryAfterDelay 解析 Retry-After 头（仅支持秒数形式，AniBT 等站点采用），
// 返回封顶后的等待时长；缺失或无法解析时 ok=false。
func retryAfterDelay(header string) (time.Duration, bool) {
	secs, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil || secs < 0 {
		return 0, false
	}
	d := time.Duration(secs) * time.Second
	return min(d, maxRetryAfterWait), true
}

// Fetch 执行 GET：网络错误、响应体超限、可重试的非 2xx（408/429 与 5xx）
// 按间隔重试；其余 4xx 立即失败不重试。响应带 Retry-After（秒）时以其
// 指示的等待为准（封顶 maxRetryAfterWait，且不小于默认间隔）。
// 返回最后一次的错误与空 body。适合爬虫/RSS 类只读抓取；需要按响应体
// 定制语义的客户端不适用，应保留自有循环。
func Fetch(client *http.Client, rawURL string, opts FetchOptions) ([]byte, error) {
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	delay := opts.RetryDelay
	if delay <= 0 {
		delay = 2 * time.Second
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range opts.Headers {
			req.Header.Set(k, v)
		}
		wait := delay
		resp, err := client.Do(req)
		if err == nil {
			if resp.StatusCode >= 400 {
				resp.Body.Close()
				lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, rawURL)
				if !retryableStatus(resp.StatusCode) {
					return nil, lastErr
				}
				if d, ok := retryAfterDelay(resp.Header.Get("Retry-After")); ok && d > wait {
					wait = d
				}
			} else {
				// MaxBodyBytes<=0 表示不限；此前 LimitReader(body, 0+1) 会把
				// 不设上限的响应静默截断成 1 字节。
				var reader io.Reader = resp.Body
				if opts.MaxBodyBytes > 0 {
					reader = io.LimitReader(resp.Body, opts.MaxBodyBytes+1)
				}
				body, readErr := io.ReadAll(reader)
				resp.Body.Close()
				if readErr == nil && opts.MaxBodyBytes > 0 && int64(len(body)) > opts.MaxBodyBytes {
					readErr = fmt.Errorf("响应超过大小上限（%dMB）", opts.MaxBodyBytes>>20)
				}
				if readErr != nil {
					lastErr = readErr
				} else {
					return body, nil
				}
			}
		} else {
			lastErr = err
		}
		if attempt < maxAttempts-1 {
			time.Sleep(wait)
		}
	}
	return nil, lastErr
}
