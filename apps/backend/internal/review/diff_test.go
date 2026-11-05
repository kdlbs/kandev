package review

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/utility/hash"
)

type fakeChangeSource struct {
	uncommitted map[string]any
	committed   map[string]any
	statusErr   error
	diffErr     error
}

func (f *fakeChangeSource) UncommittedFiles(context.Context, string) (map[string]any, error) {
	return f.uncommitted, f.statusErr
}

func (f *fakeChangeSource) CommittedFiles(context.Context, string) (map[string]any, error) {
	return f.committed, f.diffErr
}

func fileEntry(path, diff, repoName, repoID string) map[string]any {
	return map[string]any{
		"path":            path,
		"diff":            diff,
		"status":          "modified",
		"additions":       2,
		"deletions":       1,
		"repository_name": repoName,
		"repository_id":   repoID,
	}
}

func TestCollectChanges_UncommittedWinsOverCommitted(t *testing.T) {
	src := &fakeChangeSource{
		uncommitted: map[string]any{
			"a.go": fileEntry("a.go", "fresh working tree diff", "", ""),
		},
		committed: map[string]any{
			"a.go": fileEntry("a.go", "stale committed diff", "", ""),
			"b.go": fileEntry("b.go", "committed only", "", ""),
		},
	}

	files, err := CollectChanges(context.Background(), src, "sess", "")
	if err != nil {
		t.Fatalf("CollectChanges: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	byPath := map[string]ChangedFile{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	if byPath["a.go"].Diff != "fresh working tree diff" {
		t.Fatalf("uncommitted content must win, got %q", byPath["a.go"].Diff)
	}
	if byPath["a.go"].Source != sourceUncommitted {
		t.Fatalf("expected uncommitted source, got %q", byPath["a.go"].Source)
	}
	if byPath["b.go"].Source != sourceCommitted {
		t.Fatalf("expected committed source for b.go, got %q", byPath["b.go"].Source)
	}
}

func TestCollectChanges_HashesNormalizedDiff(t *testing.T) {
	const staged = "unstaged part\n--- Staged changes ---\nreal staged diff"
	src := &fakeChangeSource{
		uncommitted: map[string]any{"a.go": fileEntry("a.go", staged, "", "")},
	}

	files, err := CollectChanges(context.Background(), src, "sess", "")
	if err != nil {
		t.Fatalf("CollectChanges: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Diff != "real staged diff" {
		t.Fatalf("expected staged half kept, got %q", files[0].Diff)
	}
	if files[0].DiffHash != hash.DJB2("real staged diff") {
		t.Fatalf("hash must cover the normalized diff, got %q", files[0].DiffHash)
	}
}

func TestCollectChanges_MultiRepoKeysAndOrdering(t *testing.T) {
	src := &fakeChangeSource{
		uncommitted: map[string]any{
			"frontend\x00src/app.ts": fileEntry("src/app.ts", "fe diff", "frontend", "repo-fe"),
			"backend\x00src/app.go":  fileEntry("src/app.go", "be diff", "backend", "repo-be"),
			"backend\x00README.md":   fileEntry("README.md", "be readme", "backend", "repo-be"),
		},
	}

	files, err := CollectChanges(context.Background(), src, "sess", "")
	if err != nil {
		t.Fatalf("CollectChanges: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	// backend sorts before frontend; within backend, README.md before src/app.go.
	want := []string{"backend\x00README.md", "backend\x00src/app.go", "frontend\x00src/app.ts"}
	for i, key := range want {
		if files[i].Key() != key {
			t.Fatalf("file %d: expected key %q, got %q", i, key, files[i].Key())
		}
	}
	if RepositoryCount(files) != 2 {
		t.Fatalf("expected 2 repositories, got %d", RepositoryCount(files))
	}
}

func TestCollectChanges_ScopesToRepository(t *testing.T) {
	src := &fakeChangeSource{
		uncommitted: map[string]any{
			"frontend\x00a.ts": fileEntry("a.ts", "fe diff", "frontend", "repo-fe"),
			"backend\x00a.go":  fileEntry("a.go", "be diff", "backend", "repo-be"),
		},
	}

	files, err := CollectChanges(context.Background(), src, "sess", "repo-be")
	if err != nil {
		t.Fatalf("CollectChanges: %v", err)
	}
	if len(files) != 1 || files[0].RepositoryName != "backend" {
		t.Fatalf("expected only the backend file, got %+v", files)
	}
}

func TestCollectChanges_SkipsEmptyDiffs(t *testing.T) {
	src := &fakeChangeSource{
		uncommitted: map[string]any{
			"binary.png": fileEntry("binary.png", "", "", ""),
			"a.go":       fileEntry("a.go", "real diff", "", ""),
			"broken":     "not-an-object",
		},
	}

	files, err := CollectChanges(context.Background(), src, "sess", "")
	if err != nil {
		t.Fatalf("CollectChanges: %v", err)
	}
	if len(files) != 1 || files[0].Path != "a.go" {
		t.Fatalf("expected only the diffable file, got %+v", files)
	}
}

func TestCollectChanges_FallsBackToMapKeyForPath(t *testing.T) {
	src := &fakeChangeSource{
		// Single-repo cumulative payloads omit `path` on the value.
		committed: map[string]any{
			"pkg/thing.go": map[string]any{"diff": "committed diff", "status": "modified"},
		},
	}

	files, err := CollectChanges(context.Background(), src, "sess", "")
	if err != nil {
		t.Fatalf("CollectChanges: %v", err)
	}
	if len(files) != 1 || files[0].Path != "pkg/thing.go" {
		t.Fatalf("expected path recovered from the map key, got %+v", files)
	}
}

func TestCollectChanges_WorkingTreeErrorFailsClosed(t *testing.T) {
	src := &fakeChangeSource{statusErr: errors.New("agentctl unreachable")}

	_, err := CollectChanges(context.Background(), src, "sess", "")
	if !errors.Is(err, ErrWorkspaceUnavailable) {
		t.Fatalf("expected ErrWorkspaceUnavailable, got %v", err)
	}
}

func TestCollectChanges_CommittedErrorIsNotFatal(t *testing.T) {
	src := &fakeChangeSource{
		uncommitted: map[string]any{"a.go": fileEntry("a.go", "diff", "", "")},
		diffErr:     errors.New("no base commit"),
	}

	files, err := CollectChanges(context.Background(), src, "sess", "")
	if err != nil {
		t.Fatalf("expected uncommitted-only success, got %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestCollectChanges_FailsWhenCommittedWorkspaceComparisonIsUnavailable(t *testing.T) {
	src := &fakeChangeSource{
		uncommitted: map[string]any{"working.go": fileEntry("working.go", "working diff", "", "")},
		diffErr:     ErrWorkspaceUnavailable,
	}

	_, err := CollectChanges(context.Background(), src, "sess", "")

	if !errors.Is(err, ErrWorkspaceUnavailable) {
		t.Fatalf("CollectChanges error = %v, want unavailable workspace comparison", err)
	}
}

func TestCollectChanges_RequiresSourceAndSession(t *testing.T) {
	if _, err := CollectChanges(context.Background(), nil, "sess", ""); !errors.Is(err, ErrWorkspaceUnavailable) {
		t.Fatalf("expected ErrWorkspaceUnavailable for nil source, got %v", err)
	}
	if _, err := CollectChanges(context.Background(), &fakeChangeSource{}, "", ""); !errors.Is(err, ErrWorkspaceUnavailable) {
		t.Fatalf("expected ErrWorkspaceUnavailable for empty session, got %v", err)
	}
}

func TestNormalizeDiff(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "   \n ", ""},
		{"trims", "  diff body  ", "diff body"},
		{"keeps staged half", "unstaged\n--- Staged changes ---\nstaged", "staged"},
		{"empty staged half keeps first", "unstaged\n--- Staged changes ---\n   ", "unstaged"},
		{"no separator", "plain diff", "plain diff"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeDiff(tc.input); got != tc.want {
				t.Fatalf("NormalizeDiff(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestChangedFileKeyAndIndex(t *testing.T) {
	single := ChangedFile{Path: "a.go"}
	if single.Key() != "a.go" {
		t.Fatalf("single-repo key must be the bare path, got %q", single.Key())
	}
	root := ChangedFile{Path: "a.go", RepositoryID: "repo-root"}
	if root.Key() != "\x00a.go" {
		t.Fatalf("explicit workspace-root key must keep its scope, got %q", root.Key())
	}
	multi := ChangedFile{Path: "a.go", RepositoryName: "backend"}
	if multi.Key() != "backend\x00a.go" {
		t.Fatalf("multi-repo key mismatch, got %q", multi.Key())
	}
	index := FileByKey([]ChangedFile{single, root, multi})
	if len(index) != 3 {
		t.Fatalf("expected all scoped keys indexed, got %d", len(index))
	}
}

func TestRepositoryCount_EmptySet(t *testing.T) {
	if RepositoryCount(nil) != 0 {
		t.Fatal("expected 0 repositories for an empty change set")
	}
	if RepositoryCount([]ChangedFile{{Path: "a.go"}}) != 1 {
		t.Fatal("expected a single-repository task to report 1")
	}
}

func localReviewGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func localReviewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	localReviewGit(t, dir, "init")
	localReviewGit(t, dir, "config", "user.email", "review@example.test")
	localReviewGit(t, dir, "config", "user.name", "Review Test")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	localReviewGit(t, dir, "add", ".")
	localReviewGit(t, dir, "commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCollectLocalChangesIncludesUntrackedAndKeepsWorkingTreeContent(t *testing.T) {
	dir := localReviewRepo(t)
	name := "space [literal].go"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("untracked content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	localReviewGit(t, dir, "add", "a.go")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("latest working tree\n"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := CollectLocalChanges(context.Background(), Source{WorktreePath: dir, BaseRef: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !strings.Contains(files[0].Diff, "+latest working tree") || files[1].Path != name || !strings.Contains(files[1].Diff, "+untracked content") {
		t.Fatalf("files = %+v", files)
	}
}

func TestCollectLocalChangesIncludesStagedOnlyChange(t *testing.T) {
	dir := localReviewRepo(t)
	localReviewGit(t, dir, "add", "a.go")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := CollectLocalChanges(context.Background(), Source{WorktreePath: dir, BaseRef: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.Contains(files[0].Diff, "+new") {
		t.Fatalf("files = %+v", files)
	}
}

func TestCollectLocalChangesRejectsInvalidRefs(t *testing.T) {
	dir := localReviewRepo(t)
	for _, source := range []Source{
		{WorktreePath: dir}, {WorktreePath: dir, BaseRef: "missing"}, {WorktreePath: dir, BaseRef: "--help"},
		{WorktreePath: dir, BaseRef: "HEAD", HeadRef: "missing"}, {WorktreePath: `\\server\share`, BaseRef: "HEAD"},
		{WorktreePath: dir + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(dir), BaseRef: "HEAD"},
	} {
		_, err := CollectLocalChanges(context.Background(), source)
		if !errors.Is(err, ErrWorkspaceUnavailable) {
			t.Fatalf("invalid source %+v: %v", source, err)
		}
	}
}

func TestCollectLocalChangesLinkedWorktree(t *testing.T) {
	dir := localReviewRepo(t)
	gate := filepath.Join(t.TempDir(), "gate-slot")
	localReviewGit(t, dir, "worktree", "add", "--detach", gate, "HEAD")
	if err := os.WriteFile(filepath.Join(gate, "a.go"), []byte("gate change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := CollectLocalChanges(context.Background(), Source{WorktreePath: gate, BaseRef: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.Contains(files[0].Diff, "+gate change") {
		t.Fatalf("files = %+v", files)
	}
}

func TestCollectLocalChangesCountsHunkLinesBeginningWithMarkers(t *testing.T) {
	dir := t.TempDir()
	localReviewGit(t, dir, "init")
	localReviewGit(t, dir, "config", "user.email", "review@example.test")
	localReviewGit(t, dir, "config", "user.name", "Review Test")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("keep\n--decoy\n"), 0600); err != nil {
		t.Fatal(err)
	}
	localReviewGit(t, dir, "add", ".")
	localReviewGit(t, dir, "commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("keep\n++plus\n"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := CollectLocalChanges(context.Background(), Source{WorktreePath: dir, BaseRef: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Additions != 1 || files[0].Deletions != 1 {
		t.Fatalf("line counts = %+v", files)
	}
}

func TestCollectLocalChangesUsesAgentctlStatusVocabulary(t *testing.T) {
	dir := t.TempDir()
	localReviewGit(t, dir, "init")
	localReviewGit(t, dir, "config", "user.email", "review@example.test")
	localReviewGit(t, dir, "config", "user.name", "Review Test")
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("keep.txt", "base\n")
	write("gone.txt", "gone\n")
	localReviewGit(t, dir, "add", ".")
	localReviewGit(t, dir, "commit", "-m", "base")
	base := localReviewGit(t, dir, "rev-parse", "HEAD")
	write("keep.txt", "changed\n")
	write("new.txt", "new\n")
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	localReviewGit(t, dir, "add", ".")
	localReviewGit(t, dir, "commit", "-m", "change")
	files, err := CollectLocalChanges(context.Background(), Source{WorktreePath: dir, BaseRef: base, HeadRef: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	status := make(map[string]string, len(files))
	for _, file := range files {
		status[file.Path] = file.Status
	}
	if status["keep.txt"] != "modified" || status["new.txt"] != localStatusAdded || status["gone.txt"] != "deleted" {
		t.Fatalf("statuses = %+v", status)
	}
}
