package mysql

import (
	"errors"
	"testing"

	gomysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestMetricsPlugin_Name(t *testing.T) {
	p := NewMetricsPlugin("test_ds")
	assert.Equal(t, "aikit_sql_metrics", p.Name())
}

func TestNewMetricsPlugin(t *testing.T) {
	p := NewMetricsPlugin("mydb")
	assert.Equal(t, "mydb", p.name)
}

func TestSqlAcceptable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, true},
		{"ErrRecordNotFound", gorm.ErrRecordNotFound, true},
		{"wrapped ErrRecordNotFound", errors.Join(gorm.ErrRecordNotFound, nil), true},
		{"duplicate entry 1062", &gomysql.MySQLError{Number: 1062, Message: "Duplicate entry"}, true},
		{"other mysql error", &gomysql.MySQLError{Number: 1045, Message: "Access denied"}, false},
		{"non-mysql error", errors.New("connection refused"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, sqlAcceptable(tt.err))
		})
	}
}

func TestBreakerPlugin_Name(t *testing.T) {
	assert.Equal(t, "aikit_sql_breaker", "aikit_sql_breaker")
}

func TestConfig_BreakerField(t *testing.T) {
	cfg := &Config{
		DSN:            "user:pass@tcp(localhost:3306)/test",
		Breaker:        nil,
		DisableMetrics: true,
	}
	cfg.Fix()
	assert.Nil(t, cfg.Breaker)
	assert.True(t, cfg.DisableMetrics)
}

func TestConfig_Fix_MaxIdleTime(t *testing.T) {
	cfg := &Config{DSN: "user:pass@tcp(localhost:3306)/test"}
	cfg.Fix()
	assert.Greater(t, cfg.MaxIdleTime.Nanoseconds(), int64(0))
}

