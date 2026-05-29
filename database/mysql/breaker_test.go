package mysql

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/huangyangke/go-aikit/internal/testutil"
	"github.com/huangyangke/go-aikit/resilience"
)

// openBreakerTestDB opens a sqlite :memory: DB with the breaker plugin attached.
func openBreakerTestDB(t *testing.T, cfg resilience.Config) *gorm.DB {
	t.Helper()
	db := testutil.NewSQLiteDBWithModels(t, &testModel{})
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Use(NewBreakerPlugin(cfg)); err != nil {
		t.Fatalf("register breaker plugin: %v", err)
	}
	return db
}

// TestBreakerPlugin_OpenBlocksSQL is the headline regression test for this fix:
// when the breaker is open, db.Create must NOT actually send SQL to the
// database. We verify by counting rows via a separate gorm.DB handle (without
// the plugin) on the *same* sqlite handle — i.e. we use the underlying *sql.DB
// to bypass the plugin chain.
func TestBreakerPlugin_OpenBlocksSQL(t *testing.T) {
	cfg := resilience.Config{
		Name:                   "test-mysql-breaker-open",
		RequestVolumeThreshold: 3,
		ErrorPercentThreshold:  50,
		SleepWindow:            60 * time.Second,
	}
	db := openBreakerTestDB(t, cfg)

	// Trip the breaker by issuing failing operations. We use Find against a
	// non-existent table to generate genuine non-acceptable errors.
	type doesNotExist struct{ ID uint }
	for i := 0; i < 10; i++ {
		var dst []doesNotExist
		_ = db.Find(&dst).Error
	}

	// Now the breaker should be open. Attempt a Create; it must fail with
	// ErrCircuitOpen and must NOT insert a row.
	err := db.Create(&testModel{Name: "should_not_insert"}).Error
	assert.True(t, resilience.IsCircuitOpen(err), "expected open circuit, got %v", err)

	// Bypass the plugin: use the raw *sql.DB to count rows in test_models.
	sqlDB, err2 := db.DB()
	assert.NoError(t, err2)
	var count int
	row := sqlDB.QueryRow("SELECT COUNT(*) FROM test_models WHERE name = ?", "should_not_insert")
	assert.NoError(t, row.Scan(&count))
	assert.Equal(t, 0, count, "open breaker must block SQL — no row should be inserted")
}

// TestBreakerPlugin_RecordNotFoundDoesntCount verifies that ErrRecordNotFound
// is treated as acceptable by the plugin — a stream of NotFounds should not
// open the breaker.
func TestBreakerPlugin_RecordNotFoundDoesntCount(t *testing.T) {
	cfg := resilience.Config{
		Name:                   "test-mysql-breaker-notfound",
		RequestVolumeThreshold: 3,
		ErrorPercentThreshold:  50,
		SleepWindow:            60 * time.Second,
	}
	db := openBreakerTestDB(t, cfg)

	// 10 NotFounds.
	for i := 0; i < 10; i++ {
		var m testModel
		err := db.First(&m, "name = ?", "nonexistent").Error
		assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
	}

	// Breaker should still be closed: a normal Create must succeed.
	m := &testModel{Name: "after_notfounds"}
	assert.NoError(t, db.Create(m).Error)
	assert.NotZero(t, m.ID)
}
