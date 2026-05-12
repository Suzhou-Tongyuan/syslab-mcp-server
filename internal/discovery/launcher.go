package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func ResolveSyslabLauncher(configured string, juliaRoot string) (string, error) {
	if path, ok := normalizeIfExists(configured); ok {
		return path, nil
	}

	if path, ok := launcherFromJuliaRoot(juliaRoot); ok {
		return path, nil
	}

	return "", fmt.Errorf("could not locate %s; pass --syslab-launcher or --julia-root", launcherFileName())
}

func launcherFromJuliaRoot(root string) (string, bool) {
	if root == "" {
		return "", false
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return normalizeIfExists(filepath.Join(abs, "bin", launcherFileName()))
}

func launcherFileName() string {
	if runtime.GOOS == "windows" {
		return "julia-ty.bat"
	}
	return "julia-ty.sh"
}

func normalizeIfExists(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return "", false
	}
	return abs, true
}
