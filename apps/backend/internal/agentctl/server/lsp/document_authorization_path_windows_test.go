//go:build windows

package lsp

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDocumentWorkspaceCanonicalizesSelectedRootReparseBeforeBrowserAccess(t *testing.T) {
	targetPath := t.TempDir()
	junctionPath := filepath.Join(t.TempDir(), "workspace-junction")
	output, err := exec.Command("cmd", "/c", "mklink", "/J", junctionPath, targetPath).CombinedOutput()
	if err != nil {
		t.Skipf("directory junction unavailable: %v: %s", err, output)
	}
	wantRoot, err := canonicalExistingFilesystemPath(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	access := &recordingDocumentRootAccess{
		lstat: func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist },
	}
	var opened []string
	workspace := newDocumentWorkspaceWithRootAccess(
		canonicalFilesystemPath,
		func(path string) (documentRootAccess, error) {
			opened = append(opened, path)
			return access, nil
		},
	)
	t.Cleanup(workspace.Close)
	workspace.SetConfig(Config{WorkDir: junctionPath})
	if len(opened) != 1 || filepath.Clean(opened[0]) != filepath.Clean(wantRoot) {
		t.Fatalf("opened canonical roots = %#v, want only %q", opened, wantRoot)
	}

	wantDocument := filepath.Join(wantRoot, "Main.kt")
	got, err := workspace.CanonicalURI(
		WorkspaceFileURI(filepath.Join(junctionPath, "Main.kt")),
	)
	if err != nil {
		t.Fatalf("authorize document through selected junction root: %v", err)
	}
	if got != WorkspaceFileURI(wantDocument) {
		t.Fatalf("canonical URI = %q, want %q", got, WorkspaceFileURI(wantDocument))
	}
	if len(opened) != 1 {
		t.Fatalf("browser access reopened selected root: %#v", opened)
	}
}

func TestNormalizeWindowsFinalPath(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "drive", path: `\\?\C:\workspace`, want: `C:\workspace`},
		{name: "unc", path: `\\?\UNC\server\share\workspace`, want: `\\server\share\workspace`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeWindowsFinalPath(test.path); got != test.want {
				t.Fatalf("normalized path = %q, want %q", got, test.want)
			}
		})
	}
}
