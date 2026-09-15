// Package config 负责加载与访问 .gmk/gmk_config（JSON）。
// 配置以类型为键（configuration.Debug / configuration.Release ...），
// 文档字段（*.doc）会被静默忽略；可选项缺失时返回零值走默认行为。
package config

import (
	"encoding/json"
	"fmt"
	"gmk/internal/encode"
	"os"
	"sort"
)

// 配置键名（与原工程保持一致）
const (
	keyVSOptions   = "cmake.compiler.vs"
	keyVSEnabled   = "cmake.compiler.vs.enabled"
	keyVSWhere     = "cmake.compiler.vswhere.path"
	keyVSVersion   = "cmake.compiler.vs.version"
	keyCmakeBin    = "cmake.bin"
	keyCmakeEnv    = "cmake.compiler.env"
	keyCmakeEnvKV  = "cmake.compiler.envKeyValue"
	keyCmakeC      = "cmake.compiler.c"
	keyCmakeCxx    = "cmake.compiler.cxx"
	keyConfigTypes = "configuration"
	keyBuildDir    = "build.directory"
	keyPrefix      = "cmake.prefix.append"
	keyOptions     = "cmake.options"
)

// Config 是解析后的配置文件。
type Config struct {
	root map[string]any
}

// Load 读取并解析配置文件。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败: %s", path)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("配置文件内容为空")
	}
	var root map[string]any
	if err := json.Unmarshal(encode.StripBOM(data), &root); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	return &Config{root: root}, nil
}

// lookup 按层级路径取节点。
func (c *Config) lookup(keys ...string) any {
	var cur any = c.root
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// VSEnabled 返回 cmake.compiler.vs.enabled。
// 该字段是必填项：缺失或类型错误时返回 error。
func (c *Config) VSEnabled() (bool, error) {
	v, ok := c.lookup(keyVSOptions, keyVSEnabled).(bool)
	if !ok {
		return false, fmt.Errorf("配置项 [%s] 缺失或不是布尔值", keyVSEnabled)
	}
	return v, nil
}

// VSWhere 返回 vswhere.exe 路径，未配置返回空串。
func (c *Config) VSWhere() string {
	if s, ok := asString(c.lookup(keyVSOptions, keyVSWhere)); ok {
		return s
	}
	return ""
}

// VSVersion 返回 VS 版本号（如 17.0），未配置返回空串。
func (c *Config) VSVersion() string {
	if s, ok := asString(c.lookup(keyVSOptions, keyVSVersion)); ok {
		return s
	}
	return ""
}

// CmakeBin 返回 cmake 可执行文件，未配置返回空串（调用方用默认值）。
func (c *Config) CmakeBin() string {
	if s, ok := asString(c.lookup(keyCmakeBin)); ok {
		return s
	}
	return ""
}

// CmakeEnv 返回需要追加到 PATH 的目录串（分号分隔）。
func (c *Config) CmakeEnv() string {
	if s, ok := asString(c.lookup(keyCmakeEnv)); ok {
		return s
	}
	return ""
}

// CmakeEnvKeyValue 返回需要设置的环境变量 K=V 列表；未配置或类型错误时返回 nil。
// 调用方应在第一个 "=" 处拆分键值（值本身可能包含 "="）。
func (c *Config) CmakeEnvKeyValue() []string {
	arr, ok := c.lookup(keyCmakeEnvKV).([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil
		}
		out = append(out, s)
	}
	return out
}

func (c *Config) CCompiler() string {
	if s, ok := asString(c.lookup(keyCmakeC)); ok {
		return s
	}
	return ""
}

func (c *Config) CxxCompiler() string {
	if s, ok := asString(c.lookup(keyCmakeCxx)); ok {
		return s
	}
	return ""
}

// HasType 判断配置类型是否存在。
func (c *Config) HasType(t string) bool {
	types, ok := c.lookup(keyConfigTypes).(map[string]any)
	if !ok {
		return false
	}
	_, ok = types[t]
	return ok
}

// Types 返回全部配置类型（排序，保证确定性）。
func (c *Config) Types() []string {
	types, ok := c.lookup(keyConfigTypes).(map[string]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(types))
	for k := range types {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c *Config) typeNode(t string) map[string]any {
	types, ok := c.lookup(keyConfigTypes).(map[string]any)
	if !ok {
		return nil
	}
	node, ok := types[t].(map[string]any)
	if !ok {
		return nil
	}
	return node
}

// BuildDirectory 返回某配置类型的构建目录，未配置返回空串（调用方默认 build）。
func (c *Config) BuildDirectory(t string) string {
	node := c.typeNode(t)
	if node == nil {
		return ""
	}
	if s, ok := asString(node[keyBuildDir]); ok {
		return s
	}
	return ""
}

// PrefixAppend 返回某配置类型的 CMAKE_PREFIX_PATH 追加内容，未配置返回空串。
func (c *Config) PrefixAppend(t string) string {
	node := c.typeNode(t)
	if node == nil {
		return ""
	}
	if s, ok := asString(node[keyPrefix]); ok {
		return s
	}
	return ""
}

// Options 返回某配置类型的额外 cmake 选项；未配置或不是字符串数组时返回 nil。
func (c *Config) Options(t string) []string {
	node := c.typeNode(t)
	if node == nil {
		return nil
	}
	arr, ok := node[keyOptions].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil
		}
		out = append(out, s)
	}
	return out
}
