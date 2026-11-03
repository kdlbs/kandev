package process

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

func newGeneratedBranchOperator(t *testing.T, branch string) (string, *GitOperator) {
	t.Helper()
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	runGit(t, repoDir, "checkout", "-q", "-b", branch)
	return repoDir, NewGitOperator(repoDir, newTestLogger(t), nil)
}

func renameGeneratedBranch(t *testing.T, operator *GitOperator, expectedBranch, newName string) *GitOperationResult {
	t.Helper()
	result, err := operator.RenameGeneratedBranch(context.Background(), expectedBranch, newName)
	if err != nil {
		t.Fatalf("RenameGeneratedBranch(%q -> %q): %v", expectedBranch, newName, err)
	}
	if result == nil || !result.Success {
		t.Fatalf("RenameGeneratedBranch(%q -> %q) = %+v, want success", expectedBranch, newName, result)
	}
	return result
}

func assertUnavailableComparison(t *testing.T, comparison turnchanges.CheckpointComparison, reason turnchanges.ReasonCode) {
	t.Helper()
	if comparison.ComparisonBasis != turnchanges.ComparisonHistoryUnavailable ||
		comparison.Complete || comparison.FileCount != 0 || comparison.ComparisonReason != reason {
		t.Fatalf("comparison = %+v, want unavailable with reason %q", comparison, reason)
	}
}

func TestTurnCheckpointGeneratedBranchRenameWithEdits(t *testing.T) {
	repoDir, operator := newGeneratedBranchOperator(t, "feature/provisional")
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
	if start.Provenance.HeadBindingID == "" {
		t.Fatal("start receipt has no head binding id")
	}
	renameGeneratedBranch(t, operator, "feature/provisional", "feature/final")
	writeFile(t, repoDir, "turn.txt", "one\n")
	end := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
	if end.Provenance.HeadBindingID != start.Provenance.HeadBindingID {
		t.Fatalf("binding ids = %q -> %q, want the verified rename to transfer identity",
			start.Provenance.HeadBindingID, end.Provenance.HeadBindingID)
	}
	if end.Provenance.HeadRef != "refs/heads/feature/final" || start.Provenance.HeadRef != "refs/heads/feature/provisional" {
		t.Fatalf("endpoint refs = %q -> %q", start.Provenance.HeadRef, end.Provenance.HeadRef)
	}

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonDirect || !comparison.Complete ||
		comparison.FileCount != 1 || string(comparison.Files[0].PathBytes) != "turn.txt" {
		t.Fatalf("comparison = %+v, want one direct edited file", comparison)
	}
	if comparison.Files[0].Added == nil || *comparison.Files[0].Added != 1 ||
		comparison.Files[0].Deleted == nil || *comparison.Files[0].Deleted != 0 {
		t.Fatalf("file counts = %+v, want +1 -0", comparison.Files[0])
	}

	request := acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, "")
	export, err := operator.ExportTurnCheckpoint(context.Background(), exportRequestForPair(request))
	if err != nil {
		t.Fatalf("export renamed interval: %v", err)
	}
	if !export.Complete || len(export.Files) != 1 || string(export.Files[0].NewRendering) != "one\n" ||
		len(export.Files[0].OldRendering) != 0 {
		t.Fatalf("export = %+v, want exact added content", export)
	}

	// A later workspace edit must not alter the retained historical interval.
	fresh := NewGitOperator(repoDir, newTestLogger(t), nil)
	writeFile(t, repoDir, "turn.txt", "one\nlater edit\n")
	replayed := compareHistoryEndpoints(t, fresh, changeSetID, checkoutID)
	if replayed.ComparisonBasis != turnchanges.ComparisonDirect || replayed.FileCount != 1 ||
		string(replayed.Files[0].PathBytes) != "turn.txt" {
		t.Fatalf("fresh-operator replay = %+v, want retained historical file", replayed)
	}
}

func TestTurnCheckpointBranchBindingRejectsUnprovenChanges(t *testing.T) {
	t.Run("same commit switch", func(t *testing.T) {
		_, operator := newGeneratedBranchOperator(t, "feature/provisional")
		changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
		start := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
		runGit(t, operator.workDir, "checkout", "-q", "-b", "feature/other")
		end := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
		if start.Provenance.HeadCommitOID != end.Provenance.HeadCommitOID {
			t.Fatalf("fixture moved HEAD: %s -> %s", start.Provenance.HeadCommitOID, end.Provenance.HeadCommitOID)
		}
		if start.Provenance.HeadBindingID == end.Provenance.HeadBindingID {
			t.Fatal("same-commit switch reused the previous binding")
		}
		assertUnavailableComparison(t, compareHistoryEndpoints(t, operator, changeSetID, checkoutID),
			turnchanges.ReasonHistoryContextChanged)
	})

	t.Run("verified rename then recreated original branch", func(t *testing.T) {
		repoDir, operator := newGeneratedBranchOperator(t, "feature/a")
		changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
		start := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
		renameGeneratedBranch(t, operator, "feature/a", "feature/b")
		runGit(t, repoDir, "checkout", "-q", "-b", "feature/a")
		end := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
		if start.Provenance.HeadCommitOID != end.Provenance.HeadCommitOID {
			t.Fatalf("fixture moved HEAD: %s -> %s", start.Provenance.HeadCommitOID, end.Provenance.HeadCommitOID)
		}
		if start.Provenance.HeadRef != end.Provenance.HeadRef {
			t.Fatalf("fixture refs differ: %q -> %q", start.Provenance.HeadRef, end.Provenance.HeadRef)
		}
		if start.Provenance.HeadBindingID == end.Provenance.HeadBindingID {
			t.Fatal("recreated branch reused the previous binding")
		}
		assertUnavailableComparison(t, compareHistoryEndpoints(t, operator, changeSetID, checkoutID),
			turnchanges.ReasonHistoryContextChanged)
	})

	t.Run("generic rename then recreated intermediate branch", func(t *testing.T) {
		repoDir, operator := newGeneratedBranchOperator(t, "feature/a")
		changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
		start := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
		renameGeneratedBranch(t, operator, "feature/a", "feature/b")
		result, err := operator.RenameBranch(context.Background(), "feature/c")
		if err != nil || result == nil || !result.Success {
			t.Fatalf("generic rename = %+v, %v", result, err)
		}
		runGit(t, repoDir, "checkout", "-q", "-b", "feature/b")
		end := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
		if start.Provenance.HeadCommitOID != end.Provenance.HeadCommitOID {
			t.Fatalf("fixture moved HEAD: %s -> %s", start.Provenance.HeadCommitOID, end.Provenance.HeadCommitOID)
		}
		if start.Provenance.HeadBindingID == end.Provenance.HeadBindingID {
			t.Fatal("generic rename left the stale binding attached to the recreated branch")
		}
		assertUnavailableComparison(t, compareHistoryEndpoints(t, operator, changeSetID, checkoutID),
			turnchanges.ReasonHistoryContextChanged)
	})

	t.Run("generic rename", func(t *testing.T) {
		_, operator := newGeneratedBranchOperator(t, "feature/provisional")
		changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
		captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
		result, err := operator.RenameBranch(context.Background(), "feature/final")
		if err != nil || result == nil || !result.Success {
			t.Fatalf("generic rename = %+v, %v", result, err)
		}
		captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
		assertUnavailableComparison(t, compareHistoryEndpoints(t, operator, changeSetID, checkoutID),
			turnchanges.ReasonHistoryContextChanged)
	})

	t.Run("missing and malformed ids", func(t *testing.T) {
		valid := uuid.NewString()
		base := turnchanges.Provenance{Version: turnchanges.ProvenanceVersion2, HeadRef: "refs/heads/a", HashAlgorithm: "sha1"}
		cases := []struct {
			name  string
			start turnchanges.Provenance
			end   turnchanges.Provenance
			want  bool
		}{
			{name: "same id relaxes ref name", start: withBinding(base, "refs/heads/a", valid), end: withBinding(base, "refs/heads/b", valid), want: false},
			{name: "missing ids", start: withBinding(base, "refs/heads/a", ""), end: withBinding(base, "refs/heads/b", ""), want: true},
			{name: "malformed ids", start: withBinding(base, "refs/heads/a", "not-a-uuid"), end: withBinding(base, "refs/heads/b", "not-a-uuid"), want: true},
			{name: "different ids", start: withBinding(base, "refs/heads/a", valid), end: withBinding(base, "refs/heads/b", uuid.NewString()), want: true},
			{name: "same ref different ids", start: withBinding(base, "refs/heads/a", valid), end: withBinding(base, "refs/heads/a", uuid.NewString()), want: true},
			{name: "same ref legacy receipt", start: withBinding(base, "refs/heads/a", valid), end: withBinding(base, "refs/heads/a", ""), want: false},
			{name: "same ref", start: withBinding(base, "refs/heads/a", ""), end: withBinding(base, "refs/heads/a", ""), want: false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := turnCheckpointContextChange(&tc.start, &tc.end) == turnchanges.ReasonHistoryContextChanged
				if got != tc.want {
					t.Fatalf("context changed = %t, want %t", got, tc.want)
				}
			})
		}
	})

	t.Run("target change", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)
		target, tracker, operator, ref := setupHistoryTarget(t, repoDir)
		changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
		captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
		changed := target
		changed.TargetBranch = "develop"
		changedRef := changed.ComparisonRef()
		runGit(t, repoDir, "update-ref", changedRef, strings.TrimSpace(runGit(t, repoDir, "rev-parse", ref)))
		tracker.SetComparisonTargetReady(&changed, changedRef)
		captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
		assertUnavailableComparison(t, compareHistoryEndpoints(t, operator, changeSetID, checkoutID),
			turnchanges.ReasonHistoryContextChanged)
	})

	t.Run("active operation", func(t *testing.T) {
		start := &turnchanges.Provenance{Version: turnchanges.ProvenanceVersion2, OperationState: turnCheckpointOperationNone}
		end := &turnchanges.Provenance{Version: turnchanges.ProvenanceVersion2, OperationState: turnCheckpointOperationMerge}
		if reason := turnCheckpointOperationReason(start, end); reason != turnchanges.ReasonHistoryContextChanged {
			t.Fatalf("operation reason = %q, want history context changed", reason)
		}
	})
}

func withBinding(base turnchanges.Provenance, ref, id string) turnchanges.Provenance {
	provenance := base
	provenance.HeadRef = ref
	provenance.HeadBindingID = id
	return provenance
}

func TestTurnCheckpointBranchBindingRenameRace(t *testing.T) {
	t.Run("expected ref mismatch refuses mutation", func(t *testing.T) {
		_, operator := newGeneratedBranchOperator(t, "feature/provisional")
		changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
		start := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)

		result, err := operator.RenameGeneratedBranch(context.Background(), "feature/other", "feature/final")
		if err != nil {
			t.Fatalf("RenameGeneratedBranch returned error: %v", err)
		}
		if result == nil || result.Success || !strings.Contains(result.Error, "feature/other") {
			t.Fatalf("mismatched rename = %+v, want refusal naming the expected branch", result)
		}
		if branch := strings.TrimSpace(runGit(t, operator.workDir, "rev-parse", "--abbrev-ref", "HEAD")); branch != "feature/provisional" {
			t.Fatalf("branch = %q, want unchanged provisional branch", branch)
		}
		end := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
		if end.Provenance.HeadBindingID != start.Provenance.HeadBindingID {
			t.Fatal("refused rename changed the live binding")
		}
		comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
		if comparison.ComparisonBasis != turnchanges.ComparisonDirect || comparison.FileCount != 0 {
			t.Fatalf("comparison after refused rename = %+v, want direct zero-file", comparison)
		}
	})

	t.Run("invalid rename leaves continuity intact", func(t *testing.T) {
		_, operator := newGeneratedBranchOperator(t, "feature/provisional")
		start := captureHistoryEndpoint(t, operator, uuid.NewString(), uuid.NewString(), turnchanges.CheckpointStart)
		if _, err := operator.RenameGeneratedBranch(context.Background(), "feature/provisional", "bad..name"); !errors.Is(err, ErrInvalidBranchName) {
			t.Fatalf("invalid rename error = %v, want invalid branch name", err)
		}
		if branch := strings.TrimSpace(runGit(t, operator.workDir, "rev-parse", "--abbrev-ref", "HEAD")); branch != "feature/provisional" {
			t.Fatalf("branch = %q, want unchanged provisional branch", branch)
		}
		if start.Provenance.HeadBindingID == "" {
			t.Fatal("start receipt lost its binding")
		}
	})

	t.Run("capture and rename serialize", func(t *testing.T) {
		_, operator := newGeneratedBranchOperator(t, "feature/provisional")
		ctx := context.Background()
		gitDir, _, err := operator.turnCheckpointRepository(ctx)
		if err != nil {
			t.Fatalf("resolve checkpoint repository: %v", err)
		}
		unlock, err := acquireTurnCheckpointLock(ctx, gitDir)
		if err != nil {
			t.Fatalf("acquire capture lock: %v", err)
		}
		done := make(chan *GitOperationResult, 1)
		renameErr := make(chan error, 1)
		go func() {
			result, callErr := operator.RenameGeneratedBranch(ctx, "feature/provisional", "feature/final")
			done <- result
			renameErr <- callErr
		}()
		select {
		case result := <-done:
			t.Fatalf("rename completed while the capture lock was held: %+v", result)
		case <-time.After(150 * time.Millisecond):
		}
		unlock()
		select {
		case result := <-done:
			if callErr := <-renameErr; callErr != nil || result == nil || !result.Success {
				t.Fatalf("rename after unlock = %+v, %v", result, callErr)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("rename did not resume after the capture lock was released")
		}
	})
}

func TestTurnCheckpointBranchBindingReceiptReplay(t *testing.T) {
	t.Run("finalized receipts replay with a fresh operator", func(t *testing.T) {
		repoDir, operator := newGeneratedBranchOperator(t, "feature/provisional")
		changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
		captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
		renameGeneratedBranch(t, operator, "feature/provisional", "feature/final")
		writeFile(t, repoDir, "turn.txt", "replayed\n")
		captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

		fresh := NewGitOperator(repoDir, newTestLogger(t), nil)
		comparison := compareHistoryEndpoints(t, fresh, changeSetID, checkoutID)
		if comparison.ComparisonBasis != turnchanges.ComparisonDirect || comparison.FileCount != 1 ||
			string(comparison.Files[0].PathBytes) != "turn.txt" {
			t.Fatalf("fresh-operator comparison = %+v", comparison)
		}
	})

	t.Run("replacement between endpoints loses unrecorded continuity", func(t *testing.T) {
		repoDir, first := newGeneratedBranchOperator(t, "feature/provisional")
		changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
		start := captureHistoryEndpoint(t, first, changeSetID, checkoutID, turnchanges.CheckpointStart)
		replacement := NewGitOperator(repoDir, newTestLogger(t), nil)
		renameGeneratedBranch(t, replacement, "feature/provisional", "feature/final")
		writeFile(t, repoDir, "turn.txt", "lost continuity\n")
		end := captureHistoryEndpoint(t, replacement, changeSetID, checkoutID, turnchanges.CheckpointEnd)
		if start.Provenance.HeadBindingID == end.Provenance.HeadBindingID {
			t.Fatal("replacement operator restored an old binding from the start receipt")
		}
		assertUnavailableComparison(t, compareHistoryEndpoints(t, replacement, changeSetID, checkoutID),
			turnchanges.ReasonHistoryContextChanged)
	})

	t.Run("older receipts without ids keep the strict ref check", func(t *testing.T) {
		start := &turnchanges.Provenance{Version: turnchanges.ProvenanceVersion2, HeadRef: "refs/heads/a"}
		end := &turnchanges.Provenance{Version: turnchanges.ProvenanceVersion2, HeadRef: "refs/heads/b"}
		if reason := turnCheckpointContextChange(start, end); reason != turnchanges.ReasonHistoryContextChanged {
			t.Fatalf("legacy receipt reason = %q, want history context changed", reason)
		}
	})
}
