package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew_PanicsOnEmptyName(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty Name")
		}
	}()
	New(Config{Name: ""})
}

func TestNew_Defaults(t *testing.T) {
	c := New(Config{Name: "test"})
	if c.name != "test" {
		t.Errorf("expected name=test, got %s", c.name)
	}
	if c.timeout != 2*time.Second {
		t.Errorf("expected timeout=2s, got %s", c.timeout)
	}
}

func TestClient_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New(Config{
		Name:   "test",
		Addr:   srv.URL,
		Retry:  &RetryConfig{MaxRetries: 0}, // disable retry for test
	}, WithDisableMetrics())

	resp, err := c.Get(context.Background(), "/hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestClient_Post(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type=application/json, got %s", ct)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := New(Config{
		Name:  "test",
		Addr:  srv.URL,
		Retry: &RetryConfig{MaxRetries: 0},
	}, WithDisableMetrics())

	resp, err := c.Post(context.Background(), "/data", "application/json", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
}

func TestClient_AutoFillAddr(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Config{
		Name:  "test",
		Addr:  srv.URL,
		Retry: &RetryConfig{MaxRetries: 0},
	}, WithDisableMetrics())

	resp, err := c.Get(context.Background(), "/api/hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if receivedPath != "/api/hello" {
		t.Errorf("expected path /api/hello, got %s", receivedPath)
	}
}

// WithDisableMetrics disables metrics for test clients.
func WithDisableMetrics() Option {
	return func(c *Client) {}
}
