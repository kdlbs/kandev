package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/recoveryartifact"
)

func TestWorkspaceTreeRecoveryExclusionsKeepLookalikes(t *testing.T) {
	root := t.TempDir()
	excludedDir := filepath.Join(root, ".kandev-recovery-owned")
	if err := os.MkdirAll(filepath.Join(excludedDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(excludedDir, "nested", "artifact.txt"), []byte("owned"), 0o644); err != nil {
		t.Fatal(err)
	}
	lookalikeDir := filepath.Join(root, ".kandev-recovery-not-owned")
	if err := os.MkdirAll(lookalikeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lookalikeDir, "notes.txt"), []byte("visible"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := NewWorkspaceTracker(root, newTestLogger(t))
	wt.SetRecoveryArtifactExclusions([]string{excludedDir})
	tree, err := wt.GetFileTree("", 3)
	if err != nil {
		t.Fatalf("GetFileTree: %v", err)
	}
	if findFileTreeNode(tree, filepath.Base(excludedDir)) != nil {
		t.Fatal("registered artifact directory remains in workspace tree")
	}
	if findFileTreeNode(tree, filepath.Base(lookalikeDir)) == nil {
		t.Fatal("unregistered lookalike directory was hidden")
	}
}

func TestWorkspaceSearchRecoveryExclusions(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	for name, content := range map[string]string{
		".kandev-recovery-owned.json":     "private-artifact-needle",
		".kandev-recovery-user-note.json": "user-note-needle",
		"visible.txt":                     "visible-needle",
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wt := NewWorkspaceTracker(repo, newTestLogger(t))
	wt.SetRecoveryArtifactExclusions([]string{filepath.Join(repo, ".kandev-recovery-owned.json")})
	wt.updateFiles(context.Background())

	for _, path := range wt.SearchFiles("kandev-recovery", 20) {
		if path == ".kandev-recovery-owned.json" {
			t.Fatal("registered artifact was returned by filename search")
		}
	}
	if got := wt.SearchFiles("kandev-recovery-user-note", 20); len(got) != 1 || got[0] != ".kandev-recovery-user-note.json" {
		t.Fatalf("unregistered lookalike filename search = %v", got)
	}
	results, err := wt.SearchContent(context.Background(), "needle", 20)
	if err != nil {
		t.Fatalf("SearchContent: %v", err)
	}
	paths := make(map[string]bool, len(results))
	for _, result := range results {
		paths[result.Path] = true
	}
	if paths[".kandev-recovery-owned.json"] {
		t.Fatal("registered artifact was opened by content search")
	}
	if !paths[".kandev-recovery-user-note.json"] || !paths["visible.txt"] {
		t.Fatalf("content search hid user files: %v", paths)
	}
}

func TestWorkspaceRecoveryExclusionCanHideWholeTracker(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	if err := os.WriteFile(filepath.Join(repo, "recovery-content.txt"), []byte("whole-tracker-secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := NewWorkspaceTracker(repo, newTestLogger(t))
	wt.SetRecoveryArtifactExclusions([]string{repo})
	wt.updateFiles(context.Background())

	if _, err := wt.GetFileTree("", 3); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("whole-tracker tree error = %v, want ErrFileNotFound", err)
	}
	if got := wt.SearchFiles("recovery-content", 10); len(got) != 0 {
		t.Fatalf("whole-tracker filename search = %v, want no matches", got)
	}
	results, err := wt.SearchContent(context.Background(), "whole-tracker-secret", 10)
	if err != nil {
		t.Fatalf("SearchContent: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("whole-tracker content search = %+v, want no matches", results)
	}
}

func TestReplacedLegacyRecoveryArtifactRemainsVisibleAndSearchable(t *testing.T) {
	root, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	artifactPath := filepath.Join(root, ".kandev-recovery-owned")
	if err := os.Mkdir(artifactPath, 0o700); err != nil {
		t.Fatal(err)
	}
	identity, ok := recoveryartifact.FilesystemIdentity(artifactPath)
	if !ok {
		t.Skip("filesystem does not expose stable object identity")
	}
	registered := []recoveryartifact.Registered{{Registration: recoveryartifact.Registration{
		Provenance:    recoveryartifact.ProvenanceLegacyPublished,
		ArtifactPaths: []string{artifactPath}, ArtifactIdentities: map[string]string{artifactPath: identity},
	}}}
	paths := recoveryartifact.VerifiedLegacyArtifactPaths(registered)
	if len(paths) != 1 || paths[0] != artifactPath {
		t.Fatalf("proven artifact exclusions = %v, want %s", paths, artifactPath)
	}
	if paths := recoveryartifact.ExclusionPaths(registered, root); len(paths) != 1 || paths[0] != artifactPath {
		t.Fatalf("proven artifact exclusions from shared helper = %v, want %s", paths, artifactPath)
	}
	retainedIdentityPath := artifactPath + ".original"
	if err := os.Rename(artifactPath, retainedIdentityPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(artifactPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactPath, "user-notes.txt"), []byte("user-visible recovery note"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths = recoveryartifact.VerifiedLegacyArtifactPaths(registered)
	if len(paths) != 0 {
		t.Fatalf("replacement user object remained excluded: %v", paths)
	}
	if paths := recoveryartifact.ExclusionPaths(registered, root); len(paths) != 0 {
		t.Fatalf("replacement user object remained excluded by shared helper: %v", paths)
	}
	wt := NewWorkspaceTracker(root, newTestLogger(t))
	wt.SetRecoveryArtifactExclusions(paths)
	tree, err := wt.GetFileTree("", 3)
	if err != nil {
		t.Fatal(err)
	}
	if findFileTreeNode(tree, filepath.Base(artifactPath)) == nil {
		t.Fatal("replacement user directory is missing from the workspace tree")
	}
	wt.updateFiles(context.Background())
	if got := wt.SearchFiles("user-notes", 10); len(got) != 1 || got[0] != filepath.Base(artifactPath)+"/user-notes.txt" {
		t.Fatalf("replacement user object filename search = %v", got)
	}
	results, err := wt.SearchContent(context.Background(), "user-visible recovery note", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != filepath.Base(artifactPath)+"/user-notes.txt" {
		t.Fatalf("replacement user object content search = %+v", results)
	}
}

func findFileTreeNode(node *types.FileTreeNode, name string) *types.FileTreeNode {
	if node == nil {
		return nil
	}
	if node.Name == name {
		return node
	}
	for _, child := range node.Children {
		if found := findFileTreeNode(child, name); found != nil {
			return found
		}
	}
	return nil
}
