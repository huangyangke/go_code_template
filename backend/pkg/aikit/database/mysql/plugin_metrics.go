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
	if err := db.Callback().Create().Before("gorm:create").Register("aikit:metrics_before_create", p.beforeCallback); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:create").Register("aikit:metrics_after_create", p.afterCallback); err != nil {
		return err
	}

	if err := db.Callback().Query().Before("gorm:query").Register("aikit:metrics_before_query", p.beforeCallback); err != nil {
		return err
	}
	if err := db.Callback().Query().After("gorm:query").Register("aikit:metrics_after_query", p.afterCallback); err != nil {
		return err
	}

	if err := db.Callback().Update().Before("gorm:update").Register("aikit:metrics_before_update", p.beforeCallback); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:update").Register("aikit:metrics_after_update", p.afterCallback); err != nil {
		return err
	}

	if err := db.Callback().Delete().Before("gorm:delete").Register("aikit:metrics_before_delete", p.beforeCallback); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:delete").Register("aikit:metrics_after_delete", p.afterCallback); err != nil {
		return err
	}

	return nil
}

func (p *MetricsPlugin) beforeCallback(db *gorm.DB) {
	_ = db.InstanceSet("aikit_sql_metrics_start", time.Now())
}

func (p *MetricsPlugin) afterCallback(db *gorm.DB) {
	startVal, _ := db.InstanceGet("aikit_sql_metrics_start")
	if startVal == nil {
		return
	}
	start, ok := startVal.(time.Time)
	if !ok {
		return
	}
	_ = db.InstanceSet("aikit_sql_metrics_start", nil)

	duration := time.Since(start)
	operation := p.operationName(db)
	success := db.Error == nil

	metrics.ObserveMySQLQuery(metrics.ServiceFamily(), p.name, operation, success, duration)
}

func (p *MetricsPlugin) operationName(db *gorm.DB) string {
	if db == nil || db.Statement == nil || db.Statement.Table == "" {
		return "unknown"
	}
	return db.Statement.Table
}
