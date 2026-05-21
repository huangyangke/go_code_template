package async_queue

import (
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

	DefaultGroupName    = "aikit_consumer_group"
	DefaultConsumerName = "aikit_consumer"
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
	DefaultPullBlock          = 2 * time.Second
	DefaultStopTimeout        = 30 * time.Second
	DefaultPelMinIdle         = 300 * time.Second
	ConsumerHeartbeatMaxWindow = 30 * time.Second
	MinRecoverySchedule       = 5 * time.Second
	TaskCancelTTL             = 3600 * time.Second
)

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
	WorkerCapacity         int
	DefaultTimeout         time.Duration
	DefaultRetryOnTimeout  bool
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
