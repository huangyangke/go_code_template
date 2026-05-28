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

	db := dbmysql.MustNew(&dbmysql.Config{DSN: dsn})
	if err := db.MigrateUp(context.TODO(), migrations, "migrations"); err != nil {
		log.Error("migrate failed: %v", err)
		os.Exit(1)
	}

	log.Info("database migration completed")
}
