package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/huangyangke/go-aikit/app"
	"github.com/huangyangke/go-aikit/app/middleware"
	"github.com/huangyangke/go-aikit/config"
	dbmysql "github.com/huangyangke/go-aikit/database/mysql"
	dbredis "github.com/huangyangke/go-aikit/database/redis"
	"github.com/huangyangke/go-aikit/log"

	"github.com/example/go-template/internal/api"

	// 空导入 docs 包：触发其 init() 注册 swagger spec，供 /swagger/index.html 加载。
	// 由 `./run.sh swagger` 生成，修改接口注解后需重新生成。
	_ "github.com/example/go-template/docs"
)

// @title       Go Template API
// @version     1.0
// @description 基于 aikit 的 Go 后端服务模板 API 文档。
// @BasePath    /
func main() {
	// 生产 or 开发环境
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}
	// 配置文件
	loader := config.MustNew("configs/config.yaml",
		config.WithEnvFile(fmt.Sprintf("configs/.env.%s", env)),
	)
	// 服务名称（必须唯一）
	family := loader.GetString("app.family")
	// 日志初始化
	log.Init(&log.Config{
		Level:      loader.GetString("log.level", "info"),
		Family:     family,
		Stdout:     loader.GetBool("log.stdout", env != "prod"),
		Dir:        loader.GetString("log.dir", "logs"),
		MaxLogFile: loader.GetInt("log.max_log_file", 10),              // 最多10个日志文件
		RotateSize: int64(loader.GetInt("log.rotate_size", 104857600)), // 100M
		// log.InfoCtx(ctx, ...) 日志输出自动携带 task_id
		WithFields: map[string]log.WithField{
			"task_id": func(ctx context.Context) map[string]interface{} {
				if id := middleware.GetTaskID(ctx); id != "" {
					return map[string]interface{}{"task_id": id}
				}
				return nil
			},
		},
	})
	// 初始化app
	a := app.NewFastApp(app.FastAppConfig{
		Family: family,
		Host:   loader.GetString("app.host", "0.0.0.0"),
		Port:   loader.GetInt("app.port", 8080),
	})
	a.SetConfigLoader(loader)
	a.SetMiddlewares(app.MiddlewareConfig{
		EnableRequestID:  true,
		EnableRequestLog: true,
		EnablePrometheus: true,
		EnableSwagger:    true,
		CORSConfig: middleware.CORSConfig{
			AllowOrigins: loader.GetStringSlice("cors.allow_origins"),
		},
	})
	// 初始化数据库：DB_DSN 支持 mysql DSN 或 sqlite:///path 前缀，aikit 自动路由驱动
	if dsn := loader.GetString("db.dsn"); dsn != "" {
		cfg := &dbmysql.Config{DSN: dsn}
		if !cfg.IsSQLite() {
			loader.MustScan("mysql", cfg)
		}
		a.RegisterMySQL("default", cfg)
	}
	// 初始化 Redis：不配置 REDIS_ADDR 则跳过注册，本地开发可用 CACHE_TYPE=local 替代
	if addrs := loader.GetStringSlice("redis.addrs"); len(addrs) > 0 && addrs[0] != "" {
		var redisCfg dbredis.Config
		loader.MustScan("redis", &redisCfg)
		a.RegisterRedis("default", &redisCfg)
	}
	// 注册路由
	a.SetRouteRegistrar(func(e *gin.Engine) {
		api.RegisterRoutes(e, a.GetMySQL("default"), a.GetRedis("default"))
	})
	// 生命周期钩子（示例，按需替换 TODO 中的业务逻辑）
	// OnStart：在 HTTP 监听之前依次执行，返回 error 会中止启动并触发优雅停机。
	// 适合：预热缓存、自检外部依赖、启动业务后台 goroutine。
	a.OnStart(func(ctx context.Context) error {
		// TODO: 业务启动逻辑，例如预热缓存 / 自检依赖。
		log.InfoCtx(ctx, "app started")
		return nil
	})
	// OnStop：在 HTTP server 关闭之前依次执行，错误只记录不中断。
	// 适合：flush 业务缓冲、关闭自建的客户端、通知下游下线。
	// 注意：MySQL/Redis/cache/异步队列等由 FastApp 注册的资源会自动释放，无需在此重复关闭。
	a.OnStop(func(ctx context.Context) error {
		// TODO: 业务收尾逻辑。
		log.InfoCtx(ctx, "app stopping")
		return nil
	})
	// 启动服务
	if err := a.Run(); err != nil {
		log.Error("server error: %v", err)
		os.Exit(1)
	}
}
