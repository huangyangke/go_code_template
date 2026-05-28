package mysql

import (
	"time"

	"gorm.io/gorm"

	"github.com/huangyangke/go-aikit/metrics"
)

// MetricsPlugin gorm.Plugin 实现，记录 SQL 查询的 Prometheus 指标.
type MetricsPlugin struct {
	name string // datasource name for label
}

// NewMetricsPlugin 根据数据源名称创建指标插件.
// 参数：name - 数据源名称，用于 Prometheus 标签.
// 返回值：*MetricsPlugin - 指标插件实例.
func NewMetricsPlugin(name string) *MetricsPlugin {
	return &MetricsPlugin{name: name}
}

// Name 返回插件名称.
// 返回值：string - 插件标识.
func (p *MetricsPlugin) Name() string {
	return "aikit_sql_metrics"
}

// Initialize 注册指标采集的 before/after 回调.
// 参数：db - GORM 数据库实例.
// 返回值：err - 注册回调失败时的错误.
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
