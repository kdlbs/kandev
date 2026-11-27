package journal

import (
	"path/filepath"
	"testing"
)

func TestResolveLocationIsStableAndIndependentOfWorktree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "retained")
	first, err := ResolveLocation(root, "environment-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveLocation(root, "environment-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != second.Path || filepath.Base(filepath.Dir(first.Path)) != "environment-1" {
		t.Fatalf("locations differ: %#v %#v", first, second)
	}
	if filepath.Dir(first.Path) == filepath.Join(root, "worktree") {
		t.Fatal("journal path is worktree-scoped")
	}
}

func TestCheckStorageDoesNotAdvertiseInvalidOrMissingRoot(t *testing.T) {
	capability := CheckStorage("relative-root", "owner")
	if capability.Durable || capability.Reason != "storage_not_durable" {
		t.Fatalf("invalid root capability = %#v", capability)
	}
	capability = CheckStorage(filepath.Join(t.TempDir(), "root"), "owner")
	if !capability.Durable || capability.Version != CurrentVersion {
		t.Fatalf("retained root capability = %#v", capability)
	}
	root := filepath.Join(t.TempDir(), "root")
	if capability := CheckStorage(root, "owner"); !capability.Durable {
		t.Fatalf("new owner storage capability = %#v", capability)
	}
	if err := MarkStorageLost(root, "owner"); err != nil {
		t.Fatal(err)
	}
	if capability := CheckStorage(root, "owner"); capability.Durable || capability.Reason != "journal_lost" {
		t.Fatalf("lost owner storage capability = %#v", capability)
	}
}
