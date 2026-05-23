package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"

	apperrors "github.com/example/go-template/internal/errors"
	"github.com/example/go-template/internal/model"
	"github.com/example/go-template/internal/schema"
	"github.com/example/go-template/internal/service"
)

type mockRepo struct{ mock.Mock }

func (m *mockRepo) List(ctx context.Context, offset, limit int) ([]*model.Article, int64, error) {
	args := m.Called(ctx, offset, limit)
	return args.Get(0).([]*model.Article), args.Get(1).(int64), args.Error(2)
}
func (m *mockRepo) Create(ctx context.Context, article *model.Article) error {
	return m.Called(ctx, article).Error(0)
}
func (m *mockRepo) GetByID(ctx context.Context, id uint) (*model.Article, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*model.Article), args.Error(1)
}
func (m *mockRepo) Update(ctx context.Context, id uint, updates map[string]interface{}) (*model.Article, error) {
	args := m.Called(ctx, id, updates)
	return args.Get(0).(*model.Article), args.Error(1)
}
func (m *mockRepo) Delete(ctx context.Context, id uint) error {
	return m.Called(ctx, id).Error(0)
}

func TestArticleService_List(t *testing.T) {
	repo := &mockRepo{}
	expected := []*model.Article{{Title: "test"}}
	repo.On("List", mock.Anything, 0, 20).Return(expected, int64(1), nil)

	svc := service.NewArticleService(repo)
	articles, total, err := svc.List(context.Background(), 1, 20)

	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, expected, articles)
	repo.AssertExpectations(t)
}

func TestArticleService_Create(t *testing.T) {
	repo := &mockRepo{}
	repo.On("Create", mock.Anything, mock.AnythingOfType("*model.Article")).Return(nil)

	svc := service.NewArticleService(repo)
	article, err := svc.Create(context.Background(), &schema.CreateArticleReq{
		Title:   "Hello",
		Content: "World",
		Author:  "Alice",
	})

	assert.NoError(t, err)
	assert.Equal(t, "Hello", article.Title)
	repo.AssertExpectations(t)
}

func TestArticleService_Get_NotFound(t *testing.T) {
	repo := &mockRepo{}
	repo.On("GetByID", mock.Anything, uint(99)).Return((*model.Article)(nil), gorm.ErrRecordNotFound)

	svc := service.NewArticleService(repo)
	result, err := svc.Get(context.Background(), 99)

	assert.Nil(t, result)
	assert.Equal(t, apperrors.ErrArticleNotFound, err)
	repo.AssertExpectations(t)
}

func TestArticleService_Delete_NotFound(t *testing.T) {
	repo := &mockRepo{}
	repo.On("Delete", mock.Anything, uint(99)).Return(gorm.ErrRecordNotFound)

	svc := service.NewArticleService(repo)
	err := svc.Delete(context.Background(), 99)

	assert.Equal(t, apperrors.ErrArticleNotFound, err)
	repo.AssertExpectations(t)
}
