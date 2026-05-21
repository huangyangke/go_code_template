package main

import (
	"context"
	"fmt"
	"os"

	"github.com/example/go-template/pkg/aikit/agent/eino_plus/embedding"
	"github.com/example/go-template/pkg/aikit/agent/eino_plus/vectordb"
	"github.com/example/go-template/pkg/aikit/config"
)

func main() {
	_, err := config.New("", config.WithEnvFile(".env"), config.WithOverrideEnv(true))
	if err != nil {
		fmt.Fprintf(os.Stderr, "load .env failed: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// embedding 客户端
	emb, err := embedding.NewEmbedder(ctx, &embedding.Config{
		APIKey:  os.Getenv("SHANHAI_EMBEDDING_API_KEY"),
		BaseURL: os.Getenv("SHANHAI_EMBEDDING_BASE_URL"),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create embedder failed: %v\n", err)
		os.Exit(1)
	}

	// vectordb 客户端
	vdb, err := vectordb.New(vectordb.Config{
		URL:            os.Getenv("TENCENT_VECTORDB_URL"),
		Key:            os.Getenv("TENCENT_VECTORDB_KEY"),
		Username:       os.Getenv("TENCENT_VECTORDB_USERNAME"),
		DBName:         os.Getenv("TENCENT_VECTORDB_DB"),
		CollectionName: os.Getenv("TENCENT_VECTORDB_COLLECTION"),
		Dimensions:     1024,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create vectordb client failed: %v\n", err)
		os.Exit(1)
	}
	defer vdb.Close()

	// 确保 collection 存在
	if err := vdb.CreateCollection(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "create collection failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("collection ready")

	// 准备文档
	contents := []string{
		"人工智能是模拟人类智能的技术",
		"向量数据库用于存储和检索高维向量",
		"Go 语言以简洁和高并发著称",
	}

	// 批量 embed
	vecs, err := emb.EmbedStrings(ctx, contents)
	if err != nil {
		fmt.Fprintf(os.Stderr, "embed failed: %v\n", err)
		os.Exit(1)
	}

	// 构建文档
	docs := make([]vectordb.Doc, len(contents))
	for i, content := range contents {
		vec32 := make([]float32, len(vecs[i]))
		for j, v := range vecs[i] {
			vec32[j] = float32(v)
		}
		docs[i] = vectordb.Doc{
			Content:   content,
			Embedding: vec32,
			Name:      fmt.Sprintf("doc_%d", i),
		}
	}

	// upsert
	result, err := vdb.Upsert(ctx, docs, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "upsert failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("upsert ok: affected=%d\n", result.AffectedCount)

	// 搜索
	query := "什么是向量数据库"
	if len(os.Args) > 1 {
		query = os.Args[1]
	}

	queryVecs, err := emb.EmbedStrings(ctx, []string{query})
	if err != nil {
		fmt.Fprintf(os.Stderr, "embed query failed: %v\n", err)
		os.Exit(1)
	}
	queryVec32 := make([]float32, len(queryVecs[0]))
	for i, v := range queryVecs[0] {
		queryVec32[i] = float32(v)
	}

	hits, err := vdb.Search(ctx, queryVec32, 3, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "search failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nquery: %q\n", query)
	for i, h := range hits {
		fmt.Printf("[%d] score=%.4f  %s\n", i+1, h.Score, h.Content)
	}
}
