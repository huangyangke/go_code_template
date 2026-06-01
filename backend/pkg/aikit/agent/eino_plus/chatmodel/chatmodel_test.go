package chatmodel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudwego/eino/schema"
)

func TestNewChatModel_Validation(t *testing.T) {
	_, err := NewChatModel(context.Background(), &Config{BaseURL: "http://x"})
	assert.Error(t, err, "missing api_key should fail")

	_, err = NewChatModel(context.Background(), &Config{APIKey: "k"})
	assert.Error(t, err, "missing base_url should fail")

	m, err := NewChatModel(context.Background(), &Config{APIKey: "k", BaseURL: "http://x"})
	require.NoError(t, err)
	assert.Equal(t, "shanhai-max", m.cfg.Model, "default model applied")
}

func TestGenerate_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req chatCompletionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.False(t, req.Stream)
		assert.Len(t, req.Messages, 2)

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "你好，世界"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer srv.Close()

	m, err := NewChatModel(context.Background(), &Config{
		Model: "test-model", APIKey: "test-key", BaseURL: srv.URL,
	})
	require.NoError(t, err)

	msg, err := m.Generate(context.Background(), []*schema.Message{
		SystemMessage("你是助手"),
		UserMessage("你好"),
	})
	require.NoError(t, err)
	assert.Equal(t, schema.Assistant, msg.Role)
	assert.Equal(t, "你好，世界", msg.Content)
	require.NotNil(t, msg.ResponseMeta)
	require.NotNil(t, msg.ResponseMeta.Usage)
	assert.Equal(t, 15, msg.ResponseMeta.Usage.TotalTokens)
}

func TestStreamText_AggregatesChunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.True(t, req.Stream)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, c := range []string{"你", "好", "世界"} {
			chunk := map[string]any{
				"choices": []map[string]any{{"delta": map[string]any{"content": c}}},
			}
			b, _ := json.Marshal(chunk)
			_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	m, err := NewChatModel(context.Background(), &Config{APIKey: "k", BaseURL: srv.URL})
	require.NoError(t, err)

	var sb strings.Builder
	err = m.StreamText(context.Background(), []*schema.Message{UserMessage("hi")}, func(chunk string) error {
		sb.WriteString(chunk)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "你好世界", sb.String())
}

func TestUserMultiModalMessage(t *testing.T) {
	msg := UserMultiModalMessage("看图", "https://example.com/a.jpg")
	assert.Equal(t, schema.User, msg.Role)
	require.Len(t, msg.MultiContent, 2)
	assert.Equal(t, schema.ChatMessagePartTypeText, msg.MultiContent[0].Type)
	assert.Equal(t, schema.ChatMessagePartTypeImageURL, msg.MultiContent[1].Type)

	// 确认转换为 OpenAI 多模态分片数组.
	api := toAPIMessages([]*schema.Message{msg})
	require.Len(t, api, 1)
	parts, ok := api[0].Content.([]apiContentPart)
	require.True(t, ok, "multimodal content should be a parts array")
	assert.Len(t, parts, 2)
}

func TestGenerate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	m, err := NewChatModel(context.Background(), &Config{APIKey: "k", BaseURL: srv.URL})
	require.NoError(t, err)

	_, err = m.Generate(context.Background(), []*schema.Message{UserMessage("hi")})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}
