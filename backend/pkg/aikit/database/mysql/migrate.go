package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	gomysql "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/huangyangke/go-aikit/log"
)

// MigrateConfig 数据库迁移运行配置.
type MigrateConfig struct {
	// MigrationsTable is the name of the table used to track applied migrations.
	// Defaults to "schema_migrations".
	MigrationsTable string

	// NoLock disables advisory locking during migration (useful for some cloud DBs).
	NoLock bool
}

// Migrator 封装 golang-migrate 的迁移执行器.
type Migrator struct {
	m     *migrate.Migrate
	ownDB *sql.DB
}

// NewMigrator creates a Migrator from an embed.FS (or any fs.FS) containing SQL files.
//
// The fsys should contain files named like:
//
//	000001_create_users.up.sql
//	000001_create_users.down.sql
//
// path is the subdirectory within fsys where migration files reside (e.g. "migrations").
func (d *Database) NewMigrator(fsys fs.FS, path string, cfg ...MigrateConfig) (*Migrator, error) {
	dsn := d.cfg.DSN
	if _, err := gomysql.ParseDSN(dsn); err != nil {
		return nil, fmt.Errorf("mysql/migrate: invalid DSN: %w", err)
	}
	// Open an independent connection so Close() won't affect the app's GORM pool.
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql/migrate: open: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("mysql/migrate: ping: %w", err)
	}

	var mc MigrateConfig
	if len(cfg) > 0 {
		mc = cfg[0]
	}

	driverCfg := &migratemysql.Config{
		MigrationsTable: mc.MigrationsTable,
		NoLock:          mc.NoLock,
	}
	if driverCfg.MigrationsTable == "" {
		driverCfg.MigrationsTable = "schema_migrations"
	}

	driver, err := migratemysql.WithInstance(sqlDB, driverCfg)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	source, err := iofs.New(fsys, path)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return &Migrator{m: m, ownDB: sqlDB}, nil
}

// Up 执行所有未应用的迁移，已到最新版本时返回 nil.
// 返回值：err - 迁移失败时的错误.
func (mg *Migrator) Up() error {
	err := mg.m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		log.Info("[Migrate][up] already up-to-date")
		return nil
	}
	if err != nil {
		return err
	}
	v, _, _ := mg.m.Version()
	log.Info("[Migrate][up] migrated to version %d", v)
	return nil
}

// Down 回滚所有迁移.
// 返回值：err - 回滚失败时的错误.
func (mg *Migrator) Down() error {
	err := mg.m.Down()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Steps 执行 n 步迁移，正数向上迁移，负数向下回滚.
// 参数：n - 迁移步数.
// 返回值：err - 迁移失败时的错误.
func (mg *Migrator) Steps(n int) error {
	err := mg.m.Steps(n)
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// MigrateTo 迁移到指定版本.
// 参数：version - 目标版本号.
// 返回值：err - 迁移失败时的错误.
func (mg *Migrator) MigrateTo(version uint) error {
	err := mg.m.Migrate(version)
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Version 返回当前迁移版本号.
// 返回值：version - 当前版本, dirty - 是否脏状态, err - 查询失败时的错误.
func (mg *Migrator) Version() (version uint, dirty bool, err error) {
	v, d, e := mg.m.Version()
	if errors.Is(e, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return v, d, e
}

// Force 强制设置迁移版本号，不执行迁移脚本，用于修复脏状态.
// 参数：version - 目标版本号.
// 返回值：err - 操作失败时的错误.
func (mg *Migrator) Force(version int) error {
	return mg.m.Force(version)
}

// Drop 删除数据库中的所有内容，慎用.
// 返回值：err - 操作失败时的错误.
func (mg *Migrator) Drop() error {
	return mg.m.Drop()
}

// Close 关闭迁移器并释放资源.
// 返回值：err - 关闭失败时的错误.
func (mg *Migrator) Close() error {
	srcErr, dbErr := mg.m.Close()
	if mg.ownDB != nil {
		_ = mg.ownDB.Close()
	}
	if srcErr != nil {
		return srcErr
	}
	return dbErr
}

// MigrateUp 一步式迁移：创建迁移器并执行 Up，完成后自动关闭.
// 参数：ctx - 上下文, fsys - 包含迁移 SQL 文件的文件系统, path - 迁移文件子目录路径, cfg - 迁移配置.
// 返回值：err - 迁移失败时的错误.
func (d *Database) MigrateUp(ctx context.Context, fsys fs.FS, path string, cfg ...MigrateConfig) error {
	mg, err := d.NewMigrator(fsys, path, cfg...)
	if err != nil {
		return err
	}
	defer func() { _ = mg.Close() }()
	return mg.Up()
}
