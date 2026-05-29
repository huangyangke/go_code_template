package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewClock(t *testing.T) {
	t.Run("initial time", func(t *testing.T) {
		start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		m := NewClock(t, start)

		assert.Equal(t, start, m.Now())
	})

	t.Run("advance time", func(t *testing.T) {
		start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		m := NewClock(t, start)

		m.Add(5 * time.Second)
		assert.Equal(t, start.Add(5*time.Second), m.Now())
	})

	t.Run("sleep timer", func(t *testing.T) {
		start := time.Now()
		m := NewClock(t, start)

		timer := m.Timer(10 * time.Second)

		// 前进时间
		m.Add(10 * time.Second)

		select {
		case <-timer.C:
			// 预期行为
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timer should fire")
		}
	})
}
