package async_queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/semaphore"

	"github.com/huangyangke/go-aikit/app/middleware"
	"github.com/huangyangke/go-aikit/log"
	"github.com/huangyangke/go-aikit/metrics"
)

// Consumer 从 Redis Stream 读取任务并执行，两层并发控制：
// L1 全局 semaphore（worker_capacity），L2 端点级别 ConcurrencyLimiter.
type Consumer struct {
	rdb         redis.Cmdable
	client      *redis.Client // 用于 Subscribe，Cmdable 不含该方法
	cfg         RedisConfig
	endpoints   map[string]EndpointConfig
	statusStore *StatusStore
	namespace   string

	groupName    string
	consumerName string

	scheduler SchedulerConfig
	pel       PelConfig
	limiter   ConcurrencyLimiter
	features  FeatureConfig

	sem *semaphore.Weighted // L1 全局容量

	// 运行时状态
	mu                sync.Mutex
	running           bool
	cancel            context.CancelFunc
	activeMsgIDs      map[string]struct{}
	pendingByEndpoint map[string]*EndpointPendingQueue
	pendingOrder      []string                      // endpoint 轮询顺序
	taskCancelFuncs   map[string]context.CancelFunc // task_id → cancel
	wg                sync.WaitGroup                // tracks in-flight handler goroutines for Stop drain
}

// ConsumerOption 函数式选项.
type ConsumerOption func(*Consumer)

// WithGroupName 设置消费者组名称.
// 参数：name - 消费者组名称.
// 返回值：opt - 函数式选项.
func WithGroupName(name string) ConsumerOption {
	return func(c *Consumer) { c.groupName = name }
}

// WithConsumerName 设置消费者名称.
// 参数：name - 消费者名称.
// 返回值：opt - 函数式选项.
func WithConsumerName(name string) ConsumerOption {
	return func(c *Consumer) { c.consumerName = name }
}

// WithScheduler 设置调度器配置.
// 参数：cfg - 调度器配置.
// 返回值：opt - 函数式选项.
func WithScheduler(cfg SchedulerConfig) ConsumerOption {
	return func(c *Consumer) { c.scheduler = cfg }
}

// WithPel 设置 PEL 恢复配置.
// 参数：cfg - PEL 配置.
// 返回值：opt - 函数式选项.
func WithPel(cfg PelConfig) ConsumerOption {
	return func(c *Consumer) { c.pel = cfg }
}

// WithLimiter 设置端点并发限制器.
// 参数：l - 并发限制器实例.
// 返回值：opt - 函数式选项.
func WithLimiter(l ConcurrencyLimiter) ConsumerOption {
	return func(c *Consumer) { c.limiter = l }
}

// WithFeatures 设置特性开关配置.
// 参数：f - 特性配置.
// 返回值：opt - 函数式选项.
func WithFeatures(f FeatureConfig) ConsumerOption {
	return func(c *Consumer) { c.features = f }
}

// NewConsumer 创建消费者实例.
// 参数：rdb - Redis 客户端, cfg - Redis 连接配置, endpoints - 端点配置映射, family - 命名空间, opts - 函数式选项列表.
// 返回值：*Consumer - 消费者实例.
func NewConsumer(
	rdb *redis.Client,
	cfg RedisConfig,
	endpoints map[string]EndpointConfig,
	family string,
	opts ...ConsumerOption,
) *Consumer {
	if family == "" {
		panic("async_queue: family (namespace) is required")
	}
	if err := ValidateEndpointConfig(endpoints); err != nil {
		panic(err.Error())
	}
	c := &Consumer{
		rdb:               rdb,
		client:            rdb,
		cfg:               cfg,
		endpoints:         endpoints,
		namespace:         family,
		statusStore:       NewStatusStore(rdb, family),
		groupName:         DefaultGroupName,
		consumerName:      DefaultConsumerName,
		scheduler:         defaultSchedulerConfig(),
		pel:               defaultPelConfig(),
		limiter:           &NoopConcurrencyLimiter{},
		features:          ResolveFeatureMode(FeatureModeFull, nil),
		activeMsgIDs:      make(map[string]struct{}),
		pendingByEndpoint: make(map[string]*EndpointPendingQueue),
		taskCancelFuncs:   make(map[string]context.CancelFunc),
	}
	for _, opt := range opts {
		opt(c)
	}
	// Guard against WithScheduler(SchedulerConfig{}) leaving WorkerCapacity at 0,
	// which would make semaphore.Acquire block forever.
	if c.scheduler.WorkerCapacity <= 0 {
		c.scheduler.WorkerCapacity = DefaultWorkerCapacity
	}
	if err := ValidateSchedulerConfig(c.scheduler); err != nil {
		panic(err.Error())
	}
	if err := ValidatePelConfig(c.pel); err != nil {
		panic(err.Error())
	}
	c.sem = semaphore.NewWeighted(int64(c.scheduler.WorkerCapacity))
	return c
}

// Start 启动消费循环（阻塞，应在 goroutine 中调用）.
// 参数：ctx - 上下文，取消时停止消费.
// 返回值：err - 启动失败时的错误.
func (c *Consumer) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.running = true
	c.cancel = cancel
	c.mu.Unlock()

	consumerName := fmt.Sprintf("%d_%s_%s", os.Getpid(), c.consumerName, uuid.NewString()[:8])
	log.Info("[Consumer][start][consumer=%s][group=%s][stream=%s][namespace=%s][worker_capacity=%d]",
		consumerName, c.groupName, buildStreamKey(c.namespace), c.namespace, c.scheduler.WorkerCapacity)

	// 创建 Consumer Group（幂等）
	if err := c.createGroup(ctx); err != nil {
		log.Error("[Consumer][create_group_error][consumer=%s][group=%s][stream=%s]: %v",
			consumerName, c.groupName, buildStreamKey(c.namespace), err)
		return err
	}

	// 启动心跳
	if c.features.EnableHeartbeat {
		go c.runHeartbeat(ctx, consumerName)
	}

	// 启动取消订阅
	if c.features.EnableCancel {
		go c.runCancelSubscriber(ctx)
	}

	// 启动时做一次 PEL 恢复
	if c.features.EnablePelRecovery {
		if err := c.recoverPending(ctx, consumerName); err != nil {
			// PEL 恢复失败不阻断主流程
			log.Warn("[Consumer][PEL_recovery][startup_error][consumer=%s]: %v", consumerName, err)
		}
	}

	return c.loop(ctx, consumerName)
}

// Stop 发送停止信号并等待在途任务完成，超过 DefaultStopTimeout 后强制退出.
func (c *Consumer) Stop() {
	c.mu.Lock()
	c.running = false
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()

	drained := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(DefaultStopTimeout):
		log.Warn("[Consumer][stop][drain_timeout][consumer=%s][group=%s]", c.consumerName, c.groupName)
	}
	log.Info("[Consumer][stop][consumer=%s][group=%s][stream=%s]", c.consumerName, c.groupName, buildStreamKey(c.namespace))
}

// ================================
// 主循环
// ================================.

func (c *Consumer) loop(ctx context.Context, consumerName string) error {
	maxPending := DefaultMaxPendingMultiplier * c.scheduler.WorkerCapacity
	var nextPelRecovery time.Time
	if c.features.EnablePelRecovery && !c.pel.ScanOnStartupOnly {
		nextPelRecovery = time.Now().Add(c.pel.MinIdle)
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("[Consumer][loop][stopped][consumer=%s]", consumerName)
			return nil
		default:
		}

		// PEL 定期恢复
		if !nextPelRecovery.IsZero() && time.Now().After(nextPelRecovery) {
			if err := c.recoverPending(ctx, consumerName); err != nil {
				log.Warn("[Consumer][PEL_recovery][scheduled_error][consumer=%s]: %v", consumerName, err)
			}
			nextPelRecovery = time.Now().Add(c.pel.MinIdle)
		}

		// L1 满载：等待任意任务完成（用一个小 sleep 轮询，简单且够用）
		if !c.sem.TryAcquire(1) {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		c.sem.Release(1) // 先释放，后续 handleTask 中会再 Acquire

		// 尝试从 pending 队列调度
		if scheduled := c.drainPending(ctx); scheduled > 0 {
			continue
		}

		// pending 有积压但调度不了：等待
		c.mu.Lock()
		totalPending := 0
		for _, q := range c.pendingByEndpoint {
			totalPending += q.Len()
		}
		c.mu.Unlock()

		if totalPending > 0 {
			if totalPending >= maxPending {
				time.Sleep(100 * time.Millisecond)
			} else {
				time.Sleep(10 * time.Millisecond)
			}
			continue
		}

		// 计算可拉取数量
		available := int64(c.scheduler.WorkerCapacity) - c.runningCount()
		if available <= 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		pullCount := int64(DefaultPullCount)
		if available < pullCount {
			pullCount = available
		}

		// 拉取新消息
		msgs, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.groupName,
			Consumer: consumerName,
			Streams:  []string{buildStreamKey(c.namespace), StreamNewMessagesID},
			Count:    pullCount,
			Block:    DefaultPullBlock,
		}).Result()
		if err != nil && err != redis.Nil {
			log.Warn("[Consumer][xreadgroup_error][consumer=%s][stream=%s]: %v", consumerName, buildStreamKey(c.namespace), err)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range msgs {
			for _, msg := range stream.Messages {
				c.mu.Lock()
				c.activeMsgIDs[msg.ID] = struct{}{}
				c.mu.Unlock()
				c.spawnAdmit(ctx, msg.ID, toStringMap(msg.Values))
			}
		}
	}
}

// ================================
// 消息调度
// ================================.

// spawnAdmit launches admitMessage in a tracked goroutine so Stop() can drain it.
// Skips execution if the consumer has already stopped.
func (c *Consumer) spawnAdmit(ctx context.Context, msgID string, data map[string]any) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.mu.Lock()
		running := c.running
		c.mu.Unlock()
		// best-effort guard: Stop() may set running=false between this check
		// and admitMessage execution. wg.Wait+StopTimeout provides the hard
		// guarantee; this check simply reduces wasted work.
		if !running {
			return
		}
		c.admitMessage(ctx, msgID, data)
	}()
}

// spawnHandleTask launches handleTask in a tracked goroutine so Stop() can drain it.
// Skips execution if the consumer has already stopped.
func (c *Consumer) spawnHandleTask(ctx context.Context, msgID string, data map[string]any) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.mu.Lock()
		running := c.running
		c.mu.Unlock()
		// best-effort guard (same rationale as spawnAdmit).
		if !running {
			return
		}
		c.handleTask(ctx, msgID, data)
	}()
}

func (c *Consumer) admitMessage(ctx context.Context, msgID string, data map[string]interface{}) {
	endpoint, _ := data["endpoint"].(string)
	priority := extractPriority(data)

	ok, err := c.limiter.TryAcquire(ctx, endpoint)
	if err != nil || !ok {
		if err != nil {
			log.Warn("[Consumer][%s][limiter_acquire_error][msg_id=%s]: %v", endpoint, msgID, err)
		} else {
			log.Debug("[Consumer][%s][pending][msg_id=%s][priority=%d]", endpoint, msgID, priority)
		}
		c.mu.Lock()
		q, exists := c.pendingByEndpoint[endpoint]
		if !exists {
			q = &EndpointPendingQueue{}
			c.pendingByEndpoint[endpoint] = q
			c.pendingOrder = append(c.pendingOrder, endpoint)
		}
		q.Push(priority, msgID, data)
		c.mu.Unlock()
		return
	}

	c.spawnHandleTask(ctx, msgID, data)
}

func (c *Consumer) drainPending(ctx context.Context) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	scheduled := 0
	attempts := len(c.pendingOrder)
	newOrder := c.pendingOrder[:0]

	for i := 0; i < attempts; i++ {
		if len(c.pendingOrder) == 0 {
			break
		}
		endpoint := c.pendingOrder[0]
		c.pendingOrder = c.pendingOrder[1:]

		q := c.pendingByEndpoint[endpoint]
		if q == nil || q.IsEmpty() {
			delete(c.pendingByEndpoint, endpoint)
			continue
		}

		msgID, data, ok := q.Peek()
		if !ok {
			continue
		}

		acquired, _ := c.limiter.TryAcquire(ctx, endpoint)
		if acquired {
			q.Pop()
			if !q.IsEmpty() {
				newOrder = append(newOrder, endpoint)
			} else {
				delete(c.pendingByEndpoint, endpoint)
			}
			c.spawnHandleTask(ctx, msgID, data)
			scheduled++
		} else {
			newOrder = append(newOrder, endpoint)
		}
	}
	c.pendingOrder = append(newOrder, c.pendingOrder...)
	return scheduled
}

// ================================
// 任务处理
// ================================.

func (c *Consumer) handleTask(ctx context.Context, msgID string, data map[string]any) {
	// L1 获取全局容量
	if err := c.sem.Acquire(ctx, 1); err != nil {
		log.Warn("[Consumer][semaphore_acquire_error][msg_id=%s]: %v", msgID, err)
		return
	}
	defer c.sem.Release(1)

	startTime := time.Now()
	taskID, _ := data["task_id"].(string)
	endpoint, _ := data["endpoint"].(string)
	paramsStr, _ := data["params"].(string)
	log.Info("[Consumer][%s][begin][task_id=%s][msg_id=%s]", endpoint, taskID, msgID)

	// 注册取消
	taskCtx, cancelFn := context.WithCancel(ctx)
	taskCtx = middleware.WithTaskID(taskCtx, taskID)
	c.mu.Lock()
	c.taskCancelFuncs[taskID] = cancelFn
	c.mu.Unlock()
	defer func() {
		cancelFn()
		c.mu.Lock()
		delete(c.taskCancelFuncs, taskID)
		delete(c.activeMsgIDs, msgID)
		c.mu.Unlock()
		// Use Background context for cleanup so shutdown/timeout cancellation
		// doesn't prevent releasing the distributed limiter counter.
		_ = c.limiter.Release(context.Background(), endpoint)
	}()

	var params map[string]any
	if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
		params = map[string]any{}
		log.Warn("[Consumer][%s][params_decode_error][task_id=%s][msg_id=%s]: %v", endpoint, taskID, msgID, err)
	}

	// 取消检查
	if c.cancelKeyExists(ctx, taskID) {
		_ = c.statusStore.MarkCancelled(ctx, taskID, "用户取消")
		if err := c.ackAndDel(ctx, msgID); err != nil {
			log.Error("[Consumer][%s][ack_and_del_error][task_id=%s][msg_id=%s]: %v", endpoint, taskID, msgID, err)
		}
		metrics.ObserveAsyncQueueConsume(endpoint, "cancelled", 0*time.Second)
		log.Info("[Consumer][%s][cancelled_before_run][task_id=%s][msg_id=%s]", endpoint, taskID, msgID)
		return
	}

	if err := c.statusStore.MarkRunning(ctx, taskID); err != nil {
		log.Warn("[Consumer][%s][status_running_error][task_id=%s]: %v", endpoint, taskID, err)
	} else {
		log.Debug("[Consumer][%s][status_running][task_id=%s]", endpoint, taskID)
	}

	epCfg, ok := c.endpoints[endpoint]
	if !ok {
		_ = c.statusStore.MarkFailed(ctx, taskID, fmt.Sprintf("unknown endpoint: %s", endpoint))
		if err := c.ackAndDel(ctx, msgID); err != nil {
			log.Error("[Consumer][%s][ack_and_del_error][task_id=%s][msg_id=%s]: %v", endpoint, taskID, msgID, err)
		}
		metrics.ObserveAsyncQueueConsume(endpoint, "failure", 0*time.Second)
		log.Error("[Consumer][%s][unknown_endpoint][task_id=%s][msg_id=%s]", endpoint, taskID, msgID)
		return
	}

	taskContext := newTaskContext(taskCtx, taskID, endpoint, params, c.statusStore)

	var (
		result any
		runErr error
	)

	if epCfg.Timeout > 0 {
		timeoutCtx, timeoutCancel := context.WithTimeout(taskCtx, epCfg.Timeout)
		defer timeoutCancel()
		taskContext = newTaskContext(timeoutCtx, taskID, endpoint, params, c.statusStore)

		// Handlers must observe ctx cancellation; running synchronously avoids
		// duplicate task execution and data races when timeout retry is enabled.
		result, runErr = epCfg.Handler(taskContext)
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			runErr = context.DeadlineExceeded
			if epCfg.RetryOnTimeout {
				_ = c.statusStore.MarkQueuedForRetry(ctx, taskID, "任务超时，等待重试", "task execution timed out")
				c.mu.Lock()
				delete(c.activeMsgIDs, msgID)
				c.mu.Unlock()
				log.Warn("[Consumer][%s][timeout_retry][task_id=%s][timeout=%s]", endpoint, taskID, epCfg.Timeout)
				return
			}
		}
	} else {
		// 无超时执行
		result, runErr = epCfg.Handler(taskContext)
	}

	// handler 完成后再检查一次取消
	if c.cancelKeyExists(ctx, taskID) {
		_ = c.statusStore.MarkCancelled(ctx, taskID, "用户取消")
		if err := c.ackAndDel(ctx, msgID); err != nil {
			log.Error("[Consumer][%s][ack_and_del_error][task_id=%s][msg_id=%s]: %v", endpoint, taskID, msgID, err)
		}
		metrics.ObserveAsyncQueueConsume(endpoint, "cancelled", 0*time.Second)
		log.Info("[Consumer][%s][cancelled_after_handler][task_id=%s][msg_id=%s]", endpoint, taskID, msgID)
		return
	}

	var consumeResult string
	if runErr != nil {
		if err := c.statusStore.MarkFailed(ctx, taskID, runErr.Error()); err != nil {
			log.Warn("[Consumer][%s][status_failed_error][task_id=%s]: %v", endpoint, taskID, err)
		}
		if epCfg.CallBack != nil {
			if err := epCfg.CallBack(&TaskResponse{TaskID: taskID, Endpoint: endpoint, Err: runErr}); err != nil {
				log.Error("[Consumer][%s][call_back_error][task_id=%s]: %v", endpoint, taskID, err)
			}
		}
		consumeResult = "failure"
		if errors.Is(runErr, context.DeadlineExceeded) {
			consumeResult = "timeout"
		}
		log.Error("[Consumer][%s][%s][task_id=%s][duration=%s]: %v",
			endpoint, consumeResult, taskID, time.Since(startTime), runErr)
	} else {
		if err := c.statusStore.MarkSuccess(ctx, taskID, result); err != nil {
			log.Warn("[Consumer][%s][status_success_error][task_id=%s]: %v", endpoint, taskID, err)
		} else {
			log.Debug("[Consumer][%s][status_success][task_id=%s]", endpoint, taskID)
		}
		if epCfg.CallBack != nil {
			if err := epCfg.CallBack(&TaskResponse{TaskID: taskID, Endpoint: endpoint, Data: result}); err != nil {
				log.Error("[Consumer][%s][call_back_error][task_id=%s]: %v", endpoint, taskID, err)
			}
		}
		consumeResult = "success"
		log.Info("[Consumer][%s][success][task_id=%s][duration=%s]", endpoint, taskID, time.Since(startTime))
	}

	if err := c.ackAndDel(ctx, msgID); err != nil {
		log.Error("[Consumer][%s][ack_and_del_error][task_id=%s][msg_id=%s]: %v", endpoint, taskID, msgID, err)
	} else {
		log.Debug("[Consumer][%s][ack_and_del][task_id=%s][msg_id=%s]", endpoint, taskID, msgID)
	}

	// 记录任务消费指标
	metrics.ObserveAsyncQueueConsume(endpoint, consumeResult, time.Since(startTime))
}

// ================================
// 心跳 & 取消订阅
// ================================.

func (c *Consumer) runHeartbeat(ctx context.Context, consumerName string) {
	key := buildHeartbeatKey(c.namespace, c.groupName, consumerName)
	// Heartbeat window: min(pel.MinIdle/2, ConsumerHeartbeatMaxWindow), matching Python
	ttl := c.pel.MinIdle / 2
	if ttl > ConsumerHeartbeatMaxWindow {
		ttl = ConsumerHeartbeatMaxWindow
	}
	if ttl < 5*time.Second {
		ttl = 5 * time.Second
	}
	ticker := time.NewTicker(ttl / 3)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = c.rdb.Del(context.Background(), key)
			log.Debug("[Consumer][heartbeat][stopped][key=%s]", key)
			return
		case <-ticker.C:
			if err := c.rdb.Set(ctx, key, time.Now().Unix(), ttl).Err(); err != nil {
				log.Warn("[Consumer][heartbeat][set_error][key=%s]: %v", key, err)
			}
		}
	}
}

func (c *Consumer) runCancelSubscriber(ctx context.Context) {
	if c.client == nil {
		log.Warn("[Consumer][cancel_subscriber][skip_no_client]")
		return
	}
	pubsub := c.client.Subscribe(ctx, buildCancelChannel(c.namespace))
	defer func() { _ = pubsub.Close() }()
	log.Info("[Consumer][cancel_subscriber][subscribe][channel=%s]", buildCancelChannel(c.namespace))
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			taskID := msg.Payload
			c.mu.Lock()
			if fn, exists := c.taskCancelFuncs[taskID]; exists {
				fn()
				log.Info("[Consumer][cancel_event][cancel][task_id=%s]", taskID)
			}
			c.mu.Unlock()
		}
	}
}

// ================================
// PEL 恢复
// ================================.

func (c *Consumer) recoverPending(ctx context.Context, consumerName string) error {
	streamKey := buildStreamKey(c.namespace)
	start := "-"
	claimedCount := 0
	poisonedCount := 0

	for {
		pendingInfo, err := c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: streamKey,
			Group:  c.groupName,
			Start:  start,
			End:    "+",
			Count:  100,
		}).Result()
		if err != nil {
			log.Warn("[Consumer][PEL_recovery][xpending_error][consumer=%s]: %v", consumerName, err)
			break
		}
		if len(pendingInfo) == 0 {
			break
		}

		var eligibleIDs []string
		deliveryCounts := map[string]int64{}

		for _, entry := range pendingInfo {
			start = entry.ID // always advance cursor to avoid infinite loop
			if entry.Idle < c.pel.MinIdle {
				continue
			}
			// 自己的消息或死亡消费者的消息
			if entry.Consumer != consumerName {
				alive, _ := c.rdb.Exists(ctx,
					buildHeartbeatKey(c.namespace, c.groupName, entry.Consumer),
				).Result()
				if alive > 0 {
					continue
				}
			}
			eligibleIDs = append(eligibleIDs, entry.ID)
			deliveryCounts[entry.ID] = entry.RetryCount
		}

		if len(eligibleIDs) > 0 {
			claimed, err := c.rdb.XClaim(ctx, &redis.XClaimArgs{
				Stream:   streamKey,
				Group:    c.groupName,
				Consumer: consumerName,
				MinIdle:  c.pel.MinIdle,
				Messages: eligibleIDs,
			}).Result()
			if err == nil {
				for _, msg := range claimed {
					count := deliveryCounts[msg.ID]
					endpoint, _ := msg.Values["endpoint"].(string)
					if int(count) >= c.pel.MaxRetries {
						taskID, _ := msg.Values["task_id"].(string)
						_ = c.statusStore.MarkFailed(ctx, taskID,
							fmt.Sprintf("PEL recovery: exceeded max retries (%d)", count))
						if err := c.ackAndDel(ctx, msg.ID); err != nil {
							log.Error("[Consumer][PEL_recovery][ack_and_del_error][task_id=%s][msg_id=%s]: %v", taskID, msg.ID, err)
						}
						c.sendToDeadLetter(ctx, msg)
						metrics.ObserveAsyncQueueConsume(endpoint, "poisoned", 0)
						poisonedCount++
						log.Warn("[Consumer][PEL_recovery][poisoned][task_id=%s][msg_id=%s][retry_count=%d]", taskID, msg.ID, count)
						continue
					}
					c.mu.Lock()
					c.activeMsgIDs[msg.ID] = struct{}{}
					c.mu.Unlock()
					c.spawnAdmit(ctx, msg.ID, toStringMap(msg.Values))
					metrics.ObserveAsyncQueueConsume(endpoint, "recovered", 0)
					claimedCount++
				}
			} else {
				log.Warn("[Consumer][PEL_recovery][xclaim_error][consumer=%s][count=%d]: %v", consumerName, len(eligibleIDs), err)
			}
		}

		if len(pendingInfo) < 100 {
			break
		}
	}
	if claimedCount > 0 || poisonedCount > 0 {
		log.Info("[Consumer][PEL_recovery][done][consumer=%s][claimed=%d][poisoned=%d]", consumerName, claimedCount, poisonedCount)
	}
	return nil
}

// ================================
// 辅助
// ================================.

func (c *Consumer) createGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, buildStreamKey(c.namespace), c.groupName, StreamGroupStartID).Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (c *Consumer) ackAndDel(ctx context.Context, msgID string) error {
	pipe := c.rdb.Pipeline()
	pipe.XAck(ctx, buildStreamKey(c.namespace), c.groupName, msgID)
	pipe.XDel(ctx, buildStreamKey(c.namespace), msgID)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *Consumer) cancelKeyExists(ctx context.Context, taskID string) bool {
	n, err := c.rdb.Exists(ctx, buildCancelKey(c.namespace, taskID)).Result()
	return err == nil && n > 0
}

func (c *Consumer) runningCount() int64 {
	// 用 sem 剩余量反推已使用量
	// semaphore 没有直接暴露当前计数，用一个简单方式：尝试 acquire 0
	// 实际上我们用 activeMsgIDs 的长度作为近似值
	c.mu.Lock()
	defer c.mu.Unlock()
	return int64(len(c.activeMsgIDs))
}

func extractPriority(data map[string]interface{}) int {
	if v, ok := data["priority"]; ok {
		switch p := v.(type) {
		case int:
			return clampPriority(p)
		case int64:
			return clampPriority(int(p))
		case float64:
			return clampPriority(int(p))
		case string:
			p = strings.TrimSpace(p)
			if n, err := strconv.Atoi(p); err == nil {
				return clampPriority(n)
			}
			// Fall back to first digit for mixed strings like "3-high"
			for _, ch := range p {
				if ch >= '0' && ch <= '9' {
					return int(ch - '0')
				}
			}
		}
	}
	return 5
}

func clampPriority(v int) int {
	if v < 0 {
		return 0
	}
	if v > 9 {
		return 9
	}
	return v
}

func toStringMap(data map[string]interface{}) map[string]any {
	out := make(map[string]any, len(data))
	for k, v := range data {
		out[k] = v
	}
	return out
}
