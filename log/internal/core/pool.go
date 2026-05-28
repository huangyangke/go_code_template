package core

import "sync"

// Pool Buffer 对象池.
type Pool struct {
	p *sync.Pool
}

// NewPool 创建 Buffer 对象池.
// 参数：size - 池中 Buffer 的初始容量.
// 返回值：Pool 实例.
func NewPool(size int) Pool {
	if size == 0 {
		size = _size
	}
	return Pool{p: &sync.Pool{
		New: func() interface{} {
			return &Buffer{bs: make([]byte, 0, size)}
		},
	}}
}

func (p Pool) Get() *Buffer {
	buf := p.p.Get().(*Buffer)
	buf.Reset()
	buf.pool = p
	return buf
}

func (p Pool) put(buf *Buffer) {
	p.p.Put(buf)
}
