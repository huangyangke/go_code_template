package health

import "context"

// Status represents the health status of a service.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
)

// HealthChecker is implemented by resources that can report their health status.
// The Ping method returns nil on success or an error describing the failure.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// HealthStatus represents the overall health of the service
type HealthStatus struct {
	Status   Status                      `json:"status"`
	Services map[string]*ServiceHealth   `json:"services"`
}

// IsHealthy returns true if all registered services are healthy.
// Returns true when no services are registered.
func (s *HealthStatus) IsHealthy() bool {
	for _, svc := range s.Services {
		if svc.Status != StatusHealthy {
			return false
		}
	}
	return true
}

// ServiceHealth represents the health of a single dependency
type ServiceHealth struct {
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}
