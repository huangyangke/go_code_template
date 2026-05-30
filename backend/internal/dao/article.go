package dao

import (
	"context"

	dbmysql "github.com/huangyangke/go-aikit/database/mysql"

	"github.com/example/go-template/internal/model"
)

// ArticleDAO handles database operations for articles.
// 内嵌 aikit 泛型 Repository，消除手写 CRUD 样板；
// 所有方法经 Repository 走 Database.WithContext(ctx)，因此自动复用 ExecTx 注入的事务.
type ArticleDAO struct {
	*dbmysql.Repository[model.Article]
}

func NewArticleDAO(database *dbmysql.Database) *ArticleDAO {
	return &ArticleDAO{Repository: dbmysql.NewGenericRepository[model.Article](database)}
}

// List 与 Create 的签名与泛型 Repository 完全一致，经内嵌自动提升，无需重写.
//
// 以下三个方法收窄主键类型为 uint：保持 service 依赖接口的类型安全，
// 仅一行委托给泛型 Repository（其入参为 any）.

func (d *ArticleDAO) GetByID(ctx context.Context, id uint) (*model.Article, error) {
	return d.Repository.GetByID(ctx, id)
}

func (d *ArticleDAO) Update(ctx context.Context, id uint, updates map[string]interface{}) (*model.Article, error) {
	return d.Repository.Update(ctx, id, updates)
}

func (d *ArticleDAO) Delete(ctx context.Context, id uint) error {
	return d.Repository.Delete(ctx, id)
}

