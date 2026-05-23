package mysql

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/example/go-template/pkg/aikit/log"
	"github.com/example/go-template/pkg/aikit/resilience"
)

// Config represents MySQL connection configuration
type Config struct {
	DSN          string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	MaxLifetime     time.Duration `yaml:"max_lifetime"`
	MaxIdleTime     time.Duration `yaml:"max_idle_time"`
	Debug        bool          `yaml:"debug"`
	Name         string         `yaml:"name"`          // Datasource name for metrics labels
	Breaker      *resilience.Config `yaml:"breaker"`   // nil = no circuit breaker
	DisableMetrics bool         `yaml:"disable_metrics"` // true = no Prometheus metrics plugin
}

// Fix fills default values for zero/empty fields
func (c *Config) Fix() {
	if c.MaxIdleConns <= 0 {
		c.MaxIdleConns = 5
	}
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 20
	}
	if c.MaxLifetime <= 0 {
		c.MaxLifetime = 600 * time.Second
	}
	if c.MaxIdleTime <= 0 {
		c.MaxIdleTime = 30 * time.Second
	}
}

// Validate checks required fields and returns an error if any are missing
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("mysql: Name is required (used as Prometheus datasource label)")
	}
	if c.DSN == "" {
		return fmt.Errorf("mysql: DSN is required")
	}
	return nil
}

// Model defines a base model for all ORM models
type Model struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // Soft delete
}

// Database represents a MySQL database connection with GORM ORM support
type Database struct {
	DB  *gorm.DB
	cfg *Config
}

// Option function for configuring GORM
type Option func(*gorm.Config)

// WithNamingStrategy configures table and column name mapping strategy
func WithNamingStrategy(strategy schema.NamingStrategy) Option {
	return func(c *gorm.Config) {
		c.NamingStrategy = strategy
	}
}

// WithLogger configures GORM logger
func WithLogger(l logger.Interface) Option {
	return func(c *gorm.Config) {
		c.Logger = l
	}
}

// WithFullSaveAssociations configures whether to update all associations when saving
func WithFullSaveAssociations(enable bool) Option {
	return func(c *gorm.Config) {
		c.FullSaveAssociations = enable
	}
}

// WithSkipDefaultTransaction configures whether to skip default transaction
func WithSkipDefaultTransaction(skip bool) Option {
	return func(c *gorm.Config) {
		c.SkipDefaultTransaction = skip
	}
}

// New opens a MySQL connection using GORM and panics on error.
func New(c *Config, opts ...Option) *Database {
	c.Fix()
	if err := c.Validate(); err != nil {
		panic(err.Error())
	}
	log.Info("[MySQL][connect_start][debug=%t][max_open=%d][max_idle=%d]", c.Debug, c.MaxOpenConns, c.MaxIdleConns)

	gormConfig := &gorm.Config{}
	for _, opt := range opts {
		opt(gormConfig)
	}

	if !c.Debug {
		gormConfig.Logger = logger.Default.LogMode(logger.Silent)
	}

	db, err := gorm.Open(mysql.Open(c.DSN), gormConfig)
	if err != nil {
		log.Error("[MySQL][open_error]: %v", err)
		panic(fmt.Sprintf("mysql: open error: %v", err))
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Error("[MySQL][underlying_db_error]: %v", err)
		panic(fmt.Sprintf("mysql: get underlying sql.DB error: %v", err))
	}

	sqlDB.SetMaxOpenConns(c.MaxOpenConns)
	sqlDB.SetMaxIdleConns(c.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(c.MaxLifetime)
	sqlDB.SetConnMaxIdleTime(c.MaxIdleTime)

	if err := sqlDB.Ping(); err != nil {
		log.Error("[MySQL][ping_error]: %v", err)
		panic(fmt.Sprintf("mysql: ping error: %v", err))
	}

	// Register timestamp plugin
	if err := db.Use(&TimestampPlugin{}); err != nil {
		log.Error("[MySQL][timestamp_plugin_error]: %v", err)
		panic(fmt.Sprintf("mysql: timestamp plugin error: %v", err))
	}

	// Register circuit breaker plugin (optional)
	if c.Breaker != nil {
		if err := db.Use(NewBreakerPlugin(*c.Breaker)); err != nil {
			log.Error("[MySQL][breaker_plugin_error]: %v", err)
			panic(fmt.Sprintf("mysql: breaker plugin error: %v", err))
		}
	}

	// Register metrics plugin (default: enabled)
	if !c.DisableMetrics {
		if err := db.Use(NewMetricsPlugin(c.Name)); err != nil {
			log.Error("[MySQL][metrics_plugin_error]: %v", err)
			panic(fmt.Sprintf("mysql: metrics plugin error: %v", err))
		}
	}

	log.Info("[MySQL][connected][debug=%t]", c.Debug)
	return &Database{DB: db, cfg: c}
}

// Close closes the MySQL connection
func (d *Database) Close() error {
	if d.DB == nil {
		return nil
	}

	sqlDB, err := d.DB.DB()
	if err != nil {
		log.Error("[MySQL][close_underlying_db_error]: %v", err)
		return err
	}

	if err := sqlDB.Close(); err != nil {
		log.Error("[MySQL][close_error]: %v", err)
		return err
	}
	log.Info("[MySQL][closed]")
	return nil
}

// AutoMigrate auto-migrates the database schema
func (d *Database) AutoMigrate(models ...interface{}) error {
	return d.DB.AutoMigrate(models...)
}
