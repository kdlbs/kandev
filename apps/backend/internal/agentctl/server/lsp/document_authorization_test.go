package lsp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
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
	t.Cleanup(workspace.Close)
	workspace.SetConfig(Config{
		WorkDir:      workspacePath,
		WorkspaceURI: WorkspaceFileURI(workspacePath),
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
	t.Cleanup(workspace.Close)
	workspace.SetConfig(Config{
		WorkDir:      workspacePath,
		WorkspaceURI: WorkspaceFileURI(workspacePath),
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

func TestDocumentWorkspacePinsRootAndUsesRootRelativeAccess(t *testing.T) {
	workspacePath := t.TempDir()
	directoryInfo, err := os.Stat(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	access := &recordingDocumentRootAccess{
		stat: func(string) (fs.FileInfo, error) { return directoryInfo, nil },
		lstat: func(path string) (fs.FileInfo, error) {
			if path == "src" {
				return directoryInfo, nil
			}
			return nil, fs.ErrNotExist
		},
	}
	openCalls := 0
	workspace := newDocumentWorkspaceWithRootAccess(
		func(path string) (string, error) { return filepath.Clean(path), nil },
		func(path string) (documentRootAccess, error) {
			openCalls++
			if filepath.Clean(path) != filepath.Clean(workspacePath) {
				t.Fatalf("opened root %q, want %q", path, workspacePath)
			}
			return access, nil
		},
	)
	workspace.SetConfig(Config{WorkDir: workspacePath})
	if openCalls != 1 {
		t.Fatalf("root open calls after snapshot = %d, want 1", openCalls)
	}

	wantPath := filepath.Join(workspacePath, "src", "Main.kt")
	canonicalURI, err := workspace.CanonicalURI(WorkspaceFileURI(wantPath))
	if err != nil {
		t.Fatalf("authorize document through pinned root: %v", err)
	}
	if canonicalURI != WorkspaceFileURI(wantPath) {
		t.Fatalf("canonical URI = %q, want %q", canonicalURI, WorkspaceFileURI(wantPath))
	}
	if openCalls != 1 {
		t.Fatalf("browser authorization reopened root %d times", openCalls-1)
	}
	wantInspected := []string{"src", filepath.Join("src", "Main.kt")}
	if len(access.inspected) != len(wantInspected) {
		t.Fatalf("root-relative inspections = %#v, want %#v", access.inspected, wantInspected)
	}
	for index := range wantInspected {
		if access.inspected[index] != wantInspected[index] || filepath.IsAbs(access.inspected[index]) {
			t.Fatalf("inspection %d = %q, want root-relative %q", index, access.inspected[index], wantInspected[index])
		}
	}

	workspace.Close()
	if access.closeCalls != 1 {
		t.Fatalf("root close calls = %d, want 1", access.closeCalls)
	}
}

func TestDocumentWorkspaceRejectsRootReplacementBetweenOpenAndCanonicalization(t *testing.T) {
	if goruntime.GOOS == windowsOS {
		t.Skip("open directory rename semantics are covered by the native Windows junction test")
	}
	parent := t.TempDir()
	workspacePath := filepath.Join(parent, "workspace")
	parkedPath := filepath.Join(parent, "parked")
	outsidePath := t.TempDir()
	if err := os.Mkdir(workspacePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsidePath, "Secret.kt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolveCalls := 0
	workspace := newDocumentWorkspaceWithResolver(func(path string) (string, error) {
		resolveCalls++
		if err := os.Rename(workspacePath, parkedPath); err != nil {
			return "", err
		}
		if err := os.Symlink(outsidePath, workspacePath); err != nil {
			return "", err
		}
		return canonicalFilesystemPath(path)
	})
	t.Cleanup(workspace.Close)
	workspace.SetConfig(Config{WorkDir: workspacePath})
	if resolveCalls != 1 {
		t.Fatalf("trusted root resolver calls = %d, want 1", resolveCalls)
	}

	_, err := workspace.CanonicalURI(WorkspaceFileURI(filepath.Join(workspacePath, "Secret.kt")))
	if !errors.Is(err, errDocumentOutsideWorkspace) {
		t.Fatalf("replaced root authorization error = %v, want fail closed", err)
	}
}

func TestDocumentWorkspaceRejectsCanonicalIdentityMismatch(t *testing.T) {
	if goruntime.GOOS == windowsOS {
		t.Skip("Windows derives the canonical root directly from the pinned handle")
	}
	workspacePath := t.TempDir()
	outsidePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsidePath, "Secret.kt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := newDocumentWorkspaceWithRootAccess(
		func(string) (string, error) { return outsidePath, nil },
		openDocumentRoot,
	)
	t.Cleanup(workspace.Close)
	workspace.SetConfig(Config{WorkDir: workspacePath})

	_, err := workspace.CanonicalURI(WorkspaceFileURI(filepath.Join(workspacePath, "Secret.kt")))
	if !errors.Is(err, errDocumentOutsideWorkspace) {
		t.Fatalf("mismatched canonical identity error = %v, want fail closed", err)
	}
}

func TestDocumentWorkspaceReplacesAndClosesPinnedRoots(t *testing.T) {
	firstPath := t.TempDir()
	secondPath := t.TempDir()
	accessByPath := map[string]*recordingDocumentRootAccess{}
	workspace := newDocumentWorkspaceWithRootAccess(
		func(path string) (string, error) { return filepath.Clean(path), nil },
		func(path string) (documentRootAccess, error) {
			rootInfo, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			access := &recordingDocumentRootAccess{
				stat:  func(string) (fs.FileInfo, error) { return rootInfo, nil },
				lstat: func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist },
			}
			accessByPath[filepath.Clean(path)] = access
			return access, nil
		},
	)
	workspace.SetConfig(Config{WorkDir: firstPath})
	workspace.SetConfig(Config{WorkDir: secondPath})
	if got := accessByPath[filepath.Clean(firstPath)].closeCalls; got != 1 {
		t.Fatalf("replaced root close calls = %d, want 1", got)
	}
	if got := accessByPath[filepath.Clean(secondPath)].closeCalls; got != 0 {
		t.Fatalf("active root close calls = %d, want 0", got)
	}
	workspace.Close()
	workspace.Close()
	if got := accessByPath[filepath.Clean(secondPath)].closeCalls; got != 1 {
		t.Fatalf("active root close calls after idempotent close = %d, want 1", got)
	}
}

type recordingDocumentRootAccess struct {
	stat       func(string) (fs.FileInfo, error)
	lstat      func(string) (fs.FileInfo, error)
	readlink   func(string) (string, error)
	inspected  []string
	closeCalls int
}

func (a *recordingDocumentRootAccess) Stat(path string) (fs.FileInfo, error) {
	return a.stat(path)
}

func (a *recordingDocumentRootAccess) Lstat(path string) (fs.FileInfo, error) {
	a.inspected = append(a.inspected, path)
	return a.lstat(path)
}

func (a *recordingDocumentRootAccess) Readlink(path string) (string, error) {
	if a.readlink == nil {
		return "", errors.New("unexpected document link")
	}
	return a.readlink(path)
}

func (a *recordingDocumentRootAccess) Close() error {
	a.closeCalls++
	return nil
}

func openDocumentRootsForTest(t testing.TB, roots []documentRoot) []documentRoot {
	t.Helper()
	for index := range roots {
		if roots[index].access != nil {
			continue
		}
		access, err := openDocumentRoot(roots[index].canonical)
		if err != nil {
			t.Fatal(err)
		}
		roots[index].access = access
		t.Cleanup(func() { _ = access.Close() })
	}
	return roots
}

func TestCanonicalDocumentPathRejectsSymlinkEscapeBeforeTargetLookup(t *testing.T) {
	workspacePath := t.TempDir()
	outsidePath := t.TempDir()
	linkPath := filepath.Join(workspacePath, "redirect")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	root, err := os.OpenRoot(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := root.Close(); closeErr != nil {
			t.Errorf("close document root: %v", closeErr)
		}
	})
	access := &recordingDocumentRootAccess{lstat: root.Lstat, readlink: root.Readlink}
	roots := []documentRoot{{
		lexical: workspacePath, canonical: workspacePath, access: access,
	}}

	_, err = canonicalDocumentPath(filepath.Join(linkPath, "Secret.kt"), roots)
	if !errors.Is(err, errDocumentOutsideWorkspace) {
		t.Fatalf("symlink escape error = %v, want %v", err, errDocumentOutsideWorkspace)
	}
	if len(access.inspected) != 1 || access.inspected[0] != "redirect" {
		t.Fatalf("inspected paths = %#v, want only root-relative redirect", access.inspected)
	}
}

func TestCanonicalDocumentPathFailsClosedWhenCheckedParentBecomesOutsideLink(t *testing.T) {
	workspacePath := t.TempDir()
	sourcePath := filepath.Join(workspacePath, "src")
	if err := os.Mkdir(sourcePath, 0o700); err != nil {
		t.Fatal(err)
	}
	outsidePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsidePath, "Secret.kt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := root.Close(); closeErr != nil {
			t.Errorf("close document root: %v", closeErr)
		}
	})
	access := &recordingDocumentRootAccess{readlink: root.Readlink}
	access.lstat = func(path string) (fs.FileInfo, error) {
		info, statErr := root.Lstat(path)
		if path != "src" || statErr != nil {
			return info, statErr
		}
		if err := os.Remove(sourcePath); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsidePath, sourcePath); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		return info, nil
	}
	roots := []documentRoot{{
		lexical: workspacePath, canonical: workspacePath, access: access,
	}}

	_, err = canonicalDocumentPath(filepath.Join(sourcePath, "Secret.kt"), roots)
	if err == nil {
		t.Fatal("path authorization followed a parent replaced by an outside link")
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
	roots := openDocumentRootsForTest(t, []documentRoot{{
		lexical: workspacePath, canonical: workspacePath,
	}})

	resolved, err := canonicalDocumentPath(filepath.Join(linkPath, "Main.kt"), roots)
	if err != nil {
		t.Fatalf("resolve contained symlink: %v", err)
	}
	want := filepath.Join(targetPath, "Main.kt")
	if filepath.Clean(resolved) != filepath.Clean(want) {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}
}

func TestCanonicalDocumentPathAllowsMissingTail(t *testing.T) {
	workspacePath := t.TempDir()
	roots := openDocumentRootsForTest(t, []documentRoot{{
		lexical: workspacePath, canonical: workspacePath,
	}})
	want := filepath.Join(workspacePath, "generated", "Main.kt")

	resolved, err := canonicalDocumentPath(want, roots)
	if err != nil {
		t.Fatalf("resolve missing document tail: %v", err)
	}
	if filepath.Clean(resolved) != filepath.Clean(want) {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}
}

func TestCanonicalDocumentPathRejectsContainedLinkLoop(t *testing.T) {
	workspacePath := t.TempDir()
	loopPath := filepath.Join(workspacePath, "loop")
	if err := os.Symlink("loop", loopPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	roots := openDocumentRootsForTest(t, []documentRoot{{
		lexical: workspacePath, canonical: workspacePath,
	}})

	_, err := canonicalDocumentPath(filepath.Join(loopPath, "Main.kt"), roots)
	if err == nil || !strings.Contains(err.Error(), "too many links") {
		t.Fatalf("contained link-loop error = %v, want too many links", err)
	}
}

func TestCanonicalDocumentPathAllowsRedirectToAnotherTaskRoot(t *testing.T) {
	workspacePath := t.TempDir()
	repositoryPath := t.TempDir()
	linkPath := filepath.Join(workspacePath, "repository")
	if err := os.Symlink(repositoryPath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	roots := openDocumentRootsForTest(t, []documentRoot{
		{lexical: workspacePath, canonical: workspacePath},
		{lexical: repositoryPath, canonical: repositoryPath},
	})

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
	t.Cleanup(workspace.Close)
	workspacePath := `C:\workspace`
	workspace.SetConfig(Config{
		WorkDir:      workspacePath,
		WorkspaceURI: WorkspaceFileURI(workspacePath),
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
	access := &recordingDocumentRootAccess{
		lstat: func(path string) (fs.FileInfo, error) {
			if path == "redirect" {
				return reparsePointFileInfo{}, nil
			}
			return nil, fs.ErrNotExist
		},
		readlink: func(path string) (string, error) {
			if path != "redirect" {
				t.Fatalf("read unexpected reparse point %q", path)
			}
			return `\\attacker.invalid\share`, nil
		},
	}
	roots := []documentRoot{{
		lexical: workspacePath, canonical: workspacePath, access: access,
	}}

	_, err := canonicalDocumentPath(filepath.Join(redirectPath, "Secret.kt"), roots)
	if !errors.Is(err, errDocumentOutsideWorkspace) {
		t.Fatalf("in-root UNC reparse error = %v, want %v", err, errDocumentOutsideWorkspace)
	}
	if len(access.inspected) != 1 || access.inspected[0] != "redirect" {
		t.Fatalf("inspected paths = %#v, want only root-relative redirect", access.inspected)
	}
}

type reparsePointFileInfo struct{}

func (reparsePointFileInfo) Name() string       { return "redirect" }
func (reparsePointFileInfo) Size() int64        { return 0 }
func (reparsePointFileInfo) Mode() fs.FileMode  { return fs.ModeIrregular }
func (reparsePointFileInfo) ModTime() time.Time { return time.Time{} }
func (reparsePointFileInfo) IsDir() bool        { return false }
func (reparsePointFileInfo) Sys() any           { return nil }
