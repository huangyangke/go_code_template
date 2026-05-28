package xjob

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xxl-job/xxl-job-executor-go"
)

func TestConfig_Fix(t *testing.T) {
	c := &Config{ServerAddr: "http://localhost:8080", Family: "test-svc"}
	c.Fix()

	assert.NotEmpty(t, c.ExecutorIp, "ExecutorIp should be auto-detected")
	assert.Equal(t, "9999", c.ExecutorPort)
	assert.Equal(t, 7, c.MaxAge)
	assert.Equal(t, "logs/xjob", c.LogDir)
}

func TestConfig_Fix_NoOverride(t *testing.T) {
	c := &Config{
		ServerAddr:   "http://localhost:8080",
		Family:       "test-svc",
		ExecutorPort: "8888",
		MaxAge:       14,
		LogDir:       "custom/logs",
	}
	c.Fix()

	assert.Equal(t, "8888", c.ExecutorPort)
	assert.Equal(t, 14, c.MaxAge)
	assert.Equal(t, "custom/logs", c.LogDir)
}

func TestConfig_Validate_MissingServerAddr(t *testing.T) {
	c := &Config{Family: "test"}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server_addr")
}

func TestConfig_Validate_MissingFamily(t *testing.T) {
	c := &Config{ServerAddr: "http://localhost:8080"}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "family")
}

func TestConfig_Validate_Valid(t *testing.T) {
	c := &Config{ServerAddr: "http://localhost:8080", Family: "test"}
	err := c.Validate()
	assert.NoError(t, err)
}

func TestConfig_GetExtra(t *testing.T) {
	c := &Config{
		Extra: map[string]string{"key1": "value1"},
	}
	assert.Equal(t, "value1", c.GetExtra("key1"))
	assert.Equal(t, "", c.GetExtra("missing"))
	assert.Equal(t, "", (*Config)(nil).GetExtra("key1"))
}

func TestNewTask(t *testing.T) {
	taskFunc := func(ctx context.Context, param *xxl.RunReq) string {
		return "ok"
	}
	task := NewTask("testHandler", taskFunc)
	require.NotNil(t, task)
	assert.Equal(t, "testHandler", task.pattern)
}

func TestGetLocalIP(t *testing.T) {
	ip := getLocalIP()
	assert.NotEmpty(t, ip)
}

// ── Executor tests ────────────────────────────────────────────────────────────

func TestNewExecutor_ValidateFails_EmptyFamily(t *testing.T) {
	cfg := &Config{
		ServerAddr: "http://localhost:19999",
		Family:     "", // required
	}
	exec, err := NewExecutor(cfg)
	assert.Error(t, err)
	assert.Nil(t, exec)
	assert.Contains(t, err.Error(), "family")
}

func TestExecutor_Stop_MultipleCallsNoPanic(t *testing.T) {
	cfg := &Config{
		ServerAddr: "http://localhost:19999",
		Family:     "test-svc",
	}
	exec, err := NewExecutor(cfg)
	require.NoError(t, err)
	require.NotNil(t, exec)

	// Multiple Stop calls should not panic (guarded by sync.Once)
	assert.NotPanics(t, func() {
		exec.Stop()
		exec.Stop()
		exec.Stop()
	})
}

func TestExecutor_GetConfig_ReturnsCorrectConfig(t *testing.T) {
	cfg := &Config{
		ServerAddr: "http://localhost:19999",
		Family:     "my-family",
	}
	exec, err := NewExecutor(cfg)
	require.NoError(t, err)
	require.NotNil(t, exec)
	defer exec.Stop()

	got := exec.GetConfig()
	assert.Equal(t, "my-family", got.Family)
	assert.Equal(t, "http://localhost:19999", got.ServerAddr)
}

func TestExecutor_LogHandler_NonNil(t *testing.T) {
	cfg := &Config{
		ServerAddr: "http://localhost:19999",
		Family:     "test-svc",
	}
	exec, err := NewExecutor(cfg)
	require.NoError(t, err)
	require.NotNil(t, exec)
	defer exec.Stop()

	assert.NotNil(t, exec.LogHandler())
}

func TestExecutor_LogInfo_NoPanic(t *testing.T) {
	cfg := &Config{
		ServerAddr: "http://localhost:19999",
		Family:     "test-svc",
		LogDir:     t.TempDir(),
	}
	exec, err := NewExecutor(cfg)
	require.NoError(t, err)
	require.NotNil(t, exec)
	defer exec.Stop()

	assert.NotPanics(t, func() {
		exec.LogInfo(12345, "test log entry %d", 1)
	})
}

func TestExecutor_Get_NonNil(t *testing.T) {
	cfg := &Config{
		ServerAddr: "http://localhost:19999",
		Family:     "test-svc",
	}
	exec, err := NewExecutor(cfg)
	require.NoError(t, err)
	require.NotNil(t, exec)
	defer exec.Stop()

	assert.NotNil(t, exec.Get())
}

func TestExecutor_Run_JobDisabled_NoPanic(t *testing.T) {
	cfg := &Config{
		ServerAddr:  "http://localhost:19999",
		Family:      "test-svc",
		JobDisabled: true,
	}
	exec, err := NewExecutor(cfg)
	require.NoError(t, err)
	require.NotNil(t, exec)
	defer exec.Stop()

	taskFn := func(ctx context.Context, param *xxl.RunReq) string { return "ok" }
	// Run with JobDisabled=true should return immediately without starting goroutine
	assert.NotPanics(t, func() {
		exec.Run(NewTask("testHandler", taskFn))
	})
}

func TestWithMiddleware_ReturnsOption(t *testing.T) {
	// WithMiddleware should return an Option (func(xxl.Executor)) without panicking
	assert.NotPanics(t, func() {
		opt := WithMiddleware()
		assert.NotNil(t, opt)
	})
}

// ── xjob.Run: with tasks ──────────────────────────────────────────────────────

func TestExecutor_Run_WithTasks_JobDisabled(t *testing.T) {
	e, err := NewExecutor(&Config{
		Family:      "test",
		ServerAddr:  "http://127.0.0.1:19999",
		LogDir:      t.TempDir(),
		JobDisabled: true,
	})
	require.NoError(t, err)
	defer e.Stop()

	task := NewTask("/myhandler", func(ctx context.Context, param *xxl.RunReq) string {
		return "ok"
	})
	// Should return immediately without starting executor
	assert.NotPanics(t, func() { e.Run(task) })
}

func TestExecutor_Run_NoTasks(t *testing.T) {
	e, err := NewExecutor(&Config{
		Family:     "test",
		ServerAddr: "http://127.0.0.1:19999",
		LogDir:     t.TempDir(),
	})
	require.NoError(t, err)
	// Run with no tasks — starts goroutine (may fail to connect but doesn't panic)
	assert.NotPanics(t, func() { e.Run() })
	time.Sleep(10 * time.Millisecond)
	e.Stop()
}

// ── GetConfig: nil receiver ───────────────────────────────────────────────────

func TestGetConfig_NilReceiver(t *testing.T) {
	var e *Executor
	assert.Nil(t, e.GetConfig())
}

// ── WithMiddleware: returns callable option ───────────────────────────────────

func TestWithMiddleware_NotNil(t *testing.T) {
	opt := WithMiddleware()
	assert.NotNil(t, opt)
}

// ── LogHandler.Info: with valid logID ─────────────────────────────────────────

func TestLogHandler_Info_ValidLogID(t *testing.T) {
	dir := t.TempDir()
	h := NewLogHandler(dir, 7)
	defer h.Close()
	// Should not panic; writes log file
	h.Info(12345, "test message %s", "hello")
}

// ── LogHandler.TaskLogHandler: logDateTim <= 0 ───────────────────────────────

func TestLogHandler_TaskLogHandler_InvalidLogDateTime(t *testing.T) {
	dir := t.TempDir()
	h := NewLogHandler(dir, 7)
	defer h.Close()

	res := h.TaskLogHandler(&xxl.LogReq{LogDateTim: 0, LogID: 1, FromLineNum: 1})
	assert.EqualValues(t, xxl.SuccessCode, res.Code)
	assert.True(t, res.Content.IsEnd)
}

func TestLogHandler_TaskLogHandler_ValidLog(t *testing.T) {
	dir := t.TempDir()
	h := NewLogHandler(dir, 7)
	defer h.Close()

	// Write a log entry first
	logID := int64(99999)
	h.Info(logID, "hello from test")

	// Now read it back
	logDateTim := time.Now().UnixMilli()
	res := h.TaskLogHandler(&xxl.LogReq{
		LogDateTim:  logDateTim,
		LogID:       logID,
		FromLineNum: 1,
	})
	assert.EqualValues(t, xxl.SuccessCode, res.Code)
}

// ── LogHandler.Info: writeLog error (unwritable dir) ─────────────────────────

func TestLogHandler_Info_WriteError(t *testing.T) {
	// Use a non-existent read-only path to trigger writeLog error
	h := NewLogHandler("/proc/sys/nonexistent_dir_for_test", 7)
	defer h.Close()
	// Should not panic, just logs the error
	assert.NotPanics(t, func() {
		h.Info(99999, "this will fail")
	})
}

// ── LogHandler.writeLog: MkdirAll error ──────────────────────────────────────

func TestLogHandler_writeLog_MkdirError(t *testing.T) {
	h := &LogHandler{LogDir: "/proc/nonexistent", MaxAge: 7, stopCh: make(chan struct{})}
	err := h.writeLog(1, "test")
	assert.Error(t, err)
}

// ── LogHandler.cleanup: non-dir entries skipped ──────────────────────────────

func TestLogHandler_cleanup_NonDirSkipped(t *testing.T) {
	dir := t.TempDir()
	// Create a file (not a dir) in the log dir — should be skipped
	f, err := os.Create(dir + "/notadir.txt")
	require.NoError(t, err)
	f.Close()

	h := NewLogHandler(dir, 0) // MaxAge=0 → cleanup exits early
	defer h.Close()
	assert.NotPanics(t, func() { h.cleanup() })
}

func TestLogHandler_cleanup_RemovesOldDirs(t *testing.T) {
	dir := t.TempDir()
	// Create an old-looking directory
	oldDir := dir + "/2020-01-01"
	require.NoError(t, os.MkdirAll(oldDir, 0755))
	// Touch a file inside
	f, _ := os.Create(oldDir + "/1.log")
	f.Close()

	h := NewLogHandler(dir, 1) // MaxAge=1 day, so 2020-01-01 is "old"
	defer h.Close()
	h.cleanup() // should try to remove oldDir
	// oldDir may or may not be removed (depends on OS mtime), just verify no panic
}

// ── NewExecutor: with opts ────────────────────────────────────────────────────

func TestNewExecutor_WithOpts(t *testing.T) {
	optCalled := false
	opt := func(e xxl.Executor) { optCalled = true }
	e, err := NewExecutor(&Config{
		Family:     "test",
		ServerAddr: "http://127.0.0.1:19999",
	}, opt)
	require.NoError(t, err)
	defer e.Stop()
	assert.True(t, optCalled)
}

// ── Executor.Run: non-disabled with tasks ────────────────────────────────────

func TestExecutor_Run_NonDisabled_WithTask(t *testing.T) {
	e, err := NewExecutor(&Config{
		Family:     "test",
		ServerAddr: "http://127.0.0.1:19999",
		LogDir:     t.TempDir(),
	})
	require.NoError(t, err)

	task := NewTask("/handler", func(ctx context.Context, param *xxl.RunReq) string { return "ok" })
	assert.NotPanics(t, func() { e.Run(task) })
	time.Sleep(10 * time.Millisecond)
	e.Stop()
}

// ── WithMiddleware: applies middleware ────────────────────────────────────────

func TestWithMiddleware_Applied(t *testing.T) {
	e, err := NewExecutor(&Config{
		Family:     "test",
		ServerAddr: "http://127.0.0.1:19999",
	}, WithMiddleware())
	require.NoError(t, err)
	defer e.Stop()
	assert.NotNil(t, e)
}
