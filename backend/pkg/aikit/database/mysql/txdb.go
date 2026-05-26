package mysql

import (
	"context"

	"gorm.io/gorm"
)

type contextTxKey struct{}

// TxDB returns the transaction *gorm.DB from context if one exists,
// otherwise returns the default DB. This allows repository methods to
// automatically participate in any ongoing transaction.
func (d *Database) TxDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(contextTxKey{}).(*gorm.DB); ok {
		return tx // 有事务，返回事务
	}
	return d.DB // 无事务，返回普通数据库连接
}

// WithContext returns a *gorm.DB that is context-aware: if a transaction
// is present in ctx, it returns the transaction DB; otherwise the default DB.
// The result also has ctx set via WithContext.
func (d *Database) WithContext(ctx context.Context) *gorm.DB {
	return d.TxDB(ctx).WithContext(ctx)
}

// ExecTx runs fn inside a GORM transaction, injecting the transaction *gorm.DB
// into ctx so that any code calling TxDB(ctx) within fn will use the transaction.
func (d *Database) ExecTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ctx = context.WithValue(ctx, contextTxKey{}, tx) // ctx 绑定 事务tx
		return fn(ctx) // 执行业务逻辑函数
	})
}

// Context-aware CRUD methods

// CreateCtx inserts a record using the transaction from context if available.
func (d *Database) CreateCtx(ctx context.Context, value interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).Create(value).Error
}

// FindCtx finds records using the transaction from context if available.
func (d *Database) FindCtx(ctx context.Context, dest interface{}, conditions ...interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).Find(dest, conditions...).Error
}

// FirstCtx finds the first record using the transaction from context if available.
func (d *Database) FirstCtx(ctx context.Context, dest interface{}, conditions ...interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).First(dest, conditions...).Error
}

// DeleteCtx deletes records using the transaction from context if available.
func (d *Database) DeleteCtx(ctx context.Context, value interface{}, conditions ...interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).Delete(value, conditions...).Error
}

// UpdateCtx updates a single column using the transaction from context if available.
func (d *Database) UpdateCtx(ctx context.Context, column string, value interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).Update(column, value).Error
}

// UpdatesCtx updates multiple columns using the transaction from context if available.
func (d *Database) UpdatesCtx(ctx context.Context, values interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).Updates(values).Error
}

// Ping checks database connectivity
func (d *Database) Ping(ctx context.Context) error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
