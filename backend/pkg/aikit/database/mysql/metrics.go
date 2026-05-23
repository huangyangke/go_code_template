package mysql

import (
	"time"

	"gorm.io/gorm"

	"github.com/example/go-template/pkg/aikit/metrics"
)

// MetricsPlugin implements gorm.Plugin to record Prometheus metrics for SQL queries.
type MetricsPlugin struct {
	name string // datasource name for label
}

// NewMetricsPlugin creates a new MetricsPlugin with the given datasource name.
func NewMetricsPlugin(name string) *MetricsPlugin {
	return &MetricsPlugin{name: name}
}

func (p *MetricsPlugin) Name() string {
	return "aikit_sql_metrics"
}

func (p *MetricsPlugin) Initialize(db *gorm.DB) error {
	if err := db.Callback().Create().Before("gorm:create").Register("aikit:metrics_before_create", p.before("create")); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:create").Register("aikit:metrics_after_create", p.after("create")); err != nil {
		return err
	}

	if err := db.Callback().Query().Before("gorm:query").Register("aikit:metrics_before_query", p.before("query")); err != nil {
		return err
	}
	if err := db.Callback().Query().After("gorm:query").Register("aikit:metrics_after_query", p.after("query")); err != nil {
		return err
	}

	if err := db.Callback().Update().Before("gorm:update").Register("aikit:metrics_before_update", p.before("update")); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:update").Register("aikit:metrics_after_update", p.after("update")); err != nil {
		return err
	}

	if err := db.Callback().Delete().Before("gorm:delete").Register("aikit:metrics_before_delete", p.before("delete")); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:delete").Register("aikit:metrics_after_delete", p.after("delete")); err != nil {
		return err
	}

	return nil
}

func (p *MetricsPlugin) before(op string) func(*gorm.DB) {
	key := "aikit_sql_metrics_start_" + op
	return func(db *gorm.DB) {
		_ = db.InstanceSet(key, time.Now())
	}
}

func (p *MetricsPlugin) after(op string) func(*gorm.DB) {
	key := "aikit_sql_metrics_start_" + op
	return func(db *gorm.DB) {
		startVal, _ := db.InstanceGet(key)
		if startVal == nil {
			return
		}
		start, ok := startVal.(time.Time)
		if !ok {
			return
		}
		_ = db.InstanceSet(key, nil)
		table := "unknown"
		if db.Statement != nil && db.Statement.Table != "" {
			table = db.Statement.Table
		}
		metrics.ObserveMySQLQuery(p.name, table, op, db.Error == nil, time.Since(start))
	}
}
