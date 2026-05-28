package log

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	// Init with stdout should not panic
	assert.NotPanics(t, func() {
		Init(&Config{
			Stdout: true,
			Level:  "debug",
		})
	})
}

func TestLogLevels(t *testing.T) {
	Init(&Config{Stdout: true, Level: "debug"})
	assert.NotPanics(t, func() {
		Debug("debug %s", "msg")
		Info("info %s", "msg")
		Warn("warn %s", "msg")
		Error("error %s", "msg")
	})
}

func TestParseDSN(t *testing.T) {
	ac := parseDSN("udp://10.1.172.180:1513?chan=1024&timeout=100ms")
	assert.Equal(t, "udp", ac.Proto)
	assert.Equal(t, "10.1.172.180:1513", ac.Addr)
	assert.Equal(t, 1024, ac.Chan)
	assert.Equal(t, "100ms", ac.Timeout.String())
}

func TestParseDSNInvalid(t *testing.T) {
	assert.Panics(t, func() {
		parseDSN("://invalid url\x00")
	})
}

func TestParseLevel(t *testing.T) {
	assert.Equal(t, _debugLevel, ParseLevel("debug"))
	assert.Equal(t, _infoLevel, ParseLevel("info"))
	assert.Equal(t, _warnLevel, ParseLevel("WARN"))
	assert.Equal(t, _errorLevel, ParseLevel("ERROR"))
	assert.Equal(t, _fatalLevel, ParseLevel("FATAL"))
	assert.Equal(t, _infoLevel, ParseLevel("unknown"))
}
