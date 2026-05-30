package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/huangyangke/go-aikit/config"
	dbmysql "github.com/huangyangke/go-aikit/database/mysql"
	"github.com/huangyangke/go-aikit/log"

	"github.com/example/go-template/internal/model"
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

	dsn := loader.GetString("db.dsn")
	if dsn == "" {
		log.Error("db.dsn is not configured")
		os.Exit(1)
	}

	cfg := &dbmysql.Config{DSN: dsn}
	db := dbmysql.MustNew(cfg)

	// sqlite：GORM AutoMigrate 从 model 结构体建表（.sql 是 MySQL 方言，sqlite 用不了）。
	// 新增表时把 model 加入下面的列表。
	if cfg.IsSQLite() {
		if err := db.AutoMigrate(&model.Article{}); err != nil {
			log.Error("automigrate failed: %v", err)
			os.Exit(1)
		}
		log.Info("automigrate completed")
		return
	}

	// mysql：golang-migrate 跑 migrations/*.sql
	if err := db.MigrateUp(context.TODO(), migrations, "migrations"); err != nil {
		log.Error("migrate failed: %v", err)
		os.Exit(1)
	}

	log.Info("database migration completed")
}
