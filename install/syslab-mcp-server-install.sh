#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
INPUT_LAUNCHER_PATH="${1:-}"
INPUT_SYSLAB_ROOT="${2:-}"
NON_INTERACTIVE=0

if [ -n "$INPUT_LAUNCHER_PATH" ] || [ -n "$INPUT_SYSLAB_ROOT" ]; then
  NON_INTERACTIVE=1
fi

resolve_launcher_name() {
  local architecture
  architecture="$(uname -m 2>/dev/null || printf 'x86_64')"

  case "${architecture,,}" in
    aarch64|arm64)
      printf '%s\n' 'syslab-mcp-server-glnxarm64'
      ;;
    *)
      printf '%s\n' 'syslab-mcp-server-glnxa64'
      ;;
  esac
}

resolve_default_launcher() {
  local launcher_name
  launcher_name="$(resolve_launcher_name)"

  if [ -e "$SCRIPT_DIR/$launcher_name" ]; then
    printf '%s\n' "$SCRIPT_DIR/$launcher_name"
    return 0
  fi

  if [ -e "$SCRIPT_DIR/../$launcher_name" ]; then
    (
      cd "$SCRIPT_DIR/.." && printf '%s/%s\n' "$(pwd -P)" "$launcher_name"
    )
    return 0
  fi

  return 1
}

resolve_default_syslab_root() {
  local launcher_path launcher_dir server_dir syslab_root
  launcher_path="$1"
  launcher_dir="$(dirname "$launcher_path")"
  server_dir="$(dirname "$launcher_dir")"
  syslab_root="$(dirname "$server_dir")"
  printf '%s\n' "$syslab_root"
}

confirm_continue() {
  local prompt="$1"
  local answer
  read -r -p "$prompt" answer
  case "$answer" in
    N|n) return 1 ;;
    *) return 0 ;;
  esac
}

json_escape() {
  local value="$1"
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  value=${value//$'\t'/\\t}
  printf '%s' "$value"
}

backup_if_exists() {
  local config_path="$1"
  local label="$2"
  local backup_path

  if [ -f "$config_path" ]; then
    backup_path="${config_path}.bak.$(date +%Y%m%d%H%M%S)"
    cp "$config_path" "$backup_path"
    printf '%s existing config backed up to: %s\n' "$label" "$backup_path"
  fi
}

ensure_json_config_file() {
  local config_path="$1"
  local config_dir

  config_dir="$(dirname "$config_path")"
  mkdir -p "$config_dir"

  if [ ! -f "$config_path" ]; then
    printf '{}\n' >"$config_path"
  fi
}

ensure_toml_config_file() {
  local config_path="$1"
  local config_dir

  config_dir="$(dirname "$config_path")"
  mkdir -p "$config_dir"
  touch "$config_path"
}

toml_escape() {
  local value="$1"
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  printf '%s' "$value"
}

set_toml_section() {
  local config_path="$1"
  local section_name="$2"
  local section_content="$3"
  local tmp_file

  tmp_file="$(mktemp "${TMPDIR:-/tmp}/syslab-toml-update.XXXXXX")"
  awk -v section_name="$section_name" -v section_content="$section_content" '
    BEGIN {
      target_header = "[" section_name "]"
      in_target = 0
      found = 0
    }
    {
      if ($0 == target_header) {
        if (!found) {
          print section_content
          found = 1
        }
        in_target = 1
        next
      }

      if (in_target && $0 ~ /^\[/) {
        in_target = 0
      }

      if (!in_target) {
        print $0
      }
    }
    END {
      if (!found) {
        if (NR > 0 && $0 != "") {
          print ""
        }
        print section_content
      }
    }
  ' "$config_path" >"$tmp_file"

  mv "$tmp_file" "$config_path"
}

get_python_command() {
  if command -v python3 >/dev/null 2>&1; then
    printf '%s\n' 'python3'
    return 0
  fi

  if command -v python >/dev/null 2>&1; then
    printf '%s\n' 'python'
    return 0
  fi

  return 1
}

update_json_config() {
  local config_path="$1"
  local container_key="$2"
  local entry_key="$3"
  local entry_json="$4"
  local label="$5"
  local python_cmd tmp_script

  python_cmd="$(get_python_command || true)"
  if [ -z "$python_cmd" ]; then
    printf 'Warning: python3/python not found, unable to update %s config safely.\n' "$label"
    printf 'Skipping %s configuration.\n' "$label"
    return 0
  fi

  if [ "$NON_INTERACTIVE" -eq 0 ]; then
    backup_if_exists "$config_path" "$label"
  fi

  tmp_script="$(mktemp "${TMPDIR:-/tmp}/syslab-config-update.XXXXXX.py")"
  cat >"$tmp_script" <<'PY'
from __future__ import unicode_literals

import io
import json
import os
import sys

if sys.version_info[0] < 3:
    text_type = unicode
else:
    text_type = str


def fail(message):
    raise SystemExit(message)


def read_json_object(config_path):
    with io.open(config_path, "r", encoding="utf-8") as f:
        content = f.read()

    if not content.strip():
        content = u"{}"

    try:
        data = json.loads(content)
    except ValueError as exc:
        fail("Invalid JSON in {}: {}".format(config_path, exc))

    if not isinstance(data, dict):
        fail("Expected JSON object in {}".format(config_path))

    return data


def write_json_object(config_path, data):
    rendered = json.dumps(data, ensure_ascii=False, indent=2)
    if not isinstance(rendered, text_type):
        rendered = rendered.decode("utf-8")

    with io.open(config_path, "w", encoding="utf-8", newline=u"\n") as f:
        f.write(rendered)
        f.write(u"\n")


config_path = os.path.abspath(sys.argv[1])
container_key = sys.argv[2]
entry_key = sys.argv[3]

try:
    entry_value = json.loads(sys.argv[4])
except ValueError as exc:
    fail("Invalid JSON payload for {}: {}".format(entry_key, exc))

data = read_json_object(config_path)

container = data.get(container_key)
if container is None:
    container = {}
    data[container_key] = container
elif not isinstance(container, dict):
    fail("Expected '{}' in {} to be a JSON object".format(container_key, config_path))

container[entry_key] = entry_value
write_json_object(config_path, data)
PY

  "$python_cmd" "$tmp_script" "$config_path" "$container_key" "$entry_key" "$entry_json"
  rm -f "$tmp_script"
  printf '%s configuration updated: %s\n' "$label" "$config_path"
}

write_claude_config_file() {
  local config_path="$1"
  local command_path="$2"
  local syslab_root="$3"
  local entry_json

  entry_json="$(printf '{"type":"stdio","command":"%s","args":["--syslab-root","%s"]}' \
    "$(json_escape "$command_path")" \
    "$(json_escape "$syslab_root")")"

  update_json_config "$config_path" "mcpServers" "syslab" "$entry_json" "Claude"
}

write_opencode_config_file() {
  local config_path="$1"
  local command_path="$2"
  local syslab_root="$3"
  local entry_json

  entry_json="$(printf '{"type":"local","command":["%s","--syslab-root","%s"]}' \
    "$(json_escape "$command_path")" \
    "$(json_escape "$syslab_root")")"

  update_json_config "$config_path" "mcp" "syslab" "$entry_json" "OpenCode"
}

write_codex_config_file() {
  local config_path="$1"
  local command_path="$2"
  local syslab_root="$3"
  local escaped_command_path escaped_syslab_root section_content

  escaped_command_path="$(toml_escape "$command_path")"
  escaped_syslab_root="$(toml_escape "$syslab_root")"
  section_content=$(printf '[mcp_servers.syslab]\ncommand = "%s"\nargs = ["--syslab-root", "%s"]\nenv_vars = ["DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "DBUS_SESSION_BUS_ADDRESS"]\n' \
    "$escaped_command_path" \
    "$escaped_syslab_root")

  if [ "$NON_INTERACTIVE" -eq 0 ]; then
    backup_if_exists "$config_path" "Codex"
  fi

  ensure_toml_config_file "$config_path"
  set_toml_section "$config_path" "mcp_servers.syslab" "$section_content"
  printf 'Codex configuration updated: %s\n' "$config_path"
}

configure_claude() {
  local launcher_path="$1"
  local syslab_root="$2"
  local config_path="$HOME/.claude.json"
  local answer

  printf '1. Configuring Claude...\n'

  if [ "$NON_INTERACTIVE" -eq 0 ] && [ ! -f "$config_path" ]; then
    printf 'Claude config not found: %s\n' "$config_path"
    printf 'Skipping Claude configuration.\n'
    return 0
  fi

  if [ -f "$config_path" ]; then
    printf 'Detected existing Claude config: %s\n' "$config_path"
  else
    printf 'Claude config will be created: %s\n' "$config_path"
  fi

  if [ "$NON_INTERACTIVE" -eq 0 ]; then
    read -r -p 'Configure Claude? (Y/N, default Y) ' answer
    case "$answer" in
      N|n)
        printf 'Skipping Claude configuration.\n'
        return 0
        ;;
    esac
  fi

  ensure_json_config_file "$config_path"
  write_claude_config_file "$config_path" "$launcher_path" "$syslab_root"
}

configure_opencode() {
  local launcher_path="$1"
  local syslab_root="$2"
  local config_path="$HOME/.config/opencode/opencode.json"
  local answer

  printf '\n2. Configuring OpenCode...\n'

  if [ "$NON_INTERACTIVE" -eq 0 ] && [ ! -f "$config_path" ]; then
    printf 'OpenCode config not found: %s\n' "$config_path"
    printf 'Skipping OpenCode configuration.\n'
    return 0
  fi

  if [ -f "$config_path" ]; then
    printf 'Detected existing OpenCode config: %s\n' "$config_path"
  else
    printf 'OpenCode config will be created: %s\n' "$config_path"
  fi

  if [ "$NON_INTERACTIVE" -eq 0 ]; then
    read -r -p 'Configure OpenCode? (Y/N, default Y) ' answer
    case "$answer" in
      N|n)
        printf 'Skipping OpenCode configuration.\n'
        return 0
        ;;
    esac
  fi

  ensure_json_config_file "$config_path"
  write_opencode_config_file "$config_path" "$launcher_path" "$syslab_root"
}

configure_codex() {
  local launcher_path="$1"
  local syslab_root="$2"
  local config_path="$HOME/.codex/config.toml"
  local answer

  printf '\n3. Configuring Codex...\n'

  if [ "$NON_INTERACTIVE" -eq 0 ] && [ ! -f "$config_path" ]; then
    printf 'Codex config not found: %s\n' "$config_path"
    printf 'Skipping Codex configuration.\n'
    return 0
  fi

  if [ -f "$config_path" ]; then
    printf 'Detected existing Codex config: %s\n' "$config_path"
  else
    printf 'Codex config will be created: %s\n' "$config_path"
  fi

  if [ "$NON_INTERACTIVE" -eq 0 ]; then
    read -r -p 'Configure Codex? (Y/N, default Y) ' answer
    case "$answer" in
      N|n)
        printf 'Skipping Codex configuration.\n'
        return 0
        ;;
    esac
  fi

  write_codex_config_file "$config_path" "$launcher_path" "$syslab_root"
}

if [ -n "$INPUT_LAUNCHER_PATH" ]; then
  DEFAULT_LAUNCHER="$INPUT_LAUNCHER_PATH"
else
  DEFAULT_LAUNCHER="$(resolve_default_launcher || true)"
fi

printf '=== MWORKS Syslab MCP One-Click Configuration Tool ===\n'
printf 'Launcher Path: %s\n\n' "$DEFAULT_LAUNCHER"

if [ "$NON_INTERACTIVE" -eq 0 ] && { [ -z "$DEFAULT_LAUNCHER" ] || [ ! -e "$DEFAULT_LAUNCHER" ]; }; then
  read -r -p "Enter Syslab MCP executable path${DEFAULT_LAUNCHER:+ (default: $DEFAULT_LAUNCHER)} " user_launcher_path
  DEFAULT_LAUNCHER="${user_launcher_path:-$DEFAULT_LAUNCHER}"
  printf 'Launcher Path: %s\n\n' "$DEFAULT_LAUNCHER"
fi

if [ -z "$DEFAULT_LAUNCHER" ] || [ ! -e "$DEFAULT_LAUNCHER" ]; then
  printf 'Error: launcher not found: %s\n' "$DEFAULT_LAUNCHER" >&2
  exit 1
fi

DEFAULT_SYSLAB_DIR="${SYSLAB_HOME:-$(resolve_default_syslab_root "$DEFAULT_LAUNCHER")}"

if [ -n "$INPUT_SYSLAB_ROOT" ]; then
  syslab_root="$INPUT_SYSLAB_ROOT"
elif [ "$NON_INTERACTIVE" -eq 1 ]; then
  syslab_root="$DEFAULT_SYSLAB_DIR"
else
  read -r -p "Enter Syslab install path (default: $DEFAULT_SYSLAB_DIR) " syslab_root
  syslab_root="${syslab_root:-$DEFAULT_SYSLAB_DIR}"
fi

if [ ! -e "$syslab_root" ]; then
  printf 'Warning: The specified Syslab install path %s does not exist, please confirm if it is correct\n' "$syslab_root"
  if [ "$NON_INTERACTIVE" -eq 0 ] && ! confirm_continue "Continue? (Y/N, default Y) "; then
    exit 0
  fi
fi

printf '\nUsing configuration:\n'
printf 'Syslab Install Path: %s\n' "$syslab_root"
printf 'Server Executable Path: %s\n\n' "$DEFAULT_LAUNCHER"

configure_claude "$DEFAULT_LAUNCHER" "$syslab_root"
configure_opencode "$DEFAULT_LAUNCHER" "$syslab_root"
configure_codex "$DEFAULT_LAUNCHER" "$syslab_root"

printf '\nConfiguration completed!\n'
printf 'Please restart Claude, OpenCode, and Codex to load the new MCP configuration.\n'
printf 'To uninstall, remove the syslab entry from each client configuration.\n'
