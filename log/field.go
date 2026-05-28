package log

import (
	"math"
	"time"

	"github.com/huangyangke/go-aikit/log/internal/core"
)

// D 结构化日志字段类型，别名引用 core.Field.
type D = core.Field

// KVString 创建字符串类型的日志字段.
// 参数：key - 字段名, value - 字段值.
// 返回值：日志字段 D.
func KVString(key string, value string) D {
	return D{Key: key, Type: core.StringType, StringVal: value}
}

// KVInt 创建 int 类型的日志字段.
// 参数：key - 字段名, value - 字段值.
// 返回值：日志字段 D.
func KVInt(key string, value int) D {
	return D{Key: key, Type: core.IntType, Int64Val: int64(value)}
}

// KVInt64 创建 int64 类型的日志字段.
// 参数：key - 字段名, value - 字段值.
// 返回值：日志字段 D.
func KVInt64(key string, value int64) D {
	return D{Key: key, Type: core.Int64Type, Int64Val: value}
}

// KVUint 创建 uint 类型的日志字段.
// 参数：key - 字段名, value - 字段值.
// 返回值：日志字段 D.
func KVUint(key string, value uint) D {
	return D{Key: key, Type: core.UintType, Int64Val: int64(value)}
}

// KVUint64 创建 uint64 类型的日志字段.
// 参数：key - 字段名, value - 字段值.
// 返回值：日志字段 D.
func KVUint64(key string, value uint64) D {
	return D{Key: key, Type: core.Uint64Type, Int64Val: int64(value)}
}

// KVFloat32 创建 float32 类型的日志字段.
// 参数：key - 字段名, value - 字段值.
// 返回值：日志字段 D.
func KVFloat32(key string, value float32) D {
	return D{Key: key, Type: core.Float32Type, Int64Val: int64(math.Float32bits(value))}
}

// KVFloat64 创建 float64 类型的日志字段.
// 参数：key - 字段名, value - 字段值.
// 返回值：日志字段 D.
func KVFloat64(key string, value float64) D {
	return D{Key: key, Type: core.Float64Type, Int64Val: int64(math.Float64bits(value))}
}

// KVDuration 创建 Duration 类型的日志字段.
// 参数：key - 字段名, value - 字段值.
// 返回值：日志字段 D.
func KVDuration(key string, value time.Duration) D {
	return D{Key: key, Type: core.DurationType, Int64Val: int64(value)}
}

// KV 创建任意类型的日志字段，通过反射编码.
// 参数：key - 字段名, value - 字段值.
// 返回值：日志字段 D.
func KV(key string, value interface{}) D {
	return D{Key: key, Value: value}
}
