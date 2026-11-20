package constant

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddResourcePath(t *testing.T) {
	resourceDir := t.TempDir()
	resourceName := "runtime-resource-test"
	resourceFile := filepath.Join(resourceDir, resourceName)
	if err := os.WriteFile(resourceFile, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	AddResourcePath(resourceDir)
	AddResourcePath(resourceDir)

	foundPath, found := FindPath(resourceName)
	if !found {
		t.Fatal("resource registered at runtime was not found")
	}
	if foundPath != resourceFile {
		t.Fatalf("unexpected resource path: got %q, want %q", foundPath, resourceFile)
	}
}
