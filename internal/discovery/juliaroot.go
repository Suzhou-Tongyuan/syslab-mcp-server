package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syslab-mcp/internal/syslabenv"
)

func ResolveJuliaRoot(configured string, syslabRoot string) (string, error) {
	return resolveJuliaRootForGOOS(runtime.GOOS, configured, syslabRoot)
}

func resolveJuliaRootForGOOS(goos string, configured string, syslabRoot string) (string, error) {
	if path, ok := normalizeExistingDir(configured); ok {
		return path, nil
	}

	if path, ok := juliaRootFromSyslabEnv(); ok {
		return path, nil
	}

	if goos == "windows" {
		return "", fmt.Errorf("could not locate Julia installation root; pass --julia-root or configure JULIA_HOME in syslab-env.ini")
	}

	toolsRoot := filepath.Join(syslabRoot, "Tools")
	if path, ok := juliaRootFromSyslabTools(toolsRoot); ok {
		return path, nil
	}

	return "", fmt.Errorf("could not locate Julia installation root; pass --julia-root, configure JULIA_HOME in syslab-env.ini, or place a julia-* directory under %s", toolsRoot)
}

func juliaRootFromSyslabEnv() (string, bool) {
	env, err := syslabenv.LoadDefaultIfExists()
	if err != nil {
		return "", false
	}
	if env.Values == nil {
		return "", false
	}
	return normalizeExistingDir(strings.TrimSpace(env.Values["JULIA_HOME"]))
}

func juliaRootFromSyslabTools(toolsRoot string) (string, bool) {
	root, ok := normalizeExistingDir(toolsRoot)
	if !ok {
		return "", false
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}

	var candidates []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasPrefix(name, "julia-") {
			continue
		}
		candidates = append(candidates, filepath.Join(root, entry.Name()))
	}

	for _, candidate := range candidates {
		if _, ok := normalizeIfExists(filepath.Join(candidate, "bin", launcherFileName())); ok {
			return candidate, true
		}
	}

	if len(candidates) == 1 {
		if path, ok := normalizeExistingDir(candidates[0]); ok {
			return path, true
		}
	}

	return "", false
}
