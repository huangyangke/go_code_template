package mysql

import (
	"database/sql"
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
	MaxOpenConns int           `yaml:"max_open_conns"`
	MaxIdleConns int           `yaml:"max_idle_conns"`
	MaxLifetime  time.Duration `yaml:"max_lifetime"`
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
}

// Validate checks required fields and returns an error if any are missing
func (c *Config) Validate() error {
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
		name := c.Name
		if name == "" {
			name = "default"
		}
		if err := db.Use(NewMetricsPlugin(name)); err != nil {
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

// Create inserts a single record
func (d *Database) Create(value interface{}) error {
	return d.DB.Create(value).Error
}

// CreateInBatches inserts multiple records in batches
func (d *Database) CreateInBatches(values interface{}, batchSize int) error {
	return d.DB.CreateInBatches(values, batchSize).Error
}

// First finds the first record that matches the given conditions
func (d *Database) First(dest interface{}, conditions ...interface{}) error {
	return d.DB.First(dest, conditions...).Error
}

// Take finds a single record that matches the given conditions
func (d *Database) Take(dest interface{}, conditions ...interface{}) error {
	return d.DB.Take(dest, conditions...).Error
}

// Last finds the last record that matches the given conditions
func (d *Database) Last(dest interface{}, conditions ...interface{}) error {
	return d.DB.Last(dest, conditions...).Error
}

// Find finds all records that match the given conditions
func (d *Database) Find(dest interface{}, conditions ...interface{}) error {
	return d.DB.Find(dest, conditions...).Error
}

// Where creates a WHERE clause
func (d *Database) Where(query interface{}, args ...interface{}) *gorm.DB {
	return d.DB.Where(query, args...)
}

// Or creates an OR clause
func (d *Database) Or(query interface{}, args ...interface{}) *gorm.DB {
	return d.DB.Or(query, args...)
}

// Not creates a NOT clause
func (d *Database) Not(query interface{}, args ...interface{}) *gorm.DB {
	return d.DB.Not(query, args...)
}

// Limit sets a LIMIT clause
func (d *Database) Limit(limit int) *gorm.DB {
	return d.DB.Limit(limit)
}

// Offset sets an OFFSET clause
func (d *Database) Offset(offset int) *gorm.DB {
	return d.DB.Offset(offset)
}

// Order sets an ORDER clause
func (d *Database) Order(value interface{}) *gorm.DB {
	return d.DB.Order(value)
}

// Group sets a GROUP BY clause
func (d *Database) Group(name string) *gorm.DB {
	return d.DB.Group(name)
}

// Having sets a HAVING clause
func (d *Database) Having(query interface{}, args ...interface{}) *gorm.DB {
	return d.DB.Having(query, args...)
}

// Count counts the number of records that match the given conditions
func (d *Database) Count(count *int64) *gorm.DB {
	return d.DB.Count(count)
}

// Pluck retrieves a single column from the database
func (d *Database) Pluck(column string, dest interface{}) *gorm.DB {
	return d.DB.Pluck(column, dest)
}

// Update updates a single column with the given value
func (d *Database) Update(column string, value interface{}) *gorm.DB {
	return d.DB.Update(column, value)
}

// Updates updates multiple columns
func (d *Database) Updates(values interface{}) *gorm.DB {
	return d.DB.Updates(values)
}

// Delete deletes records that match the given conditions
func (d *Database) Delete(value interface{}, conditions ...interface{}) error {
	return d.DB.Delete(value, conditions...).Error
}

// Scopes applies scopes
func (d *Database) Scopes(funcs ...func(*gorm.DB) *gorm.DB) *gorm.DB {
	return d.DB.Scopes(funcs...)
}

// Preload preloads associations
func (d *Database) Preload(query string, args ...interface{}) *gorm.DB {
	return d.DB.Preload(query, args...)
}

// Joins joins with other tables
func (d *Database) Joins(query string, args ...interface{}) *gorm.DB {
	return d.DB.Joins(query, args...)
}

// Transaction starts a transaction
func (d *Database) Transaction(fc func(tx *gorm.DB) error, opts ...*sql.TxOptions) error {
	return d.DB.Transaction(fc, opts...)
}

// Exec executes raw SQL without returning any rows
func (d *Database) Exec(sql string, values ...interface{}) *gorm.DB {
	return d.DB.Exec(sql, values...)
}

// Raw executes raw SQL
func (d *Database) Raw(sql string, values ...interface{}) *gorm.DB {
	return d.DB.Raw(sql, values...)
}

// Table specifies the table name
func (d *Database) Table(name string, args ...interface{}) *gorm.DB {
	return d.DB.Table(name, args...)
}

// Model specifies the model
func (d *Database) Model(value interface{}) *gorm.DB {
	return d.DB.Model(value)
}

// Select specifies the columns to select
func (d *Database) Select(query interface{}, args ...interface{}) *gorm.DB {
	return d.DB.Select(query, args...)
}

// Distinct specifies a DISTINCT clause
func (d *Database) Distinct(args ...interface{}) *gorm.DB {
	return d.DB.Distinct(args...)
}

// Debug enables debug mode for the current query
func (d *Database) Debug() *gorm.DB {
	return d.DB.Debug()
}

// // // // // // // // // // // // // // // // // // // // // // // // // //

// QueryBuilder provides a fluent API for building complex queries
type QueryBuilder struct {
	db *gorm.DB
}

// NewQueryBuilder creates a new query builder
func NewQueryBuilder(db *gorm.DB) *QueryBuilder {
	return &QueryBuilder{db: db}
}

// // // // // // // // // // // // // // // // // // // // // // // // // //

// Repository defines common CRUD operations with type safety
// Note: This interface is kept for reference, see GenericRepository below
type Repository interface {
	Create(model interface{}) error
	Update(model interface{}) error
	Delete(id interface{}) error
	FindByID(id interface{}, dest interface{}) error
	FindAll(dest interface{}) error
	FindWhere(query string, args []interface{}, dest interface{}) error
	Count(model interface{}, where string, args []interface{}) (int64, error)
}

// GenericRepository provides a type-safe base repository
type GenericRepository[T any] struct {
	db *gorm.DB
}

// NewGenericRepository creates a new generic repository
func NewGenericRepository[T any](db *gorm.DB) *GenericRepository[T] {
	return &GenericRepository[T]{db: db}
}

// Create inserts a new record
func (r *GenericRepository[T]) Create(model *T) error {
	return r.db.Create(model).Error
}

// Update updates an existing record
func (r *GenericRepository[T]) Update(model *T) error {
	return r.db.Save(model).Error
}

// Delete deletes a record by ID
func (r *GenericRepository[T]) Delete(id interface{}) error {
	var model T
	return r.db.Delete(&model, id).Error
}

// FindByID finds a record by ID
func (r *GenericRepository[T]) FindByID(id interface{}) (*T, error) {
	var model T
	if err := r.db.First(&model, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

// FindAll finds all records
func (r *GenericRepository[T]) FindAll() ([]T, error) {
	var models []T
	if err := r.db.Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

// FindWhere finds records by a where clause
func (r *GenericRepository[T]) FindWhere(query string, args ...interface{}) ([]T, error) {
	var models []T
	if err := r.db.Where(query, args...).Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

// Count counts records matching the where clause
func (r *GenericRepository[T]) Count(where string, args ...interface{}) (int64, error) {
	var count int64
	var model T
	if err := r.db.Model(&model).Where(where, args...).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// DB returns the underlying GORM DB for custom queries
func (r *GenericRepository[T]) DB() *gorm.DB {
	return r.db
}

// Transaction executes a function within a database transaction
func (r *GenericRepository[T]) Transaction(fc func(*gorm.DB) error) error {
	return r.db.Transaction(fc)
}
