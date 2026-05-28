package resilience

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNew_Defaults(t *testing.T) {
	brk := New(&Config{Name: "test-circuit"})
	assert.NotNil(t, brk)
}

func TestDo_Success(t *testing.T) {
	brk := New(&Config{Name: "test-success"})
	err := brk.Do(func() error { return nil }, nil)
	assert.NoError(t, err)
}

func TestDo_Fallback(t *testing.T) {
	brk := New(&Config{Name: "test-fallback"})
	expectedErr := errors.New("original error")
	var fallbackCalled bool

	err := brk.Do(
		func() error { return expectedErr },
		func(e error) error {
			fallbackCalled = true
			return nil
		},
	)
	assert.NoError(t, err)
	assert.True(t, fallbackCalled)
}

func TestDo_OpenCircuit(t *testing.T) {
	brk := New(&Config{
		Name:                   "test-open-circuit",
		RequestVolumeThreshold: 3,
		ErrorPercentThreshold:  50,
		SleepWindow:            60 * time.Second,
	})

	// Force failures to trip the circuit
	for i := 0; i < 10; i++ {
		brk.Do(func() error { return errors.New("err") }, nil)
	}

	// Circuit should now be open
	err := brk.Do(func() error { return nil }, nil)
	assert.True(t, IsCircuitOpen(err))
}

func TestIsCircuitOpen(t *testing.T) {
	assert.False(t, IsCircuitOpen(nil))
	assert.False(t, IsCircuitOpen(errors.New("random")))
	assert.True(t, IsCircuitOpen(ErrCircuitOpen))
}

func TestNew_EmptyName_Panics(t *testing.T) {
	assert.Panics(t, func() { New(&Config{}) })
}

func TestAllow_Success_DoneSuccess(t *testing.T) {
	brk := New(&Config{Name: "test-allow-success"})
	done, err := brk.Allow()
	assert.NoError(t, err)
	assert.NotNil(t, done)
	done(true)

	// After successful done, breaker stays closed; next Allow still succeeds.
	done2, err2 := brk.Allow()
	assert.NoError(t, err2)
	assert.NotNil(t, done2)
	done2(true)
}

func TestAllow_OpenReturnsError(t *testing.T) {
	brk := New(&Config{
		Name:                   "test-allow-open",
		RequestVolumeThreshold: 3,
		ErrorPercentThreshold:  50,
		SleepWindow:            60 * time.Second,
	})

	// Force failures to trip the circuit.
	for i := 0; i < 10; i++ {
		done, err := brk.Allow()
		if err == nil {
			done(false)
		}
	}

	// Now open: Allow should return ErrCircuitOpen and no done.
	done, err := brk.Allow()
	assert.Nil(t, done)
	assert.True(t, IsCircuitOpen(err))
}

func TestAllow_HalfOpen_LimitsRequests(t *testing.T) {
	brk := New(&Config{
		Name:                   "test-allow-halfopen",
		MaxRequests:            2,
		RequestVolumeThreshold: 3,
		ErrorPercentThreshold:  50,
		SleepWindow:            50 * time.Millisecond,
	})

	// Trip circuit to open.
	for i := 0; i < 10; i++ {
		done, err := brk.Allow()
		if err == nil {
			done(false)
		}
	}

	// Confirm open.
	_, errOpen := brk.Allow()
	assert.True(t, IsCircuitOpen(errOpen))

	// Wait for SleepWindow → half-open.
	time.Sleep(80 * time.Millisecond)

	// First 2 Allow should succeed (MaxRequests=2). Don't call done yet — we
	// want to keep them in-flight to exhaust the half-open budget.
	done1, err1 := brk.Allow()
	assert.NoError(t, err1)
	assert.NotNil(t, done1)

	done2, err2 := brk.Allow()
	assert.NoError(t, err2)
	assert.NotNil(t, done2)

	// Third Allow within half-open with budget exhausted → ErrTooManyRequests.
	done3, err3 := brk.Allow()
	assert.Nil(t, done3)
	assert.True(t, IsCircuitOpen(err3)) // IsCircuitOpen covers ErrTooManyRequests too.

	// Cleanup: signal success on the in-flight tokens.
	done1(true)
	done2(true)
}
