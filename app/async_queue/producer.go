package async_queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/huangyangke/go-aikit/app/middleware"
	"github.com/huangyangke/go-aikit/app/response"
	"github.com/huangyangke/go-aikit/log"
	"github.com/huangyangke/go-aikit/metrics"
)

// TaskEventDispatcher 管理 SSE 连接和事件路由.
type TaskEventDispatcher struct {
	rdb         *redis.Client
	channel     string
	subscribers map[string][]chan map[string]any
	mu          sync.RWMutex
	running     bool
}

func newTaskEventDispatcher(rdb *redis.Client, channel string) *TaskEventDispatcher {
	return &TaskEventDispatcher{
		rdb:         rdb,
		channel:     channel,
		subscribers: make(map[string][]chan map[string]any),
	}
}

func (d *TaskEventDispatcher) start() {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	go d.run()
}

func (d *TaskEventDispatcher) run() {
	for {
		d.runOnce()
		d.mu.RLock()
		hasSubscribers := len(d.subscribers) > 0
		d.mu.RUnlock()
		if !hasSubscribers {
			d.mu.Lock()
			d.running = false
			d.mu.Unlock()
			return
		}
		log.Warn("[Producer][events][reconnecting][channel=%s]", d.channel)
		time.Sleep(time.Second)
	}
}

func (d *TaskEventDispatcher) runOnce() {
	pubsub := d.rdb.Subscribe(context.Background(), d.channel)
	defer func() { _ = pubsub.Close() }()
	log.Info("[Producer][events][subscribe][channel=%s]", d.channel)

	ch := pubsub.Channel()
	for msg := range ch {
		var event map[string]any
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
			log.Warn("[Producer][events][decode_error][channel=%s]: %v", d.channel, err)
			continue
		}

		taskID, ok := event["task_id"].(string)
		if !ok {
			log.Warn("[Producer][events][missing_task_id][channel=%s]", d.channel)
			continue
		}

		d.mu.RLock()
		taskChans := d.subscribers[taskID]
		wildcardChans := d.subscribers["*"]
		d.mu.RUnlock()

		for _, c := range taskChans {
			select {
			case c <- event:
			default:
			}
		}
		for _, c := range wildcardChans {
			select {
			case c <- event:
			default:
			}
		}
	}
	log.Warn("[Producer][events][disconnected][channel=%s]", d.channel)
}

func (d *TaskEventDispatcher) subscribe(taskID string) chan map[string]any {
	ch := make(chan map[string]any, 16)
	d.mu.Lock()
	d.subscribers[taskID] = append(d.subscribers[taskID], ch)
	if !d.running {
		d.running = true
		go d.run()
	}
	d.mu.Unlock()
	return ch
}

func (d *TaskEventDispatcher) unsubscribe(taskID string, ch chan map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()

	chans := d.subscribers[taskID]
	newChans := make([]chan map[string]any, 0, len(chans)-1)
	for _, c := range chans {
		if c != ch {
			newChans = append(newChans, c)
		}
	}

	if len(newChans) == 0 {
		delete(d.subscribers, taskID)
	} else {
		d.subscribers[taskID] = newChans
	}
}

// Producer 接收 HTTP 请求，写入 Redis Stream，立即返回 task_id.
type Producer struct {
	rdb         *redis.Client
	cfg         RedisConfig
	endpoints   map[string]EndpointConfig
	statusStore *StatusStore
	namespace   string
	dispatcher  *TaskEventDispatcher
}

// NewProducer 创建生产者实例.
// 参数：rdb - Redis 客户端, cfg - Redis 连接配置, endpoints - 端点配置映射, family - 命名空间.
// 返回值：*Producer - 生产者实例.
func NewProducer(
	rdb *redis.Client,
	cfg RedisConfig,
	endpoints map[string]EndpointConfig,
	family string,
) *Producer {
	if family == "" {
		panic("async_queue: family (namespace) is required")
	}
	p := &Producer{
		rdb:         rdb,
		cfg:         cfg,
		endpoints:   endpoints,
		statusStore: NewStatusStore(rdb, family),
		namespace:   family,
		dispatcher:  newTaskEventDispatcher(rdb, buildTaskEventsChannel(family)),
	}
	return p
}

// RegisterRoutes 在 Gin router 上注册所有端点路由及控制路由（/status、/cancel、/events）.
// 参数：r - Gin 路由器, prefix - 路径前缀.
// 返回值：无.
func (p *Producer) RegisterRoutes(r gin.IRouter, prefix string) {
	g := r.Group(prefix)

	for ep := range p.endpoints {
		ep := ep // capture
		g.POST(ep, p.handleEnqueue(ep))
		log.Info("[Producer][route_registered][endpoint=%s%s]", prefix, ep)
	}

	g.GET("/status/:task_id", p.handleStatus)
	g.POST("/cancel/:task_id", p.handleCancel)
	g.GET("/events/:task_id", p.handleEvents)
	log.Info("[Producer][control_routes_registered][prefix=%s]", prefix)
}

func (p *Producer) handleEvents(c *gin.Context) {
	taskID := c.Param("task_id")
	log.Info("[Producer][events][connect][task_id=%s]", taskID)

	// Overall timeout to prevent slow-read attacks from holding goroutines forever.
	sseCtx, sseCancel := context.WithTimeout(c.Request.Context(), DefaultSSEMaxLifetime)
	defer sseCancel()

	// SSE response headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	// Flush headers
	c.Writer.Flush()

	// Subscribe to events
	ch := p.dispatcher.subscribe(taskID)
	defer p.dispatcher.unsubscribe(taskID, ch)

	// Send initial status
	status, err := p.statusStore.Get(sseCtx, taskID)
	if err == nil && status != nil {
		fmt.Fprintf(c.Writer, "data: %s\n\n", toJSON(taskStatusToEvent(taskID, status))) //nolint:errcheck
		c.Writer.Flush()

		// If already in terminal state, don't keep connection alive
		if status.Status == TaskStatusSuccess ||
			status.Status == TaskStatusFailed ||
			status.Status == TaskStatusCancelled {
			log.Info("[Producer][events][terminal_status][task_id=%s][status=%s]", taskID, status.Status)
			return
		}
	}

	// Send heartbeats to keep connection alive
	heartbeatTicker := time.NewTicker(TaskHeartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-sseCtx.Done():
			log.Info("[Producer][events][disconnect_or_timeout][task_id=%s]", taskID)
			return
		case <-heartbeatTicker.C:
			fmt.Fprint(c.Writer, ": heartbeat\n\n") //nolint:errcheck
			c.Writer.Flush()
		case event := <-ch:
			fmt.Fprintf(c.Writer, "data: %s\n\n", toJSON(event)) //nolint:errcheck
			c.Writer.Flush()

			if s, ok := event["status"].(string); ok && (s == TaskStatusSuccess ||
				s == TaskStatusFailed ||
				s == TaskStatusCancelled) {
				log.Info("[Producer][events][close_terminal_event][task_id=%s][status=%s]", taskID, s)
				return
			}
		}
	}
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// handleEnqueue 处理任务入队请求.
func (p *Producer) handleEnqueue(ep string) gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := middleware.GetTaskID(c.Request.Context())
		if taskID == "" {
			taskID = uuid.NewString()
		}

		// 读取原始 body（不做业务校验，由 consumer 侧 handler 处理）
		var params map[string]any
		if err := c.ShouldBindJSON(&params); err != nil {
			log.Warn("[Producer][%s][request_bind_error][task_id=%s]: %v", ep, taskID, err)
			response.ParamError(c)
			metrics.ObserveAsyncQueueEnqueue(ep, "failure")
			return
		}

		priority := parseTaskPriority(c.GetHeader("X-Task-Priority"))

		paramsJSON, err := json.Marshal(params)
		if err != nil {
			log.Error("[Producer][%s][request_marshal_error][task_id=%s]: %v", ep, taskID, err)
			response.InternalError(c)
			metrics.ObserveAsyncQueueEnqueue(ep, "failure")
			return
		}

		ctx := c.Request.Context()

		// 初始化任务状态
		if err := p.statusStore.InitQueued(ctx, taskID, ep, priority); err != nil {
			if errors.Is(err, ErrTaskAlreadyExists) {
				log.Warn("[Producer][%s][duplicate_task_id][task_id=%s]", ep, taskID)
				response.Conflict(c)
				metrics.ObserveAsyncQueueEnqueue(ep, "duplicate")
				return
			}
			log.Error("[Producer][%s][status_init_error][task_id=%s]: %v", ep, taskID, err)
			response.InternalError(c)
			metrics.ObserveAsyncQueueEnqueue(ep, "failure")
			return
		}

		// 写入 Redis Stream
		if err := p.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: buildStreamKey(p.namespace),
			Values: map[string]any{
				"task_id":  taskID,
				"endpoint": ep,
				"priority": priority,
				"params":   string(paramsJSON),
			},
		}).Err(); err != nil {
			// Clean up status if we fail to enqueue
			_ = p.statusStore.Delete(ctx, taskID)
			log.Error("[Producer][%s][enqueue_error][task_id=%s]: %v", ep, taskID, err)
			response.InternalError(c)
			metrics.ObserveAsyncQueueEnqueue(ep, "failure")
			return
		}

		// Publish the initial queued event (best-effort, off the request path)
		go func() {
			p.statusStore.publishTaskEvent(context.Background(), taskID)
		}()

		metrics.ObserveAsyncQueueEnqueue(ep, "success")
		log.Info("[Producer][%s][request][task_id=%s][priority=%d]", ep, taskID, priority)
		response.JSON(c, nil, taskID)
	}
}

// handleStatus 查询任务状态.
func (p *Producer) handleStatus(c *gin.Context) {
	taskID := c.Param("task_id")
	ts, err := p.statusStore.Get(c.Request.Context(), taskID)
	if err != nil {
		log.Error("[Producer][status][query_error][task_id=%s]: %v", taskID, err)
		response.InternalError(c)
		return
	}
	if ts == nil {
		log.Warn("[Producer][status][not_found][task_id=%s]", taskID)
		response.NotFound(c)
		return
	}
	response.JSON(c, ts, taskID)
}

// handleCancel 取消任务.
func (p *Producer) handleCancel(c *gin.Context) {
	taskID := c.Param("task_id")
	ctx := c.Request.Context()

	// Check current status first
	status, err := p.statusStore.Get(ctx, taskID)
	if err != nil {
		log.Error("[Producer][cancel][query_error][task_id=%s]: %v", taskID, err)
		response.InternalError(c)
		return
	}
	if status == nil {
		log.Warn("[Producer][cancel][not_found][task_id=%s]", taskID)
		response.NotFound(c)
		return
	}

	// Check if already in terminal state
	if status.Status == TaskStatusSuccess ||
		status.Status == TaskStatusFailed ||
		status.Status == TaskStatusCancelled {
		log.Warn("[Producer][cancel][terminal][task_id=%s][status=%s]", taskID, status.Status)
		response.Conflict(c)
		return
	}

	// Write cancel key (consumer checks)
	cancelKey := buildCancelKey(p.namespace, taskID)
	if err := p.rdb.Set(ctx, cancelKey, "1", TaskCancelTTL).Err(); err != nil {
		log.Error("[Producer][cancel][set_key_error][task_id=%s]: %v", taskID, err)
		response.InternalError(c)
		return
	}

	// Publish cancel event to Pub/Sub (Consumer subscribes)
	if err := p.rdb.Publish(ctx, buildCancelChannel(p.namespace), taskID).Err(); err != nil {
		log.Warn("[Producer][cancel][publish_error][task_id=%s]: %v", taskID, err)
	}

	// If task is still queued, atomically mark as cancelled
	if status.Status == TaskStatusQueued {
		if cancelled, _ := p.statusStore.CancelIfQueued(ctx, taskID, "user cancelled"); cancelled {
			log.Info("[Producer][cancel][queued][task_id=%s]", taskID)
		} else {
			log.Info("[Producer][cancel][raced][task_id=%s]", taskID)
		}
	} else {
		log.Info("[Producer][cancel][running][task_id=%s][status=%s]", taskID, status.Status)
	}

	response.JSON(c, nil, taskID, "取消信号已发送")
}

// parseTaskPriority 解析 X-Task-Priority header，范围 0-9，默认 5.
func parseTaskPriority(raw string) int {
	if raw == "" {
		return 5
	}
	raw = strings.TrimSpace(raw)
	if v, err := strconv.Atoi(raw); err == nil {
		if v < 0 {
			return 0
		}
		if v > 9 {
			return 9
		}
		return v
	}
	// Fall back to first digit for mixed strings
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			return int(ch - '0')
		}
	}
	return 5
}
