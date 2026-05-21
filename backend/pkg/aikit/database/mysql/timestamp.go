package mysql

import (
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// TimestampPlugin implements gorm.Plugin to auto-set created_at/updated_at.
// On create: sets created_at and updated_at if zero.
// On update: sets updated_at unless gorm:update_column tag is present.
type TimestampPlugin struct{}

func (p *TimestampPlugin) Name() string {
	return "aikit_timestamp"
}

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
	now := db.Statement.DB.NowFunc()
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
	now := db.Statement.DB.NowFunc()
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
