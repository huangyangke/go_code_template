package metrics

import prom "github.com/prometheus/client_golang/prometheus"

type (
	GaugeVecOpts VectorOpts

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
	prom.MustRegister(vec)
	return &promGaugeVec{gauge: vec}
}

func (gv *promGaugeVec) Inc(labels ...string) {
	gv.gauge.WithLabelValues(labels...).Inc()
}

func (gv *promGaugeVec) Add(v float64, labels ...string) {
	gv.gauge.WithLabelValues(labels...).Add(v)
}

func (gv *promGaugeVec) Set(v float64, labels ...string) {
	gv.gauge.WithLabelValues(labels...).Set(v)
}

func (gv *promGaugeVec) Close() bool {
	return prom.Unregister(gv.gauge)
}

// RegisterGaugeFunc registers a gauge whose value is computed by fn on each scrape.
// Returns an unregister func that must be called on cleanup to avoid duplicate registration panics.
func RegisterGaugeFunc(name, help string, constLabels prom.Labels, fn func() float64) func() {
	g := prom.NewGaugeFunc(prom.GaugeOpts{
		Name:        name,
		Help:        help,
		ConstLabels: constLabels,
	}, fn)
	prom.MustRegister(g)
	return func() { prom.Unregister(g) }
}
