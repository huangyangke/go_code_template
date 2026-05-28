// Package httpclient 带中间件链的 HTTP 客户端.

package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/huangyangke/go-aikit/log"
)

// Option 自定义 Client 的配置函数.
type Option func(*Client)

// Client 带中间件链支持的 HTTP 客户端.
type Client struct {
	name       string
	baseAddr   string
	std        *http.Client
	middleware Middleware
	timeout    time.Duration
}

// New 创建新的 HTTP 客户端.
// 参数：cfg - 客户端配置, opts - 自定义选项列表.
// 返回值：*Client 客户端实例.
func New(cfg Config, opts ...Option) *Client {
	cfg.Fix()
	if err := cfg.Validate(); err != nil {
		panic(err.Error())
	}

	c := &Client{
		name:     cfg.Name,
		baseAddr: strings.TrimRight(cfg.Addr, "/"),
		std:      &http.Client{},
		timeout:  cfg.Timeout,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Build middleware chain
	var mws []Middleware

	// 1. Recovery (always first)
	mws = append(mws, RecoveryMiddleware(cfg.Name))

	// 2. Breaker
	if cfg.Breaker != nil {
		mws = append(mws, BreakerMiddleware(*cfg.Breaker))
	}

	// 3. Retry
	if cfg.Retry != nil {
		mws = append(mws, RetryMiddleware(*cfg.Retry))
	}

	// 4. Metrics
	if !cfg.DisableMetrics {
		mws = append(mws, MetricsMiddleware(cfg.Name))
	}

	// 5. RequestID propagation
	mws = append(mws, RequestIDMiddleware())

	// 6. Access log
	mws = append(mws, AccessLogMiddleware(cfg.Name))

	if len(mws) > 0 {
		c.middleware = Chain(mws...)
	}

	log.Info("[httpclient][%s] created (addr=%s, timeout=%s, breaker=%v, retry=%v)",
		cfg.Name, cfg.Addr, cfg.Timeout, cfg.Breaker != nil, cfg.Retry != nil)

	return c
}

// Do 通过中间件链发送 HTTP 请求.
// 参数：ctx - 上下文, req - HTTP 请求.
// 返回值：*http.Response HTTP 响应, err - 请求失败时的错误.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	req = req.WithContext(ctx)

	// Auto-fill base address
	if c.baseAddr != "" && !strings.HasPrefix(req.URL.String(), "http") {
		raw := c.baseAddr + "/" + strings.TrimLeft(req.URL.String(), "/")
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("httpclient: invalid URL %q: %w", raw, err)
		}
		req.URL = u
	}

	wrappedReq := &Request{req}

	if c.middleware != nil {
		handler := c.middleware(func(ctx context.Context, r *Request) (*Response, error) {
			resp, err := c.std.Do(r.Request)
			if err != nil {
				return nil, err
			}
			return &Response{resp}, nil
		})
		resp, err := handler(ctx, wrappedReq)
		if err != nil {
			return nil, err
		}
		return resp.Response, nil
	}

	return c.std.Do(req)
}

// mergeHeaders copies non-nil headers into the request.
// The first value for each key overwrites any existing value (Set);
// subsequent values are appended (Add). This prevents duplicate headers
// when the caller and convenience method both set the same key (e.g. Content-Type).
func mergeHeaders(req *http.Request, headers http.Header) {
	if headers == nil {
		return
	}
	for k, vs := range headers {
		if len(vs) > 0 {
			req.Header.Set(k, vs[0])
			for _, v := range vs[1:] {
				req.Header.Add(k, v)
			}
		}
	}
}

// Get 发送 GET 请求.
// 参数：ctx - 上下文, target - 目标地址, headers - 附加请求头.
// 返回值：*http.Response HTTP 响应, err - 请求失败时的错误.
func (c *Client) Get(ctx context.Context, target string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	mergeHeaders(req, headers)
	return c.Do(ctx, req)
}

// Post 发送 POST 请求.
// 参数：ctx - 上下文, target - 目标地址, contentType - 内容类型, body - 请求体, headers - 附加请求头.
// 返回值：*http.Response HTTP 响应, err - 请求失败时的错误.
func (c *Client) Post(ctx context.Context, target, contentType string, body io.Reader, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	mergeHeaders(req, headers)
	return c.Do(ctx, req)
}

// Put 发送 PUT 请求.
// 参数：ctx - 上下文, target - 目标地址, contentType - 内容类型, body - 请求体, headers - 附加请求头.
// 返回值：*http.Response HTTP 响应, err - 请求失败时的错误.
func (c *Client) Put(ctx context.Context, target, contentType string, body io.Reader, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	mergeHeaders(req, headers)
	return c.Do(ctx, req)
}

// Delete 发送 DELETE 请求.
// 参数：ctx - 上下文, target - 目标地址, headers - 附加请求头.
// 返回值：*http.Response HTTP 响应, err - 请求失败时的错误.
func (c *Client) Delete(ctx context.Context, target string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return nil, err
	}
	mergeHeaders(req, headers)
	return c.Do(ctx, req)
}

// Name 返回客户端名称.
// 返回值：string 客户端名称.
func (c *Client) Name() string {
	return c.name
}
