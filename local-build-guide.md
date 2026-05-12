# Syslab MCP 本地编译说明

本文档面向仓库维护者和开发者，说明如何在本地编译 `syslab-mcp`，以及如何完成基础自检。

## 1. 适用范围

- 仓库根目录：`syslab-mcp`
- 主程序入口：`cmd/syslab-mcp-core-server`
- 默认输出目录：`bin/`
- 当前默认版本号来源：
  1. 环境变量 `VERSION`
  2. CI 环境变量 `CI_COMMIT_TAG`
  3. 仓库根目录 `VERSION` 文件

## 2. 环境要求

### Windows

- PowerShell 5.1+ 或 PowerShell 7+
- Go 1.26 或更高版本

### Linux

- `/bin/sh`
- Go 1.26 或更高版本

说明：

- 项目当前使用纯 Go 构建，默认 `CGO_ENABLED=0`。
- 仅编译二进制时，不要求本机必须安装 Syslab。
- 如果要执行 MCP 接口联调，仍需准备本地 Syslab / Julia 环境。

## 3. 输出产物

构建脚本默认会生成以下文件：

- `bin/syslab-mcp-server-win64.exe`
- `bin/syslab-mcp-server-winarm64.exe`
- `bin/syslab-mcp-server-glnxa64`
- `bin/syslab-mcp-server-glnxarm64`

## 4. Windows 本地编译

在仓库根目录执行：

```powershell
./scripts/build.ps1
```

该脚本会：

- 自动查找 `go.exe`
- 自动读取版本号
- 依次编译 Windows / Linux 的 amd64 与 arm64 产物
- 将编译缓存写入 `.cache/go-build`

如需显式指定版本号：

```powershell
$env:VERSION = "0.1.0-local"
./scripts/build.ps1
```

## 5. Linux 本地编译

在仓库根目录执行：

```sh
sh ./scripts/build.sh
```

如需显式指定版本号：

```sh
VERSION=0.1.0-local sh ./scripts/build.sh
```

## 6. 仅编译当前平台单个可执行文件

如果不使用仓库脚本，也可以直接调用 `go build`。

### Windows x64

```powershell
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -trimpath -ldflags "-s -w -X main.version=0.1.0-local" -o .\bin\syslab-mcp-server-win64.exe .\cmd\syslab-mcp-core-server
```

### Linux x64

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
go build -trimpath -ldflags "-s -w -X main.version=0.1.0-local" \
  -o ./bin/syslab-mcp-server-glnxa64 ./cmd/syslab-mcp-core-server
```

说明：

- 推荐优先使用 `scripts/build.ps1` 或 `scripts/build.sh`，避免漏掉命名规则和版本注入参数。

## 7. 打包发布文件

如果需要生成发布压缩包与校验文件，在 Linux 或带 `sh` 环境的构建机执行：

```sh
sh ./scripts/package.sh
```

脚本会执行以下动作：

- 如未设置 `SKIP_BUILD=1`，先调用 `scripts/build.sh`
- 生成压缩包
- 生成 `bin/SHA256SUMS`

当前脚本会生成：

- `bin/syslab-mcp-server-win64.zip`
- `bin/syslab-mcp-server-glnxa64.zip`
- `bin/SHA256SUMS`

## 8. 基础自检

编译完成后，至少执行以下检查：

### 8.1 Go 单元测试

```powershell
go test ./...
```

或：

```sh
go test ./...
```

### 8.2 查看帮助信息

Windows：

```powershell
.\bin\syslab-mcp-server-win64.exe --help
```

Linux：

```sh
./bin/syslab-mcp-server-glnxa64 --help
```

### 8.3 最小启动验证

服务启动时要求传入 `--syslab-root`，例如：

```powershell
.\bin\syslab-mcp-server-win64.exe --syslab-root "C:\Program Files\MWORKS\Syslab 2026a"
```

```sh
./bin/syslab-mcp-server-glnxa64 --syslab-root "/opt/MWORKS/Syslab"
```

如果只是验证参数解析与进程启动，可使用本机已有的有效安装路径。

### 8.4 MCP 接口联调

仓库提供了基础接口测试脚本：

```powershell
python .\scripts\mcp_interface_test.py
```

```sh
python ./scripts/mcp_interface_test.py
```

运行该脚本前，请先确认：

- 已完成可执行文件编译
- 本机具备可用的 Syslab / Julia 环境
- 测试脚本所需参数或默认路径与本机环境一致

## 9. 常见问题

### 9.1 提示找不到 Go

Windows 构建脚本会按以下顺序查找 Go：

1. `tools/go/bin/go.exe`
2. `PATH` 中的 `go.exe`

如果仍然失败，请先执行：

```powershell
go version
```

### 9.2 首次构建较慢

首次执行 `go build` 或 `go test` 时，Go 可能需要初始化模块缓存，耗时会明显高于后续构建。

### 9.3 可执行文件存在，但联调失败

编译成功只表示二进制可生成，不代表本机 Syslab 运行环境已经配置正确。联调失败时，优先检查：

- `--syslab-root` 是否有效
- `julia-root` 是否需要显式传入
- 本机图形环境是否支持 `desktop` 模式
- 本机是否已有可用的默认 Syslab 环境

## 10. 建议的本地提交流程

提交前建议至少执行：

```powershell
go test ./...
./scripts/build.ps1
python .\scripts\mcp_interface_test.py
```

Linux 环境可对应执行：

```sh
go test ./...
sh ./scripts/build.sh
python ./scripts/mcp_interface_test.py
```

如果仅修改文档，可以不强制重新编译，但提交涉及可执行程序、启动逻辑、参数或工具行为时，应完整执行上述流程。
