package cache

import (
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"

	"github.com/huangyangke/go-aikit/metrics"
)

var enableMetricsOnce sync.Once

func ensureMetricsEnabled() {
	enableMetricsOnce.Do(metrics.Enable)
}

// readCacheCounter reads the current value of cache_requests_total for the given
// label triple from the default Prometheus registry. Returns 0 if not yet emitted.
func readCacheCounter(t *testing.T, name, level, result string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range families {
		if mf.GetName() != "aikit_cache_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelMatch(m.GetLabel(), name, level, result) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func labelMatch(labels []*dto.LabelPair, name, level, result string) bool {
	want := map[string]string{"name": name, "level": level, "result": result}
	got := map[string]string{}
	for _, l := range labels {
		got[l.GetName()] = l.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// TestPromStats_LevelLabels verifies each promStats method increments
// cache_requests_total with the correct level/result labels.
func TestPromStats_LevelLabels(t *testing.T) {
	ensureMetricsEnabled()

	const ds = "fam:promstats-level-labels"
	s := &promStats{datasource: ds}

	cases := []struct {
		method func()
		level  string
		result string
	}{
		{s.IncrLocalHit, "l1", "hit"},
		{s.IncrLocalMiss, "l1", "miss"},
		{s.IncrRemoteHit, "l2", "hit"},
		{s.IncrRemoteMiss, "l2", "miss"},
	}

	for _, tc := range cases {
		t.Run(tc.level+"_"+tc.result, func(t *testing.T) {
			before := readCacheCounter(t, ds, tc.level, tc.result)
			tc.method()
			after := readCacheCounter(t, ds, tc.level, tc.result)
			assert.Equal(t, before+1, after,
				"expected counter for level=%s result=%s to increment by 1", tc.level, tc.result)
		})
	}

	// IncrQuery / IncrQueryFail are intentional no-ops — they must not panic
	// and must not touch the counter.
	t.Run("noops", func(t *testing.T) {
		before := readCacheCounter(t, ds, "l1", "hit")
		assert.NotPanics(t, func() {
			s.IncrQuery()
			s.IncrQueryFail(nil)
		})
		after := readCacheCounter(t, ds, "l1", "hit")
		assert.Equal(t, before, after, "no-op methods must not increment counter")
	})
}
