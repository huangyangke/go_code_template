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
			"mysql:main":  {Status: StatusHealthy},
			"redis:cache": {Status: StatusUnhealthy, Error: "timeout"},
		},
	}
	status.Status = StatusUnhealthy

	assert.Equal(t, StatusUnhealthy, status.Status)
	assert.Equal(t, StatusHealthy, status.Services["mysql:main"].Status)
	assert.Equal(t, "timeout", status.Services["redis:cache"].Error)
}

func TestHealthStatus_IsHealthy(t *testing.T) {
	tests := []struct {
		name   string
		status *HealthStatus
		want   bool
	}{
		{
			name: "all healthy",
			status: &HealthStatus{
				Status:   StatusHealthy,
				Services: map[string]*ServiceHealth{
					"mysql:main":  {Status: StatusHealthy},
					"redis:cache": {Status: StatusHealthy},
				},
			},
			want: true,
		},
		{
			name: "one unhealthy",
			status: &HealthStatus{
				Status: StatusUnhealthy,
				Services: map[string]*ServiceHealth{
					"mysql:main":  {Status: StatusHealthy},
					"redis:cache": {Status: StatusUnhealthy, Error: "timeout"},
				},
			},
			want: false,
		},
		{
			name: "no services",
			status: &HealthStatus{
				Status:   StatusHealthy,
				Services: map[string]*ServiceHealth{},
			},
			want: true,
		},
		{
			name:   "nil services",
			status: &HealthStatus{Status: StatusHealthy},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsHealthy()
			assert.Equal(t, tt.want, got)
		})
	}
}
