package xjob

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xxl-job/xxl-job-executor-go"

	"github.com/example/go-template/pkg/aikit/log"
)

// LogHandler manages file-based job execution logs.
type LogHandler struct {
	LogDir string
	MaxAge int
	stopCh chan struct{}
}

// NewLogHandler creates a new LogHandler and starts the cleanup goroutine.
func NewLogHandler(logDir string, maxAge int) *LogHandler {
	h := &LogHandler{
		LogDir: logDir,
		MaxAge: maxAge,
		stopCh: make(chan struct{}),
	}
	go h.startCleanupLoop()
	return h
}

// Close stops the cleanup goroutine.
func (h *LogHandler) Close() {
	close(h.stopCh)
}

// TaskLogHandler implements xxl.LogHandler for the XXL-Job admin log pull API.
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

	return &xxl.LogRes{
		Code: xxl.SuccessCode,
		Content: xxl.LogResContent{
			FromLineNum: req.FromLineNum,
			ToLineNum:   toLineNum,
			LogContent:  content,
			IsEnd:       true,
		},
	}
}

// Info writes a job execution log entry.
func Info(logID int64, format string, args ...interface{}) {
	if defaultLogHandler == nil {
		return
	}
	defaultLogHandler.writeLog(logID, fmt.Sprintf(format, args...))
}

var defaultLogHandler *LogHandler

func setDefaultLogHandler(h *LogHandler) {
	defaultLogHandler = h
}

// writeLog appends a timestamped log entry to the job log file.
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
	defer f.Close()

	line := fmt.Sprintf("%s %s\n", now.Format("2006-01-02 15:04:05"), content)
	_, err = f.WriteString(line)
	return err
}

// readLog reads a log file starting from the given line number.
func (h *LogHandler) readLog(logPath string, fromLineNum int) (toLineNum int, content string) {
	f, err := os.Open(logPath)
	if err != nil {
		return fromLineNum, ""
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum >= fromLineNum {
			lines = append(lines, scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		log.Warn("[XxlJob][readLog][error]: %v", err)
	}

	return lineNum + 1, strings.Join(lines, "\n")
}

// getLogPath returns the log file path for a given time and log ID.
func (h *LogHandler) getLogPath(t time.Time, logID int64) string {
	dateDir := t.Format("2006-01-02")
	return filepath.Join(h.LogDir, dateDir, strconv.FormatInt(logID, 10)+".log")
}

// startCleanupLoop periodically removes old log directories.
func (h *LogHandler) startCleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
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

// cleanup removes log directories older than MaxAge days.
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
		t, err := time.Parse("2006-01-02", entry.Name())
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
