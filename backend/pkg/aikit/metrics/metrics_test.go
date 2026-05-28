package metrics

import (
	"testing"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestNewCounterVec(t *testing.T) {
	cv := NewCounterVec(&CounterVecOpts{
		Namespace: "test",
		Name:      "counter_a_total",
		Help:      "test counter",
		Labels:    []string{"label"},
	})
	assert.NotNil(t, cv)
	assert.NotPanics(t, func() {
		cv.Inc("foo")
		cv.Add(2.5, "foo")
	})
	cv.Close()
}

func TestNewGaugeVec(t *testing.T) {
	gv := NewGaugeVec(&GaugeVecOpts{
		Namespace: "test",
		Name:      "gauge_a",
		Help:      "test gauge",
		Labels:    []string{"label"},
	})
	assert.NotNil(t, gv)
	assert.NotPanics(t, func() {
		gv.Set(10.0, "bar")
		gv.Inc("bar")
		gv.Add(5.0, "bar")
	})
	gv.Close()
}

func TestNewHistogramVec(t *testing.T) {
	hv := NewHistogramVec(&HistogramVecOpts{
		Namespace: "test",
		Name:      "histogram_a",
		Help:      "test histogram",
		Labels:    []string{"label"},
		Buckets:   []float64{1, 5, 10},
	})
	assert.NotNil(t, hv)
	assert.NotPanics(t, func() { hv.Observe(3, "baz") })
	hv.Close()
}

func TestNilOpts(t *testing.T) {
	assert.Nil(t, NewCounterVec(nil))
	assert.Nil(t, NewGaugeVec(nil))
	assert.Nil(t, NewHistogramVec(nil))
}

func TestSafeRegister_Idempotent(t *testing.T) {
	vec := prom.NewCounterVec(prom.CounterOpts{
		Name: "aikit_test_safe_register_counter",
		Help: "test counter for safeRegister",
	}, []string{"label"})
	safeRegister(vec) // 首次注册
	safeRegister(vec) // 二次注册不应 panic

	vec.WithLabelValues("test").Inc()
	fam, err := prom.DefaultGatherer.Gather()
	assert.NoError(t, err)
	found := false
	for _, m := range fam {
		if m.GetName() == "aikit_test_safe_register_counter" {
			found = true
		}
	}
	assert.True(t, found)

	prom.Unregister(vec)
}

func TestSafeRegister_InvalidCollector_Panics(t *testing.T) {
	assert.Panics(t, func() {
		safeRegister(prom.NewCounterVec(prom.CounterOpts{
			Name: "1starts_with_digit",
			Help: "invalid name causes panic",
		}, nil))
	})
}
