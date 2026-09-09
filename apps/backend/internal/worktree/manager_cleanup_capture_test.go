package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaptureCleanupHeadOIDs_MissingWorktree(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	healthyPath := filepath.Join(t.TempDir(), "healthy")
	runGit(t, repoPath, "worktree", "add", healthyPath, "feature/pr-branch")
	healthyOID := strings.TrimSpace(runGit(t, healthyPath, "rev-parse", "HEAD"))
	mainOID := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "main"))

	survivingBranch := "feature/missing-surviving"
	packedBranch := "feature/missing-packed"
	descendantBranch := "feature/missing-descendant/child"
	tagOnlyBranch := "feature/tag-only"
	for _, branch := range []string{survivingBranch, packedBranch, descendantBranch} {
		runGit(t, repoPath, "branch", branch, "main")
	}
	runGit(t, repoPath, "tag", tagOnlyBranch, "main")
	runGit(t, repoPath, "pack-refs", "--all")

	worktrees := []*Worktree{
		{
			ID:             "missing-first",
			RepositoryPath: repoPath,
			Path:           filepath.Join(t.TempDir(), "missing-first"),
			Branch:         "feature/missing-descendant",
		},
		{
			ID:             "healthy",
			RepositoryPath: repoPath,
			Path:           healthyPath,
			Branch:         "feature/pr-branch",
		},
		{
			ID:             "surviving-branch",
			RepositoryPath: repoPath,
			Path:           filepath.Join(t.TempDir(), "surviving-branch"),
			Branch:         survivingBranch,
		},
		{
			ID:             "packed-branch",
			RepositoryPath: repoPath,
			Path:           filepath.Join(t.TempDir(), "packed-branch"),
			Branch:         packedBranch,
		},
		{
			ID:             "descendant-only",
			RepositoryPath: repoPath,
			Path:           filepath.Join(t.TempDir(), "descendant-only"),
			Branch:         "feature/missing-descendant",
		},
		{
			ID:             "tag-only",
			RepositoryPath: repoPath,
			Path:           filepath.Join(t.TempDir(), "tag-only"),
			Branch:         tagOnlyBranch,
		},
		{
			ID:             "missing-last",
			RepositoryPath: repoPath,
			Path:           filepath.Join(t.TempDir(), "missing-last"),
			Branch:         "feature/not-present",
		},
	}

	refsBefore := strings.TrimSpace(runGit(t, repoPath, "show-ref", "--heads"))
	got, err := mgr.CaptureCleanupHeadOIDs(context.Background(), worktrees)
	if err != nil {
		t.Fatalf("CaptureCleanupHeadOIDs() unexpected error: %v", err)
	}

	want := map[string]string{
		"healthy":          healthyOID,
		"surviving-branch": mainOID,
		"packed-branch":    mainOID,
	}
	if len(got) != len(want) {
		t.Fatalf("captured %d identities, want %d: %#v", len(got), len(want), got)
	}
	for id, wantOID := range want {
		if got[id] != wantOID {
			t.Errorf("identity[%q] = %q, want %q", id, got[id], wantOID)
		}
	}
	for _, id := range []string{"missing-first", "descendant-only", "tag-only", "missing-last"} {
		if _, ok := got[id]; ok {
			t.Errorf("identity[%q] unexpectedly captured for an absent exact branch", id)
		}
	}

	refsAfter := strings.TrimSpace(runGit(t, repoPath, "show-ref", "--heads"))
	if refsAfter != refsBefore {
		t.Fatalf("branch refs changed during capture:\nbefore:\n%s\nafter:\n%s", refsBefore, refsAfter)
	}
	for _, wt := range worktrees {
		if wt.Path == "" || wt.RepositoryPath == "" || wt.Branch == "" {
			t.Fatalf("test worktree %q is incomplete", wt.ID)
		}
		if _, err := os.Lstat(wt.Path); err == nil && wt.ID != "healthy" {
			t.Errorf("capture unexpectedly created path for %q", wt.ID)
		}
	}
}

func TestCaptureCleanupHeadOIDs_FailsClosed(t *testing.T) {
	t.Run("invalid repository", func(t *testing.T) {
		mgr := newCaptureTestManager(t)
		_, err := mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{{
			ID:             "invalid-repository",
			RepositoryPath: t.TempDir(),
			Path:           filepath.Join(t.TempDir(), "missing"),
			Branch:         "feature/missing",
		}})
		if err == nil {
			t.Fatal("CaptureCleanupHeadOIDs() error = nil, want invalid repository error")
		}
	})

	t.Run("git unavailable", func(t *testing.T) {
		repoPath := initGitRepoForWorktreeTest(t)
		t.Setenv("PATH", t.TempDir())
		mgr := newCaptureTestManager(t)
		_, err := mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{{
			ID:             "git-unavailable",
			RepositoryPath: repoPath,
			Path:           filepath.Join(t.TempDir(), "missing"),
			Branch:         "feature/missing",
		}})
		if err == nil {
			t.Fatal("CaptureCleanupHeadOIDs() error = nil, want unavailable git error")
		}
	})

	for _, tc := range []struct {
		name       string
		scriptBody string
	}{
		{
			name: "unexpected diagnostic",
			scriptBody: `
case "${1:-}" in
  for-each-ref)
    echo "warning: ref database is stale" >&2
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`,
		},
		{
			name: "malformed ref record",
			scriptBody: `
printf 'refs/heads/feature/missing\000not-an-object\000commit\n'
exit 0
`,
		},
		{
			name: "missing object",
			scriptBody: `
printf 'refs/heads/feature/missing\000aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\000unknown\n'
exit 0
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scriptDir := writeFakeGitScript(t, tc.scriptBody)
			t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			mgr := newCaptureTestManager(t)
			_, err := mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{{
				ID:             "invalid-output",
				RepositoryPath: t.TempDir(),
				Path:           filepath.Join(t.TempDir(), "missing"),
				Branch:         "feature/missing",
			}})
			if err == nil {
				t.Fatal("CaptureCleanupHeadOIDs() error = nil, want malformed Git output error")
			}
		})
	}

	t.Run("canceled", func(t *testing.T) {
		scriptDir := writeFakeGitScript(t, `
sleep 30
`)
		t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		mgr := newCaptureTestManager(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := mgr.CaptureCleanupHeadOIDs(ctx, []*Worktree{{
			ID:             "canceled",
			RepositoryPath: t.TempDir(),
			Path:           filepath.Join(t.TempDir(), "missing"),
			Branch:         "feature/missing",
		}})
		if err == nil {
			t.Fatal("CaptureCleanupHeadOIDs() error = nil, want cancellation error")
		}
	})

	t.Run("bounded timeout", func(t *testing.T) {
		scriptDir := writeFakeGitScript(t, `
sleep 30
`)
		t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		mgr := newCaptureTestManager(t)
		mgr.inspectTimeout = 100 * time.Millisecond
		start := time.Now()
		_, err := mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{{
			ID:             "timeout",
			RepositoryPath: t.TempDir(),
			Path:           filepath.Join(t.TempDir(), "missing"),
			Branch:         "feature/missing",
		}})
		if err == nil {
			t.Fatal("CaptureCleanupHeadOIDs() error = nil, want timeout error")
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("CaptureCleanupHeadOIDs() took %v, want less than two seconds", elapsed)
		}
	})

	t.Run("regular file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "regular-file")
		if err := os.WriteFile(path, []byte("not a worktree"), 0644); err != nil {
			t.Fatalf("write regular file: %v", err)
		}
		mgr := newCaptureTestManager(t)
		_, err := mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{{
			ID:             "regular-file",
			RepositoryPath: t.TempDir(),
			Path:           path,
			Branch:         "feature/missing",
		}})
		if err == nil {
			t.Fatal("CaptureCleanupHeadOIDs() error = nil, want regular-file error")
		}
	})

	t.Run("symlink", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "symlink")
		if err := os.Symlink(t.TempDir(), path); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		mgr := newCaptureTestManager(t)
		_, err := mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{{
			ID:             "symlink",
			RepositoryPath: t.TempDir(),
			Path:           path,
			Branch:         "feature/missing",
		}})
		if err == nil {
			t.Fatal("CaptureCleanupHeadOIDs() error = nil, want symlink error")
		}
	})

	t.Run("filesystem error", func(t *testing.T) {
		mgr := newCaptureTestManager(t)
		_, err := mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{{
			ID:             "filesystem-error",
			RepositoryPath: t.TempDir(),
			Path:           "invalid\x00path",
			Branch:         "feature/missing",
		}})
		if err == nil {
			t.Fatal("CaptureCleanupHeadOIDs() error = nil, want filesystem error")
		}
	})
}

func newCaptureTestManager(t *testing.T) *Manager {
	t.Helper()

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	return mgr
}
