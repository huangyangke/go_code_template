package cache

import (
	"github.com/example/go-template/pkg/aikit/metrics"
)

type promStats struct {
	family string
	name   string
}

func newPromStats(family, name string) *promStats {
	return &promStats{family: family, name: name}
}

func (s *promStats) IncrHit()            { metrics.ObserveCacheHit(s.family, s.name, "l2") }
func (s *promStats) IncrMiss()           { metrics.ObserveCacheMiss(s.family, s.name) }
func (s *promStats) IncrLocalHit()       { metrics.ObserveCacheHit(s.family, s.name, "l1") }
func (s *promStats) IncrLocalMiss()      { metrics.ObserveCacheMiss(s.family, s.name) }
func (s *promStats) IncrRemoteHit()      { metrics.ObserveCacheHit(s.family, s.name, "l2") }
func (s *promStats) IncrRemoteMiss()     { metrics.ObserveCacheMiss(s.family, s.name) }
func (s *promStats) IncrQuery()          {}
func (s *promStats) IncrQueryFail(error) {}
