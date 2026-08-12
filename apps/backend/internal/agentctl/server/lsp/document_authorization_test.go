package lsp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"
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

func TestDocumentWorkspaceDoesNotUseTrustedRootResolverForDocuments(t *testing.T) {
	workspacePath := t.TempDir()
	resolveCalls := 0
	workspace := newDocumentWorkspaceWithResolver(func(path string) (string, error) {
		resolveCalls++
		if filepath.Clean(path) == filepath.Clean(workspacePath) {
			return filepath.Clean(path), nil
		}
		return filepath.Join(t.TempDir(), "redirected.kt"), nil
	})
	workspace.SetSnapshot(Snapshot{
		WorkspacePath: workspacePath,
		WorkspaceURI:  WorkspaceFileURI(workspacePath),
	})
	resolveCalls = 0

	canonicalURI, err := workspace.CanonicalURI(
		WorkspaceFileURI(filepath.Join(workspacePath, "Main.kt")),
	)
	if err != nil {
		t.Fatalf("resolve task document: %v", err)
	}
	if canonicalURI != WorkspaceFileURI(filepath.Join(workspacePath, "Main.kt")) {
		t.Fatalf("canonical URI = %q", canonicalURI)
	}
	if resolveCalls != 0 {
		t.Fatalf("browser document reached unrestricted trusted-root resolver %d times", resolveCalls)
	}
}

func TestCanonicalDocumentPathRejectsSymlinkEscapeBeforeTargetLookup(t *testing.T) {
	workspacePath := t.TempDir()
	outsidePath := t.TempDir()
	linkPath := filepath.Join(workspacePath, "redirect")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	roots := []documentRoot{{lexical: workspacePath, canonical: workspacePath}}
	inspected := make([]string, 0, 1)
	filesystem := localDocumentFilesystem
	filesystem.lstat = func(path string) (fs.FileInfo, error) {
		inspected = append(inspected, path)
		if pathWithinRoot(path, outsidePath) {
			t.Fatalf("symlink target was inspected before authorization: %s", path)
		}
		return os.Lstat(path)
	}

	_, err := canonicalDocumentPathWithFilesystem(
		filepath.Join(linkPath, "Secret.kt"), roots, filesystem,
	)
	if !errors.Is(err, errDocumentOutsideWorkspace) {
		t.Fatalf("symlink escape error = %v, want %v", err, errDocumentOutsideWorkspace)
	}
	if len(inspected) != 1 || filepath.Clean(inspected[0]) != filepath.Clean(linkPath) {
		t.Fatalf("inspected paths = %#v, want only %q", inspected, linkPath)
	}
}

func TestCanonicalDocumentPathAllowsContainedSymlink(t *testing.T) {
	workspacePath := t.TempDir()
	targetPath := filepath.Join(workspacePath, "src")
	if err := os.Mkdir(targetPath, 0o700); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(workspacePath, "linked-src")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	roots := []documentRoot{{lexical: workspacePath, canonical: workspacePath}}

	resolved, err := canonicalDocumentPath(filepath.Join(linkPath, "Main.kt"), roots)
	if err != nil {
		t.Fatalf("resolve contained symlink: %v", err)
	}
	want := filepath.Join(targetPath, "Main.kt")
	if filepath.Clean(resolved) != filepath.Clean(want) {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}
}

func TestCanonicalDocumentPathAllowsRedirectToAnotherTaskRoot(t *testing.T) {
	workspacePath := t.TempDir()
	repositoryPath := t.TempDir()
	linkPath := filepath.Join(workspacePath, "repository")
	if err := os.Symlink(repositoryPath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	roots := []documentRoot{
		{lexical: workspacePath, canonical: workspacePath},
		{lexical: repositoryPath, canonical: repositoryPath},
	}

	resolved, err := canonicalDocumentPath(filepath.Join(linkPath, "Main.kt"), roots)
	if err != nil {
		t.Fatalf("resolve cross-root symlink: %v", err)
	}
	want := filepath.Join(repositoryPath, "Main.kt")
	if filepath.Clean(resolved) != filepath.Clean(want) {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}
}

func TestDocumentWorkspaceRejectsUntrustedUNCBeforeResolution(t *testing.T) {
	if goruntime.GOOS != windowsOS {
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

func TestDocumentWorkspaceRejectsInRootUNCReparseBeforeResolution(t *testing.T) {
	if goruntime.GOOS != windowsOS {
		t.Skip("UNC path semantics require Windows")
	}
	workspacePath := `C:\workspace`
	redirectPath := filepath.Join(workspacePath, "redirect")
	inspected := make([]string, 0, 1)
	filesystem := documentFilesystem{
		lstat: func(path string) (fs.FileInfo, error) {
			inspected = append(inspected, path)
			if filepath.Clean(path) == filepath.Clean(redirectPath) {
				return reparsePointFileInfo{}, nil
			}
			return nil, fs.ErrNotExist
		},
		readlink: func(path string) (string, error) {
			if filepath.Clean(path) != filepath.Clean(redirectPath) {
				t.Fatalf("read unexpected reparse point %q", path)
			}
			return `\\attacker.invalid\share`, nil
		},
	}
	roots := []documentRoot{{lexical: workspacePath, canonical: workspacePath}}

	_, err := canonicalDocumentPathWithFilesystem(
		filepath.Join(redirectPath, "Secret.kt"), roots, filesystem,
	)
	if !errors.Is(err, errDocumentOutsideWorkspace) {
		t.Fatalf("in-root UNC reparse error = %v, want %v", err, errDocumentOutsideWorkspace)
	}
	if len(inspected) != 1 || filepath.Clean(inspected[0]) != filepath.Clean(redirectPath) {
		t.Fatalf("inspected paths = %#v, want only %q", inspected, redirectPath)
	}
}

type reparsePointFileInfo struct{}

func (reparsePointFileInfo) Name() string       { return "redirect" }
func (reparsePointFileInfo) Size() int64        { return 0 }
func (reparsePointFileInfo) Mode() fs.FileMode  { return fs.ModeIrregular }
func (reparsePointFileInfo) ModTime() time.Time { return time.Time{} }
func (reparsePointFileInfo) IsDir() bool        { return false }
func (reparsePointFileInfo) Sys() any           { return nil }
