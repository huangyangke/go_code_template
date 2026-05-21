package mysql

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestMetricsPlugin_Name(t *testing.T) {
	p := NewMetricsPlugin("test_ds")
	assert.Equal(t, "aikit_sql_metrics", p.Name())
}

func TestMetricsPlugin_OperationName(t *testing.T) {
	p := &MetricsPlugin{name: "test"}
	assert.Equal(t, "unknown", p.operationName(nil))
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
		{"duplicate entry", dupEntryErr(), true},
		{"other error", otherErr(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, sqlAcceptable(tt.err))
		})
	}
}

func dupEntryErr() error {
	return &dupErr{}
}

type dupErr struct{}

func (e *dupErr) Error() string {
	return "Error 1062: Duplicate entry '1' for key 'PRIMARY'"
}

func otherErr() error {
	return &otherSqlErr{}
}

type otherSqlErr struct{}

func (e *otherSqlErr) Error() string {
	return "Error 1045: Access denied"
}

func TestBreakerPlugin_Name(t *testing.T) {
	assert.Equal(t, "aikit_sql_breaker", "aikit_sql_breaker")
}

func TestNewMetricsPlugin(t *testing.T) {
	p := NewMetricsPlugin("mydb")
	assert.Equal(t, "mydb", p.name)
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
