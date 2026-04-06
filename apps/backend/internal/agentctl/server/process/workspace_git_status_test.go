package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUnquoteGitPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain path", "simple-path.txt", "simple-path.txt"},
		{"path with spaces", `"path with spaces/file.md"`, "path with spaces/file.md"},
		{"path with tab", `"path\twith\ttab"`, "path\twith\ttab"},
		{"path with backslash", `"path\\backslash"`, `path\backslash`},
		{"path with quotes", `"path \"quoted\""`, `path "quoted"`},
		{"empty quotes", `""`, ""},
		{"single char not quoted", "a", "a"},
		{"mismatched quote", `"not closed`, `"not closed`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unquoteGitPath(tt.in)
			if got != tt.want {
				t.Errorf("unquoteGitPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGetGitStatus_PathsWithSpaces(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Create a directory and file with spaces in the path.
	dir := filepath.Join(repoDir, "path with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	writeFile(t, dir, "file.md", "initial content")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "Add file with spaces in path")

	// Modify the file to create an unstaged change.
	writeFile(t, dir, "file.md", "modified content")

	status, err := wt.getGitStatus(ctx)
	if err != nil {
		t.Fatalf("failed to get git status: %v", err)
	}

	expectedPath := "path with spaces/file.md"

	// The file should appear in Modified with an unquoted path.
	found := false
	for _, p := range status.Modified {
		if p == expectedPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in Modified list, got %v", expectedPath, status.Modified)
	}

	// The Files map key should be the unquoted path.
	fileInfo, exists := status.Files[expectedPath]
	if !exists {
		t.Fatalf("expected Files map to contain key %q, got keys: %v",
			expectedPath, mapKeys(status.Files))
	}
	if fileInfo.Status != "modified" {
		t.Errorf("expected status=modified, got %q", fileInfo.Status)
	}

	// Diff content should be populated (enrichWithDiffData uses the same key).
	if fileInfo.Diff == "" {
		t.Error("expected non-empty Diff for file with spaces in path")
	}
}

func TestGetGitStatus_UntrackedFileWithSpaces(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Create an untracked file with spaces in the path.
	dir := filepath.Join(repoDir, "dir with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	writeFile(t, dir, "new file.txt", "hello world")

	status, err := wt.getGitStatus(ctx)
	if err != nil {
		t.Fatalf("failed to get git status: %v", err)
	}

	expectedPath := "dir with spaces/new file.txt"

	found := false
	for _, p := range status.Untracked {
		if p == expectedPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in Untracked list, got %v", expectedPath, status.Untracked)
	}

	fileInfo, exists := status.Files[expectedPath]
	if !exists {
		t.Fatalf("expected Files map to contain key %q, got keys: %v",
			expectedPath, mapKeys(status.Files))
	}
	if fileInfo.Diff == "" {
		t.Error("expected non-empty synthetic Diff for untracked file with spaces")
	}
}

// mapKeys returns the keys of a map for diagnostic output.
func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
