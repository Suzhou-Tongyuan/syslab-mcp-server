package tydocs

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"syslab-mcp/internal/testutil"
)

func TestCatalogSearchAndRead(t *testing.T) {
	root := testutil.TestDir(t)
	home := filepath.Join(root, "home")
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	juliaRoot := filepath.Join(root, "julia-1.9.3")
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.bat")
	helpDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TySignalProcessing", "Doc", "TySignalProcessing")
	versionInfoDir := filepath.Join(syslabRoot, "versionInfo")
	depotDir := filepath.Join(root, ".julia")
	envDir := filepath.Join(depotDir, "environments", "v1.9")
	syslabEnvDir := filepath.Join(home, ".syslab")

	if err := os.MkdirAll(helpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(versionInfoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(launcherPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(syslabEnvDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcherPath, []byte("@echo off\nset JULIA_DEPOT_PATH=%~dp0../../.julia\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envBody := "[Syslab]\nJULIA_DEPOT_PATH=" + filepath.ToSlash(depotDir) + "\n"
	if err := os.WriteFile(filepath.Join(syslabEnvDir, "syslab-env.ini"), []byte(envBody), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestBody := "julia_version = \"1.9.3\"\n\n[[deps.TySignalProcessing]]\nuuid = \"1\"\nversion = \"1.0.0\"\n"
	if err := os.WriteFile(filepath.Join(envDir, "Manifest.toml"), []byte(manifestBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionInfoDir, "build_info.json"), []byte(`{"version":"26.2.0.6993"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	docPath := filepath.Join(helpDir, "hampel.md")
	docBody := "# hampel\n\nDetect and replace outliers with a moving median window.\n"
	if err := os.WriteFile(docPath, []byte(docBody), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("USERPROFILE")
	if err := os.Setenv("USERPROFILE", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("USERPROFILE", oldHome) })

	catalog := NewCatalog(syslabRoot, launcherPath, "", log.New(io.Discard, "", 0))
	result, err := catalog.Search("hampel outlier", "", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.Packages) != 1 || !result.Packages[0].HasDocs {
		t.Fatalf("unexpected packages: %+v", result.Packages)
	}
	if len(result.Matches) != 1 || result.Matches[0].Symbol != "hampel" {
		t.Fatalf("unexpected matches: %+v", result.Matches)
	}

	doc, err := catalog.Read(docPath)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !strings.Contains(strings.ToLower(doc.Content), "outliers") {
		t.Fatalf("unexpected content: %s", doc.Content)
	}
}

func TestCatalogSearchAndReadWithoutSyslabEnvIni(t *testing.T) {
	root := testutil.TestDir(t)
	home := filepath.Join(root, "home")
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	juliaRoot := filepath.Join(root, "julia-1.10.10")
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.bat")
	helpDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TySignalProcessing", "Doc", "TySignalProcessing")
	versionInfoDir := filepath.Join(syslabRoot, "versionInfo")
	depotDir := filepath.Join(root, "custom-julia-depot")
	envDir := filepath.Join(depotDir, "environments", "v1.10")

	if err := os.MkdirAll(helpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(versionInfoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(launcherPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcherPath, []byte("@echo off\nset JULIA_DEPOT_PATH=%~dp0../../custom-julia-depot\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestBody := "julia_version = \"1.10.10\"\n\n[[deps.TySignalProcessing]]\nuuid = \"1\"\nversion = \"1.0.0\"\n"
	if err := os.WriteFile(filepath.Join(envDir, "Manifest.toml"), []byte(manifestBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionInfoDir, "build_info.json"), []byte(`{"version":"26.2.0.6993"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	docPath := filepath.Join(helpDir, "hampel.md")
	docBody := "# hampel\n\nDetect and replace outliers with a moving median window.\n"
	if err := os.WriteFile(docPath, []byte(docBody), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("USERPROFILE")
	if err := os.Setenv("USERPROFILE", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("USERPROFILE", oldHome) })

	catalog := NewCatalog(syslabRoot, launcherPath, "", log.New(io.Discard, "", 0))
	result, err := catalog.Search("hampel outlier", "", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.Packages) != 1 || !result.Packages[0].HasDocs {
		t.Fatalf("unexpected packages: %+v", result.Packages)
	}
}

func TestCatalogSearchAndReadWithPosixLauncherMultiDepot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher test not applicable on Windows")
	}

	root := testutil.TestDir(t)
	home := filepath.Join(root, "home")
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	juliaRoot := filepath.Join(root, "julia-1.10.10")
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.sh")
	helpDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TySignalProcessing", "Doc", "TySignalProcessing")
	versionInfoDir := filepath.Join(syslabRoot, "versionInfo")
	validDepotDir := filepath.Join(root, "custom-julia-depot")
	envDir := filepath.Join(validDepotDir, "environments", "v1.10")
	invalidDepotDir := filepath.Join(root, "missing-depot")

	for _, dir := range []string{helpDir, versionInfoDir, envDir, filepath.Dir(launcherPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	launcherBody := "#!/bin/bash\nscript_dir=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\nexport JULIA_DEPOT_PATH=" + invalidDepotDir + ":" + validDepotDir + "\n"
	if err := os.WriteFile(launcherPath, []byte(launcherBody), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestBody := "julia_version = \"1.10.10\"\n\n[[deps.TySignalProcessing]]\nuuid = \"1\"\nversion = \"1.0.0\"\n"
	if err := os.WriteFile(filepath.Join(envDir, "Manifest.toml"), []byte(manifestBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionInfoDir, "build_info.json"), []byte(`{"version":"26.2.0.6993"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(helpDir, "hampel.md")
	docBody := "# hampel\n\nDetect and replace outliers with a moving median window.\n"
	if err := os.WriteFile(docPath, []byte(docBody), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })

	catalog := NewCatalog(syslabRoot, launcherPath, "", log.New(io.Discard, "", 0))
	result, err := catalog.Search("hampel outlier", "", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.Packages) != 1 || !result.Packages[0].HasDocs {
		t.Fatalf("unexpected packages: %+v", result.Packages)
	}
}

func TestCatalogSearchAndReadFallsBackToCloudDepot(t *testing.T) {
	root := testutil.TestDir(t)
	home := filepath.Join(root, "home")
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	juliaRoot := filepath.Join(root, "julia-1.10.10")
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.bat")
	helpDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TySignalProcessing", "Doc", "TySignalProcessing")
	versionInfoDir := filepath.Join(syslabRoot, "versionInfo")
	depotDir := filepath.Join(home, "syslabuserdata", "SyslabCloud", ".julia")
	envDir := filepath.Join(depotDir, "environments", "v1.10")

	if err := os.MkdirAll(helpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(versionInfoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(launcherPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcherPath, []byte("@echo off\nREM no depot here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestBody := "julia_version = \"1.10.10\"\n\n[[deps.TySignalProcessing]]\nuuid = \"1\"\nversion = \"1.0.0\"\n"
	if err := os.WriteFile(filepath.Join(envDir, "Manifest.toml"), []byte(manifestBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionInfoDir, "build_info.json"), []byte(`{"version":"26.2.0.6993"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(helpDir, "hampel.md")
	docBody := "# hampel\n\nDetect and replace outliers with a moving median window.\n"
	if err := os.WriteFile(docPath, []byte(docBody), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("USERPROFILE")
	if err := os.Setenv("USERPROFILE", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("USERPROFILE", oldHome) })

	catalog := NewCatalog(syslabRoot, launcherPath, "", log.New(io.Discard, "", 0))
	result, err := catalog.Search("hampel outlier", "", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.Packages) != 1 || !result.Packages[0].HasDocs {
		t.Fatalf("unexpected packages: %+v", result.Packages)
	}
}

func TestCatalogUsesConfiguredHelpDocsRoot(t *testing.T) {
	root := testutil.TestDir(t)
	home := filepath.Join(root, "home")
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	customHelpRoot := filepath.Join(root, "custom-help-projects")
	juliaRoot := filepath.Join(root, "julia-1.10.10")
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.bat")
	helpDir := filepath.Join(customHelpRoot, "TySignalProcessing", "Doc", "TySignalProcessing")
	versionInfoDir := filepath.Join(syslabRoot, "versionInfo")
	depotDir := filepath.Join(root, "custom-julia-depot")
	envDir := filepath.Join(depotDir, "environments", "v1.10")

	if err := os.MkdirAll(helpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(versionInfoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(launcherPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcherPath, []byte("@echo off\nset JULIA_DEPOT_PATH=%~dp0../../custom-julia-depot\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestBody := "julia_version = \"1.10.10\"\n\n[[deps.TySignalProcessing]]\nuuid = \"1\"\nversion = \"1.0.0\"\n"
	if err := os.WriteFile(filepath.Join(envDir, "Manifest.toml"), []byte(manifestBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionInfoDir, "build_info.json"), []byte(`{"version":"26.2.0.6993"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(helpDir, "hampel.md")
	docBody := "# hampel\n\nDetect and replace outliers with a moving median window.\n"
	if err := os.WriteFile(docPath, []byte(docBody), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("USERPROFILE")
	if err := os.Setenv("USERPROFILE", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("USERPROFILE", oldHome) })

	catalog := NewCatalog(syslabRoot, launcherPath, customHelpRoot, log.New(io.Discard, "", 0))
	result, err := catalog.Search("hampel outlier", "", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.Packages) != 1 || result.Packages[0].DocsSource != "configured_helpdocs" {
		t.Fatalf("unexpected packages: %+v", result.Packages)
	}
}

func TestCatalogLoadsPersistedIndexFromAIAssets(t *testing.T) {
	root := testutil.TestDir(t)
	home := filepath.Join(root, "home")
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	juliaRoot := filepath.Join(root, "julia-1.10.10")
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.bat")
	helpDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TySignalProcessing", "Doc", "TySignalProcessing")
	versionInfoDir := filepath.Join(syslabRoot, "versionInfo")
	depotDir := filepath.Join(root, "custom-julia-depot")
	envDir := filepath.Join(depotDir, "environments", "v1.10")

	if err := os.MkdirAll(helpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(versionInfoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(launcherPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcherPath, []byte("@echo off\nset JULIA_DEPOT_PATH=%~dp0../../custom-julia-depot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestBody := "julia_version = \"1.10.10\"\n\n[[deps.TySignalProcessing]]\nuuid = \"1\"\nversion = \"1.0.0\"\n"
	if err := os.WriteFile(filepath.Join(envDir, "Manifest.toml"), []byte(manifestBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionInfoDir, "build_info.json"), []byte(`{"version":"26.2.0.6993"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(helpDir, "hampel.md")
	docBody := "# hampel\n\nDetect and replace outliers with a moving median window.\n"
	if err := os.WriteFile(docPath, []byte(docBody), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("USERPROFILE")
	if err := os.Setenv("USERPROFILE", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("USERPROFILE", oldHome) })

	firstCatalog := NewCatalog(syslabRoot, launcherPath, "", log.New(io.Discard, "", 0))
	firstResult, err := firstCatalog.Search("hampel outlier", "", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(firstResult.Matches) != 1 {
		t.Fatalf("unexpected matches: %+v", firstResult.Matches)
	}

	if err := os.WriteFile(filepath.Join(envDir, "Manifest.toml"), []byte("julia_version = \"1.10.10\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("# unrelated\n\nThis document no longer mentions hampel.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	secondCatalog := NewCatalog(syslabRoot, launcherPath, "", log.New(io.Discard, "", 0))
	secondResult, err := secondCatalog.Search("hampel outlier", "", 5)
	if err != nil {
		t.Fatalf("Search() with persisted index error = %v", err)
	}
	if len(secondResult.Matches) != 1 || secondResult.Matches[0].Symbol != "hampel" {
		t.Fatalf("unexpected persisted-index matches: %+v", secondResult.Matches)
	}
}

func TestCatalogIndexesProjectDocsAndFunctionMappings(t *testing.T) {
	root := testutil.TestDir(t)
	home := filepath.Join(root, "home")
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	juliaRoot := filepath.Join(root, "julia-1.10.10")
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.bat")
	versionInfoDir := filepath.Join(syslabRoot, "versionInfo")
	depotDir := filepath.Join(root, "custom-julia-depot")
	envDir := filepath.Join(depotDir, "environments", "v1.10")
	tyBaseDocDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TyBase", "Doc", "TyBase", "InteractiveCommands")
	mlangDocDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "MultiLanguage", "Doc", "MultiLanguage", "TyMLang")
	functionTableDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "SearchCenter", "static", "FunctionTable")

	for _, dir := range []string{versionInfoDir, envDir, filepath.Dir(launcherPath), tyBaseDocDir, mlangDocDir, functionTableDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(launcherPath, []byte("@echo off\nset JULIA_DEPOT_PATH=%~dp0../../custom-julia-depot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "Manifest.toml"), []byte("julia_version = \"1.10.10\"\n\n[[deps.TySignalProcessing]]\nversion = \"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionInfoDir, "build_info.json"), []byte(`{"version":"26.2.0.6993"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tyBaseDocDir, "ty_format.md"), []byte("# ty_format\n\nSet the command window display format.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mlangDocDir, "Mcall.md"), []byte("# Mcall\n\nCall M functions from Julia code.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mapping := `[
  {
    "package": "TyBase",
    "name": "ty_format",
    "description": "Set the command window display format.",
    "helpUrl": "/Doc/TyBase/InteractiveCommands/ty_format.html",
    "kind": "Interactive Commands",
    "matlabFunction": "format"
  }
]`
	if err := os.WriteFile(filepath.Join(functionTableDir, "函数映射表.json"), []byte(mapping), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("USERPROFILE")
	if err := os.Setenv("USERPROFILE", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("USERPROFILE", oldHome) })

	catalog := NewCatalog(syslabRoot, launcherPath, "", log.New(io.Discard, "", 0))
	mapped, err := catalog.Search("format", "TyBase", 5)
	if err != nil {
		t.Fatalf("Search() with function mapping error = %v", err)
	}
	if len(mapped.Matches) == 0 || mapped.Matches[0].Symbol != "ty_format" {
		t.Fatalf("unexpected mapped matches: %+v", mapped.Matches)
	}

	mlang, err := catalog.Search("Call M functions", "MultiLanguage", 5)
	if err != nil {
		t.Fatalf("Search() for M docs error = %v", err)
	}
	if len(mlang.Matches) == 0 || !strings.EqualFold(mlang.Matches[0].Symbol, "Mcall") {
		t.Fatalf("unexpected M docs matches: %+v", mlang.Matches)
	}
}

func TestCatalogBuildsFromHelpDocsOnly(t *testing.T) {
	root := testutil.TestDir(t)
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	helpDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TySignalProcessing", "Doc", "TySignalProcessing")

	if err := os.MkdirAll(helpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helpDir, "hampel.md"), []byte("# hampel\n\nDetect and replace outliers with a moving median window.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog := NewCatalog(syslabRoot, "", "", log.New(io.Discard, "", 0))
	result, err := catalog.Search("hampel outlier", "", 5)
	if err != nil {
		t.Fatalf("Search() without launcher error = %v", err)
	}
	if len(result.Packages) != 1 || result.Packages[0].Version != "" {
		t.Fatalf("unexpected packages from docs-only build: %+v", result.Packages)
	}
	if len(result.Matches) != 1 || result.Matches[0].Version != "" {
		t.Fatalf("unexpected matches from docs-only build: %+v", result.Matches)
	}
}

func TestResolveMatlabSymbols(t *testing.T) {
	root := testutil.TestDir(t)
	home := filepath.Join(root, "home")
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	juliaRoot := filepath.Join(root, "julia-1.10.10")
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.bat")
	versionInfoDir := filepath.Join(syslabRoot, "versionInfo")
	depotDir := filepath.Join(root, "custom-julia-depot")
	envDir := filepath.Join(depotDir, "environments", "v1.10")
	tyBaseDocDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TyBase", "Doc", "TyBase", "InteractiveCommands")
	tyMathDocDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TyMath", "Doc", "TyMath")
	functionTableDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "SearchCenter", "static", "FunctionTable")

	for _, dir := range []string{versionInfoDir, envDir, filepath.Dir(launcherPath), tyBaseDocDir, tyMathDocDir, functionTableDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(launcherPath, []byte("@echo off\nset JULIA_DEPOT_PATH=%~dp0../../custom-julia-depot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "Manifest.toml"), []byte("julia_version = \"1.10.10\"\n\n[[deps.TyBase]]\nversion = \"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionInfoDir, "build_info.json"), []byte(`{"version":"26.2.0.6993"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tyBaseDocDir, "ty_format.md"), []byte("# ty_format\n\nSet the command window display format.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tyMathDocDir, "size.md"), []byte("# size\n\nReturn array dimensions.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mapping := `[
  {
    "package": "TyBase",
    "name": "ty_format",
    "description": "Set the command window display format.",
    "helpUrl": "/Doc/TyBase/InteractiveCommands/ty_format.html",
    "kind": "Interactive Commands",
    "matlabFunction": "format"
  }
]`
	if err := os.WriteFile(filepath.Join(functionTableDir, "函数映射表.json"), []byte(mapping), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("USERPROFILE")
	if err := os.Setenv("USERPROFILE", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("USERPROFILE", oldHome) })

	catalog := NewCatalog(syslabRoot, launcherPath, "", log.New(io.Discard, "", 0))
	result, err := catalog.ResolveMatlabSymbols([]string{"format", "size", "missing", "format"}, 2)
	if err != nil {
		t.Fatalf("ResolveMatlabSymbols() error = %v", err)
	}
	if len(result.Resolved) != 2 {
		t.Fatalf("expected 2 resolved entries, got %+v", result.Resolved)
	}
	if len(result.Unresolved) != 1 || result.Unresolved[0] != "missing" {
		t.Fatalf("unexpected unresolved entries: %+v", result.Unresolved)
	}

	if result.Resolved[0].MatlabSymbol != "format" {
		t.Fatalf("expected first resolved symbol to be format, got %+v", result.Resolved[0])
	}
	if len(result.Resolved[0].Candidates) == 0 || result.Resolved[0].Candidates[0].SyslabSymbol != "ty_format" {
		t.Fatalf("unexpected format candidates: %+v", result.Resolved[0].Candidates)
	}
	if strings.TrimSpace(result.Resolved[0].Candidates[0].Source) == "" {
		t.Fatalf("expected non-empty candidate source, got %+v", result.Resolved[0].Candidates[0])
	}

	if result.Resolved[1].MatlabSymbol != "size" {
		t.Fatalf("expected second resolved symbol to be size, got %+v", result.Resolved[1])
	}
	if len(result.Resolved[1].Candidates) == 0 || result.Resolved[1].Candidates[0].SyslabSymbol != "size" {
		t.Fatalf("unexpected size candidates: %+v", result.Resolved[1].Candidates)
	}
}

func TestBuildAndWriteIndexFromAIAssets(t *testing.T) {
	root := testutil.TestDir(t)
	aiAssetsRoot := filepath.Join(root, "AIAssets")
	helpDir := filepath.Join(aiAssetsRoot, "syslabHelpSourceCode", "projects", "TySignalProcessing", "Doc", "TySignalProcessing")
	functionTableDir := filepath.Join(aiAssetsRoot, "SearchCenter", "static", "FunctionTable")
	outputPath := filepath.Join(root, "out", PersistedIndexFilename())

	for _, dir := range []string{helpDir, functionTableDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(helpDir, "hampel.md"), []byte("# hampel\n\nDetect and replace outliers with a moving median window.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mapping := `[
  {
    "package": "TySignalProcessing",
    "name": "hampel",
    "description": "Detect and replace outliers with a moving median window.",
    "helpUrl": "/Doc/TySignalProcessing/hampel.html",
    "kind": "Signal Processing",
    "matlabFunction": "hampel"
  }
]`
	if err := os.WriteFile(filepath.Join(functionTableDir, "函数映射表.json"), []byte(mapping), 0o644); err != nil {
		t.Fatal(err)
	}

	path, index, err := BuildAndWriteIndexFromAIAssets(aiAssetsRoot, outputPath, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("BuildAndWriteIndexFromAIAssets() error = %v", err)
	}
	if path != outputPath {
		t.Fatalf("unexpected output path: %s", path)
	}
	if len(index.Packages) != 1 || len(index.Entries) != 1 {
		t.Fatalf("unexpected index: %+v", index)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	entries, ok := persisted["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("unexpected persisted entries: %#v", persisted["entries"])
	}
	entry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected persisted entry type: %#v", entries[0])
	}
	pathValue, _ := entry["path"].(string)
	if pathValue == "" || filepath.IsAbs(pathValue) || strings.Contains(pathValue, `\`) {
		t.Fatalf("expected relative slash path in persisted index, got %q", pathValue)
	}
}

func TestCatalogLoadsRelativePersistedIndexPaths(t *testing.T) {
	root := testutil.TestDir(t)
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	aiAssetsRoot := filepath.Join(syslabRoot, "Tools", "AIAssets")
	docDir := filepath.Join(aiAssetsRoot, "projects", "TySignalProcessing", "Doc", "TySignalProcessing")
	docPath := filepath.Join(docDir, "hampel.md")
	indexPath := filepath.Join(aiAssetsRoot, PersistedIndexFilename())

	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("# hampel\n\nDetect and replace outliers with a moving median window.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{
		"version":"v1",
		"packages":[{"name":"TySignalProcessing","docs_path":"projects/TySignalProcessing/Doc","docs_source":"syslab_aiassets","has_docs":true}],
		"entries":[{"package":"TySignalProcessing","title":"hampel","symbol":"hampel","summary":"Detect and replace outliers with a moving median window.","path":"projects/TySignalProcessing/Doc/TySignalProcessing/hampel.md","format":"md","source":"syslab_aiassets"}]
	}`
	if err := os.WriteFile(indexPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog := NewCatalog(syslabRoot, "", "", log.New(io.Discard, "", 0))
	result, err := catalog.Search("hampel outlier", "", 5)
	if err != nil {
		t.Fatalf("Search() with relative persisted index error = %v", err)
	}
	if len(result.Matches) != 1 || result.Matches[0].Path != docPath {
		t.Fatalf("unexpected matches from relative persisted index: %+v", result.Matches)
	}

	doc, err := catalog.Read(docPath)
	if err != nil {
		t.Fatalf("Read() with relative persisted index error = %v", err)
	}
	if !strings.Contains(strings.ToLower(doc.Content), "outliers") {
		t.Fatalf("unexpected content: %s", doc.Content)
	}
}

func TestCatalogRejectsUnexpectedPersistedIndexVersion(t *testing.T) {
	root := testutil.TestDir(t)
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	aiAssetsRoot := filepath.Join(syslabRoot, "Tools", "AIAssets")

	if err := os.MkdirAll(aiAssetsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"version":"v2","packages":[{"name":"TyBase","has_docs":true}],"entries":[]}`
	if err := os.WriteFile(filepath.Join(aiAssetsRoot, PersistedIndexFilename()), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog := NewCatalog(syslabRoot, "", "", log.New(io.Discard, "", 0))
	if err := catalog.Warmup(); err != nil {
		t.Fatalf("Warmup() should degrade gracefully, got %v", err)
	}
	packages, withDocs := catalog.Stats()
	if packages != 0 || withDocs != 0 {
		t.Fatalf("unexpected cached stats after rejected version: packages=%d withDocs=%d", packages, withDocs)
	}
}
