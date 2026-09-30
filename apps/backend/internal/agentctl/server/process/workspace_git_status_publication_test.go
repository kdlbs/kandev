package process

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWorkspaceTrackerRejectsChangedGitIdentityDuringEnrichment(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, string, *WorkspaceTracker)
		mutate  func(*testing.T, string)
	}{
		{
			name: "head move",
			mutate: func(t *testing.T, repo string) {
				t.Helper()
				runGit(t, repo, "commit", "--allow-empty", "-m", "move HEAD")
			},
		},
		{
			name: "index replacement",
			mutate: func(t *testing.T, repo string) {
				t.Helper()
				runGit(t, repo, "add", "README.md")
			},
		},
		{
			name: "comparison ref move",
			prepare: func(t *testing.T, repo string, tracker *WorkspaceTracker) {
				t.Helper()
				tracker.SetBaseBranch("staging")
				runGit(t, repo, "branch", "staging", "HEAD")
			},
			mutate: func(t *testing.T, repo string) {
				t.Helper()
				next := strings.TrimSpace(runGit(t, repo, "commit-tree", "HEAD^{tree}", "-p", "HEAD", "-m", "comparison ref move"))
				runGit(t, repo, "update-ref", "refs/heads/staging", next)
			},
		},
		{
			name: "nested submodule head move",
			prepare: func(t *testing.T, repo string, _ *WorkspaceTracker) {
				t.Helper()
				submoduleDir, cleanup := setupTestRepo(t)
				t.Cleanup(cleanup)
				runGit(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", submoduleDir, "nested")
				runGit(t, repo, "commit", "-m", "add nested submodule")
				runGit(t, repo, "-C", "nested", "commit", "--allow-empty", "-m", "initial nested head")
			},
			mutate: func(t *testing.T, repo string) {
				t.Helper()
				runGit(t, repo, "-C", "nested", "commit", "--allow-empty", "-m", "changed nested head")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repoDir, cleanup := setupTestRepo(t)
			defer cleanup()
			writeFile(t, repoDir, "README.md", "pending detail\n")
			tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
			t.Cleanup(tracker.Stop)
			if test.prepare != nil {
				test.prepare(t, repoDir, tracker)
			}
			capture, err := tracker.captureBasicGitStatus(context.Background())
			if err != nil {
				t.Fatalf("basic capture: %v", err)
			}
			if capture.job == nil {
				t.Fatal("basic capture did not retain enrichment evidence")
			}
			defer capture.job.indexCleanup()
			accepted, basicPublished := tracker.publishGitStatus(capture.status, 1, capture.fingerprint)
			if !basicPublished {
				t.Fatal("basic snapshot was not accepted before enrichment")
			}
			capture.job.status = cloneGitStatusUpdate(accepted)
			capture.job.correctionPermitted = false
			test.mutate(t, repoDir)
			if err := tracker.validateGitStatusEnrichment(context.Background(), capture.job); !errors.Is(err, errGitStatusEvidenceChanged) {
				t.Fatalf("enrichment evidence validation error = %v, want changed evidence", err)
			}
		})
	}
}

func TestWorkspaceTrackerCapturesNestedSubmoduleHead(t *testing.T) {
	parent, cleanupParent := setupTestRepo(t)
	defer cleanupParent()
	submodule, cleanupSubmodule := setupTestRepo(t)
	defer cleanupSubmodule()
	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", submodule, "nested")
	runGit(t, parent, "commit", "-m", "add nested submodule")
	runGit(t, parent, "-C", "nested", "commit", "--allow-empty", "-m", "advance submodule")
	wantHead := strings.TrimSpace(runGit(t, parent, "-C", "nested", "rev-parse", "HEAD"))

	tracker := NewWorkspaceTracker(parent, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	capture, err := tracker.captureBasicGitStatus(context.Background())
	if err != nil {
		t.Fatalf("capture basic submodule status: %v", err)
	}
	if capture.job == nil {
		t.Fatal("capture did not retain submodule evidence")
	}
	defer capture.job.indexCleanup()
	evidence, ok := capture.job.fileEvidence["nested"]
	if !ok || !evidence.IsSubmodule || evidence.SubmoduleHead != wantHead {
		t.Fatalf("nested submodule evidence = %+v, want observed HEAD %s", evidence, wantHead)
	}

	changedEvidence := evidence
	changedEvidence.SubmoduleHead = strings.Repeat("f", 40)
	changed := map[string]gitStatusFileEvidence{"nested": changedEvidence}
	if sameGitStatusFileEvidence(map[string]gitStatusFileEvidence{"nested": evidence}, changed) {
		t.Fatal("file evidence considered a changed nested submodule HEAD equivalent")
	}
}

func TestWorkspaceTrackerRejectsSubmoduleHeadMoveBeforeFinalPublication(t *testing.T) {
	parent, cleanupParent := setupTestRepo(t)
	defer cleanupParent()
	submodule, cleanupSubmodule := setupTestRepo(t)
	defer cleanupSubmodule()
	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", submodule, "nested")
	runGit(t, parent, "commit", "-m", "add nested submodule")
	runGit(t, parent, "-C", "nested", "commit", "--allow-empty", "-m", "initial nested head")

	tracker := NewWorkspaceTracker(parent, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	capture, err := tracker.captureBasicGitStatus(context.Background())
	if err != nil {
		t.Fatalf("capture basic status: %v", err)
	}
	if capture.job == nil {
		t.Fatal("capture did not retain enrichment job")
	}
	defer capture.job.indexCleanup()
	accepted, published := tracker.publishGitStatus(capture.status, 1, capture.fingerprint)
	if !published {
		t.Fatal("basic status was not published")
	}
	capture.job.status = cloneGitStatusUpdate(accepted)
	capture.job.correctionPermitted = false
	tracker.gitStatusBeforeFinalValidation = func() {
		runGit(t, parent, "-C", "nested", "commit", "--allow-empty", "-m", "move after enrichment")
	}

	err = tracker.runGitStatusEnrichment(capture.job)
	if !errors.Is(err, errGitStatusEvidenceChanged) {
		t.Fatalf("enrichment result = %v, want stale submodule evidence rejected", err)
	}
	current, err := tracker.GetGitStatusReplay(context.Background())
	if err != nil {
		t.Fatalf("read accepted status after rejection: %v", err)
	}
	if current.SnapshotRevision != accepted.SnapshotRevision || current.DetailState != gitStatusDetailPending {
		t.Fatalf("stale enrichment changed accepted status: %+v", current)
	}
}
