// Package mysql 数据库连接与 GORM ORM 封装.
// 提供连接管理、熔断与指标插件、数据库迁移、事务上下文传递.
// DSN 前缀自动路由驱动：sqlite:///path → glebarez 纯 Go 驱动，其余 → MySQL 驱动.
package mysql

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/huangyangke/go-aikit/log"
	"github.com/huangyangke/go-aikit/resilience"
)

const sqlitePrefix = "sqlite://"

// Config 数据库连接配置.
type Config struct {
	DSN           string             `yaml:"dsn"`
	MaxOpenConns  int                `yaml:"max_open_conns"`
	MaxIdleConns  int                `yaml:"max_idle_conns"`
	MaxLifetime   time.Duration      `yaml:"max_lifetime"`
	MaxIdleTime   time.Duration      `yaml:"max_idle_time"`
	Debug         bool               `yaml:"debug"`
	Name          string             `yaml:"name"`           // Datasource name for metrics labels (required when EnableMetrics)
	Breaker       *resilience.Config `yaml:"breaker"`        // nil = no circuit breaker
	EnableMetrics bool               `yaml:"enable_metrics"` // true = attach Prometheus metrics plugin. Default off; FastApp 注册时自动置 true.

	// Dialector 可选注入自定义 GORM 方言。非 nil 时优先于 DSN，仅可编程设置，不从 yaml 解析。
	// 一般无需使用——DSN 前缀 sqlite:// 自动路由到 sqlite 驱动。
	Dialector gorm.Dialector `yaml:"-"`
}

// IsSQLite 判断 DSN 是否为 sqlite 连接串（sqlite:// 前缀）.
func (c *Config) IsSQLite() bool {
	return strings.HasPrefix(c.DSN, sqlitePrefix)
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
	if c.EnableMetrics && c.Name == "" {
		return fmt.Errorf("mysql: Name is required when EnableMetrics is true (used as Prometheus datasource label)")
	}
	if c.Dialector == nil && c.DSN == "" {
		return fmt.Errorf("mysql: DSN is required")
	}
	return nil
}

// MustValidate 与 Validate 相同，但在发生错误时 panic.
func (c *Config) MustValidate() {
	if err := c.Validate(); err != nil {
		panic(err.Error())
	}
}

// Model 所有 ORM 模型的基础模型，包含主键与软删除.
// 时间戳由 TimestampPlugin 在应用层写入，故不设 DB 级默认值，保证跨方言（MySQL/sqlite）可移植.
type Model struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // Soft delete
}

// Database 数据库连接，封装 GORM 实例.
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

// New 创建数据库连接.
// DSN 前缀自动路由驱动：sqlite:///path → 纯 Go sqlite 驱动，其余 → MySQL 驱动.
// 参数：c - 连接配置, opts - GORM 配置选项.
// 返回值：*Database - 数据库实例.
func New(c *Config, opts ...Option) (*Database, error) {
	c.Fix()
	if err := c.Validate(); err != nil {
		return nil, err
	}

	isSQLite := c.IsSQLite()
	driverName := "mysql"
	if isSQLite {
		driverName = "sqlite"
	}
	log.Info("[DB][connect_start][driver=%s][debug=%t][max_open=%d][max_idle=%d]",
		driverName, c.Debug, c.MaxOpenConns, c.MaxIdleConns)

	gormConfig := &gorm.Config{}
	for _, opt := range opts {
		opt(gormConfig)
	}

	if !c.Debug {
		gormConfig.Logger = logger.Default.LogMode(logger.Silent)
	}

	// Dialector 注入 > DSN 前缀路由 > 默认 MySQL
	dialector := c.Dialector
	if dialector == nil {
		if isSQLite {
			path := strings.TrimPrefix(c.DSN, sqlitePrefix)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, fmt.Errorf("mysql: create sqlite dir: %w", err)
			}
			dialector = sqlite.Open(path)
		} else {
			dialector = mysql.Open(c.DSN)
		}
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		log.Error("[DB][open_error][driver=%s]: %v", driverName, err)
		return nil, fmt.Errorf("mysql: open error: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Error("[DB][underlying_db_error]: %v", err)
		return nil, fmt.Errorf("mysql: get underlying sql.DB error: %v", err)
	}

	if isSQLite {
		// sqlite 单文件不需要连接池
		sqlDB.SetMaxOpenConns(1)
	} else {
		sqlDB.SetMaxOpenConns(c.MaxOpenConns)
		sqlDB.SetMaxIdleConns(c.MaxIdleConns)
		sqlDB.SetConnMaxLifetime(c.MaxLifetime)
		sqlDB.SetConnMaxIdleTime(c.MaxIdleTime)
	}

	if err := sqlDB.Ping(); err != nil {
		log.Error("[DB][ping_error]: %v", err)
		return nil, fmt.Errorf("mysql: ping error: %v", err)
	}

	// Register timestamp plugin
	if err := db.Use(&TimestampPlugin{}); err != nil {
		log.Error("[DB][timestamp_plugin_error]: %v", err)
		return nil, fmt.Errorf("mysql: timestamp plugin error: %v", err)
	}

	// Register circuit breaker plugin (optional)
	if c.Breaker != nil {
		if err := db.Use(NewBreakerPlugin(*c.Breaker)); err != nil {
			log.Error("[DB][breaker_plugin_error]: %v", err)
			return nil, fmt.Errorf("mysql: breaker plugin error: %v", err)
		}
	}

	// Register metrics plugin (opt-in: default off, FastApp 注册时自动启用)
	if c.EnableMetrics {
		if err := db.Use(NewMetricsPlugin(c.Name)); err != nil {
			log.Error("[DB][metrics_plugin_error]: %v", err)
			return nil, fmt.Errorf("mysql: metrics plugin error: %v", err)
		}
	}

	log.Info("[DB][connected][driver=%s][debug=%t]", driverName, c.Debug)
	return &Database{DB: db, cfg: c}, nil
}

// MustNew 与 New 相同，但在发生错误时 panic.
// 适用于初始化阶段，错误应该导致程序崩溃的场景.
func MustNew(c *Config, opts ...Option) *Database {
	db, err := New(c, opts...)
	if err != nil {
		panic(err.Error())
	}
	return db
}

// Close 关闭数据库连接.
// 返回值：err - 关闭失败时的错误.
func (d *Database) Close() error {
	if d.DB == nil {
		return nil
	}

	sqlDB, err := d.DB.DB()
	if err != nil {
		log.Error("[DB][close_underlying_db_error]: %v", err)
		return err
	}

	if err := sqlDB.Close(); err != nil {
		log.Error("[DB][close_error]: %v", err)
		return err
	}
	log.Info("[DB][closed]")
	return nil
}

// AutoMigrate 自动迁移数据库表结构.
// 参数：models - 需迁移的模型列表.
// 返回值：err - 迁移失败时的错误.
func (d *Database) AutoMigrate(models ...interface{}) error {
	return d.DB.AutoMigrate(models...)
}
