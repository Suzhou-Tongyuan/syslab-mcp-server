package main

import (
	"os"
	"path/filepath"
	"strings"
	"syslab-mcp/internal/skills"
	"testing"
)

func TestNormalizeSyslabDisplayMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		mode                string
		hasDefaultSyslabEnv bool
		want                string
	}{
		{name: "empty defaults to desktop", mode: "", hasDefaultSyslabEnv: true, want: "desktop"},
		{name: "desktop preserved when env exists", mode: "desktop", hasDefaultSyslabEnv: true, want: "desktop"},
		{name: "empty falls back when env missing", mode: "", hasDefaultSyslabEnv: false, want: "nodesktop"},
		{name: "desktop falls back when env missing", mode: "desktop", hasDefaultSyslabEnv: false, want: "nodesktop"},
		{name: "nodesktop preserved when env missing", mode: "nodesktop", hasDefaultSyslabEnv: false, want: "nodesktop"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeSyslabDisplayMode(tt.mode, tt.hasDefaultSyslabEnv); got != tt.want {
				t.Fatalf("normalizeSyslabDisplayMode(%q, %v) = %q, want %q", tt.mode, tt.hasDefaultSyslabEnv, got, tt.want)
			}
		})
	}
}

func TestResolveInitialWorkingFolderDefaultsToCurrentDirectory(t *testing.T) {
	t.Parallel()

	want, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}

	if got := resolveInitialWorkingFolder(""); got != want {
		t.Fatalf("resolveInitialWorkingFolder(\"\") = %q, want %q", got, want)
	}
}

func TestResolveInitialWorkingFolderMakesRelativePathAbsolute(t *testing.T) {
	t.Parallel()

	got := resolveInitialWorkingFolder("relative-workdir")
	want, err := filepath.Abs("relative-workdir")
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if got != want {
		t.Fatalf("resolveInitialWorkingFolder(relative-workdir) = %q, want %q", got, want)
	}
}

func TestResolveBridgeScriptMaterializesEmbeddedFile(t *testing.T) {
	t.Parallel()

	got := resolveBridgeScript()
	if got == "" {
		t.Fatal("expected non-empty embedded bridge path")
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("expected embedded bridge file to exist at %q: %v", got, err)
	}
}

func TestBuildInitializeInstructionsIncludesSkillContent(t *testing.T) {
	t.Parallel()

	got, err := buildInitializeInstructions(nil, "# Skill\nAlways prefer Ty.", "/tmp/syslab-skills/SKILL.md")
	if err != nil {
		t.Fatalf("buildInitializeInstructions returned error: %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty initialize instructions")
	}
	for _, needle := range []string{
		"first call read_syslab_skill",
		"then call detect_syslab_toolboxes",
		"Prefer Ty libraries",
		"already installed in the current environment",
		"Loaded built-in Syslab skill from /tmp/syslab-skills/SKILL.md.",
		"Always prefer Ty.",
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("expected initialize instructions to contain %q, got %q", needle, got)
		}
	}
}

func TestResolveSkillsRootDefaultsToSiblingDirectory(t *testing.T) {
	t.Parallel()

	explicit := filepath.Join("D:\\", "custom", "syslab-skills")
	got, err := skills.ResolveRoot(explicit)
	if err != nil {
		t.Fatalf("ResolveRoot returned error for explicit root: %v", err)
	}
	want, err := filepath.Abs(explicit)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if got != want {
		t.Fatalf("ResolveRoot(%q) = %q, want %q", explicit, got, want)
	}
}

func TestResolvePrimarySkillFileUsesExplicitFile(t *testing.T) {
	t.Parallel()

	explicitFile := filepath.Join("D:\\", "custom", "syslab-skills", "SKILL.md")
	got, err := skills.ResolvePrimarySkillFile(explicitFile, "")
	if err != nil {
		t.Fatalf("ResolvePrimarySkillFile returned error: %v", err)
	}
	want, err := filepath.Abs(explicitFile)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if got != want {
		t.Fatalf("ResolvePrimarySkillFile(%q, \"\") = %q, want %q", explicitFile, got, want)
	}
}
