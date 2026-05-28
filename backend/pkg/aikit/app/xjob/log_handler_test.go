package xjob

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xxl-job/xxl-job-executor-go"
)

func TestLogHandler_WriteAndRead(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewLogHandler(tmpDir, 7)
	defer h.Close()

	err := h.writeLog(12345, "test log message")
	require.NoError(t, err)

	logPath := h.getLogPath(time.Now(), 12345)
	_, err = os.Stat(logPath)
	require.NoError(t, err)

	toLineNum, content := h.readLog(logPath, 1)
	assert.True(t, toLineNum >= 1)
	assert.Contains(t, content, "test log message")
}

func TestLogHandler_TaskLogHandler(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewLogHandler(tmpDir, 7)
	defer h.Close()

	now := time.Now()
	err := h.writeLog(99999, "task log content")
	require.NoError(t, err)

	req := &xxl.LogReq{
		LogDateTim:  now.Unix() * 1000,
		LogID:       99999,
		FromLineNum: 1,
	}
	res := h.TaskLogHandler(req)
	assert.Equal(t, int64(xxl.SuccessCode), res.Code)
	assert.Contains(t, res.Content.LogContent, "task log content")
}

func TestLogHandler_GetLogPath(t *testing.T) {
	h := &LogHandler{LogDir: "logs/xjob"}
	tm := time.Date(2026, 5, 19, 10, 30, 0, 0, time.Local)
	path := h.getLogPath(tm, 12345)

	expected := filepath.Join("logs/xjob", "2026-05-19", "12345.log")
	assert.Equal(t, expected, path)
}

func TestLogHandler_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewLogHandler(tmpDir, 0) // MaxAge=0 means no cleanup
	defer h.Close()

	// Create an old log directory
	oldDir := filepath.Join(tmpDir, "2020-01-01")
	err := os.MkdirAll(oldDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(oldDir, "test.log"), []byte("old"), 0644)
	require.NoError(t, err)

	// With MaxAge=0, cleanup should not remove anything
	h.MaxAge = 0
	h.cleanup()
	_, err = os.Stat(oldDir)
	assert.NoError(t, err, "old dir should still exist when MaxAge=0")

	// With MaxAge=1, cleanup should remove old dirs
	h.MaxAge = 1
	h.cleanup()
	_, err = os.Stat(oldDir)
	assert.True(t, os.IsNotExist(err), "old dir should be removed when MaxAge=1")
}

func TestLogHandler_ReadNonExistent(t *testing.T) {
	h := &LogHandler{LogDir: t.TempDir()}
	toLineNum, content := h.readLog("/nonexistent/path.log", 1)
	assert.Equal(t, 1, toLineNum)
	assert.Equal(t, "", content)
}
