// Command main 演示 chatmodel 的阻塞与流式调用.
//
// 运行：
//
//	export SHANHAI_LLM_API_KEY=sk-xxx
//	export SHANHAI_LLM_BASE_URL=https://linghub.shuziwenbo.cn/v1
//	go run ./agent/eino_plus/chatmodel/example
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino/schema"

	"github.com/huangyangke/go-aikit/agent/eino_plus/chatmodel"
)

func main() {
	ctx := context.Background()

	cm, err := chatmodel.NewChatModel(ctx, &chatmodel.Config{
		Model:   envOr("LLM_MODEL_NAME", "shanhai-max"),
		APIKey:  os.Getenv("SHANHAI_LLM_API_KEY"),
		BaseURL: envOr("SHANHAI_LLM_BASE_URL", "https://linghub.shuziwenbo.cn/v1"),
	})
	if err != nil {
		fmt.Println("init error:", err)
		return
	}

	messages := []*schema.Message{
		chatmodel.SystemMessage("你是一个专业的知识库问答助手。"),
		chatmodel.UserMessage("用一句话介绍向量数据库。"),
	}

	// 阻塞生成.
	fmt.Println("=== Generate ===")
	text, err := cm.GenerateText(ctx, messages)
	if err != nil {
		fmt.Println("generate error:", err)
		return
	}
	fmt.Println(text)

	// 流式生成.
	fmt.Println("\n=== Stream ===")
	err = cm.StreamText(ctx, messages, func(chunk string) error {
		fmt.Print(chunk)
		return nil
	})
	if err != nil {
		fmt.Println("\nstream error:", err)
		return
	}
	fmt.Println()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
