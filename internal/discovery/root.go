package discovery

import (
	"fmt"
	"os"
	"path/filepath"
)

func ResolveSyslabRoot(configured string) (string, error) {
	if path, ok := normalizeExistingDir(configured); ok {
		return path, nil
	}

	return "", fmt.Errorf("could not locate Syslab installation root; pass --syslab-root")
}

func normalizeExistingDir(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return abs, true
}
