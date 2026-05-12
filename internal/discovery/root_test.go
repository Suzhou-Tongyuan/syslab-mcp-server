package discovery

import (
	"strings"
	"testing"

	"syslab-mcp/internal/testutil"
)

func TestResolveSyslabRootConfiguredPath(t *testing.T) {
	dir := testutil.TestDir(t)

	got, err := ResolveSyslabRoot(dir)
	if err != nil {
		t.Fatalf("ResolveSyslabRoot() error = %v", err)
	}
	if got != dir {
		t.Fatalf("expected %s, got %s", dir, got)
	}
}

func TestResolveSyslabRootRequiresConfiguredPath(t *testing.T) {
	got, err := ResolveSyslabRoot("")
	if err == nil {
		t.Fatalf("expected error, got path %q", got)
	}
	if !strings.Contains(err.Error(), "--syslab-root") {
		t.Fatalf("unexpected error: %v", err)
	}
}
