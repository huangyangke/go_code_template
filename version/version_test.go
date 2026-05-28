package version_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/huangyangke/go-aikit/version"
)

func TestInfo_Fields(t *testing.T) {
	info := version.Info()
	// All fields should exist as keys (may be empty when not built with -ldflags)
	assert.Contains(t, info, "version")
	assert.Contains(t, info, "branch")
	assert.Contains(t, info, "commit")
	assert.Contains(t, info, "date")
}

func TestString_ContainsAllFields(t *testing.T) {
	s := version.String()
	assert.Contains(t, s, "version=")
	assert.Contains(t, s, "branch=")
	assert.Contains(t, s, "commit=")
	assert.Contains(t, s, "date=")
}

func TestString_Format(t *testing.T) {
	s := version.String()
	// should be a single line with space-separated key=value pairs
	assert.False(t, strings.Contains(s, "\n"), "should be single line")
}
