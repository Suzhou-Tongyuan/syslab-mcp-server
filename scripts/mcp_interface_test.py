import json
import os
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
SERVER_EXE = ROOT / "bin" / "syslab-mcp-server-win64.exe"
SYSLAB_ROOT = os.environ.get("SYSLAB_ROOT", r"C:\Program Files\MWORKS\Syslab 2026a")
JULIA_ROOT = os.environ.get("JULIA_ROOT", r"C:\Users\Public\TongYuan\julia-1.10.10")
SYSLAB_DISPLAY_MODE = os.environ.get("SYSLAB_DISPLAY_MODE", "nodesktop")
SCRIPT_PATH = (ROOT / "tests" / "fixtures" / "sample_script.jl").as_posix()
SKILL_FILE = (ROOT / "tests" / "fixtures" / "sample_skill.md").as_posix()
SKILL_FILE_OVERRIDE = (ROOT / "tests" / "fixtures" / "sample_skill_override.md").as_posix()


def assert_true(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def invoke_mcp_batch(requests: list[dict], timeout: int = 120) -> list[dict]:
    payload = "\n".join(json.dumps(item, ensure_ascii=False) for item in requests) + "\n"
    proc = subprocess.run(
        [
            str(SERVER_EXE),
            f"--syslab-root={SYSLAB_ROOT}",
            f"--julia-root={JULIA_ROOT}",
            f"--skill-file={SKILL_FILE}",
            f"--initial-working-folder={ROOT}",
            f"--syslab-display-mode={SYSLAB_DISPLAY_MODE}",
        ],
        input=payload,
        text=True,
        encoding="utf-8",
        errors="replace",
        capture_output=True,
        timeout=timeout,
        cwd=ROOT,
    )

    responses: list[dict] = []
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line or not line.startswith("{"):
            continue
        responses.append(json.loads(line))
    return responses


def get_response_by_id(responses: list[dict], request_id: int) -> dict:
    for response in responses:
        if response.get("id") == request_id:
            return response
    raise AssertionError(f"response missing for id={request_id}")


def get_tool_text(response: dict) -> str:
    result = response.get("result")
    assert_true(result is not None, "missing result in MCP response")
    content = result.get("content")
    assert_true(bool(content), "missing content in MCP tool response")
    return str(content[0]["text"])


def main() -> int:
    assert_true(SERVER_EXE.exists(), f"server executable not found at {SERVER_EXE}")

    phase1 = [
        {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": {
                "protocolVersion": "2025-06-18",
                "capabilities": {},
                "clientInfo": {"name": "mcp-interface-test", "version": "1.0"},
            },
        },
        {"jsonrpc": "2.0", "method": "notifications/initialized"},
        {"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}},
        {
            "jsonrpc": "2.0",
            "id": 31,
            "method": "tools/call",
            "params": {"name": "read_syslab_skill", "arguments": {}},
        },
        {
            "jsonrpc": "2.0",
            "id": 3,
            "method": "tools/call",
            "params": {"name": "detect_syslab_toolboxes", "arguments": {}},
        },
        {
            "jsonrpc": "2.0",
            "id": 4,
            "method": "tools/call",
            "params": {
                "name": "evaluate_julia_code",
                "arguments": {"code": "1+1"},
            },
        },
        {
            "jsonrpc": "2.0",
            "id": 41,
            "method": "tools/call",
            "params": {
                "name": "evaluate_julia_code",
                "arguments": {"code": "a = 1"},
            },
        },
        {
            "jsonrpc": "2.0",
            "id": 42,
            "method": "tools/call",
            "params": {
                "name": "evaluate_julia_code",
                "arguments": {"code": "a"},
            },
        },
        {
            "jsonrpc": "2.0",
            "id": 5,
            "method": "tools/call",
            "params": {
                "name": "run_julia_file",
                "arguments": {"script_path": SCRIPT_PATH},
            },
        },
        {
            "jsonrpc": "2.0",
            "id": 6,
            "method": "tools/call",
            "params": {
                "name": "restart_julia",
                "arguments": {},
            },
        },
        {
            "jsonrpc": "2.0",
            "id": 7,
            "method": "tools/call",
            "params": {
                "name": "search_syslab_docs",
                "arguments": {"query": "hampel", "max_results": 2},
            },
        },
    ]

    responses = invoke_mcp_batch(phase1)

    tools_response = get_response_by_id(responses, 2)
    tool_names = [item["name"] for item in tools_response["result"]["tools"]]
    for required_name in [
        "detect_syslab_toolboxes",
        "evaluate_julia_code",
        "run_julia_file",
        "restart_julia",
        "read_syslab_skill",
        "search_syslab_docs",
        "read_syslab_doc",
    ]:
        assert_true(required_name in tool_names, f"tools/list missing {required_name}")

    detect_payload = json.loads(get_tool_text(get_response_by_id(responses, 3)))
    assert_true(
        detect_payload["tool"] == "detect_syslab_toolboxes",
        "unexpected detect_syslab_toolboxes payload",
    )
    assert_true(
        len(detect_payload["toolboxes"]) > 0,
        "detect_syslab_toolboxes returned no toolboxes",
    )

    skill_override_request = {
        "jsonrpc": "2.0",
        "id": 32,
        "method": "tools/call",
        "params": {"name": "read_syslab_skill", "arguments": {"skill_path": SKILL_FILE_OVERRIDE}},
    }
    skill_responses = invoke_mcp_batch(phase1[:2] + [phase1[3], skill_override_request])
    skill_payload = json.loads(get_tool_text(get_response_by_id(skill_responses, 31)))
    assert_true(skill_payload["tool"] == "read_syslab_skill", "unexpected read_syslab_skill payload")
    assert_true(skill_payload["loaded"], "read_syslab_skill did not load sample skill")
    assert_true(
        skill_payload["skill_path"].replace("\\", "/").endswith("/tests/fixtures/sample_skill.md"),
        f"unexpected skill path: {skill_payload['skill_path']}",
    )
    assert_true("Sample Skill" in skill_payload["content"], "read_syslab_skill returned unexpected content")

    skill_override_payload = json.loads(get_tool_text(get_response_by_id(skill_responses, 32)))
    assert_true(
        skill_override_payload["skill_path"].replace("\\", "/").endswith("/tests/fixtures/sample_skill_override.md"),
        f"unexpected override skill path: {skill_override_payload['skill_path']}",
    )
    assert_true("Override Skill" in skill_override_payload["content"], "read_syslab_skill override returned unexpected content")

    eval_text = get_tool_text(get_response_by_id(responses, 4))
    assert_true("result:\n2" in eval_text, "evaluate_julia_code did not return expected result")

    eval_assign_text = get_tool_text(get_response_by_id(responses, 41))
    assert_true("result:\n1" in eval_assign_text, "evaluate_julia_code assignment did not return expected result")

    eval_reuse_text = get_tool_text(get_response_by_id(responses, 42))
    assert_true("result:\n1" in eval_reuse_text, "evaluate_julia_code did not preserve Main workspace state")

    run_text = get_tool_text(get_response_by_id(responses, 5))
    assert_true("hello from sample script" in run_text, "run_julia_file output missing script stdout")
    assert_true("result:\n3" in run_text, "run_julia_file output missing final result")

    restart_payload = json.loads(get_tool_text(get_response_by_id(responses, 6)))
    assert_true(restart_payload["tool"] == "restart_julia", "unexpected restart_julia payload")
    assert_true(
        restart_payload["session"]["status"] == "running",
        "restart_julia did not return a running session",
    )

    search_payload = json.loads(get_tool_text(get_response_by_id(responses, 7)))
    doc_path = ""
    if search_payload["matches"]:
        doc_path = search_payload["matches"][0]["path"]
        assert_true(bool(doc_path), "search_syslab_docs returned an empty doc path")

    if doc_path:
        phase2 = [
            {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {
                    "protocolVersion": "2025-06-18",
                    "capabilities": {},
                    "clientInfo": {"name": "mcp-interface-test", "version": "1.0"},
                },
            },
            {"jsonrpc": "2.0", "method": "notifications/initialized"},
            {
                "jsonrpc": "2.0",
                "id": 91,
                "method": "tools/call",
                "params": {"name": "read_syslab_skill", "arguments": {}},
            },
            {
                "jsonrpc": "2.0",
                "id": 9,
                "method": "tools/call",
                "params": {"name": "read_syslab_doc", "arguments": {"doc_path": doc_path}},
            },
        ]

        read_payload = json.loads(get_tool_text(get_response_by_id(invoke_mcp_batch(phase2), 9)))
        assert_true(bool(read_payload["package"]), "read_syslab_doc returned empty package")
        assert_true(bool(read_payload["content"]), "read_syslab_doc returned empty content")

    print("MCP interface test passed.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.TimeoutExpired as exc:
        print(f"MCP interface test timed out: {exc}", file=sys.stderr)
        raise SystemExit(1)
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        raise SystemExit(1)
