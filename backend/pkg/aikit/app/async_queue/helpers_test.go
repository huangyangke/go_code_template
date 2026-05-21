package async_queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTaskPriority(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string defaults to 5", "", 5},
		{"digit 0", "0", 0},
		{"digit 9", "9", 9},
		{"digit 5", "5", 5},
		{"non-numeric string should default to 5", "high", 5},
		{"non-numeric with no digits should default to 5", "abc", 5},
		{"string with leading digit", "3-high", 3},
		{"string with embedded digit", "priority7", 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseTaskPriority(tt.input))
		})
	}
}

func TestExtractPriority(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected int
	}{
		{"missing key defaults to 5", map[string]interface{}{}, 5},
		{"int value", map[string]interface{}{"priority": 3}, 3},
		{"int64 value", map[string]interface{}{"priority": int64(7)}, 7},
		{"float64 value", map[string]interface{}{"priority": float64(8)}, 8},
		{"string digit", map[string]interface{}{"priority": "2"}, 2},
		{"non-numeric string should default to 5", map[string]interface{}{"priority": "high"}, 5},
		{"string with no digits should default to 5", map[string]interface{}{"priority": "abc"}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractPriority(tt.data))
		})
	}
}
