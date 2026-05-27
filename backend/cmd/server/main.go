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
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}

	loader, err := config.New("configs/config.yaml",
		config.WithEnvFile(fmt.Sprintf("configs/.env.%s", env)),
	)
	if err != nil {
		panic(fmt.Sprintf("load config: %v", err))
	}

	log.Init(&log.Config{
		Level:  loader.GetString("log.level", "info"),
		Family: loader.GetString("app.family", "go-template"),
		Stdout: env != "prod",
		Dir:    loader.GetString("log.dir", "logs"),
		WithFields: map[string]log.WithField{
			"task_id": func(ctx context.Context) map[string]interface{} {
				if id := middleware.GetTaskID(ctx); id != "" {
					return map[string]interface{}{"task_id": id}
				}
				return nil
			},
		},
	})

	a := app.NewFastApp(app.FastAppConfig{
		Family: loader.GetString("app.family", "go-template"),
		Host:   loader.GetString("app.host", "0.0.0.0"),
		Port:   loader.GetInt("app.port", 8080),
	})
	a.SetConfigLoader(loader)
	a.SetMiddlewares(app.MiddlewareConfig{
		EnableRequestID:  true,
		EnableRequestLog: true,
		EnablePrometheus: true,
		EnableSwagger:    true,
	})

	if loader.GetString("mysql.dsn") != "" {
		var mysqlCfg dbmysql.Config
		if err := loader.Scan("mysql", &mysqlCfg); err != nil {
			panic(fmt.Sprintf("load mysql config: %v", err))
		}
		mysqlCfg.Name = "default"
		mysqlCfg.Fix()
		if err := mysqlCfg.Validate(); err != nil {
			panic(err.Error())
		}
		a.RegisterMySQL("default", &mysqlCfg)
	}

	if len(loader.GetStringSlice("redis.addrs")) > 0 {
		var redisCfg dbredis.Config
		if err := loader.Scan("redis", &redisCfg); err != nil {
			panic(fmt.Sprintf("load redis config: %v", err))
		}
		redisCfg.Name = "default"
		redisCfg.Fix()
		if err := redisCfg.Validate(); err != nil {
			panic(err.Error())
		}
		a.RegisterRedis("default", &redisCfg)
	}

	a.SetRouteRegistrar(func(e *gin.Engine) {
		api.RegisterRoutes(e, a.GetMySQL("default"), a.GetRedis("default"))
	})

	if err := a.Run(); err != nil {
		log.Error("server error: %v", err)
		os.Exit(1)
	}
}
