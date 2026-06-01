package chatmodel

import "github.com/cloudwego/eino/schema"

// ---------- OpenAI Chat Completions API types ---------- .

type chatCompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []apiMessage   `json:"messages"`
	Stream         bool           `json:"stream"`
	Temperature    *float32       `json:"temperature,omitempty"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	TopP           *float32       `json:"top_p,omitempty"`
	EnableThinking *bool          `json:"enable_thinking,omitempty"`
	StreamOptions  *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// apiMessage 是 OpenAI 消息体；Content 既可为纯文本字符串，也可为多模态分片数组.
type apiMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type apiContentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *apiImageURL `json:"image_url,omitempty"`
}

type apiImageURL struct {
	URL string `json:"url"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *apiUsage `json:"usage,omitempty"`
}

type chatCompletionStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *apiUsage `json:"usage,omitempty"`
}

type apiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// toAPIMessages 将 eino 消息转换为 OpenAI 消息体，支持多模态 (文本 + 图片).
func toAPIMessages(input []*schema.Message) []apiMessage {
	out := make([]apiMessage, 0, len(input))
	for _, msg := range input {
		if msg == nil {
			continue
		}
		out = append(out, apiMessage{
			Role:    string(msg.Role),
			Content: messageContent(msg),
		})
	}
	return out
}

// messageContent 优先使用多模态分片，否则回退到纯文本 Content.
func messageContent(msg *schema.Message) any {
	parts := msg.MultiContent
	if len(parts) == 0 {
		return msg.Content
	}

	apiParts := make([]apiContentPart, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case schema.ChatMessagePartTypeText:
			apiParts = append(apiParts, apiContentPart{Type: "text", Text: p.Text})
		case schema.ChatMessagePartTypeImageURL:
			if p.ImageURL != nil {
				url := p.ImageURL.URL
				if url == "" {
					url = p.ImageURL.URI
				}
				apiParts = append(apiParts, apiContentPart{
					Type:     "image_url",
					ImageURL: &apiImageURL{URL: url},
				})
			}
		}
	}
	if len(apiParts) == 0 {
		return msg.Content
	}
	return apiParts
}
