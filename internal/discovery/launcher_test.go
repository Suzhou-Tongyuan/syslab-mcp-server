package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"syslab-mcp/internal/testutil"
)

func TestResolveSyslabLauncherConfiguredPath(t *testing.T) {
	dir := testutil.TestDir(t)
	path := filepath.Join(dir, launcherFileName())
	if err := os.WriteFile(path, []byte("@echo off\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveSyslabLauncher(path, "")
	if err != nil {
		t.Fatalf("ResolveSyslabLauncher() error = %v", err)
	}
	if got != path {
		t.Fatalf("expected %s, got %s", path, got)
	}
}

func TestResolveSyslabLauncherRequiresConfiguredPathOrJuliaRoot(t *testing.T) {
	got, err := ResolveSyslabLauncher("", "")
	if err == nil {
		t.Fatalf("expected error, got path %q", got)
	}
	if !strings.Contains(err.Error(), "--syslab-launcher") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSyslabLauncherJuliaRoot(t *testing.T) {
	dir := testutil.TestDir(t)
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(binDir, launcherFileName())
	if err := os.WriteFile(path, []byte("@echo off\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveSyslabLauncher("", dir)
	if err != nil {
		t.Fatalf("ResolveSyslabLauncher() error = %v", err)
	}
	if got != path {
		t.Fatalf("expected %s, got %s", path, got)
	}
}
