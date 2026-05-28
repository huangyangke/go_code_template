package mysql

import (
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// TimestampPlugin gorm.Plugin 实现，自动设置 created_at 和 updated_at.
// 创建时若字段为零值则填充当前时间，更新时若非 update_column 模式则刷新 updated_at.
type TimestampPlugin struct{}

// Name 返回插件名称.
// 返回值：string - 插件标识.
func (p *TimestampPlugin) Name() string {
	return "aikit_timestamp"
}

// Initialize 注册创建与更新前的回调.
// 参数：db - GORM 数据库实例.
// 返回值：err - 注册回调失败时的错误.
func (p *TimestampPlugin) Initialize(db *gorm.DB) error {
	if err := db.Callback().Create().Before("gorm:create").Register("aikit:before_create", beforeCreateCallback); err != nil {
		return err
	}
	return db.Callback().Update().Before("gorm:update").Register("aikit:before_update", beforeUpdateCallback)
}

func beforeCreateCallback(db *gorm.DB) {
	if db.Statement.Schema == nil {
		return
	}
	now := db.Statement.NowFunc()
	setTimestampIfZero(db, "CreatedAt", now)
	setTimestampIfZero(db, "UpdatedAt", now)
}

func beforeUpdateCallback(db *gorm.DB) {
	if db.Statement.Schema == nil {
		return
	}
	if _, ok := db.Statement.Clauses["update_column"]; ok {
		return
	}
	now := db.Statement.NowFunc()
	setTimestampField(db, "UpdatedAt", now)
}

func setTimestampIfZero(db *gorm.DB, fieldName string, now interface{}) {
	field := findField(db.Statement.Schema, fieldName)
	if field == nil {
		return
	}

	switch db.Statement.ReflectValue.Kind() {
	case reflect.Struct:
		if _, isZero := field.ValueOf(db.Statement.Context, db.Statement.ReflectValue); isZero {
			_ = field.Set(db.Statement.Context, db.Statement.ReflectValue, now)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < db.Statement.ReflectValue.Len(); i++ {
			rv := reflect.Indirect(db.Statement.ReflectValue.Index(i))
			if _, isZero := field.ValueOf(db.Statement.Context, rv); isZero {
				_ = field.Set(db.Statement.Context, rv, now)
			}
		}
	}
}

func setTimestampField(db *gorm.DB, fieldName string, now interface{}) {
	field := findField(db.Statement.Schema, fieldName)
	if field == nil {
		return
	}

	switch db.Statement.ReflectValue.Kind() {
	case reflect.Struct:
		_ = field.Set(db.Statement.Context, db.Statement.ReflectValue, now)
	case reflect.Slice, reflect.Array:
		for i := 0; i < db.Statement.ReflectValue.Len(); i++ {
			rv := reflect.Indirect(db.Statement.ReflectValue.Index(i))
			_ = field.Set(db.Statement.Context, rv, now)
		}
	}
}

func findField(s *schema.Schema, name string) *schema.Field {
	for _, f := range s.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}
