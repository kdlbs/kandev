package launcher

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRejectSymlinkComponentsRejectsParentSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}

	if err := rejectSymlinkComponents(filepath.Join(link, "data")); err == nil {
		t.Fatal("expected symlink component to be rejected")
	}
}
