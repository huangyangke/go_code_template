package mysql

import (
	"errors"

	gomysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/huangyangke/go-aikit/resilience"
)

const breakerCtxKey = "aikit_sql_breaker_done"

// BreakerPlugin gorm.Plugin 实现，为 SQL 查询添加熔断保护.
// ErrRecordNotFound 与重复键错误不计为失败.
type BreakerPlugin struct {
	breaker resilience.Breaker
}

// NewBreakerPlugin 根据熔断配置创建熔断插件.
// 参数：cfg - 熔断器配置.
// 返回值：*BreakerPlugin - 熔断插件实例.
func NewBreakerPlugin(cfg resilience.Config) *BreakerPlugin {
	return &BreakerPlugin{
		breaker: resilience.New(&cfg),
	}
}

// Name 返回插件名称.
// 返回值：string - 插件标识.
func (p *BreakerPlugin) Name() string {
	return "aikit_sql_breaker"
}

// Initialize 注册熔断的 before/after 回调.
// 参数：db - GORM 数据库实例.
// 返回值：err - 注册回调失败时的错误.
func (p *BreakerPlugin) Initialize(db *gorm.DB) error {
	// Register before/after callbacks for each GORM operation type.
	// We use Before("gorm:<op>") and After("gorm:<op>") to bracket the actual operation.

	if err := db.Callback().Create().Before("gorm:create").Register("aikit:breaker_before_create", p.beforeCallback); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:create").Register("aikit:breaker_after_create", p.afterCallback); err != nil {
		return err
	}

	if err := db.Callback().Query().Before("gorm:query").Register("aikit:breaker_before_query", p.beforeCallback); err != nil {
		return err
	}
	if err := db.Callback().Query().After("gorm:query").Register("aikit:breaker_after_query", p.afterCallback); err != nil {
		return err
	}

	if err := db.Callback().Update().Before("gorm:update").Register("aikit:breaker_before_update", p.beforeCallback); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:update").Register("aikit:breaker_after_update", p.afterCallback); err != nil {
		return err
	}

	if err := db.Callback().Delete().Before("gorm:delete").Register("aikit:breaker_before_delete", p.beforeCallback); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:delete").Register("aikit:breaker_after_delete", p.afterCallback); err != nil {
		return err
	}

	return nil
}

func (p *BreakerPlugin) beforeCallback(db *gorm.DB) {
	if db.Error != nil {
		return
	}
	done, err := p.breaker.Allow()
	if err != nil {
		_ = db.AddError(err)
		return
	}
	_ = db.InstanceSet(breakerCtxKey, done)
}

func (p *BreakerPlugin) afterCallback(db *gorm.DB) {
	raw, ok := db.InstanceGet(breakerCtxKey)
	if !ok {
		return
	}
	done, ok := raw.(func(success bool))
	if !ok {
		return
	}
	done(sqlAcceptable(db.Error))
}

// sqlAcceptable returns true for errors that should NOT count toward circuit
// breaker opening: nil, ErrRecordNotFound, and MySQL duplicate entry (1062).
func sqlAcceptable(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	var myErr *gomysql.MySQLError
	if errors.As(err, &myErr) {
		return myErr.Number == 1062
	}
	return false
}
