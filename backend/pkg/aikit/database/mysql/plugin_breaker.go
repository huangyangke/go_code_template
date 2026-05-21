package mysql

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/example/go-template/pkg/aikit/resilience"
)

// BreakerPlugin implements gorm.Plugin to add circuit breaker protection to SQL queries.
// ErrRecordNotFound and duplicate entry errors are considered acceptable (not counted as failures).
type BreakerPlugin struct {
	breaker resilience.Breaker
}

// NewBreakerPlugin creates a new BreakerPlugin with the given breaker configuration.
func NewBreakerPlugin(cfg resilience.Config) *BreakerPlugin {
	return &BreakerPlugin{
		breaker: resilience.New(&cfg),
	}
}

func (p *BreakerPlugin) Name() string {
	return "aikit_sql_breaker"
}

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
	_ = db.InstanceSet("aikit_sql_breaker_active", true)
}

func (p *BreakerPlugin) afterCallback(db *gorm.DB) {
	active, _ := db.InstanceGet("aikit_sql_breaker_active")
	if active == nil {
		return
	}
	_ = db.InstanceSet("aikit_sql_breaker_active", nil)

	acceptable := sqlAcceptable(db.Error)
	// Use breaker.Do with a fallback that swallows acceptable errors
	// so they don't count as failures.
	err := p.breaker.Do(func() error {
		if db.Error != nil && !acceptable {
			return db.Error
		}
		return nil
	}, func(err error) error {
		if acceptable {
			return nil
		}
		return err
	})
	if err != nil {
		db.Error = err
	}
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
	// MySQL duplicate entry error: Error 1062
	if strings.Contains(err.Error(), "1062") && strings.Contains(err.Error(), "Duplicate entry") {
		return true
	}
	return false
}
