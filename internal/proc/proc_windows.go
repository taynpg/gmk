//go:build windows

package proc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	pathEnvName = "Path" // Windows 环境变量名大小写不敏感，规范名用 "Path"
	envKeyFold  = true
)

// applyHideWindow 为捕获模式的子进程设置 CREATE_NO_WINDOW，
// 避免从 GUI/无控制台环境启动时弹出黑色 cmd 窗口。
func applyHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// findExecutable 在 Windows 下按 PATHEXT 规则查找可执行文件。
// 调用方保证 cand 是目录+文件名的完整候选路径。
func findExecutable(cand string, env map[string]string) string {
	if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
		if hasExecExt(cand, env) {
			return cand
		}
	}
	for _, ext := range pathExts(env) {
		p := cand + ext
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

func hasExecExt(path string, env map[string]string) bool {
	cur := strings.ToLower(filepath.Ext(path))
	if cur == "" {
		return false
	}
	for _, ext := range pathExts(env) {
		if cur == strings.ToLower(ext) {
			return true
		}
	}
	return false
}

func pathExts(env map[string]string) []string {
	raw, ok := lookupEnvKey(env, "PATHEXT")
	if !ok || env[raw] == "" {
		return []string{".com", ".exe", ".bat", ".cmd"}
	}
	var out []string
	for _, e := range strings.Split(env[raw], ";") {
		e = strings.TrimSpace(e)
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}
