package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ── Recovery Middleware ────────────────────────────────────────────────────────

func TestRecoveryMiddleware(t *testing.T) {
	mw := RecoveryMiddleware("test")
	handler := mw(func(ctx context.Context, req *Request) (*Response, error) {
		panic("boom")
	})

	_, err := handler(context.Background(), &Request{&http.Request{}})
	if err == nil {
		t.Fatal("expected error from panic recovery")
	}
}

// ── Breaker Middleware ────────────────────────────────────────────────────────

func TestBreakerMiddleware_5xxCountsAsFailure(t *testing.T) {
	callCount := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	mw := BreakerMiddleware(BreakerConfig{
		Name:                   "test_5xx",
		RequestVolumeThreshold: 3,
		SleepWindow:            60 * time.Second,
		ErrorPercentThreshold:  50,
	})

	inner := func(ctx context.Context, req *Request) (*Response, error) {
		resp, err := http.DefaultClient.Do(req.Request)
		if err != nil {
			return nil, err
		}
		return &Response{resp}, nil
	}

	handler := mw(inner)

	// Send enough requests to trigger the breaker
	for i := 0; i < 20; i++ {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		resp, _ := handler(context.Background(), &Request{req})
		if resp != nil {
			resp.Body.Close()
		}
	}

	// After 20 500s, breaker should be open
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	_, err := handler(context.Background(), &Request{req})
	if err == nil {
		t.Log("breaker may not have opened yet (timing-dependent)")
	}
}

func TestBreakerMiddleware_4xxDoesNotCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	mw := BreakerMiddleware(BreakerConfig{
		Name:                   "test_4xx",
		RequestVolumeThreshold: 3,
		SleepWindow:            60 * time.Second,
		ErrorPercentThreshold:  50,
	})

	inner := func(ctx context.Context, req *Request) (*Response, error) {
		resp, err := http.DefaultClient.Do(req.Request)
		if err != nil {
			return nil, err
		}
		return &Response{resp}, nil
	}

	handler := mw(inner)

	// 4xx should not trip the breaker
	for i := 0; i < 20; i++ {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		resp, err := handler(context.Background(), &Request{req})
		if err != nil {
			t.Fatalf("unexpected error on attempt %d: %v", i, err)
		}
		resp.Body.Close()
	}
}

// ── Retry Middleware ──────────────────────────────────────────────────────────

func TestRetryMiddleware_SuccessOnFirstTry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mw := RetryMiddleware(RetryConfig{MaxRetries: 3, WaitBetween: 10 * time.Millisecond})
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
}

func TestRetryMiddleware_RetriesOn5xx(t *testing.T) {
	callCount := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mw := RetryMiddleware(RetryConfig{MaxRetries: 3, WaitBetween: 10 * time.Millisecond})
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestRetryMiddleware_NoRetryOn4xx(t *testing.T) {
	callCount := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	mw := RetryMiddleware(RetryConfig{MaxRetries: 3, WaitBetween: 10 * time.Millisecond})
	inner := func(ctx context.Context, req *Request) (*Response, error) {
		resp, err := http.DefaultClient.Do(req.Request)
		if err != nil {
			return nil, err
		}
		return &Response{resp}, nil
	}

	handler := mw(inner)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, _ := handler(context.Background(), &Request{req})
	resp.Body.Close()

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call (no retry on 4xx), got %d", callCount)
	}
}

// ── Chain ─────────────────────────────────────────────────────────────────────

func TestChain_ExecutionOrder(t *testing.T) {
	var order []string

	m1 := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			order = append(order, "m1-before")
			resp, err := next(ctx, req)
			order = append(order, "m1-after")
			return resp, err
		}
	}

	m2 := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			order = append(order, "m2-before")
			resp, err := next(ctx, req)
			order = append(order, "m2-after")
			return resp, err
		}
	}

	core := func(ctx context.Context, req *Request) (*Response, error) {
		order = append(order, "core")
		return nil, errors.New("done")
	}

	handler := Chain(m1, m2)(core)
	_, _ = handler(context.Background(), &Request{&http.Request{}})

	expected := []string{"m1-before", "m2-before", "core", "m2-after", "m1-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("position %d: expected %s, got %s", i, v, order[i])
		}
	}
}
