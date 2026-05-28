// Package health 服务健康检查.
package health

import "context"

// Status 表示服务的健康状态.
type Status string

// StatusHealthy 服务健康.
const StatusHealthy Status = "healthy"

// StatusUnhealthy 服务不健康.
const StatusUnhealthy Status = "unhealthy"

// HealthChecker 可报告自身健康状态的资源需实现此接口.
// Ping 成功返回 nil，失败返回描述原因的错误.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// HealthStatus 表示服务整体健康状态.
type HealthStatus struct {
	Status   Status                    `json:"status"`
	Services map[string]*ServiceHealth `json:"services"`
}

// IsHealthy 判断所有已注册服务是否均健康.
// 参数：无.
// 返回值：ok - 所有服务健康或未注册任何服务时为 true.
func (s *HealthStatus) IsHealthy() bool {
	for _, svc := range s.Services {
		if svc.Status != StatusHealthy {
			return false
		}
	}
	return true
}

// ServiceHealth 表示单个依赖服务的健康状态.
type ServiceHealth struct {
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}
