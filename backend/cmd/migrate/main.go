package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/huangyangke/go-aikit/config"
	dbmysql "github.com/huangyangke/go-aikit/database/mysql"
	"github.com/huangyangke/go-aikit/log"
)

//go:embed migrations/*.sql
var migrations embed.FS

func main() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}

	loader := config.MustNew("configs/config.yaml",
		config.WithEnvFile(fmt.Sprintf("configs/.env.%s", env)),
	)

	log.Init(&log.Config{
		Level:  loader.GetString("log.level", "info"),
		Family: loader.GetString("app.family", "go-template"),
		Stdout: true,
	})

	dsn := loader.GetString("mysql.dsn")
	if dsn == "" {
		log.Error("mysql.dsn is not configured")
		os.Exit(1)
	}

	// Name 是 aikit Config.Validate 的必填项（Prometheus datasource 标签），
	// 迁移是一次性 CLI，无需指标插件，故 DisableMetrics.
	db := dbmysql.MustNew(&dbmysql.Config{DSN: dsn, Name: "migrate", DisableMetrics: true})
	if err := db.MigrateUp(context.TODO(), migrations, "migrations"); err != nil {
		log.Error("migrate failed: %v", err)
		os.Exit(1)
	}

	log.Info("database migration completed")
}
