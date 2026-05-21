package mysql

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testModel struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
	Name      string
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Use(&TimestampPlugin{}); err != nil {
		t.Fatalf("register timestamp plugin: %v", err)
	}
	if err := db.AutoMigrate(&testModel{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestTimestampPlugin_Create_SetsTimestamps(t *testing.T) {
	db := openTestDB(t)

	m := &testModel{Name: "test"}
	before := time.Now()
	if err := db.Create(m).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	if m.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set on create")
	}
	if m.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set on create")
	}
	if m.CreatedAt.Before(before) {
		t.Error("CreatedAt should not be before create time")
	}
}

func TestTimestampPlugin_Create_NoOverride(t *testing.T) {
	db := openTestDB(t)

	presetTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &testModel{Name: "test", CreatedAt: presetTime, UpdatedAt: presetTime}
	if err := db.Create(m).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	if !m.CreatedAt.Equal(presetTime) {
		t.Errorf("CreatedAt = %v, want %v (should not be overwritten)", m.CreatedAt, presetTime)
	}
}

func TestTimestampPlugin_Update_SetsUpdatedAt(t *testing.T) {
	db := openTestDB(t)

	m := &testModel{Name: "before"}
	if err := db.Create(m).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	originalUpdatedAt := m.UpdatedAt

	time.Sleep(10 * time.Millisecond)

	if err := db.Model(m).Update("name", "after").Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	var updated testModel
	if err := db.First(&updated, m.ID).Error; err != nil {
		t.Fatalf("find: %v", err)
	}

	if !updated.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("UpdatedAt = %v, should be after %v", updated.UpdatedAt, originalUpdatedAt)
	}
}

func TestTimestampPlugin_UpdateColumn_SkipsUpdatedAt(t *testing.T) {
	db := openTestDB(t)

	m := &testModel{Name: "before"}
	if err := db.Create(m).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	originalUpdatedAt := m.UpdatedAt

	time.Sleep(10 * time.Millisecond)

	if err := db.Model(m).UpdateColumn("name", "after").Error; err != nil {
		t.Fatalf("update column: %v", err)
	}

	var updated testModel
	if err := db.First(&updated, m.ID).Error; err != nil {
		t.Fatalf("find: %v", err)
	}

	// UpdateColumn should NOT auto-set updated_at
	if !updated.UpdatedAt.Equal(originalUpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v (UpdateColumn should not touch updated_at)", updated.UpdatedAt, originalUpdatedAt)
	}
}
