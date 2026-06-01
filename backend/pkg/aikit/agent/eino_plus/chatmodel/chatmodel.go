// Package chatmodel 基于 OpenAI 兼容 Chat Completions API 的对话模型客户端.
//
// 兼容所有 OpenAI Chat Completions API 格式的服务（Shanhai、DashScope、OpenAI 等），
// 实现 eino 框架的 model.BaseChatModel 接口，同时提供面向流式 SSE 场景的便捷方法.
package chatmodel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// 编译期确认实现 eino BaseChatModel 接口.
var _ model.BaseChatModel = (*ChatModel)(nil)

// Config ChatModel 配置.
type Config struct {
	// Model 模型名称 (如 "shanhai-max", "gpt-4o")
	Model string `yaml:"model" json:"model"`
	// APIKey 认证密钥
	APIKey string `yaml:"api_key" json:"api_key"`
	// BaseURL API 地址 (如 "https://linghub.shuziwenbo.cn/v1"，不含 /chat/completions)
	BaseURL string `yaml:"base_url" json:"base_url"`
	// Temperature 采样温度，使用指针区分"未设置"与"显式 0"
	Temperature *float32 `yaml:"temperature" json:"temperature"`
	// MaxTokens 最大生成 token 数，0 表示不传该参数
	MaxTokens int `yaml:"max_tokens" json:"max_tokens"`
	// TopP 核采样，使用指针区分"未设置"与"显式 0"
	TopP *float32 `yaml:"top_p" json:"top_p"`
	// EnableThinking 部分国产模型支持的思考开关，nil 表示不传
	EnableThinking *bool `yaml:"enable_thinking" json:"enable_thinking"`
	// Timeout HTTP 超时
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
	// HTTPClient 可选自定义 HTTP 客户端
	HTTPClient *http.Client `yaml:"-" json:"-"`
}

// Fix 填充配置默认值.
func (c *Config) Fix() {
	if c.Model == "" {
		c.Model = "shanhai-max"
	}
	if c.Timeout <= 0 {
		// 大模型生成耗时较长，默认放宽到 5 分钟.
		c.Timeout = 5 * time.Minute
	}
}

// Validate 校验配置必要字段.
// 返回值：err - 校验失败时的错误.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("chatmodel: api_key is required")
	}
	if c.BaseURL == "" {
		return fmt.Errorf("chatmodel: base_url is required")
	}
	return nil
}

// ChatModel 对话模型客户端，实现 eino model.BaseChatModel 接口.
type ChatModel struct {
	cfg    Config
	client *http.Client
}

// NewChatModel 创建 ChatModel 实例.
// 参数：ctx - 上下文 (当前未使用), cfg - 配置.
// 返回值：ChatModel 实例, err - 配置校验失败时的错误.
func NewChatModel(_ context.Context, cfg *Config) (*ChatModel, error) {
	cfg.Fix()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}

	return &ChatModel{cfg: *cfg, client: httpClient}, nil
}

// Generate 实现 model.BaseChatModel，阻塞直至模型返回完整响应.
// 参数：ctx - 上下文, input - 对话消息列表, opts - eino 模型选项.
// 返回值：assistant 消息, err - 请求失败时的错误.
func (m *ChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	reqBody := m.buildRequest(input, opts, false)

	respBody, err := m.doRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	var apiResp chatCompletionResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("chatmodel: unmarshal response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("chatmodel: empty choices in response")
	}

	choice := apiResp.Choices[0]
	msg := &schema.Message{
		Role:             schema.Assistant,
		Content:          choice.Message.Content,
		ReasoningContent: choice.Message.ReasoningContent,
	}
	if apiResp.Usage != nil {
		msg.ResponseMeta = &schema.ResponseMeta{
			FinishReason: choice.FinishReason,
			Usage: &schema.TokenUsage{
				PromptTokens:     apiResp.Usage.PromptTokens,
				CompletionTokens: apiResp.Usage.CompletionTokens,
				TotalTokens:      apiResp.Usage.TotalTokens,
			},
		}
	}
	return msg, nil
}

// Stream 实现 model.BaseChatModel，以 SSE 流式返回消息分片.
// 调用方负责 Close 返回的 StreamReader.
// 参数：ctx - 上下文, input - 对话消息列表, opts - eino 模型选项.
// 返回值：消息分片流, err - 建立请求失败时的错误.
func (m *ChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reqBody := m.buildRequest(input, opts, true)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("chatmodel: API returned %d: %s", resp.StatusCode, string(errBody))
	}

	sr, sw := schema.Pipe[*schema.Message](16)
	go m.pumpSSE(resp, sw)
	return sr, nil
}

// pumpSSE 解析 SSE 流并将每个 delta 作为消息分片写入 writer.
func (m *ChatModel) pumpSSE(resp *http.Response, sw *schema.StreamWriter[*schema.Message]) {
	defer func() { _ = resp.Body.Close() }()
	defer sw.Close()

	scanner := bufio.NewScanner(resp.Body)
	// 单行 SSE 可能较长（含 reasoning），放宽缓冲上限到 1MB.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return
		}

		var chunk chatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// 跳过无法解析的心跳/注释行.
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content == "" && delta.ReasoningContent == "" {
			continue
		}
		msg := &schema.Message{
			Role:             schema.Assistant,
			Content:          delta.Content,
			ReasoningContent: delta.ReasoningContent,
		}
		if sw.Send(msg, nil) {
			// 接收端已关闭，停止拉取.
			return
		}
	}
	if err := scanner.Err(); err != nil {
		sw.Send(nil, fmt.Errorf("chatmodel: read stream: %w", err))
	}
}

// buildRequest 将 eino 消息与选项转换为 OpenAI Chat Completions 请求体.
func (m *ChatModel) buildRequest(input []*schema.Message, opts []model.Option, stream bool) *chatCompletionRequest {
	// 复制基础配置，再叠加 per-call 选项.
	o := model.GetCommonOptions(&model.Options{
		Temperature: m.cfg.Temperature,
		MaxTokens:   ptrInt(m.cfg.MaxTokens),
		TopP:        m.cfg.TopP,
		Model:       &m.cfg.Model,
	}, opts...)

	req := &chatCompletionRequest{
		Model:    deref(o.Model, m.cfg.Model),
		Messages: toAPIMessages(input),
		Stream:   stream,
	}
	if o.Temperature != nil {
		req.Temperature = o.Temperature
	}
	if o.MaxTokens != nil && *o.MaxTokens > 0 {
		req.MaxTokens = *o.MaxTokens
	}
	if o.TopP != nil {
		req.TopP = o.TopP
	}
	if m.cfg.EnableThinking != nil {
		req.EnableThinking = m.cfg.EnableThinking
	}
	if stream {
		// 请求流式 usage 统计（OpenAI 兼容扩展，不支持的服务会忽略）.
		req.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	return req
}

// doRequest 发送非流式请求并返回原始响应体.
func (m *ChatModel) doRequest(ctx context.Context, reqBody *chatCompletionRequest) ([]byte, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chatmodel: API returned %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func ptrInt(i int) *int {
	if i <= 0 {
		return nil
	}
	return &i
}

func deref(s *string, def string) string {
	if s != nil && *s != "" {
		return *s
	}
	return def
}
