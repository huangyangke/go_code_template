package async_queue

import (
	"errors"
	"fmt"
	"time"
)

// ================================
// 常量
// ================================

const (
	TaskStatusQueued    = "queued"
	TaskStatusRunning   = "running"
	TaskStatusSuccess   = "success"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"

	DefaultGroupName      = "aikit_consumer_group"
	DefaultConsumerName   = "aikit_consumer"
	DefaultWorkerCapacity = 100
	DefaultPullCount      = 10

	DefaultPelMaxRetries        = 3
	DefaultMaxPendingMultiplier = 10

	StreamNewMessagesID = ">"
	StreamGroupStartID  = "0"

	// Redis key 前缀（统一格式 aikit:async:{ns}:{resource}:{id}）
	keyPrefix = "aikit:async"
)

var (
	DefaultPullBlock           = 2 * time.Second
	DefaultStopTimeout         = 30 * time.Second
	DefaultPelMinIdle          = 300 * time.Second
	ConsumerHeartbeatMaxWindow = 30 * time.Second
	MinRecoverySchedule        = 5 * time.Second
	TaskCancelTTL              = 3600 * time.Second
	DefaultSSEMaxLifetime      = 5 * time.Minute
	TaskHeartbeatInterval      = 30 * time.Second
)

// ================================
// 特性预设
// ================================

// FeatureMode 控制 async queue 可选特性的预设级别。
//
//   - FeatureModeLite: 纯 consume→handle→ack，最小 Redis 开销，无状态追踪
//   - FeatureModeStandard: 状态追踪 + 任务取消，适合大多数场景
//   - FeatureModeFull（默认）: 全部特性，适合生产级分布式部署
type FeatureMode int

const (
	FeatureModeFull     FeatureMode = iota // 默认
	FeatureModeLite                        // 纯消费，无状态/心跳/PEL
	FeatureModeStandard                    // 状态+取消，无心跳/PEL
)

// FeatureConfig 控制 async queue 各个可选特性的开关。
// 零值等同于 FeatureModeFull（全部启用）。
// 调用 ResolveFeatureMode 可从预设生成，也可手动逐项覆盖。
type FeatureConfig struct {
	EnableStatusStore  bool
	EnablePelRecovery  bool
	EnableHeartbeat    bool
	EnableCancel       bool
}

// ResolveFeatureMode 从预设模式生成 FeatureConfig。
// overrides 中的非零字段会覆盖预设值；依赖约束自动修正：
//   - EnablePelRecovery=true 强制 EnableHeartbeat=true
//   - EnableCancel=true 强制 EnableStatusStore=true
func ResolveFeatureMode(mode FeatureMode, overrides *FeatureConfig) FeatureConfig {
	var cfg FeatureConfig
	switch mode {
	case FeatureModeLite:
		// 全部关闭
	case FeatureModeStandard:
		cfg.EnableStatusStore = true
		cfg.EnableCancel = true
	default: // FeatureModeFull
		cfg.EnableStatusStore = true
		cfg.EnablePelRecovery = true
		cfg.EnableHeartbeat = true
		cfg.EnableCancel = true
	}
	if overrides != nil {
		if overrides.EnableStatusStore {
			cfg.EnableStatusStore = true
		}
		if overrides.EnablePelRecovery {
			cfg.EnablePelRecovery = true
		}
		if overrides.EnableHeartbeat {
			cfg.EnableHeartbeat = true
		}
		if overrides.EnableCancel {
			cfg.EnableCancel = true
		}
	}
	// 依赖约束
	if cfg.EnablePelRecovery {
		cfg.EnableHeartbeat = true
	}
	if cfg.EnableCancel {
		cfg.EnableStatusStore = true
	}
	return cfg
}

// ================================
// 配置结构
// ================================

// EndpointConfig 单个 endpoint 的配置
type EndpointConfig struct {
	Handler        HandlerFunc
	CallBack       CallbackFunc
	MaxConcurrency int
	Timeout        time.Duration // 0 表示不限
	RetryOnTimeout bool
}

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	WorkerCapacity        int
	DefaultTimeout        time.Duration
	DefaultRetryOnTimeout bool
}

func defaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		WorkerCapacity: DefaultWorkerCapacity,
	}
}

// PelConfig PEL 恢复配置
type PelConfig struct {
	MinIdle           time.Duration
	MaxRetries        int
	ScanOnStartupOnly bool
}

func defaultPelConfig() PelConfig {
	return PelConfig{
		MinIdle:           DefaultPelMinIdle,
		MaxRetries:        DefaultPelMaxRetries,
		ScanOnStartupOnly: true,
	}
}

// EndpointLimitConfig 端点并发模式配置
type EndpointLimitConfig struct {
	Mode         string // "local" | "distributed"
	DefaultLimit int    // 0 表示不限
	KeyPrefix    string
}

// RedisConfig Redis 连接配置
type RedisConfig struct {
	URL       string
	StreamKey string
	Family    string
}

// ================================
// 函数类型
// ================================

// HandlerFunc 任务处理函数
type HandlerFunc func(ctx Context) (any, error)

// CallbackFunc 任务回调函数
type CallbackFunc func(resp *TaskResponse) error

// TaskResponse handler 完成后的回调载荷
type TaskResponse struct {
	TaskID   string
	Endpoint string
	Data     any
	Err      error
}

// ================================
// 配置校验
// ================================

// ValidateSchedulerConfig 校验 SchedulerConfig 字段合法性。
func ValidateSchedulerConfig(cfg SchedulerConfig) error {
	if cfg.WorkerCapacity < 0 {
		return errors.New("async_queue: SchedulerConfig.WorkerCapacity must be >= 0")
	}
	if cfg.DefaultTimeout < 0 {
		return errors.New("async_queue: SchedulerConfig.DefaultTimeout must be >= 0")
	}
	return nil
}

// ValidatePelConfig 校验 PelConfig 字段合法性。
func ValidatePelConfig(cfg PelConfig) error {
	if cfg.MinIdle <= 0 {
		return errors.New("async_queue: PelConfig.MinIdle must be > 0")
	}
	if cfg.MaxRetries < 0 {
		return errors.New("async_queue: PelConfig.MaxRetries must be >= 0")
	}
	return nil
}

// ValidateEndpointConfig 校验 endpoint 配置合法性：
//   - 每个 endpoint 必须提供 Handler
//   - MaxConcurrency 必须 >= 0
//   - Timeout 必须 >= 0
//   - endpoint 路径不能与系统保留路径冲突（/status, /cancel, /events）
func ValidateEndpointConfig(endpoints map[string]EndpointConfig) error {
	reserved := []string{"/status", "/cancel", "/events"}
	var errs []error
	for path, cfg := range endpoints {
		for _, r := range reserved {
			if path == r || len(path) > len(r) && path[:len(r)+1] == r+"/" {
				errs = append(errs, fmt.Errorf("async_queue: endpoint %q conflicts with reserved path %q", path, r))
			}
		}
		if cfg.Handler == nil {
			errs = append(errs, fmt.Errorf("async_queue: endpoint %q missing Handler", path))
		}
		if cfg.MaxConcurrency < 0 {
			errs = append(errs, fmt.Errorf("async_queue: endpoint %q MaxConcurrency must be >= 0", path))
		}
		if cfg.Timeout < 0 {
			errs = append(errs, fmt.Errorf("async_queue: endpoint %q Timeout must be >= 0", path))
		}
	}
	return errors.Join(errs...)
}

// ================================
// Key 构造辅助
// ================================

func buildStatusKey(ns, taskID string) string {
	return keyPrefix + ":" + ns + ":task:status:" + taskID
}

func buildCancelKey(ns, taskID string) string {
	return keyPrefix + ":" + ns + ":task:cancel:" + taskID
}

func buildCancelChannel(ns string) string {
	return keyPrefix + ":" + ns + ":channel:cancel"
}

func buildTaskEventsChannel(ns string) string {
	return keyPrefix + ":" + ns + ":channel:events"
}

func buildHeartbeatKey(ns, groupName, consumerName string) string {
	return keyPrefix + ":" + ns + ":heartbeat:" + groupName + ":" + consumerName
}

func buildEndpointLimitKeyPrefix(ns string) string {
	return keyPrefix + ":" + ns + ":limit"
}

func buildStatusScanPattern(ns string) string {
	return keyPrefix + ":" + ns + ":task:status:*"
}
