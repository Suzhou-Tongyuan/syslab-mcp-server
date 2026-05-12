package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDir(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	base := filepath.Join(wd, ".test-work")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}

	prefix := sanitizeName(t.Name()) + "-"
	dir, err := os.MkdirTemp(base, prefix)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("best-effort cleanup failed for %s: %v", dir, err)
		}
	})

	return dir
}

func sanitizeName(name string) string {
	replacer := strings.NewReplacer("\\", "-", "/", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-")
	return replacer.Replace(name)
}
