package core

var (
	_pool  = NewPool(_size)
	GetPool = _pool.Get
)
