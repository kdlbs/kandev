package process

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func saveTargetOriginal(scope string) string {
	return "draft\ncontext one\ncontext two\ncontext three\nowner=" + scope + "\n"
}

func runSaveTargetPath(t *testing.T, path string, symlink bool) {
	t.Helper()
	dir, tracker := newSaveTargetFixture(t, path, true)
	if symlink {
		link := filepath.Join(dir, "beta", path)
		writeFile(t, dir, "beta/target.txt", saveTargetOriginal("beta"))
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("target.txt", link); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("native symlink creation unavailable: %v", err)
			}
			t.Fatal(err)
		}
	}
	original := saveTargetOriginal("beta")
	desired := strings.Replace(original, "draft", "saved", 1)
	hash, resolution, err := tracker.ApplyFileDiff(t.Context(), filepath.Join("beta", path), path, saveTargetPatch(path, true),
		fmt.Sprintf("%x", sha256.Sum256([]byte(original))), nil)
	if err != nil || hash != fmt.Sprintf("%x", sha256.Sum256([]byte(desired))) || resolution != ResolutionApplied {
		t.Errorf("save = (%s, %s, %v); want applied desired hash", hash, resolution, err)
	}
	assertSaveTargetDisk(t, dir, path, "beta", desired)
	if symlink {
		assertFileSaveBytes(t, dir, "beta/target.txt", desired)
		info, err := os.Lstat(filepath.Join(dir, "beta", path))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("selected symlink was replaced: %v", err)
		}
	}
}

func runSaveTargetHunk(t *testing.T) {
	t.Helper()
	dir, tracker := newSaveTargetFixture(t, "one.txt", false)
	for _, scope := range []string{"", "alpha", "beta"} {
		writeFile(t, dir, filepath.Join(scope, "one.txt"), "-- one.txt\nkeep\nowner="+scope+"\n")
	}
	patch := "--- one.txt\t\n+++ one.txt\t\n@@ -1,2 +1,2 @@\n--- one.txt\n+++ one.txt\n keep\n"
	desired := "++ one.txt\nkeep\nowner=beta\n"
	hash, resolution, err := tracker.ApplyFileDiff(t.Context(), "beta/one.txt", "one.txt", patch, "", nil)
	if err != nil || resolution != ResolutionApplied || hash != fmt.Sprintf("%x", sha256.Sum256([]byte(desired))) {
		t.Errorf("header-like hunk save = (%s, %s, %v)", hash, resolution, err)
	}
	assertFileSaveBytes(t, dir, "beta/one.txt", desired)
	for _, scope := range []string{"", "alpha"} {
		assertFileSaveBytes(t, dir, filepath.Join(scope, "one.txt"), "-- one.txt\nkeep\nowner="+scope+"\n")
	}
}

// @covers AC-WORKSPACES-SAVED-FILE-CONTENT-001.2
func TestApplyFileDiff_RepositoryTargetCompatibility(t *testing.T) {
	t.Run("nested", func(t *testing.T) { runSaveTargetPath(t, "src/one.txt", false) })
	t.Run("literal_a_directory", func(t *testing.T) { runSaveTargetPath(t, "a/one.txt", false) })
	t.Run("symlink", func(t *testing.T) { runSaveTargetPath(t, "one.txt", true) })
	t.Run("header_like_hunk", runSaveTargetHunk)
	t.Run("fallbacks", runSaveTargetFallbacks)
	t.Run("cancelled", runSaveTargetCancellation)
	t.Run("invalid_path", func(t *testing.T) {
		dir, tracker := newSaveTargetFixture(t, "one.txt", false)
		hash, resolution, err := tracker.ApplyFileDiff(t.Context(), "../one.txt", "one.txt", saveTargetPatch("one.txt", true), "", nil)
		if err == nil || hash != "" || resolution != "" {
			t.Errorf("invalid path = (%s, %s, %v)", hash, resolution, err)
		}
		assertSaveTargetDisk(t, dir, "one.txt", "beta", saveTargetOriginal("beta"))
	})
}

func runSaveTargetFallbacks(t *testing.T) {
	t.Helper()
	for _, conflict := range []bool{false, true} {
		for _, content := range []string{"nil", "empty", "saved"} {
			t.Run(fmt.Sprintf("conflict=%t/content=%s", conflict, content), func(t *testing.T) {
				dir, tracker := newSaveTargetFixture(t, "one.txt", false)
				original := saveTargetOriginal("beta")
				hash := fmt.Sprintf("%x", sha256.Sum256([]byte(original)))
				patch := "not a patch\n"
				if conflict {
					hash, patch = "stale hash", saveTargetPatch("one.txt", true)
				}
				desired := ""
				var fallback *string
				if content != "nil" {
					fallback = &desired
				}
				if content == "saved" {
					desired = strings.Replace(original, "draft", "saved", 1)
				}
				gotHash, resolution, err := tracker.ApplyFileDiff(t.Context(), "beta/one.txt", "one.txt", patch, hash, fallback)
				if fallback == nil {
					desired = original
					if err == nil || gotHash != "" || resolution != "" {
						t.Errorf("rejected save = (%s, %s, %v)", gotHash, resolution, err)
					}
				} else if err != nil || resolution != ResolutionOverwritten || gotHash != fmt.Sprintf("%x", sha256.Sum256([]byte(desired))) {
					t.Errorf("overwrite = (%s, %s, %v)", gotHash, resolution, err)
				}
				assertSaveTargetDisk(t, dir, "one.txt", "beta", desired)
			})
		}
	}
}

func startSaveTarget(ctx context.Context, tracker *WorkspaceTracker, scope string, requests *sync.WaitGroup) <-chan fileSaveResult {
	result := make(chan fileSaveResult, 1)
	requests.Add(1)
	go func() {
		defer requests.Done()
		original := saveTargetOriginal(scope)
		desired := strings.Replace(original, "draft", "saved", 1)
		hash, resolution, err := tracker.ApplyFileDiff(ctx, filepath.Join(scope, "one.txt"), "one.txt", saveTargetPatch("one.txt", true),
			fmt.Sprintf("%x", sha256.Sum256([]byte(original))), &desired)
		result <- fileSaveResult{hash, resolution, err}
	}()
	return result
}

func runSaveTargetCancellation(t *testing.T) {
	t.Helper()
	dir, tracker := newSaveTargetFixture(t, "one.txt", false)
	ctx, release, pending := holdFileSaveAdmission(t)
	cancelledCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	beta := startSaveTarget(cancelledCtx, tracker, "beta", pending)
	awaitQueuedFileSaves(t, ctx, 1)
	alpha := startSaveTarget(ctx, tracker, "alpha", pending)
	awaitQueuedFileSaves(t, ctx, 2)
	cancel()
	b := awaitFileSaveResult(t, ctx, beta)
	if !errors.Is(b.err, context.Canceled) || b.hash != "" || b.resolution != "" {
		t.Errorf("cancelled save = %+v", b)
	}
	release()
	a := awaitFileSaveResult(t, ctx, alpha)
	pending.Wait()
	desired := strings.Replace(saveTargetOriginal("alpha"), "draft", "saved", 1)
	if a.err != nil || a.resolution != ResolutionApplied || a.hash != fmt.Sprintf("%x", sha256.Sum256([]byte(desired))) {
		t.Errorf("peer save = %+v", a)
	}
	assertSaveTargetDisk(t, dir, "one.txt", "alpha", desired)
	patches, err := filepath.Glob(filepath.Join(dir, ".kandev-patch-*"))
	if err != nil || len(patches) != 0 {
		t.Errorf("patch cleanup = %v, %v", patches, err)
	}
}

// This fixture follows generateUnifiedDiff's diff 8.0.3 formatter arguments.
func saveTargetPatch(path string, editor bool) string {
	header := "--- " + path + "\n+++ " + path + "\n"
	if editor {
		header = "Index: " + path + "\n===================================================================\n--- " + path + "\t\n+++ " + path + "\t\n"
	}
	return header + "@@ -1,4 +1,4 @@\n-draft\n+saved\n context one\n context two\n context three\n"
}

func newSaveTargetFixture(t *testing.T, path string, initialized bool) (string, *WorkspaceTracker) {
	t.Helper()
	gitConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(gitConfig, []byte("[core]\n\tautocrlf = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"", "alpha", "beta"} {
		repo := filepath.Join(dir, scope)
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, repo, path, saveTargetOriginal(scope))
		if initialized && scope != "" {
			runGit(t, repo, "init", "--initial-branch=main")
		}
	}
	writeFile(t, dir, "neighbor.txt", "unchanged neighbor\n")
	tracker := NewWorkspaceTracker(dir, newTestLogger(t))
	tracker.SetGitEnvironment(os.Environ())
	t.Cleanup(tracker.Stop)
	return dir, tracker
}

func assertSaveTargetDisk(t *testing.T, dir, path, selected, desired string) {
	t.Helper()
	for _, scope := range []string{"", "alpha", "beta"} {
		want := saveTargetOriginal(scope)
		if scope == selected {
			want = desired
		}
		assertFileSaveBytes(t, dir, filepath.Join(scope, path), want)
	}
	assertFileSaveBytes(t, dir, "neighbor.txt", "unchanged neighbor\n")
}

func assertSaveTargetEvent(t *testing.T, sub types.WorkspaceStreamSubscriber, path string) {
	t.Helper()
	select {
	case msg := <-sub:
		got := msg.FileChange
		if got == nil || filepath.ToSlash(got.Path) != filepath.ToSlash(path) || got.Operation != types.FileOpWrite || got.RepositoryName != "" {
			t.Errorf("write event fields = %+v; envelope = %+v; want root tracker write for %s", got, msg, path)
		}
	default:
		t.Error("save returned without the immediate write event")
	}
}

// @covers AC-WORKSPACES-SAVED-FILE-CONTENT-001.2
func TestApplyFileDiff_RepositoryTarget(t *testing.T) {
	for _, initialized := range []bool{false, true} {
		for _, editor := range []bool{false, true} {
			for _, fallback := range []bool{false, true} {
				for _, scope := range []string{"", "alpha", "beta"} {
					name := fmt.Sprintf("git=%t/editor=%t/fallback=%t/repo=%s", initialized, editor, fallback, scope)
					t.Run(name, func(t *testing.T) {
						dir, tracker := newSaveTargetFixture(t, "one.txt", initialized)
						original := saveTargetOriginal(scope)
						desired := strings.Replace(original, "draft", "saved", 1)
						var desiredContent *string
						if fallback {
							desiredContent = &desired
						}
						sub := tracker.SubscribeWorkspaceStream()
						t.Cleanup(func() { tracker.UnsubscribeWorkspaceStream(sub) })
						path := filepath.Join(scope, "one.txt")
						hash, resolution, err := tracker.ApplyFileDiff(t.Context(), path, "one.txt", saveTargetPatch("one.txt", editor),
							fmt.Sprintf("%x", sha256.Sum256([]byte(original))), desiredContent)
						wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(desired)))
						if err != nil || hash != wantHash || resolution != ResolutionApplied {
							t.Errorf("selected %s save = (%s, %s, %v); want applied hash %s", path, hash, resolution, err, wantHash)
						}
						assertSaveTargetDisk(t, dir, "one.txt", scope, desired)
						assertSaveTargetEvent(t, sub, path)
					})
				}
			}
		}
	}
}
