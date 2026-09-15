// Package checks 实现 config auto 与 check 子命令的辅助文件补齐功能。
package checks

import (
	"os"
	"path/filepath"
	"strings"

	"gmk/internal/assets"
	"gmk/internal/logger"
)

// normalizeRoot 把项目根目录规范化为绝对路径。
func normalizeRoot(root string) (string, bool) {
	if filepath.IsAbs(root) {
		return filepath.Clean(root), true
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		logger.Errorf("无法得到项目根目录的绝对路径: %s", root)
		return "", false
	}
	return filepath.Clean(abs), true
}

// ConfigAuto 在 <root>/.gmk/gmk_config 生成默认配置模板。
func ConfigAuto(root string, force bool) bool {
	root, ok := normalizeRoot(root)
	if !ok {
		return false
	}

	gmkDir := filepath.Join(root, ".gmk")
	if err := os.MkdirAll(gmkDir, 0o755); err != nil {
		logger.Errorf("创建目录失败: %s (%v)", gmkDir, err)
		return false
	}

	configPath := filepath.Join(gmkDir, "gmk_config")
	if _, err := os.Stat(configPath); err == nil && !force {
		logger.Errorf("配置文件已存在: %s，如需覆盖请加 -f / --force", configPath)
		return false
	}

	if err := os.WriteFile(configPath, []byte(assets.DefaultConfig()), 0o644); err != nil {
		logger.Errorf("无法创建配置文件: %s", configPath)
		return false
	}
	logger.Infof("已生成默认配置: %s", configPath)
	return true
}

// Gitignore 检查 <root>/.gitignore 是否包含 gmk 相关忽略项，缺失则追加到末尾。
func Gitignore(root string) bool {
	root, ok := normalizeRoot(root)
	if !ok {
		return false
	}
	gitignorePath := filepath.Join(root, ".gitignore")

	required := assets.DefaultGitignoreLines()

	data, err := os.ReadFile(gitignorePath)
	content := string(data)
	existed := err == nil
	if err != nil && !os.IsNotExist(err) {
		logger.Errorf("无法打开文件: %s", gitignorePath)
		return false
	}

	var missing []string
	for _, want := range required {
		if !strings.Contains(content, want) {
			missing = append(missing, want)
		}
	}

	if len(missing) == 0 {
		logger.Infof(".gitignore 已包含全部 gmk 相关忽略项 (%s)", strings.Join(required, ", "))
		return true
	}

	var sb strings.Builder
	sb.WriteString(content)
	if content != "" {
		last := content[len(content)-1]
		if last != '\n' && last != '\r' {
			sb.WriteByte('\n')
		}
	}
	sb.WriteString("# ---> gmk\n")
	for _, m := range missing {
		sb.WriteString(m)
		sb.WriteByte('\n')
	}

	if err := os.WriteFile(gitignorePath, []byte(sb.String()), 0o644); err != nil {
		logger.Errorf("无法写入 .gitignore: %s", gitignorePath)
		return false
	}

	action := "新建"
	if existed {
		action = "末尾追加"
	}
	logger.Infof("已在 %s .gitignore 忽略项: %s  → %s", action, strings.Join(missing, ", "), gitignorePath)
	return true
}

// ClangFormat 检查并生成默认 .clang-format，已存在且未指定 force 时失败。
func ClangFormat(root string, force bool) bool {
	root, ok := normalizeRoot(root)
	if !ok {
		return false
	}
	fmtPath := filepath.Join(root, ".clang-format")
	_, statErr := os.Stat(fmtPath)
	existed := statErr == nil

	if existed && !force {
		logger.Errorf(".clang-format 已存在: %s，如需覆盖请加 -f / --force", fmtPath)
		return false
	}

	if err := os.WriteFile(fmtPath, []byte(assets.DefaultClangFormat()), 0o644); err != nil {
		logger.Errorf("无法创建 .clang-format: %s", fmtPath)
		return false
	}
	if existed || force {
		logger.Infof("已覆盖 .clang-format: %s", fmtPath)
	} else {
		logger.Infof("已生成 .clang-format: %s", fmtPath)
	}
	return true
}
