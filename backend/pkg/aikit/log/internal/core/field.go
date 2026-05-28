package core

import (
	"math"
	"time"
)

// FieldType 字段值类型.
type FieldType int32

const (
	UnknownType FieldType = iota
	// StringType 字符串类型.
	StringType
	// IntType int 类型.
	IntType
	// Int64Type int64 类型.
	Int64Type
	// UintType uint 类型.
	UintType
	// Uint64Type uint64 类型.
	Uint64Type
	// Float32Type float32 类型.
	Float32Type
	// Float64Type float64 类型.
	Float64Type
	// DurationType time.Duration 类型.
	DurationType
)

// Field 结构化日志字段.
type Field struct {
	Key       string
	Value     interface{}
	Type      FieldType
	StringVal string
	Int64Val  int64
}

func (f Field) AddTo(enc ObjectEncoder) {
	if f.Type == UnknownType {
		f.assertAddTo(enc)
		return
	}
	switch f.Type {
	case StringType:
		enc.AddString(f.Key, f.StringVal)
	case IntType:
		enc.AddInt(f.Key, int(f.Int64Val))
	case Int64Type:
		enc.AddInt64(f.Key, f.Int64Val)
	case UintType:
		enc.AddUint(f.Key, uint(f.Int64Val))
	case Uint64Type:
		enc.AddUint64(f.Key, uint64(f.Int64Val))
	case Float32Type:
		enc.AddFloat32(f.Key, math.Float32frombits(uint32(f.Int64Val)))
	case Float64Type:
		enc.AddFloat64(f.Key, math.Float64frombits(uint64(f.Int64Val)))
	case DurationType:
		enc.AddDuration(f.Key, time.Duration(f.Int64Val))
	}
}

func (f Field) assertAddTo(enc ObjectEncoder) {
	switch val := f.Value.(type) {
	case bool:
		enc.AddBool(f.Key, val)
	case complex128:
		enc.AddComplex128(f.Key, val)
	case complex64:
		enc.AddComplex64(f.Key, val)
	case float64:
		enc.AddFloat64(f.Key, val)
	case float32:
		enc.AddFloat32(f.Key, val)
	case int:
		enc.AddInt(f.Key, val)
	case int64:
		enc.AddInt64(f.Key, val)
	case int32:
		enc.AddInt32(f.Key, val)
	case int16:
		enc.AddInt16(f.Key, val)
	case int8:
		enc.AddInt8(f.Key, val)
	case string:
		enc.AddString(f.Key, val)
	case uint:
		enc.AddUint(f.Key, val)
	case uint64:
		enc.AddUint64(f.Key, val)
	case uint32:
		enc.AddUint32(f.Key, val)
	case uint16:
		enc.AddUint16(f.Key, val)
	case uint8:
		enc.AddUint8(f.Key, val)
	case []byte:
		enc.AddByteString(f.Key, val)
	case uintptr:
		enc.AddUintptr(f.Key, val)
	case time.Time:
		enc.AddTime(f.Key, val)
	case time.Duration:
		enc.AddDuration(f.Key, val)
	case error:
		enc.AddString(f.Key, val.Error())
	case interface{ String() string }:
		enc.AddString(f.Key, val.String())
	default:
		err := enc.AddReflected(f.Key, val)
		if err != nil {
			enc.AddString(f.Key+"Error", err.Error())
		}
	}
}
