package log

import (
	"math"
	"testing"
	"time"

	"github.com/example/go-template/pkg/aikit/log/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestKVString(t *testing.T) {
	d := KVString("name", "alice")
	assert.Equal(t, "name", d.Key)
	assert.Equal(t, core.StringType, d.Type)
	assert.Equal(t, "alice", d.StringVal)
}

func TestKVInt(t *testing.T) {
	d := KVInt("count", 42)
	assert.Equal(t, "count", d.Key)
	assert.Equal(t, core.IntType, d.Type)
	assert.Equal(t, int64(42), d.Int64Val)
}

func TestKVInt64(t *testing.T) {
	d := KVInt64("big", int64(1<<50))
	assert.Equal(t, "big", d.Key)
	assert.Equal(t, core.Int64Type, d.Type)
	assert.Equal(t, int64(1<<50), d.Int64Val)
}

func TestKVUint(t *testing.T) {
	d := KVUint("pos", 100)
	assert.Equal(t, core.UintType, d.Type)
	assert.Equal(t, int64(100), d.Int64Val)
}

func TestKVUint64(t *testing.T) {
	d := KVUint64("big_pos", uint64(1<<60))
	assert.Equal(t, core.Uint64Type, d.Type)
	assert.Equal(t, int64(1<<60), d.Int64Val)
}

func TestKVFloat32(t *testing.T) {
	d := KVFloat32("ratio", 3.14)
	assert.Equal(t, core.Float32Type, d.Type)
	assert.Equal(t, int64(math.Float32bits(3.14)), d.Int64Val)
}

func TestKVFloat64(t *testing.T) {
	d := KVFloat64("precise", 2.718281828)
	assert.Equal(t, core.Float64Type, d.Type)
	assert.Equal(t, int64(math.Float64bits(2.718281828)), d.Int64Val)
}

func TestKVDuration(t *testing.T) {
	d := KVDuration("elapsed", 5*time.Second)
	assert.Equal(t, core.DurationType, d.Type)
	assert.Equal(t, int64(5*time.Second), d.Int64Val)
}

func TestKV(t *testing.T) {
	d := KV("data", map[string]int{"a": 1})
	assert.Equal(t, "data", d.Key)
	assert.Equal(t, map[string]int{"a": 1}, d.Value)
}
