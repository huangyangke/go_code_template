package core

// GetPool 从全局缓冲区池中获取一个 Buffer.
var (
	_pool   = NewPool(_size)
	GetPool = _pool.Get
)
