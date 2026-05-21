package health

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockChecker struct {
	err error
}

func (m *mockChecker) Ping(_ context.Context) error {
	return m.err
}

func TestHealthChecker_Healthy(t *testing.T) {
	m := &mockChecker{}
	assert.Nil(t, m.Ping(context.Background()))
}

func TestHealthChecker_Unhealthy(t *testing.T) {
	m := &mockChecker{err: errors.New("connection refused")}
	assert.NotNil(t, m.Ping(context.Background()))
}

func TestHealthStatus_Structure(t *testing.T) {
	status := &HealthStatus{
		Services: map[string]*ServiceHealth{
			"mysql:main":  {Status: "healthy"},
			"redis:cache": {Status: "unhealthy", Error: "timeout"},
		},
	}
	status.Status = "unhealthy"

	assert.Equal(t, "unhealthy", status.Status)
	assert.Equal(t, "healthy", status.Services["mysql:main"].Status)
	assert.Equal(t, "timeout", status.Services["redis:cache"].Error)
}
