package async_queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// RunningStatusTTL 运行状态 TTL（7 天）.
	RunningStatusTTL = 7 * 24 * 60 * 60
	// FinalStatusTTL 终态 TTL（1 天）.
	FinalStatusTTL = 24 * 60 * 60
)

// ErrTaskAlreadyExists InitQueued 时任务 ID 已存在的错误.
var ErrTaskAlreadyExists = errors.New("task already exists")

// initQueuedScript atomically checks for duplicates, sets all fields, and sets TTL.
// Returns 1 on success, 0 if the task already exists.
var initQueuedScript = redis.NewScript(`
if redis.call('HEXISTS', KEYS[1], 'status') == 1 then
    return 0
end
for i = 2, #ARGV, 2 do
    redis.call('HSET', KEYS[1], ARGV[i], ARGV[i + 1])
end
redis.call('EXPIRE', KEYS[1], ARGV[1])
return 1
`)

// cancelIfQueuedScript atomically checks if status is "queued" and marks as "cancelled".
// Returns 1 if cancelled, 0 if status was not "queued".
var cancelIfQueuedScript = redis.NewScript(`
local status = redis.call('HGET', KEYS[1], 'status')
if status == ARGV[2] then
    redis.call('HSET', KEYS[1],
        'status', ARGV[3],
        'message', ARGV[4],
        'finished_at', ARGV[5],
        'updated_at', ARGV[5])
    redis.call('EXPIRE', KEYS[1], ARGV[1])
    return 1
end
return 0
`)

// TaskStatus 任务状态快照.
type TaskStatus struct {
	Status           string `json:"status"`                // 当前状态
	Progress         int    `json:"progress"`              // 进度值
	Endpoint         string `json:"endpoint"`              // 端点路径
	Priority         int    `json:"priority"`              // 优先级
	CreatedAt        int64  `json:"created_at"`            // 创建时间戳
	UpdatedAt        int64  `json:"updated_at"`            // 更新时间戳
	StartedAt        int64  `json:"started_at,omitempty"`  // 开始执行时间戳
	FinishedAt       int64  `json:"finished_at,omitempty"` // 完成时间戳
	Error            string `json:"error,omitempty"`       // 错误信息
	Message          string `json:"message,omitempty"`     // 提示信息
	Result           string `json:"result,omitempty"`      // 任务结果（JSON 字符串）
	SupportsProgress bool   `json:"supports_progress"`     // 是否支持进度上报
}

// StatusStore 任务状态持久化（Redis Hash）.
type StatusStore struct {
	rdb       redis.Cmdable
	namespace string
}

// NewStatusStore 创建状态存储实例.
// 参数：rdb - Redis 客户端, namespace - 命名空间.
// 返回值：*StatusStore - 状态存储实例.
func NewStatusStore(rdb redis.Cmdable, namespace string) *StatusStore {
	return &StatusStore{rdb: rdb, namespace: namespace}
}

func (s *StatusStore) key(taskID string) string {
	return buildStatusKey(s.namespace, taskID)
}

// InitQueued 原子初始化任务为 queued 状态，若任务已存在则返回 ErrTaskAlreadyExists.
// 参数：ctx - 上下文, taskID - 任务 ID, endpoint - 端点路径, priority - 优先级.
// 返回值：err - 初始化失败或任务已存在时的错误.
func (s *StatusStore) InitQueued(ctx context.Context, taskID, endpoint string, priority int) error {
	now := time.Now().Unix()
	key := s.key(taskID)
	res, err := initQueuedScript.Run(ctx, s.rdb,
		[]string{key},
		RunningStatusTTL,
		"status", TaskStatusQueued,
		"progress", "0",
		"endpoint", endpoint,
		"priority", strconv.Itoa(priority),
		"created_at", strconv.FormatInt(now, 10),
		"updated_at", strconv.FormatInt(now, 10),
		"supports_progress", "false",
	).Int()
	if err != nil {
		return err
	}
	if res == 0 {
		return fmt.Errorf("%w: task_id=%s", ErrTaskAlreadyExists, taskID)
	}
	return nil
}

// MarkRunning 将任务状态标记为 running.
// 参数：ctx - 上下文, taskID - 任务 ID.
// 返回值：err - 标记失败时的错误.
func (s *StatusStore) MarkRunning(ctx context.Context, taskID string) error {
	now := time.Now().Unix()
	key := s.key(taskID)
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, key,
		"status", TaskStatusRunning,
		"started_at", strconv.FormatInt(now, 10),
		"updated_at", strconv.FormatInt(now, 10),
	)
	pipe.Expire(ctx, key, RunningStatusTTL*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	s.publishTaskEvent(ctx, taskID)
	return nil
}

// MarkSuccess 将任务状态标记为 success 并存储结果.
// 参数：ctx - 上下文, taskID - 任务 ID, result - 任务结果.
// 返回值：err - 标记失败时的错误.
func (s *StatusStore) MarkSuccess(ctx context.Context, taskID string, result any) error {
	resultStr := ""
	if result != nil {
		b, err := json.Marshal(result)
		if err == nil {
			resultStr = string(b)
		}
	}
	now := time.Now().Unix()
	key := s.key(taskID)
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, key,
		"status", TaskStatusSuccess,
		"progress", "100",
		"result", resultStr,
		"finished_at", strconv.FormatInt(now, 10),
		"updated_at", strconv.FormatInt(now, 10),
	)
	pipe.Expire(ctx, key, FinalStatusTTL*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	s.publishTaskEvent(ctx, taskID)
	return nil
}

// MarkFailed 将任务状态标记为 failed 并存储错误信息.
// 参数：ctx - 上下文, taskID - 任务 ID, errMsg - 错误信息.
// 返回值：err - 标记失败时的错误.
func (s *StatusStore) MarkFailed(ctx context.Context, taskID, errMsg string) error {
	now := time.Now().Unix()
	key := s.key(taskID)
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, key,
		"status", TaskStatusFailed,
		"error", errMsg,
		"finished_at", strconv.FormatInt(now, 10),
		"updated_at", strconv.FormatInt(now, 10),
	)
	pipe.Expire(ctx, key, FinalStatusTTL*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	s.publishTaskEvent(ctx, taskID)
	return nil
}

// MarkCancelled 将任务状态标记为 cancelled.
// 参数：ctx - 上下文, taskID - 任务 ID, reason - 取消原因.
// 返回值：err - 标记失败时的错误.
func (s *StatusStore) MarkCancelled(ctx context.Context, taskID, reason string) error {
	now := time.Now().Unix()
	key := s.key(taskID)
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, key,
		"status", TaskStatusCancelled,
		"message", reason,
		"finished_at", strconv.FormatInt(now, 10),
		"updated_at", strconv.FormatInt(now, 10),
	)
	pipe.Expire(ctx, key, FinalStatusTTL*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	s.publishTaskEvent(ctx, taskID)
	return nil
}

// CancelIfQueued 原子地将 queued 状态转为 cancelled.
// 参数：ctx - 上下文, taskID - 任务 ID, reason - 取消原因.
// 返回值：cancelled - 是否成功转换, err - Redis 操作失败时的错误.
func (s *StatusStore) CancelIfQueued(ctx context.Context, taskID, reason string) (bool, error) {
	now := time.Now().Unix()
	key := s.key(taskID)
	res, err := cancelIfQueuedScript.Run(ctx, s.rdb,
		[]string{key},
		FinalStatusTTL,
		TaskStatusQueued,
		TaskStatusCancelled,
		reason,
		strconv.FormatInt(now, 10),
	).Int()
	if err != nil {
		return false, err
	}
	if res == 1 {
		s.publishTaskEvent(ctx, taskID)
	}
	return res == 1, nil
}

// MarkQueuedForRetry 将任务重置为 queued 状态（等待重试）.
// 参数：ctx - 上下文, taskID - 任务 ID, message - 提示信息, errMsg - 原始错误信息.
// 返回值：err - 标记失败时的错误.
func (s *StatusStore) MarkQueuedForRetry(ctx context.Context, taskID, message, errMsg string) error {
	now := time.Now().Unix()
	key := s.key(taskID)
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, key,
		"status", TaskStatusQueued,
		"message", message,
		"error", errMsg,
		"updated_at", strconv.FormatInt(now, 10),
	)
	pipe.Expire(ctx, key, RunningStatusTTL*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	s.publishTaskEvent(ctx, taskID)
	return nil
}

// UpdateProgress 更新任务进度.
// 参数：ctx - 上下文, taskID - 任务 ID, progress - 进度值（0-99）, message - 进度提示信息.
// 返回值：err - 更新失败时的错误.
func (s *StatusStore) UpdateProgress(ctx context.Context, taskID string, progress int, message string) error {
	key := s.key(taskID)
	fields := []any{
		"progress", strconv.Itoa(progress),
		"supports_progress", "true",
		"updated_at", strconv.FormatInt(time.Now().Unix(), 10),
	}
	if message != "" {
		fields = append(fields, "message", message)
	}
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, key, fields...)
	pipe.Expire(ctx, key, RunningStatusTTL*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	s.publishTaskEvent(ctx, taskID)
	return nil
}

// Delete 删除任务状态记录，用于入队失败后的清理.
// 参数：ctx - 上下文, taskID - 任务 ID.
// 返回值：err - 删除失败时的错误.
func (s *StatusStore) Delete(ctx context.Context, taskID string) error {
	return s.rdb.Del(ctx, s.key(taskID)).Err()
}

// Get 查询任务状态.
// 参数：ctx - 上下文, taskID - 任务 ID.
// 返回值：status - 任务状态快照（不存在时为 nil），err - 查询失败时的错误.
func (s *StatusStore) Get(ctx context.Context, taskID string) (*TaskStatus, error) {
	raw, err := s.rdb.HGetAll(ctx, s.key(taskID)).Result()
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return deserializeStatus(raw), nil
}

func deserializeStatus(raw map[string]string) *TaskStatus {
	ts := &TaskStatus{}
	ts.Status = raw["status"]
	ts.Endpoint = raw["endpoint"]
	ts.Error = raw["error"]
	ts.Message = raw["message"]
	ts.Result = raw["result"]

	if v, err := strconv.Atoi(raw["progress"]); err == nil {
		ts.Progress = v
	}
	if v, err := strconv.Atoi(raw["priority"]); err == nil {
		ts.Priority = v
	}
	if v, err := strconv.ParseInt(raw["created_at"], 10, 64); err == nil {
		ts.CreatedAt = v
	}
	if v, err := strconv.ParseInt(raw["updated_at"], 10, 64); err == nil {
		ts.UpdatedAt = v
	}
	if v, err := strconv.ParseInt(raw["started_at"], 10, 64); err == nil && v > 0 {
		ts.StartedAt = v
	}
	if v, err := strconv.ParseInt(raw["finished_at"], 10, 64); err == nil && v > 0 {
		ts.FinishedAt = v
	}
	ts.SupportsProgress = raw["supports_progress"] == "true"
	return ts
}

// PublishTaskEvent 将任务状态快照序列化并发布到事件 Pub/Sub 通道.
// 参数：ctx - 上下文, taskID - 任务 ID, status - 任务状态快照.
// 返回值：err - 发布失败时的错误.
func (s *StatusStore) PublishTaskEvent(ctx context.Context, taskID string, status *TaskStatus) error {
	b, err := json.Marshal(taskStatusToEvent(taskID, status))
	if err != nil {
		return err
	}
	return s.rdb.Publish(ctx, buildTaskEventsChannel(s.namespace), string(b)).Err()
}

// publishTaskEvent reads the latest status and publishes it. Errors are
// swallowed because event publication is best-effort — the authoritative
// state lives in the Redis hash, not in the pub/sub channel.
func (s *StatusStore) publishTaskEvent(ctx context.Context, taskID string) {
	ts, err := s.Get(ctx, taskID)
	if err != nil || ts == nil {
		return
	}
	_ = s.PublishTaskEvent(ctx, taskID, ts)
}

// taskStatusToEvent builds the SSE/pubsub event payload for a task. Callers
// (publish + producer's initial-status SSE write) share this so the schema
// stays consistent.
func taskStatusToEvent(taskID string, status *TaskStatus) map[string]any {
	return map[string]any{
		"task_id":           taskID,
		"status":            status.Status,
		"progress":          status.Progress,
		"error":             status.Error,
		"message":           status.Message,
		"result":            decodeResult(status.Result),
		"created_at":        status.CreatedAt,
		"started_at":        status.StartedAt,
		"finished_at":       status.FinishedAt,
		"endpoint":          status.Endpoint,
		"priority":          status.Priority,
		"supports_progress": status.SupportsProgress,
	}
}

// decodeResult converts the stringified JSON result back to a raw JSON value
// so it serializes inline (not as a quoted string) when the event is marshalled.
// Falls back to the original string if parsing fails — handlers that returned
// non-JSON-serializable values would otherwise lose data.
func decodeResult(result string) any {
	if result == "" {
		return nil
	}
	return json.RawMessage(result)
}
