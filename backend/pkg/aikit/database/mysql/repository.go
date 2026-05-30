package mysql

import (
	"context"

	"gorm.io/gorm"
)

// Repository 泛型 CRUD 仓储，消除每个模型重复的样板代码.
// 所有方法 ctx-aware：若 ctx 由 ExecTx 注入了事务，则自动在该事务内执行，
// 否则使用默认连接，因此可无缝组合到事务中.
type Repository[T any] struct {
	db *Database
}

// NewGenericRepository 创建模型 T 的泛型仓储.
// 参数：db - aikit 数据库实例.
// 返回值：*Repository[T] - 仓储实例.
func NewGenericRepository[T any](db *Database) *Repository[T] {
	return &Repository[T]{db: db}
}

// Create 插入一条记录.
// 参数：ctx - 上下文, entity - 待插入实体.
// 返回值：err - 插入失败时的错误.
func (r *Repository[T]) Create(ctx context.Context, entity *T) error {
	return r.db.WithContext(ctx).Create(entity).Error
}

// GetByID 按主键查询，未找到返回 gorm.ErrRecordNotFound.
// 参数：ctx - 上下文, id - 主键.
// 返回值：*T - 实体, err - 查询失败或不存在时的错误.
func (r *Repository[T]) GetByID(ctx context.Context, id any) (*T, error) {
	var entity T
	if err := r.db.WithContext(ctx).First(&entity, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

// List 分页查询，返回当前页记录与符合条件的总数.
// 参数：ctx - 上下文, offset - 偏移量, limit - 每页条数.
// 返回值：[]*T - 记录列表, int64 - 总数, err - 查询失败时的错误.
func (r *Repository[T]) List(ctx context.Context, offset, limit int) ([]*T, int64, error) {
	var entities []*T
	var total int64

	base := r.db.WithContext(ctx).Model(new(T))
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := base.Offset(offset).Limit(limit).Find(&entities).Error; err != nil {
		return nil, 0, err
	}
	return entities, total, nil
}

// FindWhere 按条件查询多条记录.
// 参数：ctx - 上下文, query - 查询条件, args - 条件参数.
// 返回值：[]*T - 记录列表, err - 查询失败时的错误.
func (r *Repository[T]) FindWhere(ctx context.Context, query any, args ...any) ([]*T, error) {
	var entities []*T
	if err := r.db.WithContext(ctx).Where(query, args...).Find(&entities).Error; err != nil {
		return nil, err
	}
	return entities, nil
}

// Count 统计符合条件的记录数.
// 参数：ctx - 上下文, query - 查询条件（为空时统计全部）, args - 条件参数.
// 返回值：int64 - 记录数, err - 查询失败时的错误.
func (r *Repository[T]) Count(ctx context.Context, query any, args ...any) (int64, error) {
	var total int64
	tx := r.db.WithContext(ctx).Model(new(T))
	if query != nil {
		tx = tx.Where(query, args...)
	}
	if err := tx.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// Update 按主键部分更新字段并返回更新后的记录，主键不存在时返回 gorm.ErrRecordNotFound.
// 参数：ctx - 上下文, id - 主键, updates - 待更新字段映射.
// 返回值：*T - 更新后的实体, err - 更新失败或不存在时的错误.
func (r *Repository[T]) Update(ctx context.Context, id any, updates map[string]any) (*T, error) {
	if err := r.db.WithContext(ctx).Model(new(T)).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

// Delete 按主键删除（模型含 gorm.DeletedAt 时为软删除），无记录被删除时返回 gorm.ErrRecordNotFound.
// 参数：ctx - 上下文, id - 主键.
// 返回值：err - 删除失败或不存在时的错误.
func (r *Repository[T]) Delete(ctx context.Context, id any) error {
	result := r.db.WithContext(ctx).Delete(new(T), "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
