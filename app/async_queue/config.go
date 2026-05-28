// Package async_queue 基于 Redis Stream 的异步任务队列，提供任务生产、消费、状态追踪和死信处理.
package async_queue

import (
	"errors"
	"fmt"
	"time"
)

// ================================
// 常量
// ================================.

const (
	// TaskStatusQueued 任务已入队，等待消费.
	TaskStatusQueued = "queued"
	// TaskStatusRunning 任务正在执行中.
	TaskStatusRunning = "running"
	// TaskStatusSuccess 任务已成功完成.
	TaskStatusSuccess = "success"
	// TaskStatusFailed 任务执行失败.
	TaskStatusFailed = "failed"
	// TaskStatusCancelled 任务已被取消.
	TaskStatusCancelled = "cancelled"

	// DefaultGroupName 默认消费者组名称.
	DefaultGroupName = "aikit_consumer_group"
	// DefaultConsumerName 默认消费者名称.
	DefaultConsumerName = "aikit_consumer"
	// DefaultWorkerCapacity 全局 goroutine 上限.
	DefaultWorkerCapacity = 100
	// DefaultPullCount 每次XReadGroup最多拉取条数.
	DefaultPullCount = 10

	// DefaultPelMaxRetries 毒消息最大重试次数.
	DefaultPelMaxRetries = 3
	// DefaultMaxPendingMultiplier 本地缓冲队列上限倍数（× WorkerCapacity）.
	DefaultMaxPendingMultiplier = 10

	// StreamNewMessagesID XReadGroup 只读新消息的 ID 标记.
	StreamNewMessagesID = ">"
	// StreamGroupStartID XReadGroup 从最早消息开始的 ID 标记.
	StreamGroupStartID = "0"

	// Redis key 前缀（统一格式 aikit:async:{ns}:{resource}:{id}）.
	keyPrefix = "aikit:async"
)

var (
	// DefaultPullBlock XReadGroup 的 block 时长.
	DefaultPullBlock = 2 * time.Second
	// DefaultStopTimeout 优雅停机等待上限.
	DefaultStopTimeout = 30 * time.Second
	// DefaultPelMinIdle PEL 恢复的最小空闲时长（5分钟）.
	DefaultPelMinIdle = 300 * time.Second
	// ConsumerHeartbeatMaxWindow 消费者心跳最大窗口.
	ConsumerHeartbeatMaxWindow = 30 * time.Second
	// MinRecoverySchedule PEL 恢复最小调度间隔.
	MinRecoverySchedule = 5 * time.Second
	// TaskCancelTTL 取消标记 Key 的 TTL.
	TaskCancelTTL = 3600 * time.Second
	// DefaultSSEMaxLifetime SSE 连接最大存活时间.
	DefaultSSEMaxLifetime = 5 * time.Minute
	// TaskHeartbeatInterval SSE keep-alive 心跳间隔.
	TaskHeartbeatInterval = 30 * time.Second
)

// ================================
// 特性预设
// ================================.

// FeatureMode 控制异步队列可选特性的预设级别.
type FeatureMode int

const (
	// FeatureModeFull 默认模式，全部特性启用.
	FeatureModeFull FeatureMode = iota
	// FeatureModeLite 纯消费模式，无状态追踪/心跳/PEL 恢复.
	FeatureModeLite
	// FeatureModeStandard 标准模式，含状态追踪和任务取消，无心跳/PEL.
	FeatureModeStandard
)

// FeatureConfig 控制异步队列各个可选特性的开关，零值等同于 FeatureModeFull.
type FeatureConfig struct {
	EnableStatusStore bool // 是否启用状态持久化
	EnablePelRecovery bool // 是否启用 PEL 恢复
	EnableHeartbeat   bool // 是否启用消费者心跳
	EnableCancel      bool // 是否启用任务取消
}

// ResolveFeatureMode 从预设模式生成 FeatureConfig.
// 参数：mode - 特性预设级别, overrides - 非零字段覆盖预设值.
// 返回值：cfg - 生成的特性配置.
//
// 依赖约束自动修正：
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
// ================================.

// EndpointConfig 单个 endpoint 的配置.
type EndpointConfig struct {
	Handler        HandlerFunc   // 任务处理函数
	CallBack       CallbackFunc  // 任务回调函数
	MaxConcurrency int           // 最大并发数，0 表示不限
	Timeout        time.Duration // 超时时间，0 表示不限
	RetryOnTimeout bool          // 超时后是否重试
}

// SchedulerConfig 调度器配置.
type SchedulerConfig struct {
	WorkerCapacity        int           // 全局 goroutine 上限
	DefaultTimeout        time.Duration // 默认超时时间
	DefaultRetryOnTimeout bool          // 默认超时重试开关
}

func defaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		WorkerCapacity: DefaultWorkerCapacity,
	}
}

// PelConfig PEL 恢复配置.
type PelConfig struct {
	MinIdle           time.Duration // 最小空闲时长，低于此值不做恢复
	MaxRetries        int           // 最大重试次数，超过后转入死信
	ScanOnStartupOnly bool          // 是否仅在启动时扫描一次
}

func defaultPelConfig() PelConfig {
	return PelConfig{
		MinIdle:           DefaultPelMinIdle,
		MaxRetries:        DefaultPelMaxRetries,
		ScanOnStartupOnly: false,
	}
}

// EndpointLimitConfig 端点并发模式配置.
type EndpointLimitConfig struct {
	Mode         string // 并发限制模式，"local" 或 "distributed"
	DefaultLimit int    // 默认并发上限，0 表示不限
}

// RedisConfig Redis 连接配置.
type RedisConfig struct {
	URL    string // Redis 连接地址
	Family string // 命名空间
}

// ================================
// 函数类型
// ================================.

// HandlerFunc 任务处理函数.
type HandlerFunc func(ctx Context) (any, error)

// CallbackFunc 任务回调函数.
type CallbackFunc func(resp *TaskResponse) error

// TaskResponse handler 完成后的回调载荷.
type TaskResponse struct {
	TaskID   string // 任务 ID
	Endpoint string // 端点路径
	Data     any    // 任务结果数据
	Err      error  // 任务执行错误
}

// ================================
// 配置校验
// ================================.

// ValidateSchedulerConfig 校验调度器配置的字段合法性.
// 参数：cfg - 调度器配置.
// 返回值：err - 字段不合法时的错误.
func ValidateSchedulerConfig(cfg SchedulerConfig) error {
	if cfg.WorkerCapacity < 0 {
		return errors.New("async_queue: SchedulerConfig.WorkerCapacity must be >= 0")
	}
	if cfg.DefaultTimeout < 0 {
		return errors.New("async_queue: SchedulerConfig.DefaultTimeout must be >= 0")
	}
	return nil
}

// ValidatePelConfig 校验 PEL 恢复配置的字段合法性.
// 参数：cfg - PEL 配置.
// 返回值：err - 字段不合法时的错误.
func ValidatePelConfig(cfg PelConfig) error {
	if cfg.MinIdle <= 0 {
		return errors.New("async_queue: PelConfig.MinIdle must be > 0")
	}
	if cfg.MaxRetries < 0 {
		return errors.New("async_queue: PelConfig.MaxRetries must be >= 0")
	}
	return nil
}

// ValidateEndpointConfig 校验端点配置合法性.
// 参数：endpoints - 端点路径到配置的映射.
// 返回值：err - 配置不合法时的错误集合.
//
// 校验规则：
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
// ================================.

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

// BuildEndpointLimitKeyPrefix 构建端点并发限制的 Redis key 前缀.
// 参数：ns - 命名空间.
// 返回值：prefix - Redis key 前缀字符串.
func BuildEndpointLimitKeyPrefix(ns string) string {
	return keyPrefix + ":" + ns + ":limit"
}

func buildEndpointLimitKeyPrefix(ns string) string {
	return BuildEndpointLimitKeyPrefix(ns)
}

func buildStatusScanPattern(ns string) string {
	return keyPrefix + ":" + ns + ":task:status:*"
}

func buildStreamKey(ns string) string {
	return keyPrefix + ":" + ns + ":stream"
}

// buildDeadLetterStreamKey 构建死信 stream key，格式：aikit:async:{ns}:deadletter.
func buildDeadLetterStreamKey(ns string) string {
	return keyPrefix + ":" + ns + ":deadletter"
}
