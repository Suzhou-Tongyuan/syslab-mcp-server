# Changelog

## 0.1.0

首个公开版本，提供以下用户可感知能力和交付内容：

- 产品名：`Syslab MCP Server`
- 发布产物：
  - `syslab-mcp-server-win64.exe`
  - `syslab-mcp-server-glnxa64`
- MCP 工具：
  - `detect_syslab_toolboxes`
  - `evaluate_julia_code`
  - `run_julia_file`
  - `restart_julia`
  - `read_syslab_skill`
  - `search_syslab_docs`
  - `read_syslab_doc`
  - `map_matlab_functions_to_julia`
- Julia 执行模型：
  - 默认使用全局环境和全局 session
- 启动参数：
  - `--syslab-root`（必填）
  - `--julia-root`（选填；不传时自动查找）
  - `--initial-working-folder`
  - `--initialize-syslab-on-startup=false`
  - `--pkg-offline=true`
  - `--syslab-display-mode=desktop`
