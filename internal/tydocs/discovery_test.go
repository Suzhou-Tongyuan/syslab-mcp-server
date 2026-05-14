package tydocs

import "testing"

func TestIsTargetPackage(t *testing.T) {
	if !IsTargetPackage("TyMath") {
		t.Fatal("expected TyMath to be a target package")
	}
	if IsTargetPackage("Syslab") {
		t.Fatal("did not expect Syslab to be a target package")
	}
}

func TestShouldIncludePackage(t *testing.T) {
	if shouldIncludePackage("Random", false) {
		t.Fatal("did not expect Random when includeAllPackages=false")
	}
	if !shouldIncludePackage("Random", true) {
		t.Fatal("expected Random when includeAllPackages=true")
	}
}
