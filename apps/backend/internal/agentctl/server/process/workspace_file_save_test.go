package process

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
)

type fileSaveCase struct {
	path, original, desired, patch string
}

func distinctFileSaves() []fileSaveCase {
	return []fileSaveCase{
		{"alpha.txt", "alpha start\nalpha old\nalpha end\n", "alpha start\nalpha new\nalpha end\n",
			"--- alpha.txt\n+++ alpha.txt\n@@ -1,3 +1,3 @@\n alpha start\n-alpha old\n+alpha new\n alpha end\n"},
		{"beta.txt", "beta start\nbeta old\nbeta end\n", "beta start\nbeta new\nbeta end\n",
			"--- beta.txt\n+++ beta.txt\n@@ -1,3 +1,3 @@\n beta start\n-beta old\n+beta new\n beta end\n"},
	}
}

func newFileSaveFixture(t *testing.T) (string, *WorkspaceTracker) {
	t.Helper()
	dir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	for _, save := range distinctFileSaves() {
		writeFile(t, dir, save.path, save.original)
	}
	writeFile(t, dir, "neighbor.txt", "neighbor stays intact\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "seed distinct files")
	return dir, NewWorkspaceTracker(dir, newTestLogger(t))
}

type fileSaveResult struct {
	hash, resolution string
	err              error
}

func holdFileSaveAdmission(t *testing.T) (context.Context, func(), *sync.WaitGroup) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	restore := subproc.Git().SetCapForTest(1)
	t.Cleanup(restore)
	release, err := subproc.AcquireGit(ctx, subproc.GitInteractive)
	if err != nil {
		t.Fatal(err)
	}
	requests := &sync.WaitGroup{}
	t.Cleanup(func() {
		cancel()
		release()
		requests.Wait()
	})
	return ctx, release, requests
}

func awaitQueuedFileSaves(t *testing.T, ctx context.Context, want int) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for subproc.AdmissionSnapshot().Waiters != want {
		select {
		case <-ctx.Done():
			t.Fatalf("expected %d queued file saves: %v", want, ctx.Err())
		case <-ticker.C:
		}
	}
}

func startFileSave(ctx context.Context, tracker *WorkspaceTracker, save fileSaveCase, requests *sync.WaitGroup) <-chan fileSaveResult {
	result := make(chan fileSaveResult, 1)
	requests.Add(1)
	go func() {
		defer requests.Done()
		hash, resolution, err := tracker.ApplyFileDiff(ctx, save.path, save.path, save.patch,
			fmt.Sprintf("%x", sha256.Sum256([]byte(save.original))), &save.desired)
		result <- fileSaveResult{hash, resolution, err}
	}()
	return result
}

func awaitFileSaveResult(t *testing.T, ctx context.Context, result <-chan fileSaveResult) fileSaveResult {
	t.Helper()
	select {
	case got := <-result:
		return got
	case <-ctx.Done():
		t.Fatalf("file save did not settle: %v", ctx.Err())
		return fileSaveResult{}
	}
}

func assertSavedFile(t *testing.T, dir string, save fileSaveCase, got fileSaveResult) {
	t.Helper()
	if got.err != nil {
		t.Errorf("%s save failed: %v", save.path, got.err)
		return
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(save.desired)))
	if got.hash != wantHash || got.resolution != ResolutionApplied {
		t.Errorf("%s acknowledged hash=%s resolution=%s; want hash=%s applied", save.path, got.hash, got.resolution, wantHash)
	}
	assertFileSaveBytes(t, dir, save.path, save.desired)
}

func assertFileSaveBytes(t *testing.T, dir, path, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s disk bytes = %q; want %q", path, got, want)
	}
}

func TestApplyFileDiff_ConcurrentDistinctFiles(t *testing.T) {
	for _, independent := range []bool{false, true} {
		name := "shared_tracker"
		if independent {
			name = "independent_trackers"
		}
		t.Run(name, func(t *testing.T) {
			dir, first := newFileSaveFixture(t)
			second := first
			if independent {
				second = NewWorkspaceTracker(dir, newTestLogger(t))
			}
			ctx, release, requests := holdFileSaveAdmission(t)
			saves := distinctFileSaves()
			alpha := startFileSave(ctx, first, saves[0], requests)
			awaitQueuedFileSaves(t, ctx, 1)
			beta := startFileSave(ctx, second, saves[1], requests)
			awaitQueuedFileSaves(t, ctx, 2)
			release()
			a := awaitFileSaveResult(t, ctx, alpha)
			b := awaitFileSaveResult(t, ctx, beta)
			requests.Wait()
			assertSavedFile(t, dir, saves[0], a)
			assertSavedFile(t, dir, saves[1], b)
			assertFileSaveBytes(t, dir, "neighbor.txt", "neighbor stays intact\n")
		})
	}
}

func TestApplyFileDiff_SequentialDistinctFiles(t *testing.T) {
	dir, tracker := newFileSaveFixture(t)
	for _, save := range distinctFileSaves() {
		t.Run(save.path, func(t *testing.T) {
			hash, resolution, err := tracker.ApplyFileDiff(t.Context(), save.path, save.path, save.patch,
				fmt.Sprintf("%x", sha256.Sum256([]byte(save.original))), &save.desired)
			assertSavedFile(t, dir, save, fileSaveResult{hash, resolution, err})
		})
	}
	assertFileSaveBytes(t, dir, "neighbor.txt", "neighbor stays intact\n")
}

func assertNoFileSavePatches(t *testing.T, dir string) {
	t.Helper()
	patches, err := filepath.Glob(filepath.Join(dir, ".kandev-patch-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(patches) != 0 {
		t.Errorf("save left temporary patches: %v", patches)
	}
	assertFileSaveBytes(t, dir, ".kandev-patch.tmp", "unrelated legacy patch\n")
	assertFileSaveBytes(t, dir, "neighbor.txt", "neighbor stays intact\n")
}

func TestApplyFileDiff_PatchCleanup(t *testing.T) {
	const invalidPatch = "this is not a patch\n"
	for _, name := range []string{"applied", "overwritten", "overwritten_empty", "rejected"} {
		t.Run(name, func(t *testing.T) {
			dir, tracker := newFileSaveFixture(t)
			writeFile(t, dir, ".kandev-patch.tmp", "unrelated legacy patch\n")
			save := distinctFileSaves()[0]
			patch, desired := save.patch, &save.desired
			wantContent, wantResolution := save.desired, ResolutionApplied
			switch name {
			case "overwritten":
				patch, wantResolution = invalidPatch, ResolutionOverwritten
			case "overwritten_empty":
				save.desired = ""
				patch, wantContent, wantResolution = invalidPatch, "", ResolutionOverwritten
			case "rejected":
				patch, desired, wantContent, wantResolution = invalidPatch, nil, save.original, ""
			}
			hash, resolution, err := tracker.ApplyFileDiff(t.Context(), save.path, save.path, patch,
				fmt.Sprintf("%x", sha256.Sum256([]byte(save.original))), desired)
			if name == "rejected" {
				if err == nil || hash != "" || resolution != "" {
					t.Errorf("rejected save = (%s, %s, %v)", hash, resolution, err)
				}
			} else if err != nil || resolution != wantResolution || hash != fmt.Sprintf("%x", sha256.Sum256([]byte(wantContent))) {
				t.Errorf("save = (%s, %s, %v); want content hash and %s", hash, resolution, err, wantResolution)
			}
			assertFileSaveBytes(t, dir, save.path, wantContent)
			assertFileSaveBytes(t, dir, "beta.txt", distinctFileSaves()[1].original)
			assertNoFileSavePatches(t, dir)
		})
	}
}

func TestApplyFileDiff_CancelledQueuedSave(t *testing.T) {
	dir, tracker := newFileSaveFixture(t)
	writeFile(t, dir, ".kandev-patch.tmp", "unrelated legacy patch\n")
	ctx, release, requests := holdFileSaveAdmission(t)
	cancelledCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	saves := distinctFileSaves()
	alpha := startFileSave(cancelledCtx, tracker, saves[0], requests)
	awaitQueuedFileSaves(t, ctx, 1)
	beta := startFileSave(ctx, tracker, saves[1], requests)
	awaitQueuedFileSaves(t, ctx, 2)
	cancel()
	a := awaitFileSaveResult(t, ctx, alpha)
	if !errors.Is(a.err, context.Canceled) || a.hash != "" || a.resolution != "" {
		t.Errorf("cancelled save = %+v; want cancellation without success", a)
	}
	release()
	b := awaitFileSaveResult(t, ctx, beta)
	requests.Wait()
	assertFileSaveBytes(t, dir, saves[0].path, saves[0].original)
	assertSavedFile(t, dir, saves[1], b)
	assertNoFileSavePatches(t, dir)
}
