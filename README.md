# Syslab MCP Server

`Syslab MCP Server` 是一个面向本地 Syslab Julia 环境的 `stdio` MCP 服务，支持 Windows 和 Linux。

它提供的能力包括：

- 查询本机 Syslab Julia 环境和已安装的 Julia 包
- 执行 Julia 代码
- 执行本地 Julia 脚本
- 重启 Julia 会话
- 搜索本机 Syslab 帮助文档
- 读取本机帮助文档正文
- 查询与 MATLAB 函数等价的 Syslab Julia 函数
- 读取 Syslab skills 相关技能

## 系统要求

- Windows 10/11 64 位；具备使用大模型能力的 Linux 系统，例如 CentOS 8、Ubuntu 20.04及以上
- 已安装 Syslab V26.2.0 及以上版本

## 安装方式

安装 Syslab 时会自动安装 Syslab MCP Server，默认给 Claude code、OpenCode、Codex 三类 agent 配置。

如果失败可以参考以下方式手动执行"<Syslab 安装目录>/Tools/syslab-mcp-server" 下的安装脚本。

下文以 Windows 为例：

1. 右键点击 syslab-mcp-server-install.ps1，选择「使用 PowerShell 运行」，或在当前目录启动 Powershell 后执行

   ```powershell
   ./syslab-mcp-server-install.ps1
   ```

2. 按提示输入 Syslab 安装路径（直接回车使用默认值）

3. 脚本会自动将 MCP 配置添加到 Claude 、OpenCode、Codex 的配置文件中

4. 重启 Claude Code / OpenCode / Codex 客户端即可使用

如果一键部署脚本运行失败，也可以手动配置。

### Codex:

```powershell
# 安装
codex mcp add syslab -- "<path-to>/syslab-mcp-server-win64.exe" --syslab-root="<syslab-root>" --julia-root="<julia-root>" --syslab-display-mode=desktop

# 卸载 
codex mcp remove syslab
```

例如：

```powershell
# 安装
codex mcp add syslab -- "C:\Program Files\MWORKS\Syslab 2026a\Tools\syslab-mcp-server\syslab-mcp-server-win64.exe" --syslab-root="C:\Program Files\MWORKS\Syslab 2026a" --julia-root="C:\Users\Public\TongYuan\julia-1.10.10" --syslab-display-mode=desktop
```

安装后，在`~/.codex/config.toml`下可以查看配置：

```toml
[mcp_servers.syslab]
command = 'C:\Program Files\MWORKS\Syslab 2026a\Tools\syslab-mcp-server\syslab-mcp-server-win64.exe'
args = ['--syslab-root=C:\Program Files\MWORKS\Syslab 2026a', '--julia-root=C:\Users\Public\TongYuan\julia-1.10.10', '--syslab-display-mode=desktop']
```

如果使用 Codex 在 Linux 下启用 `desktop`，建议在 `~/.codex/config.toml` 中为 `syslab` MCP 配置最后手动添加：

```toml
env_vars = ["DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "DBUS_SESSION_BUS_ADDRESS"]
```

用于继承图形会话环境，确保图形程序能够正常启动和显示。



### Claude Desktop:

```powershell
# 安装
claude mcp add --scope user syslab -- "<path-to>/syslab-mcp-server-win64.exe" --syslab-root="<syslab-root>" --julia-root="<julia-root>" --syslab-display-mode=desktop

# 卸载 
claude mcp remove syslab
```

例如：

```powershell
claude mcp add --scope user syslab -- "C:\Program Files\MWORKS\Syslab 2026a\Tools\syslab-mcp-server\syslab-mcp-server-win64.exe" --syslab-root="C:\Program Files\MWORKS\Syslab 2026a" --julia-root="C:\Users\Public\TongYuan\julia-1.10.10" --syslab-display-mode=desktop

# --scope=
# local（默认）： 仅当前项目可用
# project：团队共享（存储在.mcp.json，可提交版本库）
# user：所有项目可用（个人全局配置）
```

安装后，在 Claude 的 MCP 配置文件（如`C:\Users\TR\.claude.json `）中加入类似配置：

```json
{
    "mcpServers": {
    "syslab": {
      "type": "stdio",
      "command": "C:\\Program Files\\MWORKS\\Syslab 2026a\\Tools\\syslab-mcp-server\\syslab-mcp-server-win64.exe",
      "args": [
        "--syslab-root=C:\\Program Files\\MWORKS\\Syslab 2026a",
        "--julia-root=C:\\Users\\Public\\TongYuan\\julia-1.10.10",
        "--syslab-display-mode=desktop"
      ],
      "env": {}
    }
  },
}
```

## 启动参数

| 参数 | 说明 | 示例 |
| --- | --- | --- |
| `syslab-root` | Syslab 安装根目录。必填。 | `--syslab-root="C:\Program Files\MWORKS\Syslab 2026a"` |
| `julia-root` | Julia 安装根目录。选填。不传时自动查找。 | `--julia-root="C:\Users\Public\TongYuan\julia-1.10.10"` |
| `initial-working-folder` | 服务启动后的初始工作目录。不传时继承当前进程工作目录。 | `--initial-working-folder="D:\workspace"` |
| `initialize-syslab-on-startup` | 是否在 MCP `initialize` 时预启动 Syslab。默认值：`false`。 | `--initialize-syslab-on-startup=true` |
| `pkg-offline` | 是否以离线包模式启动 Julia。默认值：`true`。 | `--pkg-offline=true` |
| `syslab-display-mode` | 是否启动 Syslab 桌面。`nodesktop` 不启动 Syslab 桌面；`desktop` 启动 Syslab 桌面。默认值：`desktop`。 | `--syslab-display-mode=desktop` |

## 超时配置

不同 Agent 对 MCP 工具调用有默认超时限制。如果遇到工具执行超时问题，可以按需调整超时时间。

### OpenCode

在 `~/.config/opencode/opencode.json` 中为 `syslab` MCP 配置添加 `timeout` 字段：

```json
{
  "mcp": {
    "syslab": {
      "type": "...",
      "command": ["..."],
      "timeout": 300000
    }
  }
}
```

单位为毫秒，`300000` 表示 5 分钟。

### Codex

在 `~/.codex/config.toml` 中为 `syslab` MCP 配置添加 `tool_timeout_sec` 字段：

```toml
[mcp_servers.syslab]
command = "..."
args = ["..."]
tool_timeout_sec = 300
```

单位为秒，`300` 表示 5 分钟。

## MCP Tools

1. `detect_syslab_toolboxes`
   - 返回本机 Syslab 版本信息、Julia 环境信息、Syslab Julia 环境中已安装的 Julia 包，以及可发现的本地包文档路径。
   - 输入参数：
   - `include_all_packages`（boolean，可选）：是否返回当前 Julia 全局环境中的所有已安装包。默认 `false`。不填或为 `false` 时，仅返回包名以 `Ty` 开头的包。
1. `evaluate_julia_code`
   - 执行一段 Julia 代码并返回输出与最终结果。
   - 输入参数：
   - `code`（string）：要执行的 Julia 代码。
1. `run_julia_file`
   - 执行本地 Julia 脚本并返回输出。脚本路径必须指向有效的 `.jl` 文件。
   - 输入参数：
   - `script_path`（string）：Julia 脚本绝对路径。示例：`C:\Users\name\project\demo.jl` 或 `/home/user/project/demo.jl`。
1. `restart_julia`
   - 重启全局 Julia 会话。
   - 输入参数：
   - `working_directory`（string，可选）：重启后使用的工作目录。省略时使用默认工作目录。
1. `read_syslab_skill`
   - 读取 Syslab skill markdown 文件内容。
   - 输入参数：
   - `skill_path`（string，必填）：传 `default` 时读取默认 skill；传绝对路径时读取指定 skill。
1. `search_syslab_docs`
   - 搜索本地已索引的 Syslab 帮助文档。
   - 输入参数：
   - `query`（string）：搜索关键词。
   - `package`（string，可选）：限定搜索的包名。
   - `max_results`（number，可选）：返回结果条数上限。
1. `read_syslab_doc`
   - 读取一篇已索引的 Syslab 帮助文档的正文内容。
   - 输入参数：
   - `doc_path`（string）：文档路径，通常直接使用 `search_syslab_docs` 返回结果中的 `path` 字段。
1. `map_matlab_functions_to_julia`
   - 将一组 MATLAB 函数名映射到 Syslab Julia 环境中的候选等价函数及相关文档，适用于 MATLAB 代码迁移和函数替换场景。
   - 输入参数：
   - `symbols`（string[]）：MATLAB 函数名列表。
   - `max_results_per_symbol`（number，可选）：每个 MATLAB 函数最多返回的候选数量。

## 许可证

本项目采用 MIT 许可证授权，有关详细信息，请参阅 [LICENSE](./LICENSE) 文件。