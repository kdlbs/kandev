package process

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3
func TestFileTreeRequestedDirectoryReadFailure(t *testing.T) {
	for _, depth := range []int{0, 1, 2} {
		t.Run(strconv.Itoa(depth), func(t *testing.T) {
			folder := filepath.Join(t.TempDir(), "folder")
			if err := os.Mkdir(folder, 0o755); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(folder)
			if err != nil {
				t.Fatal(err)
			}
			// Reproduce a directory disappearing between Stat and ReadDir, even as root.
			if err := os.Remove(folder); err != nil {
				t.Fatal(err)
			}
			node, err := (&WorkspaceTracker{}).buildFileTreeNode(folder, "folder", info, depth, 0)
			if !errors.Is(err, os.ErrNotExist) || node != nil {
				t.Fatalf("requested read = (%+v, %v), want nil tree and filesystem error", node, err)
			}
		})
	}
}

func TestFileTreeDescendantReadFailurePreservesPlaceholder(t *testing.T) {
	folder := t.TempDir()
	info, err := os.Stat(folder)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(folder); err != nil {
		t.Fatal(err)
	}
	for _, depth := range []int{0, 1, 2} {
		node, err := (&WorkspaceTracker{}).buildFileTreeNode(folder, "child", info, depth, 1)
		if err != nil || node == nil || !node.IsDir || node.Path != "child" || node.Children != nil {
			t.Fatalf("descendant read = (%+v, %v), want directory placeholder", node, err)
		}
	}
}

func TestFileTreeEmptyRequestedDirectoryIsSuccessful(t *testing.T) {
	root, err := (&WorkspaceTracker{workDir: t.TempDir()}).GetFileTree("", 1)
	if err != nil || root == nil || !root.IsDir || len(root.Children) != 0 {
		t.Fatalf("empty read = (%+v, %v), want empty directory", root, err)
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if _, exists := wire["children"]; exists {
		t.Fatalf("empty directory should retain omitted-children wire shape: %s", encoded)
	}
}
