package metrics

import prom "github.com/prometheus/client_golang/prometheus"

type (
	HistogramVecOpts struct {
		Namespace string
		Subsystem string
		Name      string
		Help      string
		Labels    []string
		Buckets   []float64
	}

	HistogramVec interface {
		Observe(v float64, labels ...string)
		Close() bool
	}

	promHistogramVec struct {
		histogram *prom.HistogramVec
	}
)

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
	prom.MustRegister(vec)
	return &promHistogramVec{histogram: vec}
}

func (hv *promHistogramVec) Observe(v float64, labels ...string) {
	hv.histogram.WithLabelValues(labels...).Observe(v)
}

func (hv *promHistogramVec) Close() bool {
	return prom.Unregister(hv.histogram)
}
