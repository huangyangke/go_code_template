package mysql

import (
	"context"

	"gorm.io/gorm"
)

type contextTxKey struct{}

// TxDB 从上下文获取事务 DB，若无事务则返回默认 DB.
// 参数：ctx - 上下文.
// 返回值：*gorm.DB - 事务或默认数据库实例.
func (d *Database) TxDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(contextTxKey{}).(*gorm.DB); ok {
		return tx // 有事务，返回事务
	}
	return d.DB // 无事务，返回普通数据库连接
}

// WithContext 返回绑定上下文的 DB，若上下文中存在事务则使用事务 DB.
// 参数：ctx - 上下文.
// 返回值：*gorm.DB - 带上下文的数据库实例.
func (d *Database) WithContext(ctx context.Context) *gorm.DB {
	return d.TxDB(ctx).WithContext(ctx)
}

// ExecTx 在事务中执行 fn，将事务 DB 注入上下文.
// 参数：ctx - 上下文, fn - 业务逻辑函数.
// 返回值：err - 事务失败时的错误.
func (d *Database) ExecTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ctx = context.WithValue(ctx, contextTxKey{}, tx) // ctx 绑定 事务tx
		return fn(ctx)                                   // 执行业务逻辑函数
	})
}

// CreateCtx 创建记录，若上下文中存在事务则使用事务.
// 参数：ctx - 上下文, value - 待创建的记录.
// 返回值：err - 创建失败时的错误.
func (d *Database) CreateCtx(ctx context.Context, value interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).Create(value).Error
}

// FindCtx 批量查询记录，若上下文中存在事务则使用事务.
// 参数：ctx - 上下文, dest - 查询结果容器, conditions - 查询条件.
// 返回值：err - 查询失败时的错误.
func (d *Database) FindCtx(ctx context.Context, dest interface{}, conditions ...interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).Find(dest, conditions...).Error
}

// FirstCtx 查询第一条记录，若上下文中存在事务则使用事务.
// 参数：ctx - 上下文, dest - 查询结果容器, conditions - 查询条件.
// 返回值：err - 查询失败时的错误.
func (d *Database) FirstCtx(ctx context.Context, dest interface{}, conditions ...interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).First(dest, conditions...).Error
}

// DeleteCtx 删除记录，若上下文中存在事务则使用事务.
// 参数：ctx - 上下文, value - 待删除的记录或条件, conditions - 附加条件.
// 返回值：err - 删除失败时的错误.
func (d *Database) DeleteCtx(ctx context.Context, value interface{}, conditions ...interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).Delete(value, conditions...).Error
}

// UpdateCtx 更新单列，若上下文中存在事务则使用事务.
// 参数：ctx - 上下文, column - 列名, value - 新值.
// 返回值：err - 更新失败时的错误.
func (d *Database) UpdateCtx(ctx context.Context, column string, value interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).Update(column, value).Error
}

// UpdatesCtx 更新多列，若上下文中存在事务则使用事务.
// 参数：ctx - 上下文, values - 更新字段映射或结构体.
// 返回值：err - 更新失败时的错误.
func (d *Database) UpdatesCtx(ctx context.Context, values interface{}) error {
	return d.TxDB(ctx).WithContext(ctx).Updates(values).Error
}

// Ping 检测数据库连通性.
// 参数：ctx - 上下文.
// 返回值：err - 连接失败时的错误.
func (d *Database) Ping(ctx context.Context) error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
