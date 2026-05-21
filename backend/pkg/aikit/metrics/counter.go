package metrics

import prom "github.com/prometheus/client_golang/prometheus"

type (
	CounterVecOpts VectorOpts

	CounterVec interface {
		Inc(labels ...string)
		Add(v float64, labels ...string)
		Close() bool
	}

	promCounterVec struct {
		counter *prom.CounterVec
	}
)

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
	prom.MustRegister(vec)
	return &promCounterVec{counter: vec}
}

func (cv *promCounterVec) Inc(labels ...string) {
	cv.counter.WithLabelValues(labels...).Inc()
}

func (cv *promCounterVec) Add(v float64, labels ...string) {
	cv.counter.WithLabelValues(labels...).Add(v)
}

func (cv *promCounterVec) Close() bool {
	return prom.Unregister(cv.counter)
}
