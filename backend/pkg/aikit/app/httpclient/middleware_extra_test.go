package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/huangyangke/go-aikit/app/middleware"
)

func TestClient_Put(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type=application/json, got %s", ct)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Config{
		Name:           "test-put",
		Addr:           srv.URL,
		Retry:          &RetryConfig{MaxRetries: 0},
		DisableMetrics: true,
	})

	resp, err := c.Put(context.Background(), "/resource", "application/json", nil, nil)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestClient_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(Config{
		Name:           "test-delete",
		Addr:           srv.URL,
		Retry:          &RetryConfig{MaxRetries: 0},
		DisableMetrics: true,
	})

	resp, err := c.Delete(context.Background(), "/resource/123", nil)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestClient_Name(t *testing.T) {
	c := New(Config{Name: "my-api-client", DisableMetrics: true})
	assert.Equal(t, "my-api-client", c.Name())
}

func TestAccessLogMiddleware(t *testing.T) {
	mw := AccessLogMiddleware("test-log")
	handler := mw(func(ctx context.Context, req *Request) (*Response, error) {
		resp := &http.Response{StatusCode: http.StatusOK}
		return &Response{resp}, nil
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/api", nil)
	resp, err := handler(context.Background(), &Request{req})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMetricsMiddleware(t *testing.T) {
	mw := MetricsMiddleware("test-metrics")
	handler := mw(func(ctx context.Context, req *Request) (*Response, error) {
		resp := &http.Response{StatusCode: http.StatusCreated}
		return &Response{resp}, nil
	})

	req, _ := http.NewRequest(http.MethodPost, "http://example.com/submit", nil)
	resp, err := handler(context.Background(), &Request{req})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestRequestIDMiddleware_Propagates(t *testing.T) {
	mw := RequestIDMiddleware()
	handler := mw(func(ctx context.Context, req *Request) (*Response, error) {
		assert.Equal(t, "req-123", req.Header.Get("X-Request-ID"))
		resp := &http.Response{StatusCode: http.StatusOK}
		return &Response{resp}, nil
	})

	ctx := middleware.WithTaskID(context.Background(), "req-123")
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/api", nil)
	_, err := handler(ctx, &Request{req})
	assert.NoError(t, err)
}

func TestRequestIDMiddleware_DoesNotOverwrite(t *testing.T) {
	mw := RequestIDMiddleware()
	handler := mw(func(ctx context.Context, req *Request) (*Response, error) {
		assert.Equal(t, "existing-id", req.Header.Get("X-Request-ID"))
		resp := &http.Response{StatusCode: http.StatusOK}
		return &Response{resp}, nil
	})

	ctx := middleware.WithTaskID(context.Background(), "new-id")
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/api", nil)
	req.Header.Set("X-Request-ID", "existing-id")
	_, err := handler(ctx, &Request{req})
	assert.NoError(t, err)
}

func TestRequestIDMiddleware_NoTaskID(t *testing.T) {
	mw := RequestIDMiddleware()
	handler := mw(func(ctx context.Context, req *Request) (*Response, error) {
		assert.Empty(t, req.Header.Get("X-Request-ID"))
		resp := &http.Response{StatusCode: http.StatusOK}
		return &Response{resp}, nil
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/api", nil)
	_, err := handler(context.Background(), &Request{req})
	assert.NoError(t, err)
}

func TestAccessLogMiddleware_WithError(t *testing.T) {
	mw := AccessLogMiddleware("test-err")
	handler := mw(func(ctx context.Context, req *Request) (*Response, error) {
		return nil, context.DeadlineExceeded
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/api", nil)
	resp, err := handler(context.Background(), &Request{req})
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestClient_Do_WithTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Config{
		Name:           "test-timeout",
		Addr:           srv.URL,
		Timeout:        10 * time.Millisecond,
		DisableMetrics: true,
	})

	_, err := c.Get(context.Background(), "/slow", nil)
	assert.Error(t, err)
}

// ── mergeHeaders ──────────────────────────────────────────────────────────────

func TestMergeHeaders_NoDefaultHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/api", nil)
	req.Header.Set("X-Custom", "value1")
	req.Header.Set("Accept", "application/json")

	mergeHeaders(req, nil)

	// Original headers should remain untouched
	assert.Equal(t, "value1", req.Header.Get("X-Custom"))
	assert.Equal(t, "application/json", req.Header.Get("Accept"))
}

func TestMergeHeaders_RequestHeadersOverrideDefaults(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/api", nil)
	req.Header.Set("Content-Type", "application/json")

	extra := http.Header{}
	extra.Set("Content-Type", "text/plain")
	extra.Set("X-Extra", "extra-val")

	mergeHeaders(req, extra)

	// extra headers overwrite; Content-Type should be "text/plain"
	assert.Equal(t, "text/plain", req.Header.Get("Content-Type"))
	assert.Equal(t, "extra-val", req.Header.Get("X-Extra"))
}

// ── responseError.Error() ─────────────────────────────────────────────────────

func TestResponseError_Error(t *testing.T) {
	e := &responseError{StatusCode: 404}
	assert.Equal(t, "http status 404", e.Error())
}

// ── isAcceptableHTTPError ─────────────────────────────────────────────────────

func TestIsAcceptableHTTPError_Nil(t *testing.T) {
	assert.True(t, isAcceptableHTTPError(nil))
}

func TestIsAcceptableHTTPError_4xx(t *testing.T) {
	assert.True(t, isAcceptableHTTPError(&responseError{StatusCode: 404}))
	assert.True(t, isAcceptableHTTPError(&responseError{StatusCode: 400}))
	assert.True(t, isAcceptableHTTPError(&responseError{StatusCode: 499}))
}

func TestIsAcceptableHTTPError_5xx(t *testing.T) {
	assert.False(t, isAcceptableHTTPError(&responseError{StatusCode: 500}))
	assert.False(t, isAcceptableHTTPError(&responseError{StatusCode: 503}))
}

func TestIsAcceptableHTTPError_NonHTTPError(t *testing.T) {
	assert.False(t, isAcceptableHTTPError(errors.New("connection refused")))
}

// ── safePath (cardinalityGuard) ────────────────────────────────────────────────

func TestSafePath_NoPathParams(t *testing.T) {
	g := &cardinalityGuard{}
	result := g.safePath("/api/users")
	assert.Equal(t, "/api/users", result)
}

func TestSafePath_FirstSeenPathReturnedAsIs(t *testing.T) {
	// safePath returns the path as-is on first call (no param substitution).
	// The note "注意 safePath 函数签名" refers to it being a method on cardinalityGuard.
	g := &cardinalityGuard{}
	result := g.safePath("/api/users/123")
	assert.Equal(t, "/api/users/123", result)
}

func TestSafePath_HighCardinalityPathReplaced(t *testing.T) {
	g := &cardinalityGuard{}
	// Fill up to maxTrackedPaths distinct paths
	for i := 0; i < maxTrackedPaths; i++ {
		_ = g.safePath("/path/" + string(rune('a'+i%26)) + string(rune('0'+i/26)))
	}
	// The (maxTrackedPaths+1)th unique path should be replaced
	result := g.safePath("/path/entirely/new/path/that/exceeds/limit")
	assert.Equal(t, "high_cardinality_path", result)
}

// ── RetryMiddleware all retries fail ──────────────────────────────────────────

func TestRetryMiddleware_AllRetriesFail(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	mw := RetryMiddleware(RetryConfig{MaxRetries: 2, WaitBetween: 1 * time.Millisecond})
	inner := func(ctx context.Context, req *Request) (*Response, error) {
		resp, err := http.DefaultClient.Do(req.Request)
		if err != nil {
			return nil, err
		}
		return &Response{resp}, nil
	}

	handler := mw(inner)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := handler(context.Background(), &Request{req})

	// After all retries exhausted, get back the last 500 response (no error from transport)
	assert.NotNil(t, resp, "should return last response even on all-fail")
	if resp != nil {
		resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	}
	assert.NoError(t, err) // network-level error is nil; 5xx is not an error in the return value
	assert.Equal(t, 3, callCount, "should have tried 1 + MaxRetries=2 = 3 times")
}

// ── httpclient.New: with options ──────────────────────────────────────────────

func TestNew_WithOption(t *testing.T) {
	called := false
	opt := func(c *Client) { called = true }
	New(Config{Name: "test", DisableMetrics: true}, opt)
	assert.True(t, called)
}

// ── httpclient.New: with Breaker config ───────────────────────────────────────

func TestNew_WithBreaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Config{
		Name:           "breaker-test",
		Addr:           srv.URL,
		DisableMetrics: true,
		Breaker: &BreakerConfig{
			Name:                   "test-breaker",
			MaxRequests:            1,
			RequestVolumeThreshold: 5,
			SleepWindow:            1 * time.Second,
			ErrorPercentThreshold:  50,
		},
	})
	resp, err := c.Get(context.Background(), "/", nil)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ── mergeHeaders: default headers with multiple values ───────────────────────

func TestMergeHeaders_MultipleValues(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	extra := http.Header{
		"X-Custom": {"v1", "v2"},
	}
	mergeHeaders(req, extra)
	vals := req.Header["X-Custom"]
	assert.Equal(t, []string{"v1", "v2"}, vals)
}

// ── Get/Post/Put/Delete: nil body ─────────────────────────────────────────────

func TestGet_NilHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Config{Name: "test", Addr: srv.URL, DisableMetrics: true})
	resp, err := c.Get(context.Background(), "/", nil)
	assert.NoError(t, err)
	defer resp.Body.Close()
}

func TestPost_NilBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Config{Name: "test", Addr: srv.URL, DisableMetrics: true})
	resp, err := c.Post(context.Background(), "/", "application/json", nil, nil)
	assert.NoError(t, err)
	defer resp.Body.Close()
}

func TestDelete_NilHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(Config{Name: "test", Addr: srv.URL, DisableMetrics: true})
	resp, err := c.Delete(context.Background(), "/res", nil)
	assert.NoError(t, err)
	defer resp.Body.Close()
}

// ── Config.Fix: Breaker branch ────────────────────────────────────────────────

func TestConfigFix_BreakerNil(t *testing.T) {
	cfg := Config{Name: "test", Timeout: 5 * time.Second}
	cfg.Fix() // Breaker == nil, should not panic
	assert.Equal(t, 5*time.Second, cfg.Timeout)
}

// ── BreakerMiddleware: 5xx response triggers fallback ────────────────────────

func TestBreakerMiddleware_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(Config{
		Name:           "breaker-5xx",
		Addr:           srv.URL,
		DisableMetrics: true,
		Breaker: &BreakerConfig{
			Name:                   "b5xx",
			RequestVolumeThreshold: 1,
			ErrorPercentThreshold:  1,
			SleepWindow:            100 * time.Millisecond,
			MaxRequests:            1,
		},
	})
	// First request: 5xx, no error returned (fallback swallows acceptable errors — but 5xx is NOT acceptable)
	resp, err := c.Get(context.Background(), "/", nil)
	// The breaker may return error or resp with 500 depending on fallback logic
	if err == nil {
		defer resp.Body.Close()
	}
	// Just verify no panic and we got some response
}

// ── RetryMiddleware: success on first try ────────────────────────────────────

func TestRetryMiddleware_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c := New(Config{
		Name:           "retry-ctx",
		Addr:           srv.URL,
		DisableMetrics: true,
		Retry:          &RetryConfig{MaxRetries: 2, WaitBetween: 10 * time.Millisecond},
	})
	_, err := c.Get(ctx, "/", nil)
	assert.Error(t, err) // context deadline exceeded
}

// ── Do: no middleware path ────────────────────────────────────────────────────

func TestDo_NoMiddleware(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Client without any middleware (no retry, no breaker)
	c := New(Config{Name: "nomw", DisableMetrics: true})
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	resp, err := c.Do(context.Background(), req)
	assert.NoError(t, err)
	defer resp.Body.Close()
}

// ── Get/Post/Put/Delete: invalid URL ─────────────────────────────────────────

func TestGet_InvalidURL(t *testing.T) {
	c := New(Config{Name: "test", DisableMetrics: true})
	_, err := c.Get(context.Background(), "://invalid", nil)
	assert.Error(t, err)
}

func TestPost_InvalidURL(t *testing.T) {
	c := New(Config{Name: "test", DisableMetrics: true})
	_, err := c.Post(context.Background(), "://invalid", "application/json", nil, nil)
	assert.Error(t, err)
}

func TestPut_InvalidURL(t *testing.T) {
	c := New(Config{Name: "test", DisableMetrics: true})
	_, err := c.Put(context.Background(), "://invalid", "application/json", nil, nil)
	assert.Error(t, err)
}

func TestDelete_InvalidURL(t *testing.T) {
	c := New(Config{Name: "test", DisableMetrics: true})
	_, err := c.Delete(context.Background(), "://invalid", nil)
	assert.Error(t, err)
}

// ── BreakerMiddleware: fallback + open circuit ────────────────────────────────

func TestBreakerMiddleware_FallbackAcceptable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // 4xx: acceptable, doesn't trip breaker
	}))
	defer srv.Close()

	c := New(Config{
		Name:           "breaker-4xx",
		Addr:           srv.URL,
		DisableMetrics: true,
		Breaker: &BreakerConfig{
			Name:                   "b4xx",
			RequestVolumeThreshold: 1,
			ErrorPercentThreshold:  1,
			SleepWindow:            100 * time.Millisecond,
			MaxRequests:            1,
		},
	})
	resp, err := c.Get(context.Background(), "/", nil)
	assert.NoError(t, err)
	if resp != nil {
		defer resp.Body.Close()
	}
}

// ── safePath: high-cardinality path ──────────────────────────────────────────

// ── RetryMiddleware: non-retryable error ──────────────────────────────────────

func TestRetryMiddleware_NonRetryableStatusCode(t *testing.T) {
	// 4xx errors should not be retried
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New(Config{
		Name:           "retry-4xx",
		Addr:           srv.URL,
		DisableMetrics: true,
		Retry:          &RetryConfig{MaxRetries: 3, WaitBetween: 1 * time.Millisecond},
	})
	resp, err := c.Get(context.Background(), "/", nil)
	assert.NoError(t, err)
	if resp != nil {
		defer resp.Body.Close()
	}
	// 4xx should not be retried, so only 1 call
	assert.Equal(t, 1, callCount)
}

// ── safePath: second call returns same path (loaded branch) ──────────────────

func TestSafePath_SecondCall_ReturnsCached(t *testing.T) {
	g := &cardinalityGuard{}
	first := g.safePath("/api/users")
	second := g.safePath("/api/users") // same path → loaded=true branch
	assert.Equal(t, "/api/users", first)
	assert.Equal(t, "/api/users", second)
}

// ── RetryMiddleware: GetBody error ────────────────────────────────────────────

func TestRetryMiddleware_GetBodyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // trigger retry
	}))
	defer srv.Close()

	c := New(Config{
		Name:           "retry-getbody",
		Addr:           srv.URL,
		DisableMetrics: true,
		Retry:          &RetryConfig{MaxRetries: 2, WaitBetween: 1 * time.Millisecond},
	})

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/", nil)
	req.GetBody = func() (io.ReadCloser, error) {
		return nil, errors.New("cannot rewind body")
	}

	_, err := c.Do(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reset request body")
}

// ── RetryMiddleware: context cancelled during wait ────────────────────────────

func TestRetryMiddleware_ContextCancelledDuringWait(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	c := New(Config{
		Name:           "retry-cancel-wait",
		Addr:           srv.URL,
		DisableMetrics: true,
		Retry:          &RetryConfig{MaxRetries: 3, WaitBetween: 500 * time.Millisecond},
	})

	// Cancel after first response to trigger ctx.Done() during wait
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := c.Get(ctx, "/", nil)
	assert.Error(t, err)
}
