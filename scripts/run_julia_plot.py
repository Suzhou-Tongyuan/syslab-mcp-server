import json
import subprocess
import sys
import os

SERVER_EXE = r"D:\zwh\syslab-mcp\bin\syslab-mcp-server-win64.exe"
SYSLAB_ROOT = os.environ.get("SYSLAB_ROOT", r"C:\Program Files\MWORKS\Syslab 2026a")
JULIA_ROOT = os.environ.get("JULIA_ROOT", r"C:\Users\Public\TongYuan\julia-1.10.10")

def invoke_mcp(requests):
    payload = "\n".join(json.dumps(item, ensure_ascii=False) for item in requests) + "\n"
    proc = subprocess.run(
        [SERVER_EXE, f"--syslab-root={SYSLAB_ROOT}", f"--julia-root={JULIA_ROOT}"],
        input=payload,
        text=True,
        encoding="utf-8",
        errors="replace",
        capture_output=True,
        timeout=120,
    )
    responses = []
    for line in proc.stdout.splitlines():
        line = line.strip()
        if line and line.startswith("{"):
            responses.append(json.loads(line))
    return responses

def get_tool_text(response):
    return response["result"]["content"][0]["text"]

requests = [
    {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
        "protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "test", "version": "1.0"}
    }},
    {"jsonrpc": "2.0", "method": "notifications/initialized"},
    {"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "read_syslab_skill", "arguments": {}}},
    {"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "detect_syslab_toolboxes", "arguments": {}}},
    {"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "evaluate_julia_code", "arguments": {
        "code": '''
using TyPlot

x = range(0, 2π, 100)
y = sin.(x)
figure()
plot(x, y, label="sin(x)", linewidth=2)
xlabel!("x")
ylabel!("y")
title!("Julia Plot")
legend!()
savefig("D:/zwh/syslab-mcp/test_plot.png")
println("Plot saved to test_plot.png")
'''
    }}},
]

responses = invoke_mcp(requests)
for resp in responses:
    if "result" in resp:
        print(get_tool_text(resp))