// Package engine 实现 gmk 的主业务逻辑：
// 加载配置 -> (可选)准备 VS 环境缓存 -> 执行 config/build/clean，
// 并维护 gmk_temp（无参数回放）与 gmk_cache（VS 环境缓存）。
package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gmk/internal/config"
	"gmk/internal/encode"
	"gmk/internal/logger"
	"gmk/internal/proc"
)

const (
	dirName   = ".gmk"
	cacheFile = "gmk_cache"
	tempFile  = "gmk_temp"

	// DefaultConfigPath 是配置文件相对于项目根目录的默认路径。
	DefaultConfigPath = ".gmk/gmk_config"
)

// ErrFailed 表示业务失败（详细错误已经在产生处记录日志），CLI 层据此返回退出码 1。
var ErrFailed = errors.New("执行失败")

// Args 为一次命令执行的全部参数（由 CLI 层组装或由 gmk_temp 回放得到）。
type Args struct {
	Config        string
	Root          string
	Type          string
	Action        string // config / build / clean
	CleanBuild    bool
	CleanCache    bool
	CleanTemp     bool
	CleanAllTypes bool
	CleanTypes    []string
}

// tempData 是 gmk_temp 的磁盘结构。
type tempData struct {
	Config     string   `json:"config"`
	Root       string   `json:"root"`
	Type       string   `json:"type"`
	Action     string   `json:"action"`
	CleanBuild bool     `json:"cleanBuild"`
	CleanCache bool     `json:"cleanCache"`
	CleanTypes []string `json:"cleanTypes"`
}

type Engine struct {
	cfg   *config.Config
	vsEnv map[string]string // VS 模式下缓存的完整环境变量
}

// Run 是主入口。
func Run(args Args) error {
	e := &Engine{}

	cfg, err := config.Load(args.Config)
	if err != nil {
		logger.Errorf("%v", err)
		return ErrFailed
	}
	e.cfg = cfg

	vsEnabled, err := cfg.VSEnabled()
	if err != nil {
		logger.Errorf("%v", err)
		return ErrFailed
	}

	// --clean-cache：独立于 action，任何操作下都先删除环境缓存
	if args.CleanCache {
		cachePath := filepath.Join(args.Root, dirName, cacheFile)
		_ = os.Remove(cachePath)
		logger.Infof("已删除环境缓存: %s", cachePath)
	}

	// --clean-temp：删除构建参数缓存（独立于 action）
	if args.CleanTemp {
		if err := e.removeTemp(args.Root); err != nil {
			return ErrFailed
		}
	}

	// clean 不依赖编译环境；其余动作在 VS 模式下需要先准备环境
	if vsEnabled && args.Action != "clean" {
		if !e.loadEnvCache(args.Root) {
			vcvars, ok := e.findVcvars()
			if !ok {
				return ErrFailed
			}
			if !e.saveEnvCache(args.Root, vcvars) {
				return ErrFailed
			}
		}
	}

	return e.executeAction(args)
}

// LoadReplay 在无参数执行 gmk 时尝试读取当前工作目录下的 gmk_temp。
// 返回 (nil, false) 表示没有回放文件（调用方打印 help）；
// 解析失败/内容无效也返回 false（错误已记录日志，调用方按失败退出）。
func LoadReplay() (*Args, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		logger.Errorf("获取当前工作目录失败: %v", err)
		return nil, false
	}
	cwd = filepath.Clean(cwd)

	path := filepath.Join(cwd, dirName, tempFile)
	data, err := os.ReadFile(path)
	if err != nil {
		// 不存在是正常情况，不打日志
		return nil, false
	}

	var td tempData
	td.Config = DefaultConfigPath
	if err := json.Unmarshal(encode.StripBOM(data), &td); err != nil {
		logger.Errorf("gmk_temp 解析失败 (可用 clean temp 删除后重试): %v", err)
		return nil, false
	}
	if td.Action != "build" {
		logger.Errorf("gmk_temp 内容无效 (action 不是 build)，可用 clean temp 删除")
		return nil, false
	}

	logger.Debugf("检测到 gmk_temp，自动执行上次构建参数")
	return &Args{
		Config:     td.Config,
		Root:       cwd, // 回放始终以当前工作目录为项目根目录
		Type:       td.Type,
		Action:     td.Action,
		CleanBuild: td.CleanBuild,
		CleanCache: td.CleanCache,
		CleanTypes: td.CleanTypes,
	}, true
}

// executeAction 执行 config/build/clean。
func (e *Engine) executeAction(args Args) error {
	root := args.Root
	buildDirOf := func(t string) string {
		if d := e.cfg.BuildDirectory(t); d != "" {
			return d
		}
		return "build"
	}

	removeBuildDir := func(path string) error {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil
		}
		if err := os.RemoveAll(path); err != nil {
			logger.Errorf("删除构建目录失败: %s", path)
			return ErrFailed
		}
		logger.Infof("已删除构建目录: %s", path)
		return nil
	}

	// ===== clean：只清理不构建（cache/temp 已在 Run 中处理）=====
	if args.Action == "clean" {
		if args.CleanBuild {
			types := append([]string(nil), args.CleanTypes...)
			if args.CleanAllTypes {
				types = append(types, e.cfg.Types()...)
			}
			sort.Strings(types)
			types = dedup(types)

			for _, t := range types {
				if !e.cfg.HasType(t) {
					logger.Errorf("配置类型 [%s] 在配置文件中不存在", t)
					return ErrFailed
				}
				if err := removeBuildDir(filepath.Join(root, buildDirOf(t))); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if args.Action != "config" && args.Action != "build" {
		logger.Errorf("未知的子命令: %s (支持 config/build/clean)", args.Action)
		return ErrFailed
	}

	if !e.cfg.HasType(args.Type) {
		logger.Errorf("配置类型 [%s] 在配置文件中不存在", args.Type)
		return ErrFailed
	}

	buildPath := filepath.Join(root, buildDirOf(args.Type))

	// config 强制执行；build 时未配置过（无 CMakeCache.txt）才执行
	needConfigure := args.Action == "config" || !fileExists(filepath.Join(buildPath, "CMakeCache.txt"))
	if needConfigure {
		if err := e.runConfigure(args, buildPath); err != nil {
			// configure 失败：删除过期的构建参数缓存，避免无参数回放
			_ = e.removeTemp(root)
			return ErrFailed
		}
	}

	if args.Action == "build" {
		err := e.runBuild(args, buildPath)
		// 无论构建成败都保存参数，便于无参数直接重放/重试
		e.saveTemp(args)
		if err != nil {
			return ErrFailed
		}
		return nil
	}

	// config 成功：按 build 保存参数（action 改写为 build，其余不变）
	buildArgs := args
	buildArgs.Action = "build"
	e.saveTemp(buildArgs)
	return nil
}

// saveTemp 保存本次构建参数到 root/.gmk/gmk_temp。
func (e *Engine) saveTemp(args Args) {
	gmkDir := filepath.Join(args.Root, dirName)
	if err := os.MkdirAll(gmkDir, 0o755); err != nil {
		logger.Errorf("创建目录失败: %s", gmkDir)
		return
	}

	td := tempData{
		Config:     args.Config,
		Root:       args.Root,
		Type:       args.Type,
		Action:     args.Action,
		CleanBuild: args.CleanBuild,
		CleanCache: args.CleanCache,
		CleanTypes: args.CleanTypes,
	}
	data, err := marshalJSON(td)
	if err != nil {
		logger.Errorf("序列化构建参数失败: %v", err)
		return
	}

	path := filepath.Join(gmkDir, tempFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		logger.Errorf("无法创建构建参数缓存文件: %s", path)
		return
	}
	logger.Debugf("已保存本次构建参数: %s", path)
}

// removeTemp 删除 root/.gmk/gmk_temp（不存在视为成功）。
func (e *Engine) removeTemp(root string) error {
	path := filepath.Join(root, dirName, tempFile)
	if !fileExists(path) {
		logger.Infof("构建参数缓存不存在: %s", path)
		return nil
	}
	if err := os.Remove(path); err != nil {
		logger.Errorf("删除构建参数缓存失败: %s", path)
		return ErrFailed
	}
	logger.Infof("已删除构建参数缓存: %s", path)
	return nil
}

// prepareEnv 准备子进程环境：
// VS 模式返回缓存环境（overlay）并把额外 PATH 合并进 Path；
// 非 VS 模式 overlay 为 nil，额外路径走 appendPath 追加到当前 PATH。
// 用户配置的 cmake.compiler.envKeyValue（K=V）会进入 overlay，
// 优先级高于 VS 缓存环境（用户显式配置优先）。
func (e *Engine) prepareEnv() (map[string]string, []string) {
	var overlay map[string]string
	if len(e.vsEnv) > 0 {
		overlay = make(map[string]string, len(e.vsEnv))
		for k, v := range e.vsEnv {
			overlay[k] = v
		}
	}

	// 应用用户配置的 K=V 环境变量（优先级高于 VS 缓存环境）。
	// 在第一个 "=" 处拆分，值可以为空也可以再含 "="。
	for _, kv := range e.cfg.CmakeEnvKeyValue() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			logger.Errorf("配置项 [cmake.compiler.envKeyValue] 格式无效（缺少 '='）: %q", kv)
			continue
		}
		if k = strings.TrimSpace(k); k == "" {
			logger.Errorf("配置项 [cmake.compiler.envKeyValue] 键为空: %q", kv)
			continue
		}
		if overlay == nil {
			overlay = make(map[string]string)
		}
		// 大小写不敏感地覆盖同名键（Windows 环境变量名大小写不敏感）
		for existing := range overlay {
			if strings.EqualFold(existing, k) {
				delete(overlay, existing)
				break
			}
		}
		overlay[k] = strings.TrimSpace(v)
	}

	var extras []string
	if envPath := strings.TrimSpace(e.cfg.CmakeEnv()); envPath != "" {
		for _, p := range strings.Split(envPath, ";") {
			if p = strings.TrimSpace(p); p != "" {
				extras = append(extras, p)
			}
		}
	}

	if len(overlay) == 0 {
		return nil, extras
	}

	// 统一 PATH 键名为 "Path"，避免大小写不同的重复键
	pathValue := ""
	for k, v := range overlay {
		if strings.EqualFold(k, "Path") {
			pathValue = v
			delete(overlay, k)
		}
	}
	if pathValue == "" {
		return overlay, extras
	}
	if len(extras) > 0 {
		pathValue += ";" + strings.Join(extras, ";")
	}
	overlay["Path"] = pathValue
	return overlay, nil
}

func (e *Engine) runConfigure(args Args, buildPath string) error {
	cmakeArgs := []string{
		"-S", args.Root,
		"-B", buildPath,
		"-G", "Ninja",
		"-DCMAKE_EXPORT_COMPILE_COMMANDS=ON",
	}
	if c := strings.TrimSpace(e.cfg.CCompiler()); c != "" {
		cmakeArgs = append(cmakeArgs, "-DCMAKE_C_COMPILER="+c)
	}
	if cxx := strings.TrimSpace(e.cfg.CxxCompiler()); cxx != "" {
		cmakeArgs = append(cmakeArgs, "-DCMAKE_CXX_COMPILER="+cxx)
	}
	if prefix := strings.TrimSpace(e.cfg.PrefixAppend(args.Type)); prefix != "" {
		cmakeArgs = append(cmakeArgs, "-DCMAKE_PREFIX_PATH="+prefix)
	}
	cmakeArgs = append(cmakeArgs, e.cfg.Options(args.Type)...)

	return e.runCmake(cmakeArgs, buildPath)
}

func (e *Engine) runBuild(args Args, buildPath string) error {
	return e.runCmake([]string{"--build", buildPath, "--config", args.Type}, buildPath)
}

func (e *Engine) runCmake(cmakeArgs []string, workDir string) error {
	cmakeBin := "cmake"
	if bin := strings.TrimSpace(e.cfg.CmakeBin()); bin != "" {
		cmakeBin = bin
	}

	overlay, appendPath := e.prepareEnv()

	logger.Infof("执行: %s %s", cmakeBin, strings.Join(cmakeArgs, " "))
	logger.Infof("工作目录: %s", workDir)

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		logger.Errorf("创建工作目录失败: %s", workDir)
		return ErrFailed
	}

	code, err := proc.RunInherit(cmakeBin, cmakeArgs, workDir, overlay, appendPath)
	if err != nil {
		logger.Errorf("启动 cmake 失败: %v", err)
		return ErrFailed
	}
	if code != 0 {
		logger.Errorf("cmake 执行失败, 退出码: %d", code)
		return ErrFailed
	}
	return nil
}

// ---- 小工具 ----

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func dedup(sorted []string) []string {
	if len(sorted) <= 1 {
		return sorted
	}
	out := sorted[:1]
	for _, s := range sorted[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

// marshalJSON 输出缩进 JSON：不转义 <>&（与 nlohmann 行为一致），无末尾换行。
func marshalJSON(v any) ([]byte, error) {
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return []byte(strings.TrimRight(sb.String(), "\r\n")), nil
}
