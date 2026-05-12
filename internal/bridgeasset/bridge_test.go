package bridgeasset

import (
	"bytes"
	"os"
	"testing"
)

func TestMaterializeMatchesEmbeddedBridgeScript(t *testing.T) {
	t.Parallel()

	want, err := ReadEmbedded()
	if err != nil {
		t.Fatalf("read embedded bridge: %v", err)
	}

	materializedPath, err := Materialize()
	if err != nil {
		t.Fatalf("materialize embedded bridge: %v", err)
	}

	got, err := os.ReadFile(materializedPath)
	if err != nil {
		t.Fatalf("read materialized bridge: %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatal("materialized bridge script does not match embedded bridge content")
	}
}
