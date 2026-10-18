//go:build windows

package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDocumentWorkspaceCanonicalizesSelectedRootReparseBeforeBrowserAccess(t *testing.T) {
	targetPath := t.TempDir()
	replacementPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(targetPath, "Main.kt"), []byte("class Main"), 0o600); err != nil {
		t.Fatal(err)
	}
	junctionPath := filepath.Join(t.TempDir(), "workspace-junction")
	output, err := exec.Command("cmd", "/c", "mklink", "/J", junctionPath, targetPath).CombinedOutput()
	if err != nil {
		t.Skipf("directory junction unavailable: %v: %s", err, output)
	}
	wantRoot, err := canonicalExistingFilesystemPath(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	var opened []string
	workspace := newDocumentWorkspaceWithRootAccess(
		canonicalFilesystemPath,
		func(path string) (documentRootAccess, error) {
			opened = append(opened, path)
			access, openErr := openDocumentRoot(path)
			if openErr != nil {
				return nil, openErr
			}
			if removeOutput, removeErr := exec.Command("cmd", "/c", "rmdir", junctionPath).CombinedOutput(); removeErr != nil {
				_ = access.Close()
				t.Skipf("open junction cannot be replaced: %v: %s", removeErr, removeOutput)
			}
			if replaceOutput, replaceErr := exec.Command(
				"cmd", "/c", "mklink", "/J", junctionPath, replacementPath,
			).CombinedOutput(); replaceErr != nil {
				_ = access.Close()
				t.Skipf("replacement junction unavailable: %v: %s", replaceErr, replaceOutput)
			}
			return access, nil
		},
	)
	t.Cleanup(workspace.Close)
	workspace.SetConfig(Config{WorkDir: junctionPath})
	if len(opened) != 1 || filepath.Clean(opened[0]) != filepath.Clean(junctionPath) {
		t.Fatalf("opened lexical roots = %#v, want only %q", opened, junctionPath)
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
