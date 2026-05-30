package mysql

import (
	"context"
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// repoModel 带软删除字段的测试模型，覆盖 Repository 的软删除路径.
// 不内嵌 Model：其列默认值是 MySQL 专用 DDL（ON UPDATE CURRENT_TIMESTAMP），
// SQLite 无法解析。软删除由 gorm.DeletedAt 类型本身驱动，与列默认值无关.
type repoModel struct {
	ID        uint `gorm:"primaryKey;autoIncrement"`
	Name      string
	Age       int
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (repoModel) TableName() string { return "repo_models" }

func openRepoTestDB(t *testing.T) *Database {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&repoModel{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return &Database{DB: db}
}

func TestRepository_CreateAndGetByID(t *testing.T) {
	d := openRepoTestDB(t)
	repo := NewGenericRepository[repoModel](d)
	ctx := context.Background()

	m := &repoModel{Name: "alice", Age: 30}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.ID == 0 {
		t.Fatal("expected ID to be populated after Create")
	}

	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "alice" || got.Age != 30 {
		t.Errorf("got %+v, want name=alice age=30", got)
	}
}

func TestRepository_GetByID_NotFound(t *testing.T) {
	d := openRepoTestDB(t)
	repo := NewGenericRepository[repoModel](d)

	_, err := repo.GetByID(context.Background(), 999)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("err = %v, want ErrRecordNotFound", err)
	}
}

func TestRepository_List(t *testing.T) {
	d := openRepoTestDB(t)
	repo := NewGenericRepository[repoModel](d)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := repo.Create(ctx, &repoModel{Name: "u", Age: i}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	items, total, err := repo.List(ctx, 0, 3)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(items) != 3 {
		t.Errorf("len(items) = %d, want 3 (limited)", len(items))
	}
}

func TestRepository_FindWhereAndCount(t *testing.T) {
	d := openRepoTestDB(t)
	repo := NewGenericRepository[repoModel](d)
	ctx := context.Background()

	_ = repo.Create(ctx, &repoModel{Name: "young", Age: 10})
	_ = repo.Create(ctx, &repoModel{Name: "old", Age: 50})

	found, err := repo.FindWhere(ctx, "age > ?", 18)
	if err != nil {
		t.Fatalf("FindWhere: %v", err)
	}
	if len(found) != 1 || found[0].Name != "old" {
		t.Errorf("FindWhere returned %+v, want 1 record 'old'", found)
	}

	cnt, err := repo.Count(ctx, "age > ?", 18)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if cnt != 1 {
		t.Errorf("Count = %d, want 1", cnt)
	}

	all, err := repo.Count(ctx, nil)
	if err != nil {
		t.Fatalf("Count(nil): %v", err)
	}
	if all != 2 {
		t.Errorf("Count(nil) = %d, want 2", all)
	}
}

func TestRepository_Update(t *testing.T) {
	d := openRepoTestDB(t)
	repo := NewGenericRepository[repoModel](d)
	ctx := context.Background()

	m := &repoModel{Name: "before", Age: 1}
	_ = repo.Create(ctx, m)

	updated, err := repo.Update(ctx, m.ID, map[string]any{"name": "after"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "after" {
		t.Errorf("Name = %q, want after", updated.Name)
	}
}

func TestRepository_Delete_SoftAndNotFound(t *testing.T) {
	d := openRepoTestDB(t)
	repo := NewGenericRepository[repoModel](d)
	ctx := context.Background()

	m := &repoModel{Name: "doomed"}
	_ = repo.Create(ctx, m)

	if err := repo.Delete(ctx, m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// 软删除后常规查询应找不到.
	if _, err := repo.GetByID(ctx, m.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("after soft delete GetByID err = %v, want ErrRecordNotFound", err)
	}

	// 记录仍在表中（DeletedAt 非空）.
	var raw int64
	d.DB.Unscoped().Model(&repoModel{}).Count(&raw)
	if raw != 1 {
		t.Errorf("unscoped count = %d, want 1 (soft deleted row retained)", raw)
	}

	// 再次删除已不存在的记录返回 NotFound.
	if err := repo.Delete(ctx, m.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("double delete err = %v, want ErrRecordNotFound", err)
	}
}

func TestRepository_ReusesExecTx(t *testing.T) {
	d := openRepoTestDB(t)
	repo := NewGenericRepository[repoModel](d)
	ctx := context.Background()

	// 事务内创建后返回错误，应全部回滚.
	err := d.ExecTx(ctx, func(ctx context.Context) error {
		if err := repo.Create(ctx, &repoModel{Name: "tx1"}); err != nil {
			return err
		}
		if err := repo.Create(ctx, &repoModel{Name: "tx2"}); err != nil {
			return err
		}
		return errors.New("forced rollback")
	})
	if err == nil {
		t.Fatal("expected forced rollback error")
	}

	cnt, _ := repo.Count(ctx, nil)
	if cnt != 0 {
		t.Errorf("count = %d, want 0 (Repository must honor ExecTx rollback)", cnt)
	}
}
