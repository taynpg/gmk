# gmk

通过配置文件的形式封装 cmake 指令，简化 C/C++ 工程的配置与构建流程。

## 特性

- **配置驱动**：所有构建参数集中在 `.gmk/gmk_config`，无需记忆冗长的 cmake 命令行
- **VS 环境自动检测**：通过 vswhere 定位 Visual Studio，自动加载 vcvars64 编译环境并缓存
- **Ninja 优先**：默认使用 Ninja 生成器，自动导出 `compile_commands.json`，方便 IDE 代码补全
- **无参数回放**：上次构建参数自动缓存，直接运行 `gmk` 即可复现上次构建
- **Windows 中文兼容**：自动识别子进程输出的 GBK 编码并转为 UTF-8，控制台中文不乱码
- **跨平台**：支持 Windows / Linux / macOS（VS 环境检测仅 Windows 有效）

## 安装

### 从源码构建

```bash
git clone https://github.com/taynpg/gmk.git
cd gmk
go build -o gmk .
```

将生成的 `gmk`（Windows 下为 `gmk.exe`）放入 `PATH` 即可。

### 运行依赖

- cmake
- Ninja（gmk 默认使用 Ninja 生成器）

## 快速开始

```bash
# 1. 在工程根目录生成默认配置
gmk config auto -f

# 2. 编辑 .gmk/gmk_config，按需修改编译器、构建选项等

# 3. 配置工程（运行 cmake configure）
gmk config

# 4. 构建
gmk build

# 5. 无参数运行，自动回放上次构建
gmk
```

## 命令说明

### config

```bash
gmk config [-t Debug|Release]    # 运行 cmake configure
gmk config auto [-f]             # 生成默认配置模板 .gmk/gmk_config
```

### build

```bash
gmk build [-t Debug|Release]     # 构建工程（未配置过则自动先 configure）
```

### clean

```bash
gmk clean build -t Debug         # 清理指定类型的构建目录
gmk clean build --all            # 清理所有配置类型的构建目录
gmk clean cache                  # 清理 VS 环境缓存 (.gmk/gmk_cache)
gmk clean temp                   # 清理构建参数缓存 (.gmk/gmk_temp)
gmk clean all                    # 清理全部（构建目录 + 缓存 + 参数）
```

### check

```bash
gmk check gitignore              # 检查 .gitignore 是否包含 gmk 忽略项，缺失则追加
gmk check format [-f]            # 检查是否存在 .clang-format，不存在则生成默认版本
```

### 全局参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-r, --root` | 项目根目录 | 当前目录 |
| `-c, --config` | 配置文件路径 | `.gmk/gmk_config` |
| `-V, --version` | 显示版本信息 | - |

## 配置文件

配置文件路径为 `<root>/.gmk/gmk_config`，格式为 JSON。可通过 `gmk config auto -f` 生成默认模板。

```json
{
    "cmake.bin": "cmake",
    "cmake.compiler.vs": {
        "cmake.compiler.vs.enabled": true,
        "cmake.compiler.vswhere.path": "C:/Program Files (x86)/Microsoft Visual Studio/Installer/vswhere.exe",
        "cmake.compiler.vs.version": "17.0"
    },
    "cmake.compiler.env": "",
    "cmake.compiler.envKeyValue": [],
    "cmake.compiler.c": "",
    "cmake.compiler.cxx": "",
    "configuration": {
        "Debug": {
            "build.directory": "build",
            "cmake.prefix.append": "",
            "cmake.options": [
                "-Wno-dev",
                "-DCMAKE_BUILD_TYPE=Debug"
            ]
        },
        "Release": {
            "build.directory": "build-Release",
            "cmake.prefix.append": "",
            "cmake.options": [
                "-Wno-dev",
                "-DCMAKE_BUILD_TYPE=Release"
            ]
        }
    }
}
```

### 配置项说明

| 配置项 | 说明 |
|--------|------|
| `cmake.bin` | cmake 可执行文件路径，留空使用系统 PATH 中的 cmake |
| `cmake.compiler.vs.enabled` | 是否启用 Visual Studio 环境（Windows 默认为 true） |
| `cmake.compiler.vswhere.path` | vswhere.exe 路径，留空使用默认安装路径 |
| `cmake.compiler.vs.version` | VS 版本号（如 17.0 = VS2022），留空使用最新版本 |
| `cmake.compiler.env` | 追加到 PATH 的目录，多个路径用分号分隔 |
| `cmake.compiler.envKeyValue` | 需要设置的环境变量，格式 `K=V`，支持多对，如 `["CXX=cl", "CC=clang"]`；优先级高于 VS 缓存环境 |
| `cmake.compiler.c` / `cmake.compiler.cxx` | C/C++ 编译器路径 |
| `configuration.<Type>.build.directory` | 构建目录，默认为 `build` |
| `configuration.<Type>.cmake.prefix.append` | 追加到 `CMAKE_PREFIX_PATH` 的路径 |
| `configuration.<Type>.cmake.options` | 额外的 cmake 选项 |

## 工作目录结构

```
<project>/
├── .gmk/
│   ├── gmk_config    # 配置文件
│   ├── gmk_cache     # VS 环境变量缓存（自动生成）
│   └── gmk_temp      # 上次构建参数（自动生成，用于无参数回放）
├── build/            # 构建目录（Debug）
├── build-Release/    # 构建目录（Release）
└── CMakeLists.txt
```

## 环境变量

| 变量 | 说明 |
|------|------|
| `GMK_LOG_LEVEL` | 日志级别：trace/debug/info/warning/error/critical/off，默认 info |
| `SPDLOG_LEVEL` | 兼容原 cmk 的日志级别变量，优先级低于 `GMK_LOG_LEVEL` |

## License

MIT
