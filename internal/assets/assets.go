// Package assets 保存编译进二进制的默认资源（配置模板 / .clang-format / .gitignore 追加行）。
package assets

import (
	_ "embed"
	"runtime"
	"strings"
)

//go:embed gmk_config
var defaultConfig string

//go:embed clang-format
var defaultClangFormat string

// DefaultConfig 返回默认配置模板内容（gmk config auto 使用）。
// 模板中的 __VS_ENABLED__ 占位符会在运行时替换：Windows 下为 true，其余平台为 false。
func DefaultConfig() string {
	vsEnabled := "false"
	if runtime.GOOS == "windows" {
		vsEnabled = "true"
	}
	return strings.Replace(defaultConfig, "__VS_ENABLED__", vsEnabled, 1)
}

// DefaultClangFormat 返回默认 .clang-format 内容（gmk check format 使用）。
func DefaultClangFormat() string { return defaultClangFormat }

// DefaultGitignoreLines 返回需要保证出现在目标 .gitignore 中的行。
func DefaultGitignoreLines() []string {
	return []string{"gmk_cache", "gmk_temp"}
}
