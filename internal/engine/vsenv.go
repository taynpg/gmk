package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gmk/internal/encode"
	"gmk/internal/logger"
	"gmk/internal/proc"
)

const defaultVSWhere = `C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe`

// findVcvars 通过 vswhere.exe 定位 vcvars64.bat 的绝对路径。
func (e *Engine) findVcvars() (string, bool) {
	vswhere := strings.TrimSpace(e.cfg.VSWhere())
	if vswhere == "" {
		vswhere = defaultVSWhere
	}

	var exeArgs []string
	if ver := strings.TrimSpace(e.cfg.VSVersion()); ver != "" {
		v, err := strconv.ParseFloat(ver, 64)
		if err != nil {
			logger.Errorf("配置的 VS 版本号无效: %s (%v)", ver, err)
			return "", false
		}
		exeArgs = append(exeArgs, "-version", fmt.Sprintf("[%.1f,%.1f)", v, v+1.0))
	} else {
		exeArgs = append(exeArgs, "-latest")
	}
	exeArgs = append(exeArgs,
		"-products", "*",
		"-requires", "Microsoft.VisualStudio.Component.VC.Tools.x86.x64",
		"-find", `VC\Auxiliary\Build\vcvars64.bat`,
	)

	output, code, err := proc.RunCapture(vswhere, exeArgs, ".", nil, nil)
	if err != nil {
		logger.Errorf("启动 vswhere.exe 失败: %v", err)
		return "", false
	}
	if code != 0 {
		logger.Errorf("vswhere.exe 执行失败, 输出: %s", output)
		return "", false
	}
	output = strings.TrimSpace(output)
	if output == "" {
		logger.Errorf("vswhere.exe 未找到匹配的 VS 版本。")
		return "", false
	}
	// 正常只输出一行路径；防御性地取第一行，避免末尾空行干扰
	if idx := strings.IndexAny(output, "\r\n"); idx >= 0 {
		output = strings.TrimSpace(output[:idx])
	}
	return output, true
}

// loadEnvCache 从 root/.gmk/gmk_cache 加载缓存的 VS 环境变量。
func (e *Engine) loadEnvCache(root string) bool {
	path := filepath.Join(root, dirName, cacheFile)
	data, err := os.ReadFile(path)
	if err != nil {
		// 缓存不存在是正常情况，不打日志
		return false
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		logger.Errorf("缓存文件内容为空")
		return false
	}

	env := make(map[string]string)
	if err := json.Unmarshal(encode.StripBOM(data), &env); err != nil {
		logger.Errorf("解析缓存文件失败: %v", err)
		return false
	}

	e.vsEnv = env
	logger.Debugf("从缓存加载 VS 环境成功，共 %d 个变量", len(env))
	return true
}

// saveEnvCache 执行临时批处理（call vcvars64.bat + set），
// 解析全部环境变量并保存到 root/.gmk/gmk_cache。
func (e *Engine) saveEnvCache(root, vcvarsBat string) bool {
	gmkDir := filepath.Join(root, dirName)
	if err := os.MkdirAll(gmkDir, 0o755); err != nil {
		logger.Errorf("创建目录失败: %s", gmkDir)
		return false
	}

	// 临时批处理放系统 Temp 目录，文件名带 PID 避免多实例冲突
	tempBat := filepath.Join(os.TempDir(), fmt.Sprintf("_gmk_vs_env_%d.bat", os.Getpid()))

	// CRLF 是批处理的规范换行；内容按 GBK 编码，兼容路径中出现中文的场景
	content := "@echo off\r\n" +
		"call \"" + vcvarsBat + "\" >nul\r\n" +
		"if errorlevel 1 exit /b 1\r\n" +
		"set\r\n"
	if err := os.WriteFile(tempBat, encode.UTF8ToGBK([]byte(content)), 0o644); err != nil {
		logger.Errorf("无法创建临时批处理文件: %s", tempBat)
		return false
	}

	output, code, err := proc.RunCapture("cmd.exe", []string{"/c", tempBat}, ".", nil, nil)
	_ = os.Remove(tempBat) // 无论成败都清理临时文件

	if err != nil {
		logger.Errorf("启动批处理失败: %v", err)
		return false
	}
	if code != 0 {
		logger.Errorf("执行 vcvars64.bat 失败, 输出: %s", output)
		return false
	}
	if strings.TrimSpace(output) == "" {
		logger.Errorf("vcvars64.bat 执行后无输出")
		return false
	}

	env := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			// 跳过无 '=' 的行以及 =C:= 这类 cmd 内部变量
			continue
		}
		env[key] = value
	}
	if len(env) == 0 {
		logger.Errorf("未能从 vcvars64.bat 输出中解析到环境变量")
		return false
	}

	data, err := marshalJSON(env)
	if err != nil {
		logger.Errorf("序列化环境缓存失败: %v", err)
		return false
	}
	cachePath := filepath.Join(gmkDir, cacheFile)
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		logger.Errorf("无法创建缓存文件: %s", cachePath)
		return false
	}

	e.vsEnv = env
	logger.Infof("VS 环境已保存到缓存，共 %d 个变量", len(env))
	return true
}
