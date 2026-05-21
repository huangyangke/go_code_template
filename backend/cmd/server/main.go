package main

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/example/go-template/pkg/aikit/app"
	"github.com/example/go-template/pkg/aikit/config"
	dbmysql "github.com/example/go-template/pkg/aikit/database/mysql"
	dbredis "github.com/example/go-template/pkg/aikit/database/redis"
	"github.com/example/go-template/pkg/aikit/log"

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

	if dsn := loader.GetString("mysql.dsn"); dsn != "" {
		a.RegisterMySQL("default", &dbmysql.Config{
			DSN:          dsn,
			MaxOpenConns: loader.GetInt("mysql.max_open_conns", 20),
			MaxIdleConns: loader.GetInt("mysql.max_idle_conns", 5),
		})
	}

	if addrs := loader.GetString("redis.addr"); addrs != "" {
		a.RegisterRedis("default", &dbredis.Config{
			Addrs:        []string{addrs},
			UserPassword: loader.GetString("redis.password"),
			DB:           loader.GetInt("redis.db", 0),
		})
	}

	a.SetRouteRegistrar(func(e *gin.Engine) {
		api.RegisterRoutes(e, a.GetMySQL("default"), a.GetRedis("default"))
	})

	if err := a.Run(); err != nil {
		log.Error("server error: %v", err)
		os.Exit(1)
	}
}
