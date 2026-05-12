package session

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"syslab-mcp/internal/config"
	"syslab-mcp/internal/testutil"
)

func TestResolveRequestContextFallsBackToGlobalSession(t *testing.T) {
	root := testutil.TestDir(t)
	workDir := filepath.Join(root, "scratch")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(config.Config{InitialWorkingFolder: root}, log.New(io.Discard, "", 0))
	key, envPath, cwd := mgr.resolveRequestContext("", workDir)

	if key != "" {
		t.Fatalf("expected global key, got %q", key)
	}
	if envPath != "" {
		t.Fatalf("expected empty env path, got %q", envPath)
	}
	if cwd != workDir {
		t.Fatalf("expected cwd %q, got %q", workDir, cwd)
	}
}

func TestResolveRequestContextDoesNotInjectInitialWorkingFolderIntoCalls(t *testing.T) {
	root := testutil.TestDir(t)
	mgr := NewManager(config.Config{InitialWorkingFolder: root}, log.New(io.Discard, "", 0))

	key, envPath, cwd := mgr.resolveRequestContext("", "")

	if key != "" {
		t.Fatalf("expected global key, got %q", key)
	}
	if envPath != "" {
		t.Fatalf("expected empty env path, got %q", envPath)
	}
	if cwd != "" {
		t.Fatalf("expected empty cwd for call context, got %q", cwd)
	}
}

func TestBuildBridgeCommandSetsPkgOfflineEnv(t *testing.T) {
	cmd := buildBridgeCommand(context.Background(), "julia-ty.bat", "bridge.jl", "", true)
	found := false
	for _, item := range cmd.Env {
		if strings.EqualFold(item, "JULIA_PKG_OFFLINE=true") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected JULIA_PKG_OFFLINE=true in env, got %+v", cmd.Env)
	}
}

func TestResolveSyslabDesktopExecutable(t *testing.T) {
	root := testutil.TestDir(t)

	if runtime.GOOS == "windows" {
		syslabExe := filepath.Join(root, "Bin", "syslab.exe")
		if err := os.MkdirAll(filepath.Dir(syslabExe), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(syslabExe, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := resolveSyslabDesktopExecutable(root)
		if err != nil {
			t.Fatalf("resolveSyslabDesktopExecutable() error = %v", err)
		}
		if got != syslabExe {
			t.Fatalf("expected executable %q, got %q", syslabExe, got)
		}
		return
	}

	syslabScript := filepath.Join(root, "syslab.sh")
	if err := os.WriteFile(syslabScript, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveSyslabDesktopExecutable(root)
	if err != nil {
		t.Fatalf("resolveSyslabDesktopExecutable() error = %v", err)
	}
	if got != syslabScript {
		t.Fatalf("expected executable %q, got %q", syslabScript, got)
	}
}

func TestDesktopReplyToBridgeResultStripsCodeFence(t *testing.T) {
	result := desktopReplyToBridgeResult(desktopMessage{
		Command: desktopEvalCommand,
		Result: map[string]any{
			"inline": "✓",
			"all":    "```\n2\n```",
		},
	})
	if strings.TrimSpace(result.Result) != "2" {
		t.Fatalf("expected stripped result 2, got %q", result.Result)
	}
	if !strings.Contains(result.Stdout, "2") {
		t.Fatalf("expected stdout to preserve original body, got %q", result.Stdout)
	}
}

func TestDesktopWithTerminalOutputPrefersTerminalData(t *testing.T) {
	base := BridgeResult{Stdout: "old", Result: "2"}
	got := mergeDesktopTerminalOutput(base, desktopMessage{
		Command: desktopTerminalDataCommand,
		Result: map[string]any{
			"all": "terminal output",
		},
	})
	if got.Result != "2" {
		t.Fatalf("expected result to be preserved, got %q", got.Result)
	}
	if got.Stdout != "terminal output" {
		t.Fatalf("expected stdout to be replaced by terminal data, got %q", got.Stdout)
	}
}

func TestBuildDesktopCommandDetachesFromRequestContextAndStdin(t *testing.T) {
	cmd := buildDesktopCommand(context.Background(), "/tmp/syslab.sh", "", "/tmp/test.sock", nil)
	if cmd.Cancel != nil {
		t.Fatal("expected desktop command to outlive request context")
	}
	if cmd.Stdin == nil {
		t.Fatal("expected desktop command stdin to be detached from MCP stdin")
	}
	found := false
	for _, item := range cmd.Env {
		if item == desktopPipeEnvVar+"=/tmp/test.sock" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s in env, got %+v", desktopPipeEnvVar, cmd.Env)
	}
}

func TestBuildDesktopCommandUsesBashForShellScriptOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script launcher behavior is unix-only")
	}
	cmd := buildDesktopCommand(context.Background(), "/tmp/syslab.sh", "", "/tmp/test.sock", nil)
	if filepath.Base(cmd.Path) != "bash" {
		t.Fatalf("expected bash launcher, got %q", cmd.Path)
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "/tmp/syslab.sh" {
		t.Fatalf("expected bash to launch target script, got %+v", cmd.Args)
	}
}

func TestBuildDesktopCommandStartsAfterRequestContextCanceled(t *testing.T) {
	dir := testutil.TestDir(t)
	outputPath := filepath.Join(dir, "started.txt")

	scriptPath := filepath.Join(dir, "desktop-launch")
	if runtime.GOOS == "windows" {
		scriptPath += ".cmd"
		body := "@echo off\r\necho started>\"" + outputPath + "\"\r\n"
		if err := os.WriteFile(scriptPath, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	} else {
		scriptPath += ".sh"
		body := "#!/bin/sh\nprintf 'started' > \"" + outputPath + "\"\n"
		if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := buildDesktopCommand(ctx, scriptPath, dir, filepath.Join(dir, "test.sock"), nil)
	if err := cmd.Start(); err != nil {
		t.Fatalf("expected detached desktop command to start after context cancellation: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("expected detached desktop command to exit cleanly: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(outputPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected desktop command to create %q", outputPath)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestValidateDesktopEnvironmentWindowsDoesNotRequireDisplay(t *testing.T) {
	err := validateDesktopEnvironment("windows", desktopEnvStatus{})
	if err != nil {
		t.Fatalf("expected windows desktop env check to pass, got %v", err)
	}
}

func TestValidateDesktopEnvironmentLinuxRequiresDisplayOrWayland(t *testing.T) {
	err := validateDesktopEnvironment("linux", desktopEnvStatus{})
	if err == nil {
		t.Fatal("expected linux desktop env check to fail without display variables")
	}
	if !strings.Contains(err.Error(), "DISPLAY") || !strings.Contains(err.Error(), "WAYLAND_DISPLAY") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDesktopEnvironmentLinuxAcceptsDisplay(t *testing.T) {
	err := validateDesktopEnvironment("linux", desktopEnvStatus{Display: ":0"})
	if err != nil {
		t.Fatalf("expected DISPLAY to satisfy linux desktop env check, got %v", err)
	}
}

func TestValidateDesktopEnvironmentLinuxAcceptsWayland(t *testing.T) {
	err := validateDesktopEnvironment("linux", desktopEnvStatus{WaylandDisplay: "wayland-0"})
	if err != nil {
		t.Fatalf("expected WAYLAND_DISPLAY to satisfy linux desktop env check, got %v", err)
	}
}
