package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/example/go-template/pkg/aikit/log"
)

// Option customises a Client.
type Option func(*Client)

// Client is an HTTP client with middleware chain support.
type Client struct {
	name       string
	baseAddr   string
	std        *http.Client
	middleware Middleware
	timeout    time.Duration
}

// New creates a new HTTP client with the given config and options.
func New(cfg Config, opts ...Option) *Client {
	cfg.Fix()
	if err := cfg.Validate(); err != nil {
		panic(err.Error())
	}

	c := &Client{
		name:     cfg.Name,
		baseAddr: strings.TrimRight(cfg.Addr, "/"),
		std: &http.Client{
			Timeout: cfg.Timeout,
		},
		timeout: cfg.Timeout,
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

// Do sends an HTTP request through the middleware chain.
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

// Get sends a GET request.
func (c *Client) Get(ctx context.Context, target string, headers ...http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if len(headers) > 0 {
		for k, vs := range headers[0] {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
	}
	return c.Do(ctx, req)
}

// Post sends a POST request.
func (c *Client) Post(ctx context.Context, target, contentType string, body io.Reader, headers ...http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if len(headers) > 0 {
		for k, vs := range headers[0] {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
	}
	return c.Do(ctx, req)
}

// Put sends a PUT request.
func (c *Client) Put(ctx context.Context, target, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.Do(ctx, req)
}

// Delete sends a DELETE request.
func (c *Client) Delete(ctx context.Context, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// Name returns the client name.
func (c *Client) Name() string {
	return c.name
}
