package bridgeasset

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed syslab_bridge.jl
var bridgeFS embed.FS

const embeddedBridgeName = "syslab_bridge.jl"

func ReadEmbedded() ([]byte, error) {
	content, err := bridgeFS.ReadFile(embeddedBridgeName)
	if err != nil {
		return nil, fmt.Errorf("read embedded bridge script: %w", err)
	}
	return content, nil
}

func Materialize() (string, error) {
	content, err := ReadEmbedded()
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(content)
	fileName := fmt.Sprintf("syslab_bridge-%x.jl", sum[:8])

	candidateRoots := make([]string, 0, 2)
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		candidateRoots = append(candidateRoots, filepath.Join(cacheDir, "syslab-mcp", "bridge"))
	}
	if tempDir := os.TempDir(); tempDir != "" {
		candidateRoots = append(candidateRoots, filepath.Join(tempDir, "syslab-mcp", "bridge"))
	}

	var errs []error
	for _, targetDir := range candidateRoots {
		targetPath, writeErr := materializeAt(targetDir, fileName, content)
		if writeErr == nil {
			return targetPath, nil
		}
		errs = append(errs, writeErr)
	}

	return "", fmt.Errorf("write embedded bridge script: %w", errors.Join(errs...))
}

func materializeAt(targetDir, fileName string, content []byte) (string, error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create embedded bridge cache dir %q: %w", targetDir, err)
	}

	targetPath := filepath.Join(targetDir, fileName)
	if existing, err := os.ReadFile(targetPath); err == nil {
		if bytes.Equal(existing, content) {
			return targetPath, nil
		}
	}

	if err := os.WriteFile(targetPath, content, 0o644); err != nil {
		return "", fmt.Errorf("write embedded bridge script %q: %w", targetPath, err)
	}
	return targetPath, nil
}
