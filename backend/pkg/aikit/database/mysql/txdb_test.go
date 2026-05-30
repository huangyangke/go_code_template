package mysql

import (
	"context"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openTxTestDB(t *testing.T) *Database {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&testModel{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return &Database{DB: db}
}

func TestTxDB_NoTx(t *testing.T) {
	d := openTxTestDB(t)
	ctx := context.Background()

	got := d.TxDB(ctx)
	if got != d.DB {
		t.Error("TxDB should return default DB when no tx in context")
	}
}

func TestTxDB_WithTx(t *testing.T) {
	d := openTxTestDB(t)
	ctx := context.Background()

	var txDB *gorm.DB
	err := d.ExecTx(ctx, func(ctx context.Context) error {
		txDB = d.TxDB(ctx)
		if txDB == d.DB {
			t.Error("TxDB should return tx DB when inside transaction")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecTx: %v", err)
	}
}

func TestExecTx_Commit(t *testing.T) {
	d := openTxTestDB(t)
	ctx := context.Background()

	err := d.ExecTx(ctx, func(ctx context.Context) error {
		return d.CreateCtx(ctx, &testModel{Name: "in_tx"})
	})
	if err != nil {
		t.Fatalf("ExecTx: %v", err)
	}

	var count int64
	d.DB.Model(&testModel{}).Count(&count)
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestExecTx_Rollback(t *testing.T) {
	d := openTxTestDB(t)
	ctx := context.Background()

	err := d.ExecTx(ctx, func(ctx context.Context) error {
		if err := d.CreateCtx(ctx, &testModel{Name: "will_rollback"}); err != nil {
			return err
		}
		return fmt.Errorf("forced error")
	})
	if err == nil {
		t.Fatal("expected error from ExecTx")
	}

	var count int64
	d.DB.Model(&testModel{}).Count(&count)
	if count != 0 {
		t.Errorf("count = %d, want 0 (rolled back)", count)
	}
}

func TestCreateCtx_NoTx(t *testing.T) {
	d := openTxTestDB(t)
	ctx := context.Background()

	if err := d.CreateCtx(ctx, &testModel{Name: "no_tx"}); err != nil {
		t.Fatalf("CreateCtx: %v", err)
	}

	var m testModel
	if err := d.DB.First(&m, "name = ?", "no_tx").Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if m.Name != "no_tx" {
		t.Errorf("Name = %q, want %q", m.Name, "no_tx")
	}
}

func TestFindCtx_InsideTx(t *testing.T) {
	d := openTxTestDB(t)
	ctx := context.Background()

	// Create outside tx
	d.DB.Create(&testModel{Name: "existing"})

	// Create inside tx (not committed yet)
	var found []testModel
	err := d.ExecTx(ctx, func(ctx context.Context) error {
		if err := d.CreateCtx(ctx, &testModel{Name: "in_tx"}); err != nil {
			return err
		}
		return d.FindCtx(ctx, &found)
	})
	if err != nil {
		t.Fatalf("ExecTx: %v", err)
	}

	if len(found) != 2 {
		t.Errorf("found %d records, want 2", len(found))
	}
}

func TestPing(t *testing.T) {
	d := openTxTestDB(t)
	ctx := context.Background()

	if err := d.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
