package vectordb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeDocID_CompatPython_NoPart(t *testing.T) {
	assert.Equal(t, "5eb63bbbe01eeed093cb22bb8f5acdc3", computeDocID("hello world", nil, nil))
}

func TestComputeDocID_CompatPython_WithPartition(t *testing.T) {
	// Python: json.dumps(["hello world", {"file_id": "abc-123"}], ensure_ascii=False)
	// => '["hello world", {"file_id": "abc-123"}]'
	meta := map[string]any{"file_id": "abc-123", "chunk_id": 0}
	assert.Equal(t, "45553547ccca084a53851cdbe7f5a891", computeDocID("hello world", meta, []string{"file_id"}))
}

func TestComputeDocID_CompatPython_Chinese(t *testing.T) {
	// Python: json.dumps(["你好世界", {"name": "测试"}], ensure_ascii=False)
	// => '["你好世界", {"name": "测试"}]'
	meta := map[string]any{"name": "测试"}
	assert.Equal(t, "e1e907fed649566316bb69c1fd4fbe72", computeDocID("你好世界", meta, []string{"name"}))
}
