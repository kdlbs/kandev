package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/worktree"
)

func TestGatherOrphanReapRootCandidatesResolvesWhileTheyExist(t *testing.T) {
	base := t.TempDir()
	taskDir := filepath.Join(base, "task-1")
	repoDir := filepath.Join(taskDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	quickChatDir := t.TempDir()
	sessionDir := filepath.Join(quickChatDir, "sess-1")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	svc := &Service{quickChatDir: quickChatDir}
	snapshot := &taskResourceCleanupSnapshot{
		Worktrees: []*worktree.Worktree{{ID: "wt-1", Path: repoDir}},
	}

	candidates := svc.gatherOrphanReapRootCandidates(snapshot, []string{"sess-1"})

	wantRepo := resolveOrphanReapPathBestEffort(repoDir)
	wantTaskDir := resolveOrphanReapPathBestEffort(taskDir)
	wantSession := resolveOrphanReapPathBestEffort(sessionDir)
	got := map[string]bool{}
	for _, c := range candidates {
		got[c] = true
	}
	for _, want := range []string{wantRepo, wantTaskDir, wantSession} {
		if !got[want] {
			t.Errorf("expected candidate %q in %v", want, candidates)
		}
	}
}

func TestGatherOrphanReapRootCandidatesSkipsMissingPaths(t *testing.T) {
	svc := &Service{}
	snapshot := &taskResourceCleanupSnapshot{
		// Nest two levels below the (real) temp root so neither the worktree
		// path nor its would-be task directory actually exists.
		Worktrees: []*worktree.Worktree{{ID: "wt-1", Path: filepath.Join(t.TempDir(), "task-x", "does-not-exist")}},
	}
	candidates := svc.gatherOrphanReapRootCandidates(snapshot, nil)
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates for a path that never existed, got %v", candidates)
	}
}

func TestConfirmOrphanReapRootsRemoved(t *testing.T) {
	base := t.TempDir()
	stillThere := filepath.Join(base, "still-there")
	removed := filepath.Join(base, "removed")
	if err := os.MkdirAll(stillThere, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(removed, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.RemoveAll(removed); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	got := confirmOrphanReapRootsRemoved([]string{stillThere, removed})
	if len(got) != 1 || got[0] != removed {
		t.Fatalf("expected only %q confirmed removed, got %v (AC-TASKS-ORPHAN-REAP-001.3: a path not removed is never a root)", removed, got)
	}
}

func TestMergeOrphanReapRootsPersistsAcrossAttempts(t *testing.T) {
	// AC-TASKS-ORPHAN-REAP-001.1: "a root persists for the life of the job."
	existing := []string{"/tasks/task-a"}
	merged := mergeOrphanReapRoots(existing, []string{"/tasks/task-a/repo", "/tasks/task-a"})
	want := []string{"/tasks/task-a", "/tasks/task-a/repo"}
	if len(merged) != len(want) {
		t.Fatalf("mergeOrphanReapRoots = %v, want %v", merged, want)
	}
	for i, w := range want {
		if merged[i] != w {
			t.Fatalf("mergeOrphanReapRoots = %v, want %v", merged, want)
		}
	}
}

// recordOrphanReapCandidate supersedes rather than duplicates an
// earlier record for the same PID (AC-TASKS-ORPHAN-REAP-006.5).
func TestRecordOrphanReapCandidateSupersedesSamePID(t *testing.T) {
	svc := &Service{}
	snapshot := &taskResourceCleanupSnapshot{}
	svc.recordOrphanReapCandidate(snapshot, "task-a", orphanReapCandidateRecord{
		PID: 500, Outcome: orphanReapOutcomeSkipped, Reason: "first pass",
	})
	svc.recordOrphanReapCandidate(snapshot, "task-a", orphanReapCandidateRecord{
		PID: 500, Outcome: orphanReapOutcomeTerminated, Reason: "second pass",
	})

	if len(snapshot.OrphanReapRecords) != 1 {
		t.Fatalf("expected exactly one record for pid 500, got %+v", snapshot.OrphanReapRecords)
	}
	if snapshot.OrphanReapRecords[0].Outcome != orphanReapOutcomeTerminated || snapshot.OrphanReapRecords[0].Reason != "second pass" {
		t.Fatalf("expected the later record to replace the earlier one, got %+v", snapshot.OrphanReapRecords[0])
	}
}

// A stat error that does NOT confirm absence (e.g. ENOTDIR from a path
// segment that is now a file) must never be read as "removed" — only a
// confirmed os.ErrNotExist may.
func TestConfirmOrphanReapRootsRemovedFailsClosedOnAmbiguousStatError(t *testing.T) {
	base := t.TempDir()
	notADir := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ambiguous := filepath.Join(notADir, "child") // Lstat returns ENOTDIR, not ErrNotExist

	got := confirmOrphanReapRootsRemoved([]string{ambiguous})
	if len(got) != 0 {
		t.Fatalf("expected an ambiguous stat error to not be treated as removed, got %v", got)
	}
}
