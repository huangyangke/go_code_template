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
