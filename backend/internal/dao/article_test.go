package dao_test

import (
	"context"
	"errors"
	"os"
	"testing"

	dbmysql "github.com/huangyangke/go-aikit/database/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/huangyangke/go_code_template/backend/internal/dao"
	"github.com/huangyangke/go_code_template/backend/internal/model"
)

// DAO 集成测试需要真实 MySQL：设置 TEST_MYSQL_DSN 后运行，否则跳过.
// 每个用例在 ExecTx 内执行并强制回滚，保证不污染数据库（AGENTS.md §七）.
//
//	TEST_MYSQL_DSN="root:pass@tcp(127.0.0.1:3306)/go_template?parseTime=true" go test ./internal/dao/...
func newTestDAO(t *testing.T) (*dao.ArticleDAO, *dbmysql.Database) {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN 未设置，跳过 DAO 集成测试")
	}
	db := dbmysql.MustNew(&dbmysql.Config{DSN: dsn})
	return dao.NewArticleDAO(db), db
}

// errRollback 用于在断言后强制回滚事务.
var errRollback = errors.New("rollback")

func TestArticleDAO_CreateAndGetByID(t *testing.T) {
	d, db := newTestDAO(t)

	_ = db.ExecTx(context.Background(), func(ctx context.Context) error {
		a := &model.Article{Title: "t", Content: "c", Author: "a"}
		require.NoError(t, d.Create(ctx, a))
		require.NotZero(t, a.ID)

		got, err := d.GetByID(ctx, a.ID)
		require.NoError(t, err)
		assert.Equal(t, "t", got.Title)
		return errRollback
	})
}

func TestArticleDAO_GetByID_NotFound(t *testing.T) {
	d, db := newTestDAO(t)

	_ = db.ExecTx(context.Background(), func(ctx context.Context) error {
		_, err := d.GetByID(ctx, 1<<40) // 不存在的主键
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
		return errRollback
	})
}

func TestArticleDAO_Update(t *testing.T) {
	d, db := newTestDAO(t)

	_ = db.ExecTx(context.Background(), func(ctx context.Context) error {
		a := &model.Article{Title: "old", Content: "c", Author: "a"}
		require.NoError(t, d.Create(ctx, a))

		updated, err := d.Update(ctx, a.ID, map[string]interface{}{"title": "new"})
		require.NoError(t, err)
		assert.Equal(t, "new", updated.Title)
		return errRollback
	})
}

func TestArticleDAO_Delete_SoftAndNotFound(t *testing.T) {
	d, db := newTestDAO(t)

	_ = db.ExecTx(context.Background(), func(ctx context.Context) error {
		a := &model.Article{Title: "t", Content: "c", Author: "a"}
		require.NoError(t, d.Create(ctx, a))

		require.NoError(t, d.Delete(ctx, a.ID))

		// 软删除后常规查询找不到.
		_, err := d.GetByID(ctx, a.ID)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

		// 再删返回 NotFound.
		assert.ErrorIs(t, d.Delete(ctx, a.ID), gorm.ErrRecordNotFound)
		return errRollback
	})
}

func TestArticleDAO_List(t *testing.T) {
	d, db := newTestDAO(t)

	_ = db.ExecTx(context.Background(), func(ctx context.Context) error {
		for i := 0; i < 3; i++ {
			require.NoError(t, d.Create(ctx, &model.Article{Title: "t", Content: "c", Author: "a"}))
		}
		items, total, err := d.List(ctx, 0, 2)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, int64(3))
		assert.Len(t, items, 2) // limit 生效
		return errRollback
	})
}
