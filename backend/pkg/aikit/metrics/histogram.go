// Package metrics Prometheus 指标采集与管理.
package metrics

import prom "github.com/prometheus/client_golang/prometheus"

type (
	// HistogramVecOpts 直方图向量配置.
	HistogramVecOpts struct {
		Namespace string
		Subsystem string
		Name      string
		Help      string
		Labels    []string
		Buckets   []float64
	}

	// HistogramVec 直方图向量接口.
	HistogramVec interface {
		Observe(v float64, labels ...string)
		Close() bool
	}

	promHistogramVec struct {
		histogram *prom.HistogramVec
	}
)

// NewHistogramVec 创建直方图向量.
// 参数：cfg - 直方图向量配置.
// 返回值：HistogramVec - 直方图向量实例, cfg 为 nil 时返回 nil.
func NewHistogramVec(cfg *HistogramVecOpts) HistogramVec {
	if cfg == nil {
		return nil
	}
	vec := prom.NewHistogramVec(prom.HistogramOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      cfg.Name,
		Help:      cfg.Help,
		Buckets:   cfg.Buckets,
	}, cfg.Labels)
	safeRegister(vec)
	return &promHistogramVec{histogram: vec}
}

// Observe 记录指定标签的观测值.
// 参数：v - 观测值, labels - 标签值列表.
// 返回值：无.
func (hv *promHistogramVec) Observe(v float64, labels ...string) {
	hv.histogram.WithLabelValues(labels...).Observe(v)
}

// Close 注销直方图向量.
// 参数：无.
// 返回值：bool - 是否成功注销.
func (hv *promHistogramVec) Close() bool {
	return prom.Unregister(hv.histogram)
}
