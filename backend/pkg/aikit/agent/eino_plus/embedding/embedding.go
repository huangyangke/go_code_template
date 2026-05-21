package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/embedding"
)

var _ embedding.Embedder = (*Embedder)(nil)

type Config struct {
	// Model 模型名称 (如 "shanhai-embedding", "text-embedding-v4")
	Model string `yaml:"model" json:"model"`
	// APIKey 认证密钥
	APIKey string `yaml:"api_key" json:"api_key"`
	// BaseURL API 地址 (如 "https://linghub.shuziwenbo.cn/v1")
	BaseURL string `yaml:"base_url" json:"base_url"`
	// Dimensions 向量维度
	Dimensions int `yaml:"dimensions" json:"dimensions"`
	// Timeout HTTP 超时
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
	// BatchSize 批量请求大小
	BatchSize int `yaml:"batch_size" json:"batch_size"`
	// HTTPClient 可选自定义 HTTP 客户端
	HTTPClient *http.Client `yaml:"-" json:"-"`
}

func (c *Config) Fix() {
	if c.Model == "" {
		c.Model = "shanhai-embedding"
	}
	if c.Dimensions <= 0 {
		c.Dimensions = 1024
	}
	if c.Timeout <= 0 {
		c.Timeout = 60 * time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 25
	}
}

func (c *Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("embedding: api_key is required")
	}
	if c.BaseURL == "" {
		return fmt.Errorf("embedding: base_url is required")
	}
	return nil
}

type Embedder struct {
	cfg    Config
	client *http.Client
}

func NewEmbedder(_ context.Context, cfg *Config) (*Embedder, error) {
	cfg.Fix()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}

	return &Embedder{cfg: *cfg, client: httpClient}, nil
}

// EmbedStrings 实现 eino embedding.Embedder 接口。
// 支持批量处理，自动按 BatchSize 分批请求。
func (e *Embedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	result := make([][]float64, 0, len(texts))

	for i := 0; i < len(texts); i += e.cfg.BatchSize {
		end := i + e.cfg.BatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		embeddings, err := e.embedBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("embedding: batch [%d:%d]: %w", i, end, err)
		}
		result = append(result, embeddings...)
	}

	return result, nil
}

func (e *Embedder) embedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	inputs := make([]map[string]string, len(texts))
	for i, t := range texts {
		inputs[i] = map[string]string{"text": t}
	}
	return e.doRequest(ctx, apiRequest{
		Model:      e.cfg.Model,
		Input:      inputs,
		Dimensions: e.cfg.Dimensions,
	})
}

func (e *Embedder) doRequest(ctx context.Context, reqBody apiRequest) ([][]float64, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding: API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("embedding: unmarshal response: %w", err)
	}

	embeddings := make([][]float64, len(apiResp.Data))
	for i, item := range apiResp.Data {
		embeddings[i] = item.Embedding
	}
	return embeddings, nil
}

// ---------- API types ----------

type apiRequest struct {
	Model      string              `json:"model"`
	Input      []map[string]string `json:"input"`
	Dimensions int                 `json:"dimensions"`
}

type apiResponse struct {
	Data  []apiEmbeddingData `json:"data"`
	Usage *apiUsage          `json:"usage,omitempty"`
}

type apiEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type apiUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
