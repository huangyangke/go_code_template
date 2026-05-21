package resilience_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/go-template/pkg/aikit/resilience"
)

// ── Retry ─────────────────────────────────────────────────────────────────────

func TestRetry_SucceedsOnFirstAttempt(t *testing.T) {
	calls := 0
	err := resilience.Retry(func() error {
		calls++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetry_RetriesUpToDefaultThreeTimes(t *testing.T) {
	calls := 0
	err := resilience.Retry(func() error {
		calls++
		return errors.New("fail")
	})
	assert.Error(t, err)
	assert.Equal(t, 3, calls)
}

func TestRetry_StopsWhenSucceeds(t *testing.T) {
	calls := 0
	err := resilience.Retry(func() error {
		calls++
		if calls < 2 {
			return errors.New("not yet")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestRetry_WithTimes(t *testing.T) {
	calls := 0
	err := resilience.Retry(func() error {
		calls++
		return errors.New("fail")
	}, resilience.WithTimes(5))
	assert.Error(t, err)
	assert.Equal(t, 5, calls)
}

func TestRetry_WithAcceptable_StopsOnAcceptableError(t *testing.T) {
	sentinel := errors.New("terminal")
	calls := 0
	err := resilience.Retry(func() error {
		calls++
		return sentinel
	}, resilience.WithAcceptable(func(err error) bool {
		return !errors.Is(err, sentinel)
	}))
	assert.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, calls, "should not retry an unacceptable error")
}

// ── Backoff ───────────────────────────────────────────────────────────────────

func TestBackoff_ZeroRetriesReturnsBaseDelay(t *testing.T) {
	b := resilience.NewBackoff()
	d := b.Delay(0)
	// base=1s × jitter ± 20%, so must be in [0.8s, 1.2s]
	assert.GreaterOrEqual(t, d, 800*time.Millisecond)
	assert.LessOrEqual(t, d, 1200*time.Millisecond)
}

func TestBackoff_DelayGrowsWithRetries(t *testing.T) {
	b := resilience.NewBackoff(resilience.WithBackoffJitter(0)) // no jitter for determinism
	d0 := b.Delay(0)
	d1 := b.Delay(1)
	d2 := b.Delay(2)
	assert.Less(t, d0, d1)
	assert.Less(t, d1, d2)
}

func TestBackoff_MaxDelayRespected(t *testing.T) {
	b := resilience.NewBackoff(
		resilience.WithBackoffMax(5*time.Second),
		resilience.WithBackoffJitter(0),
	)
	for i := 0; i < 20; i++ {
		assert.LessOrEqual(t, b.Delay(i), 5*time.Second)
	}
}

func TestBackoff_CustomBaseDelay(t *testing.T) {
	b := resilience.NewBackoff(
		resilience.WithBackoffBase(100*time.Millisecond),
		resilience.WithBackoffJitter(0),
	)
	d := b.Delay(0)
	assert.Equal(t, 100*time.Millisecond, d)
}

// ── RetryWithBackoff ──────────────────────────────────────────────────────────

func TestRetryWithBackoff_UsesBackoffBetweenAttempts(t *testing.T) {
	b := resilience.NewBackoff(
		resilience.WithBackoffBase(10*time.Millisecond),
		resilience.WithBackoffJitter(0),
	)
	calls := 0
	start := time.Now()
	_ = resilience.RetryWithBackoff(func() error {
		calls++
		return errors.New("fail")
	}, b, resilience.WithTimes(3))
	elapsed := time.Since(start)
	assert.Equal(t, 3, calls)
	// 3 attempts: delay after attempt 1 (10ms) + delay after attempt 2 (16ms) = 26ms
	assert.GreaterOrEqual(t, elapsed, 20*time.Millisecond)
}
