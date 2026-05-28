// Package metrics Prometheus 指标采集与管理.
package metrics

import prom "github.com/prometheus/client_golang/prometheus"

type (
	// CounterVecOpts 计数器向量配置.
	CounterVecOpts VectorOpts

	// CounterVec 计数器向量接口.
	CounterVec interface {
		Inc(labels ...string)
		Add(v float64, labels ...string)
		Close() bool
	}

	promCounterVec struct {
		counter *prom.CounterVec
	}
)

// NewCounterVec 创建计数器向量.
// 参数：cfg - 计数器向量配置.
// 返回值：CounterVec - 计数器向量实例, cfg 为 nil 时返回 nil.
func NewCounterVec(cfg *CounterVecOpts) CounterVec {
	if cfg == nil {
		return nil
	}
	vec := prom.NewCounterVec(prom.CounterOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      cfg.Name,
		Help:      cfg.Help,
	}, cfg.Labels)
	safeRegister(vec)
	return &promCounterVec{counter: vec}
}

// Inc 将指定标签的计数器加 1.
// 参数：labels - 标签值列表.
// 返回值：无.
func (cv *promCounterVec) Inc(labels ...string) {
	cv.counter.WithLabelValues(labels...).Inc()
}

// Add 将指定标签的计数器加 v.
// 参数：v - 增量值, labels - 标签值列表.
// 返回值：无.
func (cv *promCounterVec) Add(v float64, labels ...string) {
	cv.counter.WithLabelValues(labels...).Add(v)
}

// Close 注销计数器向量.
// 参数：无.
// 返回值：bool - 是否成功注销.
func (cv *promCounterVec) Close() bool {
	return prom.Unregister(cv.counter)
}
