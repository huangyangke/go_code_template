package dao

import (
	"context"

	dbmysql "github.com/example/go-template/pkg/aikit/database/mysql"
	"gorm.io/gorm"

	"github.com/example/go-template/internal/model"
)

// ArticleDAO handles database operations for articles.
type ArticleDAO struct {
	db *gorm.DB
}

func NewArticleDAO(database *dbmysql.Database) *ArticleDAO {
	return &ArticleDAO{db: database.DB}
}

func (d *ArticleDAO) List(ctx context.Context, offset, limit int) ([]*model.Article, int64, error) {
	var articles []*model.Article
	var total int64

	if err := d.db.WithContext(ctx).Model(&model.Article{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := d.db.WithContext(ctx).Offset(offset).Limit(limit).Find(&articles).Error; err != nil {
		return nil, 0, err
	}

	return articles, total, nil
}

func (d *ArticleDAO) Create(ctx context.Context, article *model.Article) error {
	return d.db.WithContext(ctx).Create(article).Error
}

func (d *ArticleDAO) GetByID(ctx context.Context, id uint) (*model.Article, error) {
	var article model.Article
	if err := d.db.WithContext(ctx).First(&article, id).Error; err != nil {
		return nil, err
	}
	return &article, nil
}

func (d *ArticleDAO) Update(ctx context.Context, id uint, updates map[string]interface{}) (*model.Article, error) {
	if err := d.db.WithContext(ctx).Model(&model.Article{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return d.GetByID(ctx, id)
}

func (d *ArticleDAO) Delete(ctx context.Context, id uint) error {
	return d.db.WithContext(ctx).Delete(&model.Article{}, id).Error
}
