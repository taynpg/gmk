// Package logger 提供与原 spdlog 一致的轻量日志：
// 格式 [HH:MM:SS.ms] [level] message（不含年月日），级别用颜色高亮。
// 日志级别通过环境变量调整：GMK_LOG_LEVEL（兼容 SPDLOG_LEVEL），
// 取值 trace/debug/info/warning/error/critical/off。
package logger

import (
	"fmt"
	"gmk/internal/encode"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelCritical
	LevelOff
)

var (
	mu      sync.Mutex
	out     io.Writer
	level   = LevelInfo
	colored = isTerminal(os.Stdout)
)

func init() {
	// 控制台走 WriteConsoleW(UTF-16)，管道/文件走 UTF-8 字节
	out = encode.StdoutWriter()

	// 兼容原 SPDLOG_LEVEL，新增 GMK_LOG_LEVEL（优先级更高）
	v := os.Getenv("GMK_LOG_LEVEL")
	if v == "" {
		v = os.Getenv("SPDLOG_LEVEL")
	}
	if v != "" {
		if lv, ok := parseLevel(v); ok {
			level = lv
		}
	}
}

func parseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return LevelTrace, true
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "warn", "warning":
		return LevelWarn, true
	case "error", "err":
		return LevelError, true
	case "critical":
		return LevelCritical, true
	case "off":
		return LevelOff, true
	default:
		return LevelInfo, false
	}
}

// isTerminal 仅用标准库判断 stdout 是否为字符设备（控制台）。
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// spdlog 默认配色：debug=cyan info=green warning=yellow(bold) error=red(bold) ...
var levelStyle = map[Level]struct {
	name  string
	color string
}{
	LevelTrace:    {"trace", "37"},
	LevelDebug:    {"debug", "36"},
	LevelInfo:     {"info", "32"},
	LevelWarn:     {"warning", "1;33"},
	LevelError:    {"error", "1;31"},
	LevelCritical: {"critical", "1;31"},
}

func logf(lv Level, format string, args ...any) {
	if lv < level {
		return
	}
	msg := fmt.Sprintf(format, args...)
	now := time.Now().Format("15:04:05.000")
	style := levelStyle[lv]

	var line string
	if colored {
		line = fmt.Sprintf("[%s] [\x1b[%sm%s\x1b[0m] %s\n", now, style.color, style.name, msg)
	} else {
		line = fmt.Sprintf("[%s] [%s] %s\n", now, style.name, msg)
	}

	mu.Lock()
	defer mu.Unlock()
	_, _ = io.WriteString(out, line)
}

func Tracef(format string, args ...any)    { logf(LevelTrace, format, args...) }
func Debugf(format string, args ...any)    { logf(LevelDebug, format, args...) }
func Infof(format string, args ...any)     { logf(LevelInfo, format, args...) }
func Warnf(format string, args ...any)     { logf(LevelWarn, format, args...) }
func Errorf(format string, args ...any)    { logf(LevelError, format, args...) }
func Criticalf(format string, args ...any) { logf(LevelCritical, format, args...) }
