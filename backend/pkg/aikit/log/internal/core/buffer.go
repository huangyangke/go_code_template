// Package core 日志内部编码与缓冲区基础设施.
package core

import "strconv"

const _size = 1024

// NewBuffer 创建指定初始容量的 Buffer.
// 参数：size - 初始容量，0 时使用默认 1024.
// 返回值：Buffer 实例.
func NewBuffer(size int) *Buffer {
	if size == 0 {
		size = _size
	}
	return &Buffer{bs: make([]byte, 0, size)}
}

// Buffer 可复用的字节缓冲区.
type Buffer struct {
	bs   []byte
	pool Pool
}

// AppendByte 追加单个字节.
func (b *Buffer) AppendByte(v byte) {
	b.bs = append(b.bs, v)
}

// AppendString 追加字符串.
func (b *Buffer) AppendString(s string) {
	b.bs = append(b.bs, s...)
}

// AppendInt 追加 int64 的十进制字符串.
func (b *Buffer) AppendInt(i int64) {
	b.bs = strconv.AppendInt(b.bs, i, 10)
}

// AppendUint 追加 uint64 的十进制字符串.
func (b *Buffer) AppendUint(i uint64) {
	b.bs = strconv.AppendUint(b.bs, i, 10)
}

// AppendBool 追加布尔值的字符串.
func (b *Buffer) AppendBool(v bool) {
	b.bs = strconv.AppendBool(b.bs, v)
}

// AppendFloat 追加浮点数的字符串.
func (b *Buffer) AppendFloat(f float64, bitSize int) {
	b.bs = strconv.AppendFloat(b.bs, f, 'f', -1, bitSize)
}

// Len 返回缓冲区已用字节长度.
func (b *Buffer) Len() int { return len(b.bs) }

// Cap 返回缓冲区总容量.
func (b *Buffer) Cap() int { return cap(b.bs) }

// Bytes 返回缓冲区底字节切片.
func (b *Buffer) Bytes() []byte { return b.bs }

// String 返回缓冲区内容的字符串形式.
func (b *Buffer) String() string { return string(b.bs) }

// Reset 清空缓冲区已用内容.
func (b *Buffer) Reset() {
	b.bs = b.bs[:0]
}

// Write 写入字节切片到缓冲区.
func (b *Buffer) Write(bs []byte) (int, error) {
	b.bs = append(b.bs, bs...)
	return len(bs), nil
}

// TrimNewline 移除缓冲区末尾的换行符.
func (b *Buffer) TrimNewline() {
	if i := len(b.bs) - 1; i >= 0 {
		if b.bs[i] == '\n' {
			b.bs = b.bs[:i]
		}
	}
}

// Free 将缓冲区归还到对象池.
func (b *Buffer) Free() {
	b.pool.put(b)
}
