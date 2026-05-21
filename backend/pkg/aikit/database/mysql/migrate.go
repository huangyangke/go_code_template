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

	"github.com/example/go-template/pkg/aikit/log"
)

// MigrateConfig configures the migration runner.
type MigrateConfig struct {
	// MigrationsTable is the name of the table used to track applied migrations.
	// Defaults to "schema_migrations".
	MigrationsTable string

	// NoLock disables advisory locking during migration (useful for some cloud DBs).
	NoLock bool
}

// Migrator wraps golang-migrate for the Database instance.
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
		sqlDB.Close()
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
		sqlDB.Close()
		return nil, err
	}

	source, err := iofs.New(fsys, path)
	if err != nil {
		sqlDB.Close()
		return nil, err
	}

	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		sqlDB.Close()
		return nil, err
	}

	return &Migrator{m: m, ownDB: sqlDB}, nil
}

// Up applies all available migrations.
// Returns nil if already up-to-date.
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

// Down rolls back all migrations.
func (mg *Migrator) Down() error {
	err := mg.m.Down()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Steps applies n migrations. Positive n migrates up; negative n migrates down.
func (mg *Migrator) Steps(n int) error {
	err := mg.m.Steps(n)
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// MigrateTo migrates to a specific version.
func (mg *Migrator) MigrateTo(version uint) error {
	err := mg.m.Migrate(version)
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Version returns the currently active migration version.
// Returns 0, false if no migrations have been applied.
func (mg *Migrator) Version() (version uint, dirty bool, err error) {
	v, d, e := mg.m.Version()
	if errors.Is(e, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return v, d, e
}

// Force sets the migration version without running migrations.
// Useful for fixing a dirty state after a failed migration.
func (mg *Migrator) Force(version int) error {
	return mg.m.Force(version)
}

// Drop drops everything in the database. Use with extreme caution.
func (mg *Migrator) Drop() error {
	return mg.m.Drop()
}

// Close closes the migrator and frees resources.
func (mg *Migrator) Close() error {
	srcErr, dbErr := mg.m.Close()
	if mg.ownDB != nil {
		mg.ownDB.Close()
	}
	if srcErr != nil {
		return srcErr
	}
	return dbErr
}

// MigrateUp is a convenience method that creates a Migrator and runs Up in one step.
// It closes the migrator after completion.
func (d *Database) MigrateUp(ctx context.Context, fsys fs.FS, path string, cfg ...MigrateConfig) error {
	mg, err := d.NewMigrator(fsys, path, cfg...)
	if err != nil {
		return err
	}
	defer mg.Close()
	return mg.Up()
}
