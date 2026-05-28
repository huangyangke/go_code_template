// Package mysql MySQL 连接与 GORM ORM 封装.
// 提供连接管理、熔断与指标插件、数据库迁移、事务上下文传递.
package mysql

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/huangyangke/go-aikit/log"
	"github.com/huangyangke/go-aikit/resilience"
)

// Config MySQL 连接配置.
type Config struct {
	DSN            string             `yaml:"dsn"`
	MaxOpenConns   int                `yaml:"max_open_conns"`
	MaxIdleConns   int                `yaml:"max_idle_conns"`
	MaxLifetime    time.Duration      `yaml:"max_lifetime"`
	MaxIdleTime    time.Duration      `yaml:"max_idle_time"`
	Debug          bool               `yaml:"debug"`
	Name           string             `yaml:"name"`            // Datasource name for metrics labels
	Breaker        *resilience.Config `yaml:"breaker"`         // nil = no circuit breaker
	DisableMetrics bool               `yaml:"disable_metrics"` // true = no Prometheus metrics plugin
}

// Fix 填充零值字段的默认值.
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

// Validate 校验必填字段，缺少时返回错误.
// 返回值：err - 缺少必填字段时的错误.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("mysql: Name is required (used as Prometheus datasource label)")
	}
	if c.DSN == "" {
		return fmt.Errorf("mysql: DSN is required")
	}
	return nil
}

// Model 所有 ORM 模型的基础模型，包含主键与软删除.
type Model struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // Soft delete
}

// Database MySQL 数据库连接，封装 GORM 实例.
type Database struct {
	DB  *gorm.DB
	cfg *Config
}

// Option GORM 配置函数.
type Option func(*gorm.Config)

// WithNamingStrategy 配置表名与列名映射策略.
// 参数：strategy - 命名策略.
// 返回值：Option - 配置函数.
func WithNamingStrategy(strategy schema.NamingStrategy) Option {
	return func(c *gorm.Config) {
		c.NamingStrategy = strategy
	}
}

// WithLogger 配置 GORM 日志器.
// 参数：l - 日志接口.
// 返回值：Option - 配置函数.
func WithLogger(l logger.Interface) Option {
	return func(c *gorm.Config) {
		c.Logger = l
	}
}

// WithFullSaveAssociations 配置保存时是否更新全部关联.
// 参数：enable - 是否启用.
// 返回值：Option - 配置函数.
func WithFullSaveAssociations(enable bool) Option {
	return func(c *gorm.Config) {
		c.FullSaveAssociations = enable
	}
}

// WithSkipDefaultTransaction 配置是否跳过默认事务.
// 参数：skip - 是否跳过.
// 返回值：Option - 配置函数.
func WithSkipDefaultTransaction(skip bool) Option {
	return func(c *gorm.Config) {
		c.SkipDefaultTransaction = skip
	}
}

// New 创建 MySQL 数据库连接，出错时 panic.
// 参数：c - 连接配置, opts - GORM 配置选项.
// 返回值：*Database - 数据库实例.
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

// Close 关闭 MySQL 连接.
// 返回值：err - 关闭失败时的错误.
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

// AutoMigrate 自动迁移数据库表结构.
// 参数：models - 需迁移的模型列表.
// 返回值：err - 迁移失败时的错误.
func (d *Database) AutoMigrate(models ...interface{}) error {
	return d.DB.AutoMigrate(models...)
}
