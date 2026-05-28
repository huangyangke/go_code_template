package vectordb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanContent(t *testing.T) {
	assert.Equal(t, "hello�world", cleanContent("hello\x00world"))
	assert.Equal(t, "normal", cleanContent("normal"))
}

func TestComputeDocID_NoPartition(t *testing.T) {
	// 与 Python 对齐: md5("你好".encode()).hexdigest()
	id := computeDocID("你好", nil, nil)
	assert.Equal(t, "7eca689f0d3389d9dea66ae112e5cfd7", id)
}

func TestComputeDocID_WithPartition(t *testing.T) {
	meta := map[string]any{"is_qa": 1, "field": "universal"}
	id := computeDocID("你好", meta, []string{"is_qa"})
	// Python: json.dumps(["你好", {"is_qa": 1}], ensure_ascii=False) = '["你好", {"is_qa": 1}]'
	// md5 = e50a9b611c7cdf8b7fc0f9f9905991ee
	assert.Equal(t, "e50a9b611c7cdf8b7fc0f9f9905991ee", id)

	// 同样 meta 同样 content 应该得到相同 id
	id2 := computeDocID("你好", meta, []string{"is_qa"})
	assert.Equal(t, id, id2)
}

func TestComputeDocID_PartitionFieldMissing(t *testing.T) {
	meta := map[string]any{"other": "val"}
	// partition_fields=["is_qa"] but meta has no "is_qa" → extra is empty → same as no partition
	id := computeDocID("你好", meta, []string{"is_qa"})
	assert.Equal(t, "7eca689f0d3389d9dea66ae112e5cfd7", id)
}

func TestBuildFilterExpr(t *testing.T) {
	tests := []struct {
		name    string
		filters map[string]any
		want    string
	}{
		{"nil", nil, ""},
		{"empty", map[string]any{}, ""},
		{"string", map[string]any{"name": "test"}, `meta_data.name = "test"`},
		{"int", map[string]any{"is_qa": 1}, `meta_data.is_qa = 1`},
		{"nil value skipped", map[string]any{"x": nil}, ""},
		{"list string", map[string]any{"tags": []string{"a", "b"}}, `meta_data.tags in ("a", "b")`},
		{"list any", map[string]any{"ids": []any{"x", 1}}, `meta_data.ids in ("x", 1)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildFilterExpr(tt.filters)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfigFix(t *testing.T) {
	cfg := Config{URL: "http://localhost", Key: "k", Dimensions: 1024}
	cfg.Fix()
	assert.Equal(t, "root", cfg.Username)
	assert.Equal(t, SearchTypeVector, cfg.SearchType)
	assert.Equal(t, MetricIP, cfg.Metric)
	assert.Equal(t, 30, cfg.Timeout)
	assert.Equal(t, 0.7, cfg.HybridVectorWeight)
}

func TestConfigValidate(t *testing.T) {
	cfg := Config{}
	assert.Error(t, cfg.Validate())

	cfg.URL = "http://localhost"
	assert.Error(t, cfg.Validate())

	cfg.Key = "key"
	assert.Error(t, cfg.Validate())

	cfg.DBName = "mydb"
	assert.Error(t, cfg.Validate())

	cfg.CollectionName = "mycoll"
	assert.Error(t, cfg.Validate())

	cfg.Dimensions = 1024
	assert.NoError(t, cfg.Validate())
}
