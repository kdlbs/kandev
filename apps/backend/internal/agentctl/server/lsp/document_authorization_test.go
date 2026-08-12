package lsp

import (
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
)

func TestDocumentWorkspaceRejectsOutsidePathBeforeSymlinkResolution(t *testing.T) {
	workspacePath := t.TempDir()
	outsidePath := t.TempDir()
	loopPath := filepath.Join(outsidePath, "loop")
	if err := os.Symlink(loopPath, loopPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	workspace := newDocumentWorkspace()
	workspace.SetSnapshot(Snapshot{
		WorkspacePath: workspacePath,
		WorkspaceURI:  WorkspaceFileURI(workspacePath),
	})

	_, err := workspace.CanonicalURI(WorkspaceFileURI(filepath.Join(loopPath, "Secret.kt")))
	if !errors.Is(err, errDocumentOutsideWorkspace) {
		t.Fatalf("outside symlink-loop error = %v, want %v", err, errDocumentOutsideWorkspace)
	}
}

func TestDocumentWorkspaceRejectsUntrustedUNCBeforeResolution(t *testing.T) {
	if goruntime.GOOS != "windows" {
		t.Skip("UNC path semantics require Windows")
	}
	resolveCalls := 0
	workspace := newDocumentWorkspaceWithResolver(func(path string) (string, error) {
		resolveCalls++
		return filepath.Clean(path), nil
	})
	workspacePath := `C:\workspace`
	workspace.SetSnapshot(Snapshot{
		WorkspacePath: workspacePath,
		WorkspaceURI:  WorkspaceFileURI(workspacePath),
	})
	if resolveCalls == 0 {
		t.Fatal("trusted workspace roots were not resolved")
	}
	resolveCalls = 0

	_, err := workspace.CanonicalURI(`file://attacker.invalid/share/Secret.kt`)
	if !errors.Is(err, errDocumentOutsideWorkspace) {
		t.Fatalf("untrusted UNC error = %v, want %v", err, errDocumentOutsideWorkspace)
	}
	if resolveCalls != 0 {
		t.Fatalf("untrusted UNC path reached filesystem resolver %d times", resolveCalls)
	}
}
