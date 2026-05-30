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
)

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
		MaxLogFile: loader.GetInt("log.max_log_file", 10), // 最多10个日志文件
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
	// 初始化mysql
	if loader.GetString("mysql.dsn") != "" {
		var mysqlCfg dbmysql.Config
		loader.MustScan("mysql", &mysqlCfg)
		a.RegisterMySQL("default", &mysqlCfg)
	}
	// 初始化redis
	if len(loader.GetStringSlice("redis.addrs")) > 0 {
		var redisCfg dbredis.Config
		loader.MustScan("redis", &redisCfg)
		a.RegisterRedis("default", &redisCfg)
	}
	// 注册路由
	a.SetRouteRegistrar(func(e *gin.Engine) {
		api.RegisterRoutes(e, a.GetMySQL("default"), a.GetRedis("default"))
	})
	// 启动服务
	if err := a.Run(); err != nil {
		log.Error("server error: %v", err)
		os.Exit(1)
	}
}
