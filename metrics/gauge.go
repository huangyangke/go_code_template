// Package metrics Prometheus 指标采集与管理.
package metrics

import prom "github.com/prometheus/client_golang/prometheus"

type (
	// GaugeVecOpts 仪表盘向量配置.
	GaugeVecOpts VectorOpts

	// GaugeVec 仪表盘向量接口.
	GaugeVec interface {
		Set(v float64, labels ...string)
		Inc(labels ...string)
		Add(v float64, labels ...string)
		Close() bool
	}

	promGaugeVec struct {
		gauge *prom.GaugeVec
	}
)

// NewGaugeVec 创建仪表盘向量.
// 参数：cfg - 仪表盘向量配置.
// 返回值：GaugeVec - 仪表盘向量实例, cfg 为 nil 时返回 nil.
func NewGaugeVec(cfg *GaugeVecOpts) GaugeVec {
	if cfg == nil {
		return nil
	}
	vec := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      cfg.Name,
		Help:      cfg.Help,
	}, cfg.Labels)
	safeRegister(vec)
	return &promGaugeVec{gauge: vec}
}

// Inc 将指定标签的仪表盘值加 1.
// 参数：labels - 标签值列表.
// 返回值：无.
func (gv *promGaugeVec) Inc(labels ...string) {
	gv.gauge.WithLabelValues(labels...).Inc()
}

// Add 将指定标签的仪表盘值加 v.
// 参数：v - 增量值, labels - 标签值列表.
// 返回值：无.
func (gv *promGaugeVec) Add(v float64, labels ...string) {
	gv.gauge.WithLabelValues(labels...).Add(v)
}

// Set 设置指定标签的仪表盘值.
// 参数：v - 目标值, labels - 标签值列表.
// 返回值：无.
func (gv *promGaugeVec) Set(v float64, labels ...string) {
	gv.gauge.WithLabelValues(labels...).Set(v)
}

// Close 注销仪表盘向量.
// 参数：无.
// 返回值：bool - 是否成功注销.
func (gv *promGaugeVec) Close() bool {
	return prom.Unregister(gv.gauge)
}

// RegisterGaugeFunc 注册按需计算的仪表盘函数.
// 参数：name - 指标名称, help - 帮助文本, constLabels - 常量标签, fn - 值计算函数.
// 返回值：func() - 注销函数，清理时必须调用以避免重复注册 panic.
func RegisterGaugeFunc(name, help string, constLabels prom.Labels, fn func() float64) func() {
	g := prom.NewGaugeFunc(prom.GaugeOpts{
		Name:        name,
		Help:        help,
		ConstLabels: constLabels,
	}, fn)
	safeRegister(g)
	return func() { prom.Unregister(g) }
}
