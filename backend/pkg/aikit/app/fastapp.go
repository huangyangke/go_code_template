// Package app 一站式应用编排框架，整合 Gin 服务器、中间件、异步队列与优雅停机.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/huangyangke/go-aikit/app/async_queue"
	"github.com/huangyangke/go-aikit/app/auth"
	"github.com/huangyangke/go-aikit/app/health"
	"github.com/huangyangke/go-aikit/app/httpclient"
	"github.com/huangyangke/go-aikit/app/middleware"
	"github.com/huangyangke/go-aikit/app/xjob"
	"github.com/huangyangke/go-aikit/cache"
	"github.com/huangyangke/go-aikit/config"
	dbmysql "github.com/huangyangke/go-aikit/database/mysql"
	dbpulsar "github.com/huangyangke/go-aikit/database/pulsar"
	dbredis "github.com/huangyangke/go-aikit/database/redis"
	"github.com/huangyangke/go-aikit/log"
	"github.com/huangyangke/go-aikit/metrics"

	"github.com/xxl-job/xxl-job-executor-go"
)

// FastAppConfig FastApp 配置.
type FastAppConfig struct {
	Family string
	Host   string
	Port   int
	Mode   string // "debug" | "release" | "test"
}

// ServerConfig HTTP 服务器调优参数.
type ServerConfig struct {
	ReadTimeout     time.Duration // default 30s
	WriteTimeout    time.Duration // default 0 (disabled) — set non-zero only when SSE / long-poll is not used
	IdleTimeout     time.Duration // default 120s
	ShutdownTimeout time.Duration // default 30s
}

// MiddlewareConfig 内置中间件开关与配置.
type MiddlewareConfig struct {
	EnableRequestID  bool
	EnableRequestLog bool
	EnablePrometheus bool
	DisableCORS      bool // Set true to disable CORS; CORS is enabled by default (matching Python)
	CORSConfig       middleware.CORSConfig
	EnableTokenAuth  bool
	TokenVerifyFunc  middleware.VerifyFunc
	// ExtraVerifyFunc is an additional verify function composed with TokenVerifyFunc via OR logic:
	// the request is accepted if either function returns true. Typically used to layer a
	// third-party JWT verifier on top of the built-in aikit JWT verifier.
	ExtraVerifyFunc middleware.VerifyFunc
	// InternalToken is a static token for internal service-to-service calls.
	// Requests carrying this token bypass all other verification.
	// Use constant-time comparison is applied automatically — safe against timing attacks.
	// Load from an environment variable; rotate immediately on leakage.
	InternalToken   string
	TokenWhitelist  []string
	EnableRateLimit bool
	RateLimitRDB    redis.Cmdable
	RateLimitConfig middleware.RateLimitConfig
	EnablePprof     bool
	EnableSwagger   bool
}

// AsyncQueueConfig 异步队列集成配置.
type AsyncQueueConfig struct {
	RedisClient      *redis.Client
	RedisConfig      async_queue.RedisConfig
	Endpoints        map[string]async_queue.EndpointConfig
	Prefix           string
	GroupName        string
	ConsumerName     string
	SchedulerConfig  *async_queue.SchedulerConfig
	PelConfig        *async_queue.PelConfig
	EndpointLimitCfg *async_queue.EndpointLimitConfig
	// FeatureMode sets the feature preset (Lite / Standard / Full).
	// Defaults to FeatureModeFull when zero. Use FeatureOverrides for fine-grained control.
	FeatureMode async_queue.FeatureMode
	// FeatureOverrides selectively enables features on top of FeatureMode.
	FeatureOverrides *async_queue.FeatureConfig
}

// XxlJobConfig XXL-Job 集成配置.
type XxlJobConfig struct {
	ServerAddr   string
	AccessToken  string
	ExecutorIp   string
	ExecutorPort string
	LogDir       string
	MaxAge       int
	JobDisabled  bool
	Extra        map[string]string
}

// FastApp 一站式应用编排，整合 Gin HTTP 服务器、中间件、异步队列、资源管理、XXL-Job 和优雅停机.
type FastApp struct {
	cfg    FastAppConfig
	engine *gin.Engine
	server *http.Server

	// server config
	svrCfg ServerConfig

	// middleware config
	mwCfg MiddlewareConfig

	// auth
	authManager *auth.Manager

	// async queue
	aqCfg    *AsyncQueueConfig
	producer *async_queue.Producer
	consumer *async_queue.Consumer

	// config loader
	configLoader *config.ConfigLoader

	// resources
	redisInstances      map[string]*dbredis.Redis
	mysqlInstances      map[string]*dbmysql.Database
	cacheInstances      map[string]*cache.MultiLevelCache
	httpClientInstances map[string]*httpclient.Client
	pulsarInstances     map[string]*dbpulsar.Client

	// xxl-job
	xjobConfig   *XxlJobConfig
	xjobExecutor *xjob.Executor
	xjobTasks    []*xjob.Task

	// lifecycle hooks
	onStart []func(ctx context.Context) error
	onStop  []func(ctx context.Context) error

	// route registration
	routeRegistrar func(r *gin.Engine)
}

// NewFastApp 创建 FastApp 实例，补全默认 Host/Port/Mode.
// 参数：cfg - FastApp 配置.
// 返回值：*FastApp - 初始化完成的实例.
func NewFastApp(cfg FastAppConfig) *FastApp {
	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.Mode == "" {
		cfg.Mode = gin.ReleaseMode
	}

	gin.SetMode(cfg.Mode)

	return &FastApp{
		cfg:                 cfg,
		engine:              gin.New(),
		redisInstances:      make(map[string]*dbredis.Redis),
		mysqlInstances:      make(map[string]*dbmysql.Database),
		cacheInstances:      make(map[string]*cache.MultiLevelCache),
		httpClientInstances: make(map[string]*httpclient.Client),
		pulsarInstances:     make(map[string]*dbpulsar.Client),
	}
}

// Engine 返回底层 Gin 引擎，用于自定义路由注册.
// 参数：无.
// 返回值：*gin.Engine - Gin 引擎实例.
func (a *FastApp) Engine() *gin.Engine {
	return a.engine
}

// SetRouteRegistrar 设置自定义路由注册函数，在中间件和内置端点之后调用.
// 参数：fn - 路由注册函数.
func (a *FastApp) SetRouteRegistrar(fn func(r *gin.Engine)) {
	a.routeRegistrar = fn
}

// SetServer 设置 HTTP 服务器调优参数.
// 参数：cfg - 服务器配置.
func (a *FastApp) SetServer(cfg ServerConfig) {
	a.svrCfg = cfg
}

// applyServerDefaults fills zero-value timeouts with sensible defaults.
// Idempotent: safe to call multiple times.
func (a *FastApp) applyServerDefaults() {
	if a.svrCfg.ReadTimeout == 0 {
		a.svrCfg.ReadTimeout = 30 * time.Second
	}
	// WriteTimeout defaults to 0 (disabled). http.Server interprets 0 as no
	// timeout, which is required for SSE and long-poll connections. Set an
	// explicit value only when the service has no long-lived response streams.
	if a.svrCfg.IdleTimeout == 0 {
		a.svrCfg.IdleTimeout = 120 * time.Second
	}
	if a.svrCfg.ShutdownTimeout == 0 {
		a.svrCfg.ShutdownTimeout = 30 * time.Second
	}
}

// SetMiddlewares 设置内置中间件配置.
// 参数：cfg - 中间件配置.
func (a *FastApp) SetMiddlewares(cfg MiddlewareConfig) {
	a.mwCfg = cfg
}

// SetAuth 注册认证管理器及其路由，须在 Run 之前调用.
// 参数：m - 认证管理器.
func (a *FastApp) SetAuth(m *auth.Manager) {
	a.authManager = m
}

// AuthManager 返回已注册的认证管理器，未注册时返回 nil.
// 参数：无.
// 返回值：*auth.Manager - 认证管理器实例.
func (a *FastApp) AuthManager() *auth.Manager {
	return a.authManager
}

// SetAsyncQueue 设置异步队列集成配置.
// 参数：cfg - 异步队列配置.
func (a *FastApp) SetAsyncQueue(cfg AsyncQueueConfig) {
	a.aqCfg = &cfg
}

// SetConfigLoader 设置配置加载器.
// 参数：loader - 配置加载器.
func (a *FastApp) SetConfigLoader(loader *config.ConfigLoader) {
	a.configLoader = loader
}

// OnStart 注册启动钩子，Run 时依次执行.
// 参数：fn - 启动钩子函数.
func (a *FastApp) OnStart(fn func(ctx context.Context) error) {
	a.onStart = append(a.onStart, fn)
}

// OnStop 注册停机钩子，优雅停机时依次执行.
// 参数：fn - 停机钩子函数.
func (a *FastApp) OnStop(fn func(ctx context.Context) error) {
	a.onStop = append(a.onStop, fn)
}

// SetXxlJob 设置 XXL-Job 集成配置.
// 参数：cfg - XXL-Job 配置.
func (a *FastApp) SetXxlJob(cfg XxlJobConfig) {
	a.xjobConfig = &cfg
}

// RegisterXxlJobTask 注册 XXL-Job 任务.
// 参数：pattern - 任务标识, taskFunc - 任务执行函数.
func (a *FastApp) RegisterXxlJobTask(pattern string, taskFunc xxl.TaskFunc) {
	a.xjobTasks = append(a.xjobTasks, xjob.NewTask(pattern, taskFunc))
}

// GetXxlJob 返回 XXL-Job 执行器，未配置时返回 nil.
// 参数：无.
// 返回值：*xjob.Executor - XXL-Job 执行器.
func (a *FastApp) GetXxlJob() *xjob.Executor {
	return a.xjobExecutor
}

// RegisterRedis 注册命名 Redis 实例.
// 参数：name - 实例名称, cfg - Redis 配置.
// 返回值：*dbredis.Redis - Redis 实例.
func (a *FastApp) RegisterRedis(name string, cfg *dbredis.Config) *dbredis.Redis {
	if cfg.Name == "" {
		cfg.Name = a.cfg.Family + "/" + name
	}
	rdb := dbredis.MustNew(cfg)
	a.redisInstances[name] = rdb
	return rdb
}

// GetRedis 返回命名 Redis 实例，未注册时返回 nil.
// 参数：name - 实例名称.
// 返回值：*dbredis.Redis - Redis 实例.
func (a *FastApp) GetRedis(name string) *dbredis.Redis {
	return a.redisInstances[name]
}

// RegisterMySQL 注册命名 MySQL 实例.
// 参数：name - 实例名称, cfg - MySQL 配置, opts - 可选扩展选项.
// 返回值：*dbmysql.Database - MySQL 实例.
func (a *FastApp) RegisterMySQL(name string, cfg *dbmysql.Config, opts ...dbmysql.Option) *dbmysql.Database {
	if cfg.Name == "" {
		cfg.Name = a.cfg.Family + "/" + name
	}
	db := dbmysql.MustNew(cfg, opts...)
	a.mysqlInstances[name] = db
	return db
}

// GetMySQL 返回命名 MySQL 实例，未注册时返回 nil.
// 参数：name - 实例名称.
// 返回值：*dbmysql.Database - MySQL 实例.
func (a *FastApp) GetMySQL(name string) *dbmysql.Database {
	return a.mysqlInstances[name]
}

// RegisterCache 注册命名多级缓存实例，自动复用同名 Redis 作为远端后端.
// 参数：name - 实例名称, cfg - 缓存配置.
// 返回值：*cache.MultiLevelCache - 缓存实例.
func (a *FastApp) RegisterCache(name string, cfg cache.Config) *cache.MultiLevelCache {
	if cfg.Family == "" {
		cfg.Family = a.cfg.Family
	}
	if cfg.Name == "" {
		cfg.Name = name
	}
	if cfg.RedisCmdable == nil {
		if rdb, ok := a.redisInstances[name]; ok {
			cfg.RedisCmdable = rdb.Cmdable()
		}
	}
	c, err := cache.New(cfg)
	if err != nil {
		panic(fmt.Sprintf("cache %q config error: %v", name, err))
	}
	a.cacheInstances[name] = c
	return c
}

// GetCache 返回命名缓存实例，未注册时返回 nil.
// 参数：name - 实例名称.
// 返回值：*cache.MultiLevelCache - 缓存实例.
func (a *FastApp) GetCache(name string) *cache.MultiLevelCache {
	return a.cacheInstances[name]
}

// RegisterHTTPClient 注册命名 HTTP 客户端实例.
// 参数：name - 实例名称, cfg - HTTP 客户端配置, opts - 可选扩展选项.
// 返回值：*httpclient.Client - HTTP 客户端实例.
func (a *FastApp) RegisterHTTPClient(name string, cfg httpclient.Config, opts ...httpclient.Option) *httpclient.Client {
	c := httpclient.New(cfg, opts...)
	a.httpClientInstances[name] = c
	return c
}

// GetHTTPClient 返回命名 HTTP 客户端实例，未注册时返回 nil.
// 参数：name - 实例名称.
// 返回值：*httpclient.Client - HTTP 客户端实例.
func (a *FastApp) GetHTTPClient(name string) *httpclient.Client {
	return a.httpClientInstances[name]
}

// RegisterPulsar 注册命名 Pulsar 客户端实例.
// 参数：name - 实例名称, cfg - Pulsar 配置.
// 返回值：*dbpulsar.Client - Pulsar 客户端实例.
func (a *FastApp) RegisterPulsar(name string, cfg *dbpulsar.Config) *dbpulsar.Client {
	if cfg.Name == "" {
		cfg.Name = a.cfg.Family + "/" + name
	}
	client := dbpulsar.New(cfg)
	a.pulsarInstances[name] = client
	return client
}

// GetPulsar 返回命名 Pulsar 客户端实例，未注册时返回 nil.
// 参数：name - 实例名称.
// 返回值：*dbpulsar.Client - Pulsar 客户端实例.
func (a *FastApp) GetPulsar(name string) *dbpulsar.Client {
	return a.pulsarInstances[name]
}

// ConfigLoader 返回已注册的配置加载器，未注册时返回 nil.
// 参数：无.
// 返回值：*config.ConfigLoader - 配置加载器实例.
func (a *FastApp) ConfigLoader() *config.ConfigLoader {
	return a.configLoader
}

// Family 返回服务家族名称.
// 参数：无.
// 返回值：string - 家族名称.
func (a *FastApp) Family() string {
	return a.cfg.Family
}

// buildMiddlewareChain constructs the middleware chain in execution order:
// Recovery (outermost) -> Prometheus -> RequestID -> RequestLog -> CORS -> RateLimit -> TokenAuth (innermost).
func (a *FastApp) buildMiddlewareChain() {
	// Recovery must be outermost so panics in any subsequent middleware are caught.
	a.engine.Use(gin.Recovery())

	if a.mwCfg.EnablePrometheus {
		a.engine.Use(middleware.Prometheus())
	}
	if a.mwCfg.EnableRequestID {
		a.engine.Use(middleware.RequestID())
	}
	if a.mwCfg.EnableRequestLog {
		a.engine.Use(middleware.RequestLog())
	}
	// CORS: enabled by default (matching Python behavior: always register, use ["*"] if not configured).
	// Set DisableCORS=true to disable.
	if !a.mwCfg.DisableCORS {
		a.engine.Use(middleware.CORS(a.mwCfg.CORSConfig))
	}
	if a.mwCfg.EnableRateLimit {
		if a.mwCfg.RateLimitRDB == nil {
			panic("fastapp: EnableRateLimit=true but RateLimitRDB is nil")
		}
		if a.mwCfg.RateLimitConfig.Limit <= 0 {
			panic("fastapp: EnableRateLimit=true but RateLimitConfig.Limit is 0 — rate limiter would never block")
		}
		a.engine.Use(middleware.RateLimit(a.mwCfg.RateLimitRDB, a.mwCfg.RateLimitConfig))
	}
	if a.mwCfg.EnableTokenAuth {
		if a.mwCfg.TokenVerifyFunc == nil && a.mwCfg.ExtraVerifyFunc == nil && a.mwCfg.InternalToken == "" {
			panic("fastapp: EnableTokenAuth=true but no verify function or InternalToken configured")
		}
		whitelist := a.mwCfg.TokenWhitelist
		whitelist = append(whitelist,
			"/healthz",
			"/monitor/prometheus",
		)
		if a.mwCfg.EnableSwagger {
			whitelist = append(whitelist, "/swagger/")
		}

		verify := a.mwCfg.TokenVerifyFunc
		// Compose OR logic: accept if either primary or extra verifier passes.
		if verify != nil && a.mwCfg.ExtraVerifyFunc != nil {
			verify = middleware.OrVerify(verify, a.mwCfg.ExtraVerifyFunc)
		} else if verify == nil {
			verify = a.mwCfg.ExtraVerifyFunc
		}
		// Internal token bypasses all other verification (checked first, constant-time).
		if a.mwCfg.InternalToken != "" {
			verify = middleware.WithInternalToken(a.mwCfg.InternalToken, verify)
		}
		a.engine.Use(middleware.TokenAuth(verify, whitelist...))
	}
}

// healthCheckHandler returns a handler that checks registered MySQL and Redis instances.
func (a *FastApp) healthCheckHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		status := &health.HealthStatus{
			Services: make(map[string]*health.ServiceHealth),
		}

		for name, db := range a.mysqlInstances {
			h := &health.ServiceHealth{}
			if err := db.Ping(ctx); err != nil {
				h.Status = health.StatusUnhealthy
				h.Error = err.Error()
			} else {
				h.Status = health.StatusHealthy
			}
			status.Services["mysql:"+name] = h
		}

		for name, rdb := range a.redisInstances {
			h := &health.ServiceHealth{}
			if !rdb.Ping(ctx) {
				h.Status = health.StatusUnhealthy
				h.Error = "redis ping failed"
			} else {
				h.Status = health.StatusHealthy
			}
			status.Services["redis:"+name] = h
		}

		if status.IsHealthy() {
			status.Status = health.StatusHealthy
			c.JSON(http.StatusOK, status)
		} else {
			status.Status = health.StatusUnhealthy
			c.JSON(http.StatusServiceUnavailable, status)
		}
	}
}

// registerBuiltinEndpoints registers /healthz, /metrics, and optional debug endpoints.
func (a *FastApp) registerBuiltinEndpoints() {
	a.engine.GET("/healthz", a.healthCheckHandler())
	a.engine.GET("/monitor/prometheus", gin.WrapH(promhttp.Handler()))

	if a.mwCfg.EnablePprof {
		pprof.RouteRegister(a.engine.Group("/debug"), "pprof")
	}

	if a.mwCfg.EnableSwagger {
		a.engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}
}

// setupAsyncQueue initializes producer and consumer.
func (a *FastApp) setupAsyncQueue() error {
	if a.aqCfg == nil {
		return nil
	}

	cfg := a.aqCfg

	if cfg.RedisConfig.Family == "" {
		cfg.RedisConfig.Family = a.cfg.Family
	}

	if cfg.Prefix == "" {
		cfg.Prefix = "/v1/async_queue"
	}

	// Create producer and register routes
	a.producer = async_queue.NewProducer(
		cfg.RedisClient,
		cfg.RedisConfig,
		cfg.Endpoints,
		a.cfg.Family,
	)
	a.producer.RegisterRoutes(a.engine, cfg.Prefix)

	// Create consumer
	opts := []async_queue.ConsumerOption{
		async_queue.WithGroupName(cfg.GroupName),
		async_queue.WithConsumerName(cfg.ConsumerName),
		async_queue.WithFeatures(async_queue.ResolveFeatureMode(cfg.FeatureMode, cfg.FeatureOverrides)),
	}
	if cfg.SchedulerConfig != nil {
		opts = append(opts, async_queue.WithScheduler(*cfg.SchedulerConfig))
	}
	if cfg.PelConfig != nil {
		opts = append(opts, async_queue.WithPel(*cfg.PelConfig))
	}
	if lc := cfg.EndpointLimitCfg; lc != nil {
		limits := make(map[string]int, len(cfg.Endpoints))
		for ep, ec := range cfg.Endpoints {
			if ec.MaxConcurrency > 0 {
				limits[ep] = ec.MaxConcurrency
			}
		}
		switch lc.Mode {
		case "distributed":
			opts = append(opts, async_queue.WithLimiter(
				async_queue.NewRedisConcurrencyLimiter(
					cfg.RedisClient,
					limits,
					lc.DefaultLimit,
					async_queue.BuildEndpointLimitKeyPrefix(a.cfg.Family),
					60,
				),
			))
		default: // "local"
			opts = append(opts, async_queue.WithLimiter(
				async_queue.NewLocalConcurrencyLimiter(limits, lc.DefaultLimit),
			))
		}
	}

	a.consumer = async_queue.NewConsumer(
		cfg.RedisClient,
		cfg.RedisConfig,
		cfg.Endpoints,
		a.cfg.Family,
		opts...,
	)

	return nil
}

// Run 启动 FastApp：初始化中间件、路由、异步队列、XXL-Job 并启动 HTTP 服务器，阻塞直至信号触发优雅停机.
// 参数：无.
// 返回值：err - 启动或停机过程中的错误.
func (a *FastApp) Run() error {
	if f := log.GetFamily(); f == "" || f == "dev" {
		log.SetFamily(a.cfg.Family)
	}

	a.applyServerDefaults()

	if a.mwCfg.EnablePrometheus {
		metrics.Enable()
	}

	if a.authManager != nil && a.mwCfg.EnableTokenAuth {
		a.mwCfg.TokenWhitelist = append(a.mwCfg.TokenWhitelist, a.authManager.Prefix())
	}

	// Build middleware chain before registering routes so every route gets the
	// configured global middleware.
	a.buildMiddlewareChain()

	// Register auth routes
	if a.authManager != nil {
		a.authManager.RegisterRoutes(a.engine)
	}

	// Register built-in endpoints
	a.registerBuiltinEndpoints()

	// Setup async queue
	if err := a.setupAsyncQueue(); err != nil {
		return fmt.Errorf("async queue setup failed: %w", err)
	}

	// Custom route registration
	if a.routeRegistrar != nil {
		a.routeRegistrar(a.engine)
	}

	// Print routes
	a.printRoutes()

	// Create HTTP server
	a.server = &http.Server{
		Addr:           fmt.Sprintf("%s:%d", a.cfg.Host, a.cfg.Port),
		Handler:        a.engine,
		ReadTimeout:    a.svrCfg.ReadTimeout,
		WriteTimeout:   a.svrCfg.WriteTimeout,
		IdleTimeout:    a.svrCfg.IdleTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	// Start async queue consumer
	if a.consumer != nil {
		go func() {
			if err := a.consumer.Start(context.Background()); err != nil {
				log.Error("async queue consumer error: %v", err)
			}
		}()
	}

	// Start XXL-Job executor
	if a.xjobConfig != nil {
		cfg := xjob.Config{
			Family:       a.cfg.Family,
			ServerAddr:   a.xjobConfig.ServerAddr,
			AccessToken:  a.xjobConfig.AccessToken,
			ExecutorIp:   a.xjobConfig.ExecutorIp,
			ExecutorPort: a.xjobConfig.ExecutorPort,
			LogDir:       a.xjobConfig.LogDir,
			MaxAge:       a.xjobConfig.MaxAge,
			JobDisabled:  a.xjobConfig.JobDisabled,
			Extra:        a.xjobConfig.Extra,
		}
		executor, err := xjob.NewExecutor(&cfg)
		if err != nil {
			return fmt.Errorf("xxl-job setup failed: %w", err)
		}
		a.xjobExecutor = executor
		a.xjobExecutor.Run(a.xjobTasks...)
	}

	// Run startup hooks
	ctx := context.Background()
	for _, hook := range a.onStart {
		if err := hook(ctx); err != nil {
			_ = a.shutdown()
			return fmt.Errorf("startup hook failed: %w", err)
		}
	}

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		log.Info("FastApp listening on %s:%d (family: %s)", a.cfg.Host, a.cfg.Port, a.cfg.Family)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Discard SIGPIPE on its own channel so a closed downstream pipe (e.g. log
	// forwarder hangup on stdout) doesn't terminate the process via Go's
	// default SIGPIPE handler, and can never starve SIGTERM in a shared buffer.
	sigpipe := make(chan os.Signal, 1)
	signal.Notify(sigpipe, syscall.SIGPIPE)
	defer signal.Stop(sigpipe)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	for {
		select {
		case <-sigpipe:
			continue
		case err := <-errCh:
			_ = a.shutdown()
			return fmt.Errorf("server error: %w", err)
		case sig := <-quit:
			log.Info("received signal %v, shutting down...", sig)
			return a.shutdown()
		}
	}
}

// shutdown performs graceful shutdown.
func (a *FastApp) shutdown() error {
	// Hard kill if graceful shutdown stalls. Use NewTimer so the goroutine
	// can be stopped cleanly on success, avoiding a leaked timer goroutine.
	done := make(chan struct{})
	defer close(done)
	timer := time.NewTimer(a.svrCfg.ShutdownTimeout + 5*time.Second)
	go func() {
		select {
		case <-done:
			timer.Stop()
			return
		case <-timer.C:
			log.Error("shutdown timeout exceeded, forcing exit")
			os.Exit(1)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), a.svrCfg.ShutdownTimeout)
	defer cancel()

	// Stop async queue consumer
	if a.consumer != nil {
		a.consumer.Stop()
		log.Info("async queue consumer stopped")
	}

	// Run shutdown hooks
	for _, hook := range a.onStop {
		if err := hook(ctx); err != nil {
			log.Error("shutdown hook error: %v", err)
		}
	}

	// Shutdown HTTP server
	if a.server != nil {
		if err := a.server.Shutdown(ctx); err != nil {
			log.Error("server shutdown error: %v", err)
			return err
		}
		log.Info("server shutdown complete")
	}

	// Close caches
	for name, c := range a.cacheInstances {
		_ = c.Close()
		log.Info("cache [%s] closed", name)
	}
	cache.CloseAllCaches() //nolint:errcheck

	// Stop XXL-Job executor
	if a.xjobExecutor != nil {
		a.xjobExecutor.Stop()
		log.Info("xxl-job executor stopped")
	}

	// Close config loader
	if a.configLoader != nil {
		_ = a.configLoader.Close()
	}

	// Close MySQL instances
	for name, db := range a.mysqlInstances {
		sqlDB, err := db.DB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		log.Info("mysql [%s] closed", name)
	}

	// Close Pulsar instances
	for name, client := range a.pulsarInstances {
		client.Close()
		log.Info("pulsar [%s] closed", name)
	}

	// Close Redis instances
	for name, rdb := range a.redisInstances {
		_ = rdb.Close()
		log.Info("redis [%s] closed", name)
	}

	return nil
}

// printRoutes prints all registered routes.
func (a *FastApp) printRoutes() {
	routes := a.engine.Routes()
	log.Info("registered routes (%d):", len(routes))
	for _, r := range routes {
		log.Info("  %s %s", r.Method, r.Path)
	}
}

// MustGetRedis 返回命名 Redis 实例，未注册时 panic.
// 参数：name - 实例名称.
// 返回值：*dbredis.Redis - Redis 实例.
func (a *FastApp) MustGetRedis(name string) *dbredis.Redis {
	r := a.GetRedis(name)
	if r == nil {
		panic(fmt.Sprintf("redis %q not registered", name))
	}
	return r
}

// MustGetMySQL 返回命名 MySQL 实例，未注册时 panic.
// 参数：name - 实例名称.
// 返回值：*dbmysql.Database - MySQL 实例.
func (a *FastApp) MustGetMySQL(name string) *dbmysql.Database {
	db := a.GetMySQL(name)
	if db == nil {
		panic(fmt.Sprintf("mysql %q not registered", name))
	}
	return db
}

// MustGetCache 返回命名缓存实例，未注册时 panic.
// 参数：name - 实例名称.
// 返回值：*cache.MultiLevelCache - 缓存实例.
func (a *FastApp) MustGetCache(name string) *cache.MultiLevelCache {
	c := a.GetCache(name)
	if c == nil {
		panic(fmt.Sprintf("cache %q not registered", name))
	}
	return c
}

// MustGetHTTPClient 返回命名 HTTP 客户端实例，未注册时 panic.
// 参数：name - 实例名称.
// 返回值：*httpclient.Client - HTTP 客户端实例.
func (a *FastApp) MustGetHTTPClient(name string) *httpclient.Client {
	c := a.GetHTTPClient(name)
	if c == nil {
		panic(fmt.Sprintf("httpclient %q not registered", name))
	}
	return c
}

// MustGetPulsar 返回命名 Pulsar 客户端实例，未注册时 panic.
// 参数：name - 实例名称.
// 返回值：*dbpulsar.Client - Pulsar 客户端实例.
func (a *FastApp) MustGetPulsar(name string) *dbpulsar.Client {
	c := a.GetPulsar(name)
	if c == nil {
		panic(fmt.Sprintf("pulsar %q not registered", name))
	}
	return c
}
