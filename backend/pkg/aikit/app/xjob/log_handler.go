package xjob

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xxl-job/xxl-job-executor-go"

	"github.com/huangyangke/go-aikit/log"
)

// LogHandler 管理文件型任务执行日志.
type LogHandler struct {
	LogDir    string
	MaxAge    int
	stopCh    chan struct{}
	closeOnce sync.Once
}

// NewLogHandler 创建日志处理器并启动清理协程.
// 参数：logDir - 日志目录, maxAge - 保留天数.
// 返回值：日志处理器.
func NewLogHandler(logDir string, maxAge int) *LogHandler {
	h := &LogHandler{
		LogDir: logDir,
		MaxAge: maxAge,
		stopCh: make(chan struct{}),
	}
	go h.startCleanupLoop()
	return h
}

// Close 停止清理协程，可安全多次调用.
func (h *LogHandler) Close() {
	h.closeOnce.Do(func() {
		close(h.stopCh)
	})
}

// Info 写入任务执行日志.
// 参数：logID - 日志 ID, format - 格式字符串, args - 格式参数.
func (h *LogHandler) Info(logID int64, format string, args ...interface{}) {
	if err := h.writeLog(logID, fmt.Sprintf(format, args...)); err != nil {
		log.Error("[XxlJob][writeLog_error]: %v", err)
	}
}

// TaskLogHandler 实现 xxl.LogHandler，供 XXL-Job 调度中心拉取日志.
// 参数：req - 日志请求.
// 返回值：日志响应.
func (h *LogHandler) TaskLogHandler(req *xxl.LogReq) *xxl.LogRes {
	logDateTim := req.LogDateTim
	if logDateTim <= 0 {
		return &xxl.LogRes{
			Code: xxl.SuccessCode,
			Content: xxl.LogResContent{
				FromLineNum: req.FromLineNum,
				ToLineNum:   0,
				LogContent:  "invalid logDateTime",
				IsEnd:       true,
			},
		}
	}

	logPath := h.getLogPath(time.Unix(logDateTim/1000, 0), req.LogID)
	toLineNum, content := h.readLog(logPath, req.FromLineNum)

	// 仅当无更多内容可读时 IsEnd 为 true
	isEnd := content == ""
	return &xxl.LogRes{
		Code: xxl.SuccessCode,
		Content: xxl.LogResContent{
			FromLineNum: req.FromLineNum,
			ToLineNum:   toLineNum,
			LogContent:  content,
			IsEnd:       isEnd,
		},
	}
}

// writeLog 向日志文件追加带时间戳的日志行.
func (h *LogHandler) writeLog(logID int64, content string) error {
	now := time.Now()
	logPath := h.getLogPath(now, logID)

	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	line := fmt.Sprintf("%s %s\n", now.Format("2006-01-02 15:04:05"), content)
	_, err = f.WriteString(line)
	return err
}

// readLog 从指定行号（1-based）起读取日志文件.
func (h *LogHandler) readLog(logPath string, fromLineNum int) (toLineNum int, content string) {
	f, err := os.Open(logPath)
	if err != nil {
		return fromLineNum, ""
	}
	defer func() { _ = f.Close() }()

	var buf bytes.Buffer
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum >= fromLineNum {
			buf.WriteString(scanner.Text())
			buf.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		log.Warn("[XxlJob][readLog][error]: %v", err)
	}

	return lineNum, strings.TrimRight(buf.String(), "\n")
}

// getLogPath 返回指定时间和日志 ID 的日志文件路径.
func (h *LogHandler) getLogPath(t time.Time, logID int64) string {
	dateDir := t.Format("2006-01-02")
	return filepath.Join(h.LogDir, dateDir, strconv.FormatInt(logID, 10)+".log")
}

// startCleanupLoop 定期清理过期日志目录.
func (h *LogHandler) startCleanupLoop() {
	h.cleanup() // 启动时立即执行一次清理

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.cleanup()
		}
	}
}

// cleanup 删除超过 MaxAge 天数的日志目录.
func (h *LogHandler) cleanup() {
	if h.MaxAge <= 0 {
		return
	}

	entries, err := os.ReadDir(h.LogDir)
	if err != nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -h.MaxAge)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02", entry.Name(), time.Local)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			dirPath := filepath.Join(h.LogDir, entry.Name())
			_ = os.RemoveAll(dirPath)
			log.Info("[XxlJob][log_cleanup][removed=%s]", dirPath)
		}
	}
}
