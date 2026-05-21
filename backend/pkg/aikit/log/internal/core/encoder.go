package core

import (
	"time"
)

const DefaultLineEnding = "\n"

type ObjectEncoder interface {
	AddArray(key string, marshaler ArrayMarshaler) error
	AddObject(key string, marshaler ObjectMarshaler) error

	AddBinary(key string, value []byte)
	AddByteString(key string, value []byte)
	AddBool(key string, value bool)
	AddComplex128(key string, value complex128)
	AddComplex64(key string, value complex64)
	AddDuration(key string, value time.Duration)
	AddFloat64(key string, value float64)
	AddFloat32(key string, value float32)
	AddInt(key string, value int)
	AddInt64(key string, value int64)
	AddInt32(key string, value int32)
	AddInt16(key string, value int16)
	AddInt8(key string, value int8)
	AddString(key, value string)
	AddTime(key string, value time.Time)
	AddUint(key string, value uint)
	AddUint64(key string, value uint64)
	AddUint32(key string, value uint32)
	AddUint16(key string, value uint16)
	AddUint8(key string, value uint8)
	AddUintptr(key string, value uintptr)

	AddReflected(key string, value interface{}) error
	OpenNamespace(key string)
}

type ObjectMarshaler interface {
	MarshalLogObject(ObjectEncoder) error
}

type ObjectMarshalerFunc func(ObjectEncoder) error

func (f ObjectMarshalerFunc) MarshalLogObject(enc ObjectEncoder) error {
	return f(enc)
}

type ArrayMarshaler interface {
	MarshalLogArray(ArrayEncoder) error
}

type ArrayMarshalerFunc func(ArrayEncoder) error

func (f ArrayMarshalerFunc) MarshalLogArray(enc ArrayEncoder) error {
	return f(enc)
}

type ArrayEncoder interface {
	PrimitiveArrayEncoder

	AppendDuration(time.Duration)
	AppendTime(time.Time)

	AppendArray(ArrayMarshaler) error
	AppendObject(ObjectMarshaler) error
	AppendReflected(value interface{}) error
}

type PrimitiveArrayEncoder interface {
	AppendBool(bool)
	AppendByteString([]byte)
	AppendComplex128(complex128)
	AppendComplex64(complex64)
	AppendFloat64(float64)
	AppendFloat32(float32)
	AppendInt(int)
	AppendInt64(int64)
	AppendInt32(int32)
	AppendInt16(int16)
	AppendInt8(int8)
	AppendString(string)
	AppendUint(uint)
	AppendUint64(uint64)
	AppendUint32(uint32)
	AppendUint16(uint16)
	AppendUint8(uint8)
	AppendUintptr(uintptr)
}

type EncoderConfig struct {
	EncodeTime     TimeEncoder
	EncodeDuration DurationEncoder
}

type Encoder interface {
	ObjectEncoder
	Clone() Encoder
	Encode(*Buffer, ...Field) error
}

type TimeEncoder func(time.Time, PrimitiveArrayEncoder)

type DurationEncoder func(time.Duration, PrimitiveArrayEncoder)

func EpochTimeEncoder(t time.Time, enc PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05.999999"))
}

func TimeEncoderOfLayout(layout string) TimeEncoder {
	return func(t time.Time, enc PrimitiveArrayEncoder) {
		encodeTimeLayout(t, layout, enc)
	}
}

func encodeTimeLayout(t time.Time, layout string, enc PrimitiveArrayEncoder) {
	type appendTimeEncoder interface {
		AppendTimeLayout(time.Time, string)
	}
	if enc, ok := enc.(appendTimeEncoder); ok {
		enc.AppendTimeLayout(t, layout)
		return
	}
	enc.AppendString(t.Format(layout))
}

func SecondsDurationEncoder(d time.Duration, enc PrimitiveArrayEncoder) {
	enc.AppendFloat64(float64(d) / float64(time.Second))
}
