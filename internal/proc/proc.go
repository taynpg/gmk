// Package proc 封装子进程执行：
//   - RunCapture：捕获 stdout+stderr，整块按 GBK->UTF-8 转码后返回（vswhere / vcvars 用）
//   - RunInherit：stdio 直连父进程，输出经行级编码转码实时透传（cmake / ninja 用）
//
// 环境变量语义与原实现一致：以"当前进程环境"为底，overlay 覆盖（VS 模式传入缓存环境），
// 再把额外 PATH 追加到 Path/PATH。
package proc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gmk/internal/encode"
)

// RunCapture 执行命令并捕获合并输出（stdout 在前、stderr 在后），自动转 UTF-8。
// overlay 为需要覆盖的环境变量（nil 表示仅用当前环境），appendPath 为追加的 PATH 目录。
// 返回转码后的输出、进程退出码、启动错误（命令不存在等）。
func RunCapture(exe string, args []string, dir string, overlay map[string]string, appendPath []string) (string, int, error) {
	envMap := effectiveEnv(overlay, appendPath)
	resolved, err := resolveExe(exe, envMap, dir)
	if err != nil {
		return "", -1, err
	}

	cmd := exec.Command(resolved, args...)
	cmd.Dir = dir
	cmd.Env = envToSlice(envMap)
	cmd.Stdin = nil
	applyHideWindow(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			combined := append(append([]byte(nil), stdout.Bytes()...), stderr.Bytes()...)
			return string(encode.EnsureUTF8(combined)), ee.ExitCode(), nil
		}
		return "", -1, err
	}

	combined := append(append([]byte(nil), stdout.Bytes()...), stderr.Bytes()...)
	return string(encode.EnsureUTF8(combined)), 0, nil
}

// RunInherit 执行命令，子进程 stdin/stdout/stderr 与父进程共享。
// stdout/stderr 经 LineWriter 实时转码（GBK->UTF-8），保留颜色与进度输出。
func RunInherit(exe string, args []string, dir string, overlay map[string]string, appendPath []string) (int, error) {
	envMap := effectiveEnv(overlay, appendPath)
	resolved, err := resolveExe(exe, envMap, dir)
	if err != nil {
		return -1, err
	}

	cmd := exec.Command(resolved, args...)
	cmd.Dir = dir
	cmd.Env = envToSlice(envMap)
	cmd.Stdin = os.Stdin

	lw := encode.NewLineWriter(encode.StdoutWriter())
	cmd.Stdout = lw
	cmd.Stderr = lw

	runErr := cmd.Run()
	_ = lw.Flush()
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			return ee.ExitCode(), nil
		}
		return -1, runErr
	}
	return 0, nil
}

// effectiveEnv 以当前进程环境为底，应用 overlay（大小写不敏感的同名键覆盖），
// 最后追加 appendPath 到 PATH 变量。
func effectiveEnv(overlay map[string]string, appendPath []string) map[string]string {
	env := make(map[string]string, len(overlay)+64)
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		env[k] = v
	}
	for k, v := range overlay {
		if k == "" {
			continue
		}
		setEnvKey(env, k, v)
	}
	if len(appendPath) > 0 {
		extra := strings.Join(appendPath, string(os.PathListSeparator))
		if curKey, ok := lookupEnvKey(env, pathEnvName); ok {
			setEnvKey(env, curKey, env[curKey]+string(os.PathListSeparator)+extra)
		} else {
			setEnvKey(env, pathEnvName, extra)
		}
	}
	return env
}

// setEnvKey 写入环境变量，Windows 下同名（忽略大小写）键做覆盖而不是产生重复键。
func setEnvKey(env map[string]string, key, value string) {
	if old, ok := lookupEnvKey(env, key); ok && old != key {
		delete(env, old)
	}
	env[key] = value
}

// lookupEnvKey 按键查找，Windows 下忽略大小写。
func lookupEnvKey(env map[string]string, key string) (string, bool) {
	if _, ok := env[key]; ok {
		return key, true
	}
	if envKeyFold {
		for k := range env {
			if strings.EqualFold(k, key) {
				return k, true
			}
		}
	}
	return "", false
}

func envToSlice(env map[string]string) []string {
	list := make([]string, 0, len(env))
	for k, v := range env {
		list = append(list, k+"="+v)
	}
	// 稳定顺序，便于排查与测试
	sort.Strings(list)
	return list
}

// resolveExe 模拟原 boost environment::find_executable：
// 绝对路径直接使用；带路径分隔符的相对名相对工作目录解析；其余在 PATH 中按 PATHEXT 查找。
func resolveExe(exe string, env map[string]string, dir string) (string, error) {
	if filepath.IsAbs(exe) {
		return exe, nil
	}

	searchDir := dir
	if searchDir == "" {
		searchDir, _ = os.Getwd()
	}

	// 带目录限定的名字（如 sub/tool 或 ./tool）：先相对工作目录找
	if strings.ContainsAny(exe, `/\`) {
		cand := filepath.Join(searchDir, exe)
		if found := findExecutable(cand, env); found != "" {
			return found, nil
		}
	}

	pathValue, _ := lookupEnvKey(env, pathEnvName)
	pathDirs := filepath.SplitList(env[pathValue])
	for _, p := range pathDirs {
		if p == "" {
			continue
		}
		if found := findExecutable(filepath.Join(p, exe), env); found != "" {
			return found, nil
		}
	}
	return "", fmt.Errorf("找不到可执行文件: %s", exe)
}
