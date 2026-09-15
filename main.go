// gmk：通过配置的形式封装 cmake 指令，简化命令行调用（cmk 的 Go 重写版）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/spf13/cobra"

	"gmk/internal/checks"
	"gmk/internal/encode"
	"gmk/internal/engine"
	"gmk/internal/logger"
)

// 构建时可通过 -ldflags "-X main.gitBranch=xxx -X main.gitCommit=xxx" 注入。
var (
	gitBranch = "unknown"
	gitCommit = "unknown"
)

func versionString() string {
	branch, commit := gitBranch, gitCommit
	// 未通过 ldflags 注入时，尝试读取 Go 构建信息里的 VCS 版本
	if commit == "unknown" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 8 {
					commit = s.Value[:8]
				}
			}
		}
	}
	return fmt.Sprintf("gmk %s@%s", branch, commit)
}

// resolvePaths 把项目根目录规范化为绝对路径，并把配置文件路径解析为绝对路径
// （配置为相对路径时相对项目根目录；绝对路径直接使用）。
func resolvePaths(a *engine.Args) {
	if abs, err := filepath.Abs(a.Root); err == nil {
		a.Root = filepath.Clean(abs)
	}
	if filepath.IsAbs(a.Config) {
		a.Config = filepath.Clean(a.Config)
	} else {
		a.Config = filepath.Join(a.Root, a.Config)
	}
}

func main() {
	// 捕获任何未预期的 panic，避免在 cmd 中表现为"闪退"而无任何提示
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "gmk 内部错误: %v\n%s\n", r, debug.Stack())
			os.Exit(1)
		}
	}()

	// 为 cmd.exe 控制台开启 ANSI 颜色支持（仅影响转义序列解释，不改代码页）
	encode.EnableVirtualTerminalProcessing()

	var (
		rootFlag    string
		configFlag  string
		showVersion bool
	)

	rootCmd := &cobra.Command{
		Use:               "gmk",
		Short:             "通过配置的形式封装cmake指令，简化命令行调用。",
		Args:              cobra.NoArgs,
		SilenceUsage:      true,
		Version:           "",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			if showVersion {
				fmt.Println(versionString())
				os.Exit(0)
			}
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			// 无任何参数：当前目录存在 gmk_temp 则自动回放，否则打印完整 help
			if len(os.Args) == 1 {
				if replayArgs, ok := engine.LoadReplay(); ok {
					resolvePaths(replayArgs)
					if err := engine.Run(*replayArgs); err != nil {
						os.Exit(1)
					}
					return nil
				}
			}
			_ = cmd.Help()
			os.Exit(1)
			return nil
		},
	}
	rootCmd.PersistentFlags().StringVarP(&rootFlag, "root", "r", ".", "项目根目录 (默认当前目录)")
	rootCmd.PersistentFlags().StringVarP(&configFlag, "config", "c", engine.DefaultConfigPath,
		"配置文件路径 (默认 .gmk/gmk_config)")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "V", false,
		"显示版本与构建信息 (branch + commit)")

	runAction := func(action, typ string) {
		a := &engine.Args{
			Root:   rootFlag,
			Config: configFlag,
			Type:   typ,
			Action: action,
		}
		resolvePaths(a)
		if err := engine.Run(*a); err != nil {
			os.Exit(1)
		}
	}

	// ========== config ==========
	var configType string
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "配置 cmake 工程 / 生成默认配置文件",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runAction("config", configType)
			return nil
		},
	}
	configCmd.Flags().StringVarP(&configType, "type", "t", "Debug", "配置类型 (Debug/Release...)")

	var autoForce bool
	autoCmd := &cobra.Command{
		Use:   "auto",
		Short: "在工程根目录生成默认的 .gmk/gmk_config 模板",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !checks.ConfigAuto(rootFlag, autoForce) {
				os.Exit(1)
			}
			return nil
		},
	}
	autoCmd.Flags().BoolVarP(&autoForce, "force", "f", false, "若配置文件已存在则强制覆盖")
	configCmd.AddCommand(autoCmd)

	// ========== build ==========
	var buildType string
	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "构建工程",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runAction("build", buildType)
			return nil
		},
	}
	buildCmd.Flags().StringVarP(&buildType, "type", "t", "Debug", "配置类型 (Debug/Release...)")

	// ========== clean ==========
	var (
		cleanTypes []string
		cleanAll   bool
	)
	runClean := func(target string) {
		a := &engine.Args{
			Root:          rootFlag,
			Config:        configFlag,
			Action:        "clean",
			CleanTypes:    cleanTypes,
			CleanAllTypes: cleanAll,
		}
		switch target {
		case "build":
			a.CleanBuild = true
			if len(cleanTypes) == 0 && !cleanAll {
				logger.Errorf("clean build 需要 -t <配置类型> 或 --all")
				os.Exit(1)
			}
		case "cache":
			a.CleanCache = true
		case "temp":
			a.CleanTemp = true
		case "all":
			a.CleanBuild = true
			a.CleanCache = true
			a.CleanTemp = true
			a.CleanAllTypes = true
		}
		resolvePaths(a)
		if err := engine.Run(*a); err != nil {
			os.Exit(1)
		}
	}

	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "清理构建产物/缓存",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			logger.Errorf("clean 需要指定目标: build / cache / temp / all")
			os.Exit(1)
			return nil
		},
	}

	cleanBuildCmd := &cobra.Command{
		Use:   "build",
		Short: "清理构建目录",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runClean("build")
			return nil
		},
	}
	cleanBuildCmd.Flags().StringArrayVarP(&cleanTypes, "type", "t", nil, "要清理的配置类型 (可多次指定)")
	cleanBuildCmd.Flags().BoolVar(&cleanAll, "all", false, "清理配置文件中所有配置类型的构建目录")

	cleanCacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "清理环境缓存 (.gmk/gmk_cache)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runClean("cache")
			return nil
		},
	}
	cleanTempCmd := &cobra.Command{
		Use:   "temp",
		Short: "清理构建参数缓存 (.gmk/gmk_temp)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runClean("temp")
			return nil
		},
	}
	cleanAllCmd := &cobra.Command{
		Use:   "all",
		Short: "清理所有配置类型的构建目录 + 环境缓存 + 参数缓存",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runClean("all")
			return nil
		},
	}
	cleanCmd.AddCommand(cleanBuildCmd, cleanCacheCmd, cleanTempCmd, cleanAllCmd)

	// ========== check ==========
	var checkForce bool
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "检查&补齐工程辅助文件 (.gitignore / .clang-format)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			logger.Errorf("check 需要指定子命令: gitignore / format")
			os.Exit(1)
			return nil
		},
	}

	checkGitignoreCmd := &cobra.Command{
		Use:   "gitignore",
		Short: "检查 .gitignore 是否包含 gmk 的忽略项，缺失则自动追加",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !checks.Gitignore(rootFlag) {
				os.Exit(1)
			}
			return nil
		},
	}
	checkGitignoreCmd.Flags().BoolVarP(&checkForce, "force", "f", false,
		"忽略已存在判定：gitignore 追加模式始终写入末尾（仍不会重复行）")

	checkFormatCmd := &cobra.Command{
		Use:   "format",
		Short: "检查是否存在 .clang-format，不存在则生成默认版本",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !checks.ClangFormat(rootFlag, checkForce) {
				os.Exit(1)
			}
			return nil
		},
	}
	checkFormatCmd.Flags().BoolVarP(&checkForce, "force", "f", false,
		"强制覆盖已有 .clang-format")

	checkCmd.AddCommand(checkGitignoreCmd, checkFormatCmd)

	rootCmd.AddCommand(configCmd, buildCmd, cleanCmd, checkCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
