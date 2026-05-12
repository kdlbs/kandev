package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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

	svc := &Service{}
	got, err := svc.ListDirectory(context.Background(), root)
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
	// A t.TempDir is not the filesystem root, so the parent should be set.
	if got.Parent == "" {
		t.Errorf("expected parent to be set for nested path, got empty")
	}
}

func TestListDirectory_DefaultsToHome(t *testing.T) {
	svc := &Service{}
	got, err := svc.ListDirectory(context.Background(), "")
	if err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}
	home, _ := os.UserHomeDir()
	if got.Path != filepath.Clean(home) {
		t.Errorf("got Path = %q; want home %q", got.Path, home)
	}
}

func TestListDirectory_RejectsNonDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	svc := &Service{}
	_, err := svc.ListDirectory(context.Background(), file)
	if err == nil {
		t.Fatalf("expected error for non-directory path, got nil")
	}
}

func TestListDirectory_BrowsesOutsideHome(t *testing.T) {
	// The picker deliberately lets users browse any directory the kandev
	// process has read access to, not just $HOME or the discoveryRoots.
	// /tmp is the canonical "outside $HOME but accessible" path on most
	// Unix CI runners; assert we can list it without error.
	svc := &Service{}
	got, err := svc.ListDirectory(context.Background(), "/tmp")
	if err != nil {
		t.Fatalf("ListDirectory(/tmp): %v", err)
	}
	if got.Path != "/tmp" {
		t.Errorf("got Path = %q; want /tmp", got.Path)
	}
}

func TestListDirectory_ParentEmptyAtFilesystemRoot(t *testing.T) {
	svc := &Service{}
	got, err := svc.ListDirectory(context.Background(), "/")
	if err != nil {
		t.Fatalf("ListDirectory(/): %v", err)
	}
	if got.Parent != "" {
		t.Errorf("expected empty parent at /, got %q", got.Parent)
	}
}
