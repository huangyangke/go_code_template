package health

import "context"

// HealthChecker is implemented by resources that can report their health status.
// The Ping method returns nil on success or an error describing the failure.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// HealthStatus represents the overall health of the service
type HealthStatus struct {
	Status   string                    `json:"status"`
	Services map[string]*ServiceHealth `json:"services"`
}

// ServiceHealth represents the health of a single dependency
type ServiceHealth struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}
