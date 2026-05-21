package async_queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Status TTL constants (seconds)
	RunningStatusTTL = 7 * 24 * 60 * 60 // 7 days
	FinalStatusTTL   = 24 * 60 * 60     // 1 day
)

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

// TaskStatus 任务状态快照
type TaskStatus struct {
	Status          string `json:"status"`
	Progress        int    `json:"progress"`
	Endpoint        string `json:"endpoint"`
	Priority        int    `json:"priority"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	StartedAt       int64  `json:"started_at,omitempty"`
	FinishedAt      int64  `json:"finished_at,omitempty"`
	Error           string `json:"error,omitempty"`
	Message         string `json:"message,omitempty"`
	Result          string `json:"result,omitempty"`
	SupportsProgress bool  `json:"supports_progress"`
}

// StatusStore 任务状态持久化（Redis Hash）
type StatusStore struct {
	rdb       redis.Cmdable
	namespace string
}

func NewStatusStore(rdb redis.Cmdable, namespace string) *StatusStore {
	return &StatusStore{rdb: rdb, namespace: namespace}
}

func (s *StatusStore) key(taskID string) string {
	return buildStatusKey(s.namespace, taskID)
}

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
		"started_at", "-",
		"finished_at", "-",
		"supports_progress", "false",
	).Int()
	if err != nil {
		return err
	}
	if res == 0 {
		return fmt.Errorf("task with task_id=%s already exists", taskID)
	}
	return nil
}

func (s *StatusStore) MarkRunning(ctx context.Context, taskID string) error {
	now := time.Now().Unix()
	key := s.key(taskID)
	if err := s.rdb.HSet(ctx, key,
		"status", TaskStatusRunning,
		"started_at", strconv.FormatInt(now, 10),
		"updated_at", strconv.FormatInt(now, 10),
	).Err(); err != nil {
		return err
	}
	return s.rdb.Expire(ctx, key, RunningStatusTTL*time.Second).Err()
}

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
	if err := s.rdb.HSet(ctx, key,
		"status", TaskStatusSuccess,
		"progress", "100",
		"result", resultStr,
		"finished_at", strconv.FormatInt(now, 10),
		"updated_at", strconv.FormatInt(now, 10),
	).Err(); err != nil {
		return err
	}
	return s.rdb.Expire(ctx, key, FinalStatusTTL*time.Second).Err()
}

func (s *StatusStore) MarkFailed(ctx context.Context, taskID, errMsg string) error {
	now := time.Now().Unix()
	key := s.key(taskID)
	if err := s.rdb.HSet(ctx, key,
		"status", TaskStatusFailed,
		"error", errMsg,
		"finished_at", strconv.FormatInt(now, 10),
		"updated_at", strconv.FormatInt(now, 10),
	).Err(); err != nil {
		return err
	}
	return s.rdb.Expire(ctx, key, FinalStatusTTL*time.Second).Err()
}

func (s *StatusStore) MarkCancelled(ctx context.Context, taskID, reason string) error {
	now := time.Now().Unix()
	key := s.key(taskID)
	if err := s.rdb.HSet(ctx, key,
		"status", TaskStatusCancelled,
		"message", reason,
		"finished_at", strconv.FormatInt(now, 10),
		"updated_at", strconv.FormatInt(now, 10),
	).Err(); err != nil {
		return err
	}
	return s.rdb.Expire(ctx, key, FinalStatusTTL*time.Second).Err()
}

// CancelIfQueued atomically transitions from "queued" to "cancelled".
// Returns true if the transition happened, false if the status was not "queued".
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
	return res == 1, nil
}

func (s *StatusStore) MarkQueuedForRetry(ctx context.Context, taskID, message, errMsg string) error {
	now := time.Now().Unix()
	key := s.key(taskID)
	if err := s.rdb.HSet(ctx, key,
		"status", TaskStatusQueued,
		"message", message,
		"error", errMsg,
		"started_at", "-",
		"finished_at", "-",
		"updated_at", strconv.FormatInt(now, 10),
	).Err(); err != nil {
		return err
	}
	return s.rdb.Expire(ctx, key, RunningStatusTTL*time.Second).Err()
}

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
	if err := s.rdb.HSet(ctx, key, fields...).Err(); err != nil {
		return err
	}
	return s.rdb.Expire(ctx, key, RunningStatusTTL*time.Second).Err()
}

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

// PublishTaskEvent 向 Pub/Sub 频道发布任务状态变更事件（供 SSE 推送）
func (s *StatusStore) PublishTaskEvent(ctx context.Context, taskID string, status *TaskStatus) error {
	b, err := json.Marshal(map[string]any{
		"task_id":            taskID,
		"status":             status.Status,
		"progress":           status.Progress,
		"error":              status.Error,
		"message":            status.Message,
		"result":             status.Result,
		"created_at":         status.CreatedAt,
		"started_at":         status.StartedAt,
		"finished_at":        status.FinishedAt,
		"endpoint":           status.Endpoint,
		"priority":           status.Priority,
		"supports_progress":  status.SupportsProgress,
	})
	if err != nil {
		return err
	}
	return s.rdb.Publish(ctx, buildTaskEventsChannel(s.namespace), string(b)).Err()
}

func (s *StatusStore) publishTaskEvent(ctx context.Context, taskID string) {
	// Get the updated status and publish
	if ts, err := s.Get(ctx, taskID); err == nil && ts != nil {
		s.PublishTaskEvent(ctx, taskID, ts)
	}
}
