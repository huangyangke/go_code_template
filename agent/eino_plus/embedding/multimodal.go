package embedding

import (
	"context"
	"fmt"
)

// MultiModalInput 多模态输入，与 Python shanhai_embedding.py 的 dict 输入对齐.
type MultiModalInput struct {
	Text  string // 文本内容
	Image string // 图片 (URL / Base64 Data URI / 本地路径不支持，Go 端只支持 URL 和 Base64)
}

// EmbedMultiModal 支持多模态输入的 embedding 请求.
// 对应 Python 端 shanhai_embedding.get_embedding({"text": "...", "image": "..."}) 的行为.
// 参数：ctx - 上下文, inputs - 多模态输入列表.
// 返回值：嵌入向量列表, err - 请求失败时的错误.
func (e *Embedder) EmbedMultiModal(ctx context.Context, inputs []MultiModalInput) ([][]float64, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	result := make([][]float64, 0, len(inputs))

	for i := 0; i < len(inputs); i += e.cfg.BatchSize {
		end := i + e.cfg.BatchSize
		if end > len(inputs) {
			end = len(inputs)
		}
		batch := inputs[i:end]

		apiInputs := make([]map[string]string, len(batch))
		for j, inp := range batch {
			m := make(map[string]string)
			if inp.Text != "" {
				m["text"] = inp.Text
			}
			if inp.Image != "" {
				m["image"] = inp.Image
			}
			if len(m) == 0 {
				return nil, fmt.Errorf("embedding: input[%d] has neither text nor image", i+j)
			}
			apiInputs[j] = m
		}

		embeddings, err := e.embedRawBatch(ctx, apiInputs)
		if err != nil {
			return nil, fmt.Errorf("embedding: multimodal batch [%d:%d]: %w", i, end, err)
		}
		result = append(result, embeddings...)
	}

	return result, nil
}

func (e *Embedder) embedRawBatch(ctx context.Context, inputs []map[string]string) ([][]float64, error) {
	reqBody := apiRequest{
		Model:      e.cfg.Model,
		Input:      inputs,
		Dimensions: e.cfg.Dimensions,
	}
	return e.doRequest(ctx, reqBody)
}
