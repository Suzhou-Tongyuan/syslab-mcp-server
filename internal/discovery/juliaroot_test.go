package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"syslab-mcp/internal/testutil"
)

func TestResolveJuliaRootConfiguredPath(t *testing.T) {
	dir := testutil.TestDir(t)

	got, err := resolveJuliaRootForGOOS("windows", dir, "")
	if err != nil {
		t.Fatalf("resolveJuliaRootForGOOS() error = %v", err)
	}
	if got != dir {
		t.Fatalf("expected %s, got %s", dir, got)
	}
}

func TestResolveJuliaRootWindowsRequiresConfiguredPath(t *testing.T) {
	dir := testutil.TestDir(t)
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("USERPROFILE", dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
	})

	got, err := resolveJuliaRootForGOOS("windows", "", "")
	if err == nil {
		t.Fatalf("expected error, got path %q", got)
	}
	if !strings.Contains(err.Error(), "syslab-env.ini") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveJuliaRootFallsBackToSyslabEnvIni(t *testing.T) {
	dir := testutil.TestDir(t)
	juliaRoot := filepath.Join(dir, "julia-1.10.10")
	if err := os.MkdirAll(juliaRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	syslabEnvDir := filepath.Join(dir, ".syslab")
	if err := os.MkdirAll(syslabEnvDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(syslabEnvDir, "syslab-env.ini")
	content := "[Syslab]\nJULIA_HOME=" + filepath.ToSlash(juliaRoot) + "\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("USERPROFILE")
	oldHomeEnv := os.Getenv("HOME")
	if err := os.Setenv("USERPROFILE", dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("USERPROFILE", oldHome)
		_ = os.Setenv("HOME", oldHomeEnv)
	})

	got, err := resolveJuliaRootForGOOS("windows", "", "")
	if err != nil {
		t.Fatalf("resolveJuliaRootForGOOS() error = %v", err)
	}
	if got != juliaRoot {
		t.Fatalf("expected %s, got %s", juliaRoot, got)
	}
}

func TestResolveJuliaRootLinuxFindsJuliaDirectoryUnderSyslabTools(t *testing.T) {
	syslabRoot := testutil.TestDir(t)
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	if err := os.Setenv("HOME", syslabRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("USERPROFILE", syslabRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
	})

	juliaRoot := filepath.Join(syslabRoot, "Tools", "julia-1.10.10")
	if err := os.MkdirAll(filepath.Join(juliaRoot, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.sh")
	if err := os.WriteFile(launcherPath, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveJuliaRootForGOOS("linux", "", syslabRoot)
	if err != nil {
		t.Fatalf("resolveJuliaRootForGOOS() error = %v", err)
	}
	if got != juliaRoot {
		t.Fatalf("expected %s, got %s", juliaRoot, got)
	}
}
