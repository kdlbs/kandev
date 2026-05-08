package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// svcWithRoot returns a Service whose discovery roots contain only `root`.
// All ListDirectory tests need this — the production service defaults to
// $HOME, but tests have to operate inside a t.TempDir to stay hermetic.
func svcWithRoot(root string) *Service {
	return &Service{
		discoveryConfig: RepositoryDiscoveryConfig{Roots: []string{root}},
	}
}

func TestListDirectory_ListsImmediateSubdirsOnly(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	// File should not appear in listing.
	if err := os.WriteFile(filepath.Join(root, "ignore-me.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	// Hidden directory should be excluded.
	if err := os.Mkdir(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatalf("mkdir hidden: %v", err)
	}
	// Nested dir should NOT appear (immediate children only).
	if err := os.MkdirAll(filepath.Join(root, "alpha", "deep"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	got, err := svcWithRoot(root).ListDirectory(context.Background(), root)
	if err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}

	want := []string{"alpha", "beta", "gamma"}
	if len(got.Entries) != len(want) {
		t.Fatalf("entries: got %d, want %d (%+v)", len(got.Entries), len(want), got.Entries)
	}
	for i, e := range got.Entries {
		if e.Name != want[i] {
			t.Errorf("entry[%d].Name = %q; want %q", i, e.Name, want[i])
		}
	}
	// At the discovery root: parent should be "" (no escape past the allowed root).
	if got.Parent != "" {
		t.Errorf("expected empty parent at root, got %q", got.Parent)
	}
}

func TestListDirectory_DefaultsToFirstRoot(t *testing.T) {
	root := t.TempDir()
	got, err := svcWithRoot(root).ListDirectory(context.Background(), "")
	if err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}
	if got.Path != filepath.Clean(root) {
		t.Errorf("got Path = %q; want %q", got.Path, root)
	}
}

func TestListDirectory_RejectsNonDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := svcWithRoot(root).ListDirectory(context.Background(), file)
	if err == nil {
		t.Fatalf("expected error for non-directory path, got nil")
	}
}

func TestListDirectory_RejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir() // separate temp dir, not inside root
	_, err := svcWithRoot(root).ListDirectory(context.Background(), outside)
	if !errors.Is(err, ErrPathNotAllowed) {
		t.Fatalf("expected ErrPathNotAllowed for path outside allowed roots, got %v", err)
	}
}

func TestListDirectory_RejectsTraversalEscape(t *testing.T) {
	root := t.TempDir()
	// "../" attempt — should not be able to escape via path traversal.
	traversal := filepath.Join(root, "..")
	_, err := svcWithRoot(root).ListDirectory(context.Background(), traversal)
	if !errors.Is(err, ErrPathNotAllowed) {
		t.Fatalf("expected ErrPathNotAllowed for traversal, got %v", err)
	}
}

func TestListDirectory_ParentSetForNestedPathInsideRoot(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "child")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	got, err := svcWithRoot(root).ListDirectory(context.Background(), nested)
	if err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}
	// Parent is the root — still inside the allowed prefix, so it's exposed.
	if got.Parent != filepath.Clean(root) {
		t.Errorf("got Parent = %q; want %q", got.Parent, root)
	}
}
