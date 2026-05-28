// Package metrics Prometheus 指标采集与管理.
package metrics

import (
	"errors"

	prom "github.com/prometheus/client_golang/prometheus"
)

var registry []func()

// Register 注册指标初始化函数，在 Enable 时统一调用.
// 参数：fn - 初始化函数，通常在 init() 中调用.
// 返回值：无.
func Register(fn func()) {
	registry = append(registry, fn)
}

// Enable 执行所有已注册的初始化函数，仅在启动且 Prometheus 采集开启时调用一次.
// 参数：无.
// 返回值：无.
func Enable() {
	for _, fn := range registry {
		fn()
	}
}

// VectorOpts 指标向量的通用配置.
type VectorOpts struct {
	Namespace string
	Subsystem string
	Name      string
	Help      string
	Labels    []string
}

// safeRegister 注册 collector，若已注册则复用.
// 替代 prom.MustRegister，避免二次初始化 panic.
func safeRegister(c prom.Collector) {
	if err := prom.Register(c); err != nil {
		var are prom.AlreadyRegisteredError
		if errors.As(err, &are) {
			// 已注册，忽略 — collector 指针相等保证后续 Inc/Observe 操作命中同一底层 metric
			return
		}
		// 非 AlreadyRegisteredError（如 collector 不合法），仍应 panic
		panic(err)
	}
}
