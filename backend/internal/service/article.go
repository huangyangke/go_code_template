package service

import (
	"context"

	"github.com/example/go-template/internal/model"
	"github.com/example/go-template/internal/schema"
)

type articleRepository interface {
	List(ctx context.Context, offset, limit int) ([]*model.Article, int64, error)
	Create(ctx context.Context, article *model.Article) error
	GetByID(ctx context.Context, id uint) (*model.Article, error)
	Update(ctx context.Context, id uint, updates map[string]interface{}) (*model.Article, error)
	Delete(ctx context.Context, id uint) error
}

type ArticleService struct {
	repo articleRepository
}

func NewArticleService(repo articleRepository) *ArticleService {
	return &ArticleService{repo: repo}
}

func (s *ArticleService) List(ctx context.Context, page, size int) ([]*model.Article, int64, error) {
	return s.repo.List(ctx, (page-1)*size, size)
}

func (s *ArticleService) Create(ctx context.Context, req *schema.CreateArticleReq) (*model.Article, error) {
	article := &model.Article{
		Title:   req.Title,
		Content: req.Content,
		Author:  req.Author,
	}
	if err := s.repo.Create(ctx, article); err != nil {
		return nil, err
	}
	return article, nil
}

func (s *ArticleService) Get(ctx context.Context, id uint) (*model.Article, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ArticleService) Update(ctx context.Context, id uint, req *schema.UpdateArticleReq) (*model.Article, error) {
	updates := map[string]interface{}{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Content != "" {
		updates["content"] = req.Content
	}
	if len(updates) == 0 {
		return s.repo.GetByID(ctx, id)
	}
	return s.repo.Update(ctx, id, updates)
}

func (s *ArticleService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}
