package process

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestResolveNonExistentPath(t *testing.T) {
	// Create a real temp dir as the existing ancestor
	tmpDir := t.TempDir()

	t.Run("fully existing path returns resolved path", func(t *testing.T) {
		existingFile := filepath.Join(tmpDir, "existing.txt")
		if err := os.WriteFile(existingFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := resolveNonExistentPath(existingFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected, _ := filepath.EvalSymlinks(existingFile)
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})

	t.Run("non-existent leaf with existing parent", func(t *testing.T) {
		nonExistent := filepath.Join(tmpDir, "noexist.txt")
		result, err := resolveNonExistentPath(nonExistent)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resolvedParent, _ := filepath.EvalSymlinks(tmpDir)
		expected := filepath.Join(resolvedParent, "noexist.txt")
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})

	t.Run("non-existent nested directories", func(t *testing.T) {
		deep := filepath.Join(tmpDir, "a", "b", "c", "file.txt")
		result, err := resolveNonExistentPath(deep)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resolvedBase, _ := filepath.EvalSymlinks(tmpDir)
		expected := filepath.Join(resolvedBase, "a", "b", "c", "file.txt")
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})

	t.Run("existing intermediate directory", func(t *testing.T) {
		subDir := filepath.Join(tmpDir, "sub")
		if err := os.Mkdir(subDir, 0o755); err != nil {
			t.Fatal(err)
		}
		deep := filepath.Join(subDir, "deep", "file.txt")
		result, err := resolveNonExistentPath(deep)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resolvedSub, _ := filepath.EvalSymlinks(subDir)
		expected := filepath.Join(resolvedSub, "deep", "file.txt")
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})

	t.Run("symlinked ancestor resolves correctly", func(t *testing.T) {
		realDir := filepath.Join(tmpDir, "real")
		if err := os.Mkdir(realDir, 0o755); err != nil {
			t.Fatal(err)
		}
		linkDir := filepath.Join(tmpDir, "link")
		if err := os.Symlink(realDir, linkDir); err != nil {
			t.Skip("symlinks not supported")
		}
		path := filepath.Join(linkDir, "new", "file.txt")
		result, err := resolveNonExistentPath(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// realDir itself may be under a symlink (e.g. /var -> /private/var on macOS)
		resolvedReal, _ := filepath.EvalSymlinks(realDir)
		expected := filepath.Join(resolvedReal, "new", "file.txt")
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})

	t.Run("permission error is propagated", func(t *testing.T) {
		// Create a directory, then make it unreadable
		restrictedDir := filepath.Join(tmpDir, "restricted")
		if err := os.Mkdir(restrictedDir, 0o755); err != nil {
			t.Fatal(err)
		}
		innerDir := filepath.Join(restrictedDir, "inner")
		if err := os.Mkdir(innerDir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Remove read+execute permission on the parent
		if err := os.Chmod(restrictedDir, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(restrictedDir, 0o755) })

		path := filepath.Join(innerDir, "file.txt")
		_, err := resolveNonExistentPath(path)
		if err == nil {
			t.Error("expected error for permission-denied path, got nil")
		}
	})
}

// requireChild finds a child node by name in the tree, failing the test if not found.
func requireChild(t *testing.T, node *types.FileTreeNode, name string) *types.FileTreeNode {
	t.Helper()
	for _, c := range node.Children {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("%s not found in tree children", name)
	return nil // unreachable, but satisfies staticcheck
}

func findChild(node *types.FileTreeNode, name string) *types.FileTreeNode {
	for _, c := range node.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestGetFileTree_Symlinks(t *testing.T) {
	tmpDir := t.TempDir()
	wt := &WorkspaceTracker{workDir: tmpDir}

	t.Run("symlink to file shows as file with IsSymlink", func(t *testing.T) {
		content := []byte("target content")
		if err := os.WriteFile(filepath.Join(tmpDir, "target.txt"), content, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("target.txt", filepath.Join(tmpDir, "link.txt")); err != nil {
			t.Skip("symlinks not supported")
		}

		tree, err := wt.GetFileTree("", 1)
		if err != nil {
			t.Fatalf("GetFileTree failed: %v", err)
		}

		node := requireChild(t, tree, "link.txt")
		if node.IsDir {
			t.Error("symlink to file should not be a directory")
		}
		if !node.IsSymlink {
			t.Error("symlink entry should have IsSymlink=true")
		}
		if node.Size != int64(len(content)) {
			t.Errorf("size = %d, want %d", node.Size, len(content))
		}
	})

	t.Run("symlink to directory shows as directory with IsSymlink", func(t *testing.T) {
		realDir := filepath.Join(tmpDir, "realdir")
		if err := os.Mkdir(realDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(realDir, "child.txt"), []byte("hi"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("realdir", filepath.Join(tmpDir, "linkdir")); err != nil {
			t.Skip("symlinks not supported")
		}

		tree, err := wt.GetFileTree("", 2)
		if err != nil {
			t.Fatalf("GetFileTree failed: %v", err)
		}

		node := requireChild(t, tree, "linkdir")
		if !node.IsDir {
			t.Error("symlink to directory should have IsDir=true")
		}
		if !node.IsSymlink {
			t.Error("symlink entry should have IsSymlink=true")
		}
		child := findChild(node, "child.txt")
		if child == nil {
			t.Error("child.txt not found inside symlinked directory")
		}
	})

	t.Run("broken symlink is skipped", func(t *testing.T) {
		if err := os.Symlink("/nonexistent-target", filepath.Join(tmpDir, "broken")); err != nil {
			t.Skip("symlinks not supported")
		}

		tree, err := wt.GetFileTree("", 1)
		if err != nil {
			t.Fatalf("GetFileTree failed: %v", err)
		}

		if findChild(tree, "broken") != nil {
			t.Error("broken symlink should be skipped in tree")
		}
	})
}
