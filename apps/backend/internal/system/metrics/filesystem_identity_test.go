package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemIdentityUsesTheContainingFilesystem(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	rootIdentity, err := FilesystemIdentity(root)
	if err != nil {
		t.Fatalf("FilesystemIdentity(root): %v", err)
	}
	nestedIdentity, err := FilesystemIdentity(nested)
	if err != nil {
		t.Fatalf("FilesystemIdentity(nested): %v", err)
	}
	if rootIdentity == "" || rootIdentity != nestedIdentity {
		t.Fatalf("identities root=%q nested=%q, want the same non-empty filesystem", rootIdentity, nestedIdentity)
	}
}

func TestFilesystemIdentityReportsMissingPath(t *testing.T) {
	if _, err := FilesystemIdentity(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("FilesystemIdentity(missing) error = nil, want an error")
	}
}
