package main

import (
	"context"
	"fmt"
	"os"

	"github.com/huangyangke/go-aikit/agent/eino_plus/embedding"
	"github.com/huangyangke/go-aikit/config"
)

func main() {
	_, err := config.New("", config.WithEnvFile(".env"), config.WithOverrideEnv(true))
	if err != nil {
		fmt.Fprintf(os.Stderr, "load .env failed: %v\n", err)
		os.Exit(1)
	}

	apiKey := os.Getenv("SHANHAI_EMBEDDING_API_KEY")
	baseURL := os.Getenv("SHANHAI_EMBEDDING_BASE_URL")
	if apiKey == "" || baseURL == "" {
		fmt.Fprintln(os.Stderr, "SHANHAI_EMBEDDING_API_KEY or SHANHAI_EMBEDDING_BASE_URL not set, check .env")
		os.Exit(1)
	}

	emb, err := embedding.NewEmbedder(context.Background(), &embedding.Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create embedder failed: %v\n", err)
		os.Exit(1)
	}

	texts := []string{"你好世界", "hello world"}
	if len(os.Args) > 1 {
		texts = os.Args[1:]
	}

	vecs, err := emb.EmbedStrings(context.Background(), texts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "embed failed: %v\n", err)
		os.Exit(1)
	}

	for i, vec := range vecs {
		fmt.Printf("[%d] %q => dim=%d, head=%v\n", i, texts[i], len(vec), vec[:4])
	}
}
