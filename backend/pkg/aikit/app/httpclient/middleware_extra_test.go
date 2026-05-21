package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/go-template/pkg/aikit/app/middleware"
	"github.com/stretchr/testify/assert"
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

	resp, err := c.Put(context.Background(), "/resource", "application/json", nil)
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

	resp, err := c.Delete(context.Background(), "/resource/123")
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

	_, err := c.Get(context.Background(), "/slow")
	assert.Error(t, err)
}
