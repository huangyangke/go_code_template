package chatmodel

import (
	"context"
	"errors"
	"io"

	"github.com/cloudwego/eino/schema"
)

// HistoryMessage 简化的历史对话消息，便于业务层构造多轮对话.
type HistoryMessage struct {
	Role    string // "user" / "assistant" / "system"
	Content string
}

// ToMessage 转换为 eino schema.Message.
func (h HistoryMessage) ToMessage() *schema.Message {
	switch h.Role {
	case "assistant":
		return AssistantMessage(h.Content)
	case "system":
		return SystemMessage(h.Content)
	default:
		return UserMessage(h.Content)
	}
}

// HistoryToMessages 批量转换历史消息.
// 参数：history - 历史消息列表.
// 返回值：[]*schema.Message.
func HistoryToMessages(history []HistoryMessage) []*schema.Message {
	out := make([]*schema.Message, 0, len(history))
	for _, h := range history {
		out = append(out, h.ToMessage())
	}
	return out
}

// SystemMessage 构造 system 角色消息.
// 参数：content - 系统提示词.
// 返回值：*schema.Message.
func SystemMessage(content string) *schema.Message {
	return &schema.Message{Role: schema.System, Content: content}
}

// UserMessage 构造纯文本 user 消息.
// 参数：content - 用户文本.
// 返回值：*schema.Message.
func UserMessage(content string) *schema.Message {
	return &schema.Message{Role: schema.User, Content: content}
}

// AssistantMessage 构造 assistant 消息.
// 参数：content - 助手文本.
// 返回值：*schema.Message.
func AssistantMessage(content string) *schema.Message {
	return &schema.Message{Role: schema.Assistant, Content: content}
}

// UserMultiModalMessage 构造图文混合的 user 消息.
// 参数：text - 文本内容, imageURLs - 图片地址 (URL 或 Base64 Data URI).
// 返回值：*schema.Message.
func UserMultiModalMessage(text string, imageURLs ...string) *schema.Message {
	parts := make([]schema.ChatMessagePart, 0, len(imageURLs)+1)
	if text != "" {
		parts = append(parts, schema.ChatMessagePart{
			Type: schema.ChatMessagePartTypeText,
			Text: text,
		})
	}
	for _, url := range imageURLs {
		if url == "" {
			continue
		}
		parts = append(parts, schema.ChatMessagePart{
			Type:     schema.ChatMessagePartTypeImageURL,
			ImageURL: &schema.ChatMessageImageURL{URL: url},
		})
	}
	return &schema.Message{
		Role:         schema.User,
		Content:      text,
		MultiContent: parts,
	}
}

// GenerateText 便捷方法：阻塞生成并直接返回文本内容.
// 参数：ctx - 上下文, messages - 对话消息.
// 返回值：助手回复文本, err - 请求失败时的错误.
func (m *ChatModel) GenerateText(ctx context.Context, messages []*schema.Message) (string, error) {
	msg, err := m.Generate(ctx, messages)
	if err != nil {
		return "", err
	}
	return msg.Content, nil
}

// StreamText 便捷方法：流式生成，通过回调逐片返回文本内容.
// fn 返回 error 时提前终止并关闭流.
// 参数：ctx - 上下文, messages - 对话消息, fn - 分片回调.
// 返回值：err - 请求或回调过程中的错误.
func (m *ChatModel) StreamText(ctx context.Context, messages []*schema.Message, fn func(chunk string) error) error {
	sr, err := m.Stream(ctx, messages)
	if err != nil {
		return err
	}
	defer sr.Close()

	for {
		msg, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if msg.Content == "" {
			continue
		}
		if err := fn(msg.Content); err != nil {
			return err
		}
	}
}
