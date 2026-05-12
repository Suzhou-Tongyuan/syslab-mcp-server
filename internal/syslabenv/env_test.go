package syslabenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"syslab-mcp/internal/testutil"
)

func TestLoadFromWindowsLauncher(t *testing.T) {
	dir := testutil.TestDir(t)
	// Create the target depot directory so it exists
	depotDir := filepath.Join(dir, "..", "..", ".julia")
	if err := os.MkdirAll(depotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(dir, "julia-ty.bat")
	content := "@echo off\nset JULIA_DEPOT_PATH=%~dp0../../.julia\n"
	if err := os.WriteFile(launcher, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	env, err := LoadFromLauncher(launcher)
	if err != nil {
		t.Fatalf("LoadFromLauncher() error = %v", err)
	}
	want := filepath.Clean(depotDir)
	if got := env.Values["JULIA_DEPOT_PATH"]; got != want {
		t.Fatalf("expected JULIA_DEPOT_PATH %q, got %q", want, got)
	}
}

func TestDefaultExists(t *testing.T) {
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

	exists, err := DefaultExists()
	if err != nil {
		t.Fatalf("DefaultExists() error = %v", err)
	}
	if exists {
		t.Fatal("expected syslab-env.ini to be absent")
	}

	envDir := filepath.Join(dir, ".syslab")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "syslab-env.ini"), []byte("[Syslab]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	exists, err = DefaultExists()
	if err != nil {
		t.Fatalf("DefaultExists() error after create = %v", err)
	}
	if !exists {
		t.Fatal("expected syslab-env.ini to exist")
	}
}

func TestLoadExpandsVariables(t *testing.T) {
	dir := testutil.TestDir(t)
	path := filepath.Join(dir, "syslab-env.ini")
	content := `[Syslab]
JULIA_HOME=C:/Users/Public/TongYuan/julia-1.9.3
JULIA_DEPOT_PATH=C:/Users/Public/TongYuan/.julia
TY_CONDA3=${JULIA_DEPOT_PATH}/miniforge3
PYTHON=${TY_CONDA3}/python.exe
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	env, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	expectedPython := filepath.Clean("C:/Users/Public/TongYuan/.julia/miniforge3/python.exe")
	if got := env.Values["PYTHON"]; got != expectedPython {
		t.Fatalf("expected PYTHON %q, got %q", expectedPython, got)
	}
}

func TestMergePrefersPrimaryValues(t *testing.T) {
	primary := Env{Path: "primary", Values: map[string]string{"JULIA_DEPOT_PATH": "A"}}
	fallback := Env{Path: "fallback", Values: map[string]string{"JULIA_DEPOT_PATH": "B", "PYTHON": "C"}}

	merged := Merge(primary, fallback)
	if merged.Path != "primary" {
		t.Fatalf("expected primary path, got %q", merged.Path)
	}
	if merged.Values["JULIA_DEPOT_PATH"] != "A" {
		t.Fatalf("expected primary JULIA_DEPOT_PATH, got %q", merged.Values["JULIA_DEPOT_PATH"])
	}
	if merged.Values["PYTHON"] != "C" {
		t.Fatalf("expected fallback PYTHON, got %q", merged.Values["PYTHON"])
	}
}

func TestEnvDepotPathIfExists(t *testing.T) {
	dir := testutil.TestDir(t)
	depotPath := filepath.Join(dir, ".julia")
	if err := os.MkdirAll(depotPath, 0o755); err != nil {
		t.Fatal(err)
	}

	oldDepot := os.Getenv("JULIA_DEPOT_PATH")
	if err := os.Setenv("JULIA_DEPOT_PATH", depotPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("JULIA_DEPOT_PATH", oldDepot) })

	got, ok, err := EnvDepotPathIfExists()
	if err != nil {
		t.Fatalf("EnvDepotPathIfExists() error = %v", err)
	}
	if !ok {
		t.Fatal("expected depot path to exist")
	}
	if got != depotPath {
		t.Fatalf("expected depot path %q, got %q", depotPath, got)
	}
}

func TestEnvDepotPathIfExists_MultiplePaths(t *testing.T) {
	dir := testutil.TestDir(t)
	depotPath := filepath.Join(dir, ".julia")
	invalidPath := filepath.Join(dir, "nonexistent")
	if err := os.MkdirAll(depotPath, 0o755); err != nil {
		t.Fatal(err)
	}

	separator := ";"
	if filepath.Separator != '\\' {
		separator = ":"
	}
	multiPaths := invalidPath + separator + depotPath

	oldDepot := os.Getenv("JULIA_DEPOT_PATH")
	if err := os.Setenv("JULIA_DEPOT_PATH", multiPaths); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("JULIA_DEPOT_PATH", oldDepot) })

	got, ok, err := EnvDepotPathIfExists()
	if err != nil {
		t.Fatalf("EnvDepotPathIfExists() error = %v", err)
	}
	if !ok {
		t.Fatal("expected depot path to exist")
	}
	if got != depotPath {
		t.Fatalf("expected depot path %q, got %q", depotPath, got)
	}
}

func TestLoadFromLauncherPrefersTYDepotPath(t *testing.T) {
	dir := testutil.TestDir(t)
	launcher := filepath.Join(dir, "julia-ty.bat")
	content := "@echo off\nset JULIA_DEPOT_PATH=%~dp0../../.julia\n"
	if err := os.WriteFile(launcher, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	old := os.Getenv("TY_DEPOT_PATH")
	if err := os.Setenv("TY_DEPOT_PATH", filepath.Join(dir, "custom-depot")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("TY_DEPOT_PATH", old) })

	env, err := LoadFromLauncher(launcher)
	if err != nil {
		t.Fatalf("LoadFromLauncher() error = %v", err)
	}
	want := filepath.Clean(filepath.Join(dir, "custom-depot"))
	if got := env.Values["JULIA_DEPOT_PATH"]; got != want {
		t.Fatalf("expected JULIA_DEPOT_PATH %q, got %q", want, got)
	}
}

func TestLoadFromLauncherPosixMultiplePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher test not applicable on Windows")
	}
	dir := testutil.TestDir(t)
	launcher := filepath.Join(dir, "julia-ty.sh")
	// Multiple paths: first doesn't exist, second exists
	validDepot := filepath.Join(dir, "valid-depot")
	if err := os.MkdirAll(validDepot, 0o755); err != nil {
		t.Fatal(err)
	}
	invalidDepot := filepath.Join(dir, "nonexistent")
	content := "#!/bin/bash\nscript_dir=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\nexport JULIA_DEPOT_PATH=" + invalidDepot + ":" + validDepot + "\n"
	if err := os.WriteFile(launcher, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	env, err := LoadFromLauncher(launcher)
	if err != nil {
		t.Fatalf("LoadFromLauncher() error = %v", err)
	}
	want := invalidDepot + ":" + validDepot
	if got := env.Values["JULIA_DEPOT_PATH"]; got != want {
		t.Fatalf("expected JULIA_DEPOT_PATH %q, got %q", want, got)
	}
}

func TestLoadFromLauncherWindowsMultiplePaths(t *testing.T) {
	dir := testutil.TestDir(t)
	launcher := filepath.Join(dir, "julia-ty.bat")
	// Multiple paths: first doesn't exist, second exists
	validDepot := filepath.Join(dir, "valid-depot")
	if err := os.MkdirAll(validDepot, 0o755); err != nil {
		t.Fatal(err)
	}
	invalidDepot := filepath.Join(dir, "nonexistent")
	content := "@echo off\nset JULIA_DEPOT_PATH=" + invalidDepot + ";" + validDepot + "\n"
	if err := os.WriteFile(launcher, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	env, err := LoadFromLauncher(launcher)
	if err != nil {
		t.Fatalf("LoadFromLauncher() error = %v", err)
	}
	want := invalidDepot + ";" + validDepot
	if got := env.Values["JULIA_DEPOT_PATH"]; got != want {
		t.Fatalf("expected JULIA_DEPOT_PATH %q, got %q", want, got)
	}
}

func TestFirstExistingDepotPath(t *testing.T) {
	dir := testutil.TestDir(t)
	validDepot := filepath.Join(dir, "valid-depot")
	if err := os.MkdirAll(validDepot, 0o755); err != nil {
		t.Fatal(err)
	}
	invalidDepot := filepath.Join(dir, "nonexistent")

	separator := string(os.PathListSeparator)
	got, ok, err := FirstExistingDepotPath(invalidDepot + separator + validDepot)
	if err != nil {
		t.Fatalf("FirstExistingDepotPath() error = %v", err)
	}
	if !ok {
		t.Fatal("expected depot path to exist")
	}
	if got != validDepot {
		t.Fatalf("expected depot path %q, got %q", validDepot, got)
	}
}
