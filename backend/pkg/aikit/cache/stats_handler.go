package cache

import (
	"github.com/example/go-template/pkg/aikit/metrics"
)

type promStats struct {
	datasource string
}

func newPromStats(family, name string) *promStats {
	return &promStats{datasource: cacheName(family, name)}
}

func (s *promStats) IncrHit()            { metrics.ObserveCache(s.datasource, "l2", "hit") }
func (s *promStats) IncrMiss()           { metrics.ObserveCache(s.datasource, "l2", "miss") }
func (s *promStats) IncrLocalHit()       { metrics.ObserveCache(s.datasource, "l1", "hit") }
func (s *promStats) IncrLocalMiss()      { metrics.ObserveCache(s.datasource, "l1", "miss") }
func (s *promStats) IncrRemoteHit()      { metrics.ObserveCache(s.datasource, "l2", "hit") }
func (s *promStats) IncrRemoteMiss()     { metrics.ObserveCache(s.datasource, "l2", "miss") }
func (s *promStats) IncrQuery()          {}
func (s *promStats) IncrQueryFail(error) {}
