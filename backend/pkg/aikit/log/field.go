package log

import (
	"math"
	"time"

	"github.com/example/go-template/pkg/aikit/log/internal/core"
)

type D = core.Field

func KVString(key string, value string) D {
	return D{Key: key, Type: core.StringType, StringVal: value}
}

func KVInt(key string, value int) D {
	return D{Key: key, Type: core.IntType, Int64Val: int64(value)}
}

func KVInt64(key string, value int64) D {
	return D{Key: key, Type: core.Int64Type, Int64Val: value}
}

func KVUint(key string, value uint) D {
	return D{Key: key, Type: core.UintType, Int64Val: int64(value)}
}

func KVUint64(key string, value uint64) D {
	return D{Key: key, Type: core.Uint64Type, Int64Val: int64(value)}
}

func KVFloat32(key string, value float32) D {
	return D{Key: key, Type: core.Float32Type, Int64Val: int64(math.Float32bits(value))}
}

func KVFloat64(key string, value float64) D {
	return D{Key: key, Type: core.Float64Type, Int64Val: int64(math.Float64bits(value))}
}

func KVDuration(key string, value time.Duration) D {
	return D{Key: key, Type: core.DurationType, Int64Val: int64(value)}
}

func KV(key string, value interface{}) D {
	return D{Key: key, Value: value}
}
