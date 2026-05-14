package tools

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"syslab-mcp/internal/config"
	"syslab-mcp/internal/session"
	"syslab-mcp/internal/testutil"
	"syslab-mcp/internal/tydocs"
)

func TestObjectSchema(t *testing.T) {
	s := objectSchema(map[string]any{
		"code": map[string]any{"type": "string"},
	}, []string{"code"})

	if s["type"] != "object" {
		t.Fatalf("unexpected schema type: %v", s["type"])
	}
}

func TestRequiredString(t *testing.T) {
	value, err := requiredString(map[string]any{"code": "println(1)"}, "code")
	if err != nil {
		t.Fatalf("requiredString() error = %v", err)
	}
	if value != "println(1)" {
		t.Fatalf("unexpected value: %s", value)
	}
}

func TestOptionalBool(t *testing.T) {
	value, err := optionalBool(map[string]any{"include_all_packages": true}, "include_all_packages")
	if err != nil {
		t.Fatalf("optionalBool() error = %v", err)
	}
	if !value {
		t.Fatal("expected true")
	}
}

func TestOptionalBoolRejectsNonBoolean(t *testing.T) {
	_, err := optionalBool(map[string]any{"include_all_packages": "true"}, "include_all_packages")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCatalogCallUnknownTool(t *testing.T) {
	c := &Catalog{tools: map[string]Tool{}}
	if _, err := c.Call(context.Background(), "missing_tool", map[string]any{}); err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestDetectSyslabToolboxesSchema(t *testing.T) {
	c := NewCatalog(nil, nil, "", false)
	tool, ok := c.tools["detect_syslab_toolboxes"]
	if !ok {
		t.Fatal("missing detect_syslab_toolboxes tool")
	}
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected properties type: %T", tool.InputSchema["properties"])
	}
	field, ok := properties["include_all_packages"].(map[string]any)
	if !ok {
		t.Fatal("expected include_all_packages property")
	}
	if field["type"] != "boolean" {
		t.Fatalf("unexpected include_all_packages type: %v", field["type"])
	}
}

func TestRestartJuliaSchema(t *testing.T) {
	c := NewCatalog(nil, nil, "", false)
	tool, ok := c.tools["restart_julia"]
	if !ok {
		t.Fatal("missing restart_julia tool")
	}
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected properties type: %T", tool.InputSchema["properties"])
	}
	field, ok := properties["working_directory"].(map[string]any)
	if !ok {
		t.Fatal("expected working_directory property")
	}
	if field["type"] != "string" {
		t.Fatalf("unexpected working_directory type: %v", field["type"])
	}
}

func TestSearchTyDocs(t *testing.T) {
	root := testutil.TestDir(t)
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	juliaRoot := filepath.Join(root, "julia-1.9.3")
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.bat")
	helpDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TyMath", "Doc")
	versionInfoDir := filepath.Join(syslabRoot, "versionInfo")
	envDir := filepath.Join(root, ".julia", "environments", "v1.9")

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
	if err := os.WriteFile(launcherPath, []byte("@echo off\nset JULIA_DEPOT_PATH=%~dp0../../.julia\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "Manifest.toml"), []byte("julia_version = \"1.9.3\"\n\n[[deps.TyMath]]\nversion = \"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionInfoDir, "build_info.json"), []byte(`{"version":"26.2.0.6993"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helpDir, "fft.md"), []byte("# fft\nFast Fourier transform"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCatalog(nil, tydocs.NewCatalog(syslabRoot, launcherPath, "", log.New(io.Discard, "", 0)), "", false)
	out, err := c.Call(context.Background(), "search_syslab_docs", map[string]any{"query": "fft"})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if !strings.Contains(out, "TyMath") || !strings.Contains(out, "fft") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReadSyslabSkill(t *testing.T) {
	root := testutil.TestDir(t)
	skillPath := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("# Sample Skill\n\nAlways inspect the current environment first.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCatalog(nil, nil, skillPath, false)
	out, err := c.Call(context.Background(), "read_syslab_skill", map[string]any{})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	var payload struct {
		Tool      string `json:"tool"`
		SkillPath string `json:"skill_path"`
		Loaded    bool   `json:"loaded"`
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; out=%s", err, out)
	}
	if payload.Tool != "read_syslab_skill" {
		t.Fatalf("unexpected tool: %q", payload.Tool)
	}
	if payload.SkillPath != skillPath {
		t.Fatalf("unexpected skill path: %q", payload.SkillPath)
	}
	if !payload.Loaded {
		t.Fatal("expected loaded=true")
	}
	if !strings.Contains(payload.Content, "Always inspect the current environment first.") {
		t.Fatalf("unexpected content: %q", payload.Content)
	}
	if payload.Truncated {
		t.Fatal("did not expect truncated content")
	}
}

func TestReadSyslabSkillWithExplicitPath(t *testing.T) {
	root := testutil.TestDir(t)
	defaultSkillPath := filepath.Join(root, "default.md")
	overrideSkillPath := filepath.Join(root, "override.md")
	if err := os.WriteFile(defaultSkillPath, []byte("# Default Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overrideSkillPath, []byte("# Override Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCatalog(nil, nil, defaultSkillPath, false)
	out, err := c.Call(context.Background(), "read_syslab_skill", map[string]any{
		"skill_path": overrideSkillPath,
	})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	var payload struct {
		SkillPath string `json:"skill_path"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; out=%s", err, out)
	}
	if payload.SkillPath != overrideSkillPath {
		t.Fatalf("unexpected skill path: %q", payload.SkillPath)
	}
	if !strings.Contains(payload.Content, "Override Skill") {
		t.Fatalf("unexpected content: %q", payload.Content)
	}
}

func TestReadSyslabSkillWithDefaultSentinel(t *testing.T) {
	root := testutil.TestDir(t)
	skillPath := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("# Sample Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCatalog(nil, nil, skillPath, false)
	out, err := c.Call(context.Background(), "read_syslab_skill", map[string]any{
		"skill_path": "default",
	})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if !strings.Contains(out, `"tool": "read_syslab_skill"`) {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResolveMatlabSymbols(t *testing.T) {
	root := testutil.TestDir(t)
	home := filepath.Join(root, "home")
	syslabRoot := filepath.Join(root, "Syslab 2026a")
	juliaRoot := filepath.Join(root, "julia-1.10.10")
	launcherPath := filepath.Join(juliaRoot, "bin", "julia-ty.bat")
	versionInfoDir := filepath.Join(syslabRoot, "versionInfo")
	depotDir := filepath.Join(root, ".julia")
	envDir := filepath.Join(depotDir, "environments", "v1.10")
	tyBaseDocDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "syslabHelpSourceCode", "projects", "TyBase", "Doc", "TyBase", "InteractiveCommands")
	functionTableDir := filepath.Join(syslabRoot, "Tools", "AIAssets", "SearchCenter", "static", "FunctionTable")

	for _, dir := range []string{versionInfoDir, envDir, filepath.Dir(launcherPath), tyBaseDocDir, functionTableDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(launcherPath, []byte("@echo off\nset JULIA_DEPOT_PATH=%~dp0../../.julia\n"), 0o644); err != nil {
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

	c := NewCatalog(nil, tydocs.NewCatalog(syslabRoot, launcherPath, "", log.New(io.Discard, "", 0)), "", false)
	out, err := c.Call(context.Background(), "map_matlab_functions_to_julia", map[string]any{
		"symbols": []any{"format", "missing"},
	})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if !strings.Contains(out, `"matlab_symbol": "format"`) || !strings.Contains(out, `"syslab_symbol": "ty_format"`) {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, `"unresolved": [`) || !strings.Contains(out, `"missing"`) {
		t.Fatalf("unexpected unresolved output: %s", out)
	}
}

func TestParseDetectEnvironmentOutput(t *testing.T) {
	metadata := parseDetectEnvironmentOutput("julia_version: 1.9.3\nbindir: C:\\julia\\bin\n")
	if metadata["julia_version"] != "1.9.3" {
		t.Fatalf("unexpected julia version: %q", metadata["julia_version"])
	}
	if metadata["bindir"] != "C:\\julia\\bin" {
		t.Fatalf("unexpected bindir: %q", metadata["bindir"])
	}
}

func TestDetectSyslabToolboxesResolvesRuntimeConfigBeforeDiscovery(t *testing.T) {
	root := testutil.TestDir(t)
	juliaRoot := filepath.Join(root, "julia-1.10.10")
	launcherName := "julia-ty.sh"
	launcherBody := "#!/bin/sh\n"
	if runtime.GOOS == "windows" {
		launcherName = "julia-ty.bat"
		launcherBody = "@echo off\r\n"
	}
	launcherPath := filepath.Join(juliaRoot, "bin", launcherName)
	if err := os.MkdirAll(filepath.Dir(launcherPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcherPath, []byte(launcherBody), 0o755); err != nil {
		t.Fatal(err)
	}

	syslabEnvDir := filepath.Join(root, ".syslab")
	if err := os.MkdirAll(syslabEnvDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(syslabEnvDir, "syslab-env.ini"), []byte("[Syslab]\nJULIA_HOME="+filepath.ToSlash(juliaRoot)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	versionDir := filepath.Join(root, "versionInfo")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "build_info.json"), []byte(`{"version":"2026a"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	if err := os.Setenv("HOME", root); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("USERPROFILE", root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
	})

	sess := session.NewManager(config.Config{SyslabRoot: root}, log.New(io.Discard, "", 0))
	_, err := detectSyslabToolboxes(context.Background(), sess, tydocs.NewCatalog(root, "", "", log.New(io.Discard, "", 0)), false)
	if err == nil || !strings.Contains(err.Error(), "JULIA_DEPOT_PATH") {
		t.Fatalf("expected downstream depot-path error after runtime config resolution, got %v", err)
	}
	if got := sess.LauncherPath(); got != launcherPath {
		t.Fatalf("expected resolved launcher path %q, got %q", launcherPath, got)
	}
}

func TestEvaluateJuliaCodeRequiresEnvironmentInspectionWhenPolicyEnabled(t *testing.T) {
	root := testutil.TestDir(t)
	skillPath := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("# Sample Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCatalog(nil, nil, skillPath, true)
	if _, err := c.Call(context.Background(), "read_syslab_skill", map[string]any{}); err != nil {
		t.Fatalf("read_syslab_skill error = %v", err)
	}

	_, err := c.Call(context.Background(), "evaluate_julia_code", map[string]any{"code": "1+1"})
	if err == nil {
		t.Fatal("expected policy enforcement error")
	}
	if !strings.Contains(err.Error(), "read_syslab_skill") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "detect_syslab_toolboxes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectSyslabToolboxesRequiresSkillInspectionWhenPolicyEnabled(t *testing.T) {
	c := NewCatalog(nil, nil, "", true)

	_, err := c.Call(context.Background(), "detect_syslab_toolboxes", map[string]any{})
	if err == nil {
		t.Fatal("expected policy enforcement error")
	}
	if !strings.Contains(err.Error(), "read_syslab_skill") {
		t.Fatalf("unexpected error: %v", err)
	}
}
