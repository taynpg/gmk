//go:build !windows

package proc

import (
	"os"
	"os/exec"
)

const (
	pathEnvName = "PATH"
	envKeyFold  = false
)

func applyHideWindow(cmd *exec.Cmd) {}

// findExecutable 在类 Unix 下要求是普通文件且带任意执行权限位。
func findExecutable(cand string, _ map[string]string) string {
	fi, err := os.Stat(cand)
	if err != nil || fi.IsDir() || fi.Mode().Perm()&0o111 == 0 {
		return ""
	}
	return cand
}
