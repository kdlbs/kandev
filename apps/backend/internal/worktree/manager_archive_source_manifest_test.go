package worktree

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func TestArchiveManifestRejectsForeignRepositoryPath(t *testing.T) {
	foreignRepo := initGitRepoForWorktreeTest(t)
	if err := os.WriteFile(filepath.Join(foreignRepo, "foreign-secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "recorded-worktree", TaskID: "recorded-task", RepositoryID: "recorded-repository",
		Path: foreignRepo, RepositoryPath: foreignRepo,
	}})
	if err == nil {
		t.Fatalf("accepted foreign repository path as task worktree: %+v", manifest)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.4
func TestArchiveManifestRejectsMissingGitMetadata(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	_, err = mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: t.TempDir(), RepositoryPath: repo,
	}})
	if err == nil {
		t.Fatal("captured a worktree path without Git metadata")
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.2
func TestArchiveManifestRejectsForeignLinkedWorktree(t *testing.T) {
	recordedRepo := initGitRepoForWorktreeTest(t)
	foreignRepo := initGitRepoForWorktreeTest(t)
	foreignWorktree := filepath.Join(t.TempDir(), "foreign-worktree")
	runGit(t, foreignRepo, "worktree", "add", foreignWorktree, "feature/pr-branch")
	if err := os.WriteFile(filepath.Join(foreignWorktree, "foreign-secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "recorded-worktree", TaskID: "recorded-task", RepositoryID: "recorded-repository",
		Path: foreignWorktree, RepositoryPath: recordedRepo,
	}})
	if err == nil {
		t.Fatalf("accepted foreign linked worktree as task source: %+v", manifest)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.2
func TestArchiveManifestRejectsSiblingWorktreeGitdir(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	firstPath := filepath.Join(t.TempDir(), "task-one")
	secondPath := filepath.Join(t.TempDir(), "task-two")
	runGit(t, repo, "worktree", "add", "-b", "feature/task-one", firstPath)
	runGit(t, repo, "worktree", "add", "-b", "feature/task-two", secondPath)
	if err := os.WriteFile(filepath.Join(secondPath, "task-two-only.txt"), []byte("task two"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, secondPath, "add", "task-two-only.txt")
	runGit(t, secondPath, "commit", "-m", "add task two file")
	secondGitPointer, err := os.ReadFile(filepath.Join(secondPath, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(firstPath, ".git"), secondGitPointer, 0o600); err != nil {
		t.Fatal(err)
	}

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "task-one-worktree", TaskID: "task-one", RepositoryID: "repo",
		Path: firstPath, RepositoryPath: repo,
	}})
	if err == nil {
		t.Fatalf("captured sibling worktree state under task-one identity: %+v", manifest)
	}
}

func TestArchiveManifestRejectsDisappearedUntrackedPath(t *testing.T) {
	entries, err := archiveSourceManifestEntries(context.Background(), t.TempDir(), "?? disappeared.txt\x00")
	if err == nil {
		t.Fatalf("accepted disappeared untracked path without content identity: %+v", entries)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
func TestArchiveManifestCapturesIgnoredUntrackedFile(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("ignored.env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitignore")
	runGit(t, repo, "commit", "-m", "ignore generated file")
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature/task", worktreePath)
	if err := os.WriteFile(filepath.Join(worktreePath, "ignored.env"), []byte("local value"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manifests, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: worktreePath, RepositoryPath: repo,
	}})
	if err != nil {
		t.Fatalf("capture ignored untracked source: %v", err)
	}
	runGit(t, repo, "worktree", "remove", "--force", worktreePath)
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree after cleanup: %v, want removed", err)
	}
	for _, entry := range manifests["wt"].Entries {
		if entry.Path == "ignored.env" && entry.ContentSHA256 != "" {
			return
		}
	}
	t.Fatalf("ignored untracked file has no path and digest in manifest: %+v", manifests["wt"].Entries)
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.7
func TestArchiveManifestOmitsIgnoredDirectoryContents(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	ignoreRules := "node_modules/\n**/node_modules/\ndist/\ngenerated-cache/\nignored.env\nignored-link\n"
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(ignoreRules), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitignore")
	runGit(t, repo, "commit", "-m", "ignore generated content")
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature/task", worktreePath)
	directoryFiles := map[string]string{
		"node_modules/root-package/index.js":                "root package",
		"packages/app/node_modules/nested-package/index.js": "nested package",
		"dist/bundle.js":           "bundle",
		"generated-cache/data.bin": "arbitrary ignored directory",
	}
	for path, contents := range directoryFiles {
		fullPath := filepath.Join(worktreePath, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "ignored.env"), []byte("local value"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideLinkTarget := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outsideLinkTarget, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideLinkTarget, filepath.Join(worktreePath, "ignored-link")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manifests, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: worktreePath, RepositoryPath: repo,
	}})
	if err != nil {
		t.Fatalf("capture ignored directory source: %v", err)
	}
	manifest := manifests["wt"]
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var roundTripped ArchiveSourceManifest
	if err := json.Unmarshal(manifestJSON, &roundTripped); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manifest, roundTripped) {
		t.Fatalf("manifest snapshot round trip changed omission evidence:\nbefore=%+v\nafter=%+v", manifest, roundTripped)
	}
	wantDirectories := []string{"dist", "generated-cache", "node_modules", "packages/app/node_modules"}
	for _, wantPath := range wantDirectories {
		found := false
		for _, entry := range manifest.Entries {
			entryPath := strings.TrimSuffix(filepath.ToSlash(entry.Path), "/")
			if entryPath != wantPath {
				continue
			}
			found = true
			encoded, marshalErr := json.Marshal(entry)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var persisted map[string]any
			if unmarshalErr := json.Unmarshal(encoded, &persisted); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			if persisted["content_omission"] != "ignored_directory" || entry.ContentSHA256 != "" {
				t.Fatalf("ignored directory evidence for %q = %s", wantPath, encoded)
			}
		}
		if !found {
			t.Fatalf("ignored directory %q was not recorded in manifest: %+v", wantPath, manifest.Entries)
		}
	}
	for _, entry := range manifest.Entries {
		path := strings.TrimSuffix(filepath.ToSlash(entry.Path), "/")
		for _, directory := range wantDirectories {
			if path != directory && strings.HasPrefix(path, directory+"/") {
				t.Fatalf("manifest opened a descendant of omitted directory %q: %+v", directory, entry)
			}
		}
	}
	var ignoredFile, ignoredLink *ArchiveSourceManifestEntry
	for index := range manifest.Entries {
		entry := &manifest.Entries[index]
		switch entry.Path {
		case "ignored.env":
			ignoredFile = entry
		case "ignored-link":
			ignoredLink = entry
		}
	}
	if ignoredFile == nil || ignoredFile.ContentSHA256 == "" {
		t.Fatalf("ignored regular file lost its digest: %+v", manifest.Entries)
	}
	if ignoredLink == nil || ignoredLink.ContentSHA256 == "" {
		t.Fatalf("ignored symlink lost its link identity: %+v", manifest.Entries)
	}
	oldSnapshot, err := json.Marshal(ArchiveSourceManifestEntry{Path: "cache/", Status: "!!"})
	if err != nil {
		t.Fatal(err)
	}
	var oldEntry ArchiveSourceManifestEntry
	if err := json.Unmarshal(oldSnapshot, &oldEntry); err != nil || oldEntry.Path != "cache/" || oldEntry.ContentSHA256 != "" {
		t.Fatalf("old manifest entry no longer decodes: entry=%+v err=%v", oldEntry, err)
	}
	var oldManifest ArchiveSourceManifest
	if err := json.Unmarshal([]byte(`{"task_id":"task","worktree_id":"wt","entries":[{"path":"cache/","status":"!!"}]}`), &oldManifest); err != nil {
		t.Fatalf("old source manifest snapshot no longer decodes: %v", err)
	}
	if len(oldManifest.Entries) != 1 || oldManifest.Entries[0].ContentOmission != "" {
		t.Fatalf("old source manifest entry gained an omission value: %+v", oldManifest.Entries)
	}

	for path := range directoryFiles {
		if err := os.WriteFile(filepath.Join(worktreePath, filepath.FromSlash(path)), []byte("mutated local content"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mutated, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: worktreePath, RepositoryPath: repo,
	}})
	if err != nil {
		t.Fatalf("recapture mutated ignored directories: %v", err)
	}
	if !reflect.DeepEqual(manifest.Entries, mutated["wt"].Entries) {
		t.Fatalf("ignored directory contents changed manifest evidence:\nbefore=%+v\nafter=%+v", manifest.Entries, mutated["wt"].Entries)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.7
func TestArchiveManifestHashesTrackedDependencyPath(t *testing.T) {
	root := t.TempDir()
	trackedPath := filepath.Join(root, "node_modules", "tracked.txt")
	if err := os.MkdirAll(filepath.Dir(trackedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trackedPath, []byte("tracked source"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := archiveSourceManifestEntries(context.Background(), root, " M node_modules/tracked.txt\x00")
	if err != nil {
		t.Fatalf("capture tracked path under dependency directory: %v", err)
	}
	if len(entries) != 1 || entries[0].ContentSHA256 == "" {
		t.Fatalf("tracked dependency path has no digest: %+v", entries)
	}
	encoded, err := json.Marshal(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatal(err)
	}
	if _, exists := persisted["content_omission"]; exists {
		t.Fatalf("tracked dependency path was marked as omitted: %s", encoded)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.7
func TestArchiveManifestIgnoredDirectoryDoesNotOpenDescendants(t *testing.T) {
	handle := &rejectingArchiveSourceDirectoryHandle{}
	digest, omitted, err := archiveSourceManifestDigestEntryAt(context.Background(), handle, "node_modules", "!!")
	if err != nil {
		t.Fatalf("classify ignored directory: %v", err)
	}
	if !omitted || digest != "" {
		t.Fatalf("ignored directory evidence = digest %q, omitted %t", digest, omitted)
	}
	if handle.openedFiles != 0 || handle.openedSubdirectories != 0 {
		t.Fatalf("ignored directory opened descendant handles: files=%d directories=%d", handle.openedFiles, handle.openedSubdirectories)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.8
func TestArchiveManifestDigestStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	closeErr := errors.New("reader close failed")
	reader := &cancelingArchiveManifestReader{cancel: cancel, chunks: [][]byte{
		[]byte("first chunk"), []byte("must not be read"),
	}, closeErr: closeErr}
	_, err := archiveSourceManifestReadDigest(ctx, reader)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("digest error = %v, want context cancellation", err)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("digest error = %v, want reader close failure preserved", err)
	}
	if reader.reads != 1 {
		t.Fatalf("digest performed %d reads after cancellation; want one initial read", reader.reads)
	}
	if reader.closed != 1 {
		t.Fatalf("reader closed %d times, want once", reader.closed)
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("reader context error = %v, want canceled", ctx.Err())
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.8
func TestArchiveManifestDigestStopsBeforeTraversal(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handle := openTrackingArchiveManifestDirectory(t, root, &archiveManifestDirectoryReadState{})
	var records []archiveSourceManifestDirectoryRecord
	err := archiveSourceManifestCollectDirectory(ctx, handle, "", &records)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("directory capture error = %v, want cancellation", err)
	}
	if handle.state.readDirCalls != 0 {
		t.Fatalf("directory traversal read %d directory listings after cancellation", handle.state.readDirCalls)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.8
func TestArchiveManifestDigestStopsBetweenDirectoryEntries(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := &archiveManifestDirectoryReadState{cancel: cancel, cancelOnOpen: "a.txt"}
	handle := openTrackingArchiveManifestDirectory(t, root, state)
	var records []archiveSourceManifestDirectoryRecord
	err := archiveSourceManifestCollectDirectory(ctx, handle, "", &records)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("directory capture error = %v, want cancellation", err)
	}
	if !reflect.DeepEqual(state.openedFiles, []string{"a.txt"}) {
		t.Fatalf("file opens = %v, want only the first entry before cancellation", state.openedFiles)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.8
func TestArchiveManifestDigestStopsDuringDirtySubmoduleCapture(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "module")
	if err := os.Mkdir(module, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "tracked.txt"), []byte("dirty module source"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := &archiveManifestDirectoryReadState{cancel: cancel, cancelOnOpen: "tracked.txt"}
	handle := openTrackingArchiveManifestDirectory(t, root, state)
	digest, omitted, err := archiveSourceManifestDigestEntryAt(ctx, handle, "module", " M")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dirty submodule capture error = %v, want cancellation", err)
	}
	if digest != "" || omitted {
		t.Fatalf("partial dirty submodule evidence = digest %q, omitted %t", digest, omitted)
	}
	if !reflect.DeepEqual(state.openedFiles, []string{"tracked.txt"}) {
		t.Fatalf("dirty submodule file opens = %v", state.openedFiles)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.8
func TestArchiveManifestDigestStopsOnDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	reader := &cancelingArchiveManifestReader{chunks: [][]byte{[]byte("unread")}}
	_, err := archiveSourceManifestReadDigest(ctx, reader)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("digest error = %v, want deadline expiration", err)
	}
	if reader.reads != 0 || reader.closed != 1 {
		t.Fatalf("deadline capture reads=%d closes=%d, want no read and one close", reader.reads, reader.closed)
	}
}

func TestArchiveManifestDigestReturnsCloseFailure(t *testing.T) {
	closeErr := errors.New("reader close failed")
	reader := &cancelingArchiveManifestReader{
		chunks:   [][]byte{[]byte("complete content")},
		closeErr: closeErr,
	}
	_, err := archiveSourceManifestReadDigest(context.Background(), reader)
	if !errors.Is(err, closeErr) {
		t.Fatalf("digest error = %v, want reader close failure", err)
	}
	if reader.closed != 1 {
		t.Fatalf("reader closed %d times, want once", reader.closed)
	}
}

type cancelingArchiveManifestReader struct {
	cancel   context.CancelFunc
	chunks   [][]byte
	reads    int
	closed   int
	closeErr error
}

func (r *cancelingArchiveManifestReader) Read(buffer []byte) (int, error) {
	r.reads++
	if r.reads > len(r.chunks) {
		return 0, io.EOF
	}
	chunk := r.chunks[r.reads-1]
	n := copy(buffer, chunk)
	if r.reads == 1 && r.cancel != nil {
		r.cancel()
	}
	return n, nil
}

func (r *cancelingArchiveManifestReader) Close() error {
	r.closed++
	return r.closeErr
}

type archiveManifestDirectoryReadState struct {
	cancel       context.CancelFunc
	cancelOnOpen string
	openedFiles  []string
	readDirCalls int
}

type trackingArchiveManifestDirectory struct {
	storageworkspaces.DirectoryHandle
	state *archiveManifestDirectoryReadState
}

func openTrackingArchiveManifestDirectory(
	t *testing.T,
	path string,
	state *archiveManifestDirectoryReadState,
) *trackingArchiveManifestDirectory {
	t.Helper()
	handle, err := storageworkspaces.OpenDirectoryNoFollow(filepath.Dir(path), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	return &trackingArchiveManifestDirectory{DirectoryHandle: handle, state: state}
}

func (h *trackingArchiveManifestDirectory) ReadDir() ([]os.DirEntry, error) {
	h.state.readDirCalls++
	return h.DirectoryHandle.ReadDir()
}

func (h *trackingArchiveManifestDirectory) OpenSubdirectory(name string) (storageworkspaces.DirectoryHandle, error) {
	child, err := h.DirectoryHandle.OpenSubdirectory(name)
	if err != nil {
		return nil, err
	}
	return &trackingArchiveManifestDirectory{DirectoryHandle: child, state: h.state}, nil
}

func (h *trackingArchiveManifestDirectory) OpenFile(name string) (io.ReadCloser, error) {
	h.state.openedFiles = append(h.state.openedFiles, name)
	if h.state.cancelOnOpen == name {
		return &cancelingArchiveManifestReader{
			cancel: h.state.cancel,
			chunks: [][]byte{[]byte("first chunk"), []byte("must not be read")},
		}, nil
	}
	return h.DirectoryHandle.OpenFile(name)
}

type rejectingArchiveSourceDirectoryHandle struct {
	storageworkspaces.DirectoryHandle
	openedFiles          int
	openedSubdirectories int
}

func (h *rejectingArchiveSourceDirectoryHandle) LstatEntry(name string) (os.FileMode, error) {
	if name != "node_modules" {
		return 0, os.ErrNotExist
	}
	return os.ModeDir | 0o700, nil
}

func (h *rejectingArchiveSourceDirectoryHandle) OpenFile(string) (io.ReadCloser, error) {
	h.openedFiles++
	return nil, os.ErrPermission
}

func (h *rejectingArchiveSourceDirectoryHandle) OpenSubdirectory(string) (storageworkspaces.DirectoryHandle, error) {
	h.openedSubdirectories++
	return nil, os.ErrPermission
}

func TestArchiveManifestFileDigestRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.txt")
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if _, err := archiveSourceManifestFileDigest(context.Background(), root, path); err == nil {
		t.Fatal("archive source digest followed a symlink")
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.6
func TestArchiveManifestRejectsSymlinkedPathAncestor(t *testing.T) {
	root := t.TempDir()
	foreign := t.TempDir()
	if err := os.Symlink("outside-target", filepath.Join(foreign, "secret-link")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if err := os.Symlink(foreign, filepath.Join(root, "cache")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	// The status was captured before cache was replaced by a symlink.
	_, err := archiveSourceManifestEntries(context.Background(), root, "?? cache/secret-link\x00")
	if err == nil {
		t.Fatal("archive source manifest captured a path through a symlinked ancestor")
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
func TestArchiveManifestHandlesDeletedRenameDestination(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "next.txt"), []byte("next"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := archiveSourceManifestEntries(context.Background(), root, "RD renamed.txt\x00original.txt\x00?? next.txt\x00")
	if err != nil {
		t.Fatalf("parse staged rename with deleted destination: %v", err)
	}
	if len(entries) != 2 || entries[0].Path != "renamed.txt" || entries[0].Status != "RD" || entries[1].Path != "next.txt" {
		t.Fatalf("rename evidence = %+v, want one deleted destination", entries)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
func TestArchiveManifestCapturesUnmergedIndex(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	if err := os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "conflict.txt")
	runGit(t, repo, "commit", "-m", "base")
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature/task", worktreePath)
	runGit(t, repo, "checkout", "-b", "feature/upstream")
	if err := os.WriteFile(filepath.Join(worktreePath, "conflict.txt"), []byte("task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreePath, "commit", "-am", "task")
	if err := os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("upstream\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "commit", "-am", "upstream")
	merge := exec.Command("git", "merge", "feature/conflict")
	merge.Dir = repo
	if err := merge.Run(); err == nil {
		t.Fatal("merge succeeded; expected an unmerged index")
	}
	merge = exec.Command("git", "merge", "feature/upstream")
	merge.Dir = worktreePath
	if err := merge.Run(); err == nil {
		t.Fatal("merge succeeded; expected an unmerged index")
	}

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manifests, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: worktreePath, RepositoryPath: repo,
	}})
	if err != nil {
		t.Fatalf("capture unmerged index: %v", err)
	}
	manifest := manifests["wt"]
	if len(manifest.IndexStateSHA256) != 64 {
		t.Fatalf("unmerged index state digest = %q", manifest.IndexStateSHA256)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.4
func TestArchiveManifestRejectsCorruptIndex(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature/task", worktreePath)
	indexPath := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "--path-format=absolute", "--git-path", "index"))
	if err := os.WriteFile(indexPath, []byte("corrupt index"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: worktreePath, RepositoryPath: repo,
	}}); err == nil {
		t.Fatal("captured source state with a corrupt Git index")
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
func TestArchiveManifestInspectionDoesNotWriteGitObjects(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature/task", worktreePath)
	if err := os.WriteFile(filepath.Join(worktreePath, "staged.txt"), []byte("staged"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreePath, "add", "staged.txt")
	before := gitLooseObjectCount(t, repo)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: worktreePath, RepositoryPath: repo,
	}}); err != nil {
		t.Fatalf("capture staged source state: %v", err)
	}
	if after := gitLooseObjectCount(t, repo); after != before {
		t.Fatalf("capture wrote Git objects: count before=%d after=%d", before, after)
	}
}

func gitLooseObjectCount(t *testing.T, repo string) int {
	t.Helper()
	output := runGit(t, repo, "count-objects", "-v")
	for _, line := range strings.Split(output, "\n") {
		if value, found := strings.CutPrefix(line, "count: "); found {
			count, err := strconv.Atoi(value)
			if err != nil {
				t.Fatal(err)
			}
			return count
		}
	}
	t.Fatal("git count-objects omitted the loose object count")
	return 0
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
func TestArchiveManifestHashesDirtySubmoduleDirectory(t *testing.T) {
	root := t.TempDir()
	submodule := filepath.Join(root, "module")
	if err := os.MkdirAll(submodule, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(submodule, ".git"), []byte("gitdir: ../.git/modules/module\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(submodule, "tracked.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := archiveSourceManifestEntries(context.Background(), root, " M module\x00")
	if err != nil {
		t.Fatalf("hash dirty submodule: %v", err)
	}
	if len(entries) != 1 || entries[0].ContentSHA256 == "" {
		t.Fatalf("submodule evidence = %+v", entries)
	}
}
