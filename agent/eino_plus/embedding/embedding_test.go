package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func TestEmbedStrings(t *testing.T) {
	srv := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req apiRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "shanhai-embedding", req.Model)
		assert.Equal(t, 1024, req.Dimensions)
		assert.Len(t, req.Input, 2)
		assert.Equal(t, "你好", req.Input[0]["text"])
		assert.Equal(t, "世界", req.Input[1]["text"])

		resp := apiResponse{
			Data: []apiEmbeddingData{
				{Embedding: []float64{0.1, 0.2, 0.3}, Index: 0},
				{Embedding: []float64{0.4, 0.5, 0.6}, Index: 1},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	emb, err := NewEmbedder(context.Background(), &Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})
	require.NoError(t, err)

	vecs, err := emb.EmbedStrings(context.Background(), []string{"你好", "世界"})
	require.NoError(t, err)
	assert.Len(t, vecs, 2)
	assert.Equal(t, []float64{0.1, 0.2, 0.3}, vecs[0])
	assert.Equal(t, []float64{0.4, 0.5, 0.6}, vecs[1])
}

func TestEmbedStrings_Batching(t *testing.T) {
	callCount := 0
	srv := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req apiRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := apiResponse{Data: make([]apiEmbeddingData, len(req.Input))}
		for i := range req.Input {
			resp.Data[i] = apiEmbeddingData{Embedding: []float64{float64(callCount), float64(i)}, Index: i}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	emb, _ := NewEmbedder(context.Background(), &Config{
		APIKey:    "k",
		BaseURL:   srv.URL,
		BatchSize: 2,
	})

	// 3 texts → 2 batches (2+1)
	vecs, err := emb.EmbedStrings(context.Background(), []string{"a", "b", "c"})
	require.NoError(t, err)
	assert.Len(t, vecs, 3)
	assert.Equal(t, 2, callCount)
}

func TestEmbedStrings_Empty(t *testing.T) {
	emb, _ := NewEmbedder(context.Background(), &Config{
		APIKey:  "k",
		BaseURL: "http://unused",
	})

	vecs, err := emb.EmbedStrings(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, vecs)
}

func TestEmbedStrings_APIError(t *testing.T) {
	srv := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal"}`))
	})
	defer srv.Close()

	emb, _ := NewEmbedder(context.Background(), &Config{
		APIKey:  "k",
		BaseURL: srv.URL,
	})

	_, err := emb.EmbedStrings(context.Background(), []string{"test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestEmbedMultiModal(t *testing.T) {
	srv := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req apiRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Len(t, req.Input, 2)
		assert.Equal(t, "hello", req.Input[0]["text"])
		assert.Equal(t, "https://example.com/img.jpg", req.Input[1]["image"])

		resp := apiResponse{
			Data: []apiEmbeddingData{
				{Embedding: []float64{1.0}, Index: 0},
				{Embedding: []float64{2.0}, Index: 1},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	emb, _ := NewEmbedder(context.Background(), &Config{
		APIKey:  "k",
		BaseURL: srv.URL,
	})

	vecs, err := emb.EmbedMultiModal(context.Background(), []MultiModalInput{
		{Text: "hello"},
		{Image: "https://example.com/img.jpg"},
	})
	require.NoError(t, err)
	assert.Len(t, vecs, 2)
}

func TestConfigFix(t *testing.T) {
	cfg := Config{APIKey: "k", BaseURL: "http://x"}
	cfg.Fix()
	assert.Equal(t, "shanhai-embedding", cfg.Model)
	assert.Equal(t, 1024, cfg.Dimensions)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
	assert.Equal(t, 25, cfg.BatchSize)
}

func TestConfigValidate(t *testing.T) {
	assert.Error(t, (&Config{}).Validate())
	assert.Error(t, (&Config{APIKey: "k"}).Validate())
	assert.NoError(t, (&Config{APIKey: "k", BaseURL: "http://x"}).Validate())
}
