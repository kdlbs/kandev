package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
)

func TestTurnCheckpointProvenanceReceiptSurvivesHeadMove(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	headBefore := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	start, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture start: %v", err)
	}
	if start.Provenance == nil || start.Provenance.Version != turnchanges.ProvenanceVersion2 {
		t.Fatalf("start provenance = %+v", start.Provenance)
	}
	if start.Provenance.HeadCommitOID != headBefore || start.Provenance.HeadRef != "refs/heads/main" {
		t.Fatalf("captured HEAD evidence = %+v", start.Provenance)
	}
	writeFile(t, repoDir, "moved.txt", "head moved\n")
	runGit(t, repoDir, "add", "moved.txt")
	runGit(t, repoDir, "commit", "-m", "move head after capture")

	retried, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	})
	if err != nil || !retried.Reused || retried.CommitOID != start.CommitOID || retried.Provenance == nil {
		t.Fatalf("reused start = %+v, %v", retried, err)
	}
	if retried.Provenance.HeadCommitOID != headBefore {
		t.Fatalf("retry observed current HEAD instead of the accepted receipt: %+v", retried.Provenance)
	}
	parents := strings.Fields(strings.TrimSpace(runGit(t, repoDir, "rev-list", "--parents", "-n", "1", start.CommitOID)))
	if len(parents) < 2 || parents[1] != headBefore {
		t.Fatalf("checkpoint parents = %v, want observed HEAD %s", parents, headBefore)
	}
}

func TestTurnCheckpointHistoryRebaseExcludesUpstream(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	target, tracker, operator, ref := setupHistoryTarget(t, repoDir)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "checkout", "-q", "-b", "feature/turn")
	writeFile(t, repoDir, "README.md", "local turn edit\n")
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)

	runGit(t, repoDir, "checkout", "-q", "--detach", base)
	writeFile(t, repoDir, "upstream.txt", "integrated upstream\n")
	runGit(t, repoDir, "add", "upstream.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "upstream advance")
	upstream := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", ref, upstream)
	tracker.SetComparisonTargetReady(&target, ref)
	runGit(t, repoDir, "checkout", "-q", "-B", "feature/turn", upstream)
	if got := string(readFile(t, filepath.Join(repoDir, "README.md"))); got != "local turn edit\n" {
		t.Fatalf("rebased working tree lost the starting edit: %q", got)
	}
	writeFile(t, repoDir, "README.md", "local turn edit after rebase\n")
	writeFile(t, repoDir, "turn.txt", "turn file\n")
	end := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
	if end.Provenance == nil || end.Provenance.MergeBaseOID != upstream {
		t.Fatalf("end merge base = %+v, want %s", end.Provenance, upstream)
	}

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonUpstreamAdjusted || !comparison.Complete {
		t.Fatalf("comparison = %+v", comparison)
	}
	if comparison.FileCount != 2 || len(comparison.Files) != 2 {
		t.Fatalf("adjusted files = %+v", comparison.Files)
	}
	paths := map[string]bool{}
	for _, file := range comparison.Files {
		paths[string(file.PathBytes)] = true
	}
	if !paths["README.md"] || !paths["turn.txt"] || paths["upstream.txt"] || paths["ref.txt"] {
		t.Fatalf("adjusted paths = %v", paths)
	}
	if comparison.EffectiveStartTreeOID == start.TreeOID || comparison.EffectiveStartTreeOID == "" {
		t.Fatalf("adjusted baseline did not carry the starting content: %+v", comparison)
	}
	if comparison.ComparisonRef == "" {
		t.Fatal("adjusted comparison did not retain an owned baseline ref")
	}
	if start.Provenance.MergeBaseOID != base {
		t.Fatalf("start merge base = %q, want %q", start.Provenance.MergeBaseOID, base)
	}
}

func TestTurnCheckpointHistoryPreservesDirtyBaseline(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	_, tracker, operator, ref := setupHistoryTarget(t, repoDir)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	writeFile(t, repoDir, "README.md", "pre-existing dirty edit\n")
	writeFile(t, repoDir, "staged.txt", "staged before turn\n")
	runGit(t, repoDir, "add", "staged.txt")
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)

	runGit(t, repoDir, "checkout", "-q", "--detach", base)
	writeFile(t, repoDir, "upstream.txt", "upstream\n")
	runGit(t, repoDir, "add", "upstream.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "upstream advance")
	upstream := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", ref, upstream)
	tracker.SetComparisonTargetReady(turnChangeTargetPointer(), ref)
	runGit(t, repoDir, "checkout", "-q", "-B", "main", upstream)
	writeFile(t, repoDir, "README.md", "changed during turn\n")
	writeFile(t, repoDir, "turn.txt", "turn file\n")
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonUpstreamAdjusted {
		t.Fatalf("dirty baseline comparison = %+v", comparison)
	}
	if len(comparison.Files) != 2 {
		t.Fatalf("dirty baseline comparison files = %+v", comparison.Files)
	}
	paths := map[string]bool{}
	for _, file := range comparison.Files {
		paths[string(file.PathBytes)] = true
	}
	if !paths["README.md"] || !paths["turn.txt"] || paths["staged.txt"] {
		t.Fatalf("dirty baseline paths = %v", paths)
	}
}

func TestTurnCheckpointHistoryConflictUnavailable(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	_, tracker, operator, ref := setupHistoryTarget(t, repoDir)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	writeFile(t, repoDir, "README.md", "local conflict\n")
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)

	runGit(t, repoDir, "checkout", "-q", "--detach", base)
	writeFile(t, repoDir, "README.md", "upstream conflict\n")
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-q", "-m", "upstream conflict")
	upstream := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", ref, upstream)
	tracker.SetComparisonTargetReady(turnChangeTargetPointer(), ref)
	runGit(t, repoDir, "checkout", "-q", "-B", "main", upstream)
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonHistoryUnavailable ||
		comparison.ComparisonReason != turnchanges.ReasonHistoryConflict {
		t.Fatalf("conflicting comparison = %+v", comparison)
	}
	if comparison.FileCount != 0 || len(comparison.Files) != 0 {
		t.Fatalf("unavailable comparison retained turn totals: %+v", comparison)
	}
}

func TestTurnCheckpointHistoryRetainsRevert(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	_, _, operator, _ := setupHistoryTarget(t, repoDir)
	base := strings.TrimSpace(runGit(t, repoDir, "show", "HEAD:README.md"))
	writeFile(t, repoDir, "README.md", "temporary local edit\n")
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
	writeFile(t, repoDir, "README.md", base)
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonDirect || comparison.FileCount != 1 {
		t.Fatalf("revert comparison = %+v", comparison)
	}
	if string(comparison.Files[0].PathBytes) != "README.md" ||
		comparison.Files[0].Kind != "modified" {
		t.Fatalf("revert file = %+v", comparison.Files[0])
	}
}

func TestTurnCheckpointHistoryHeadMoveWithoutTargetUnavailable(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
	writeFile(t, repoDir, "moved.txt", "moved\n")
	runGit(t, repoDir, "add", "moved.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "move without target evidence")
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonHistoryUnavailable ||
		comparison.ComparisonReason != turnchanges.ReasonHistoryUnsupported {
		t.Fatalf("head move comparison = %+v", comparison)
	}
}

func setupHistoryTarget(t *testing.T, repoDir string) (models.ComparisonTarget, *WorkspaceTracker, *GitOperator, string) {
	t.Helper()
	target := comparisonTargetProcessTestTarget()
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetComparisonTarget(&target)
	ref := target.ComparisonRef()
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", ref, base)
	tracker.SetComparisonTargetReady(&target, ref)
	return target, tracker, NewGitOperator(repoDir, newTestLogger(t), tracker), ref
}

func turnChangeTargetPointer() *models.ComparisonTarget {
	target := comparisonTargetProcessTestTarget()
	return &target
}

func captureHistoryEndpoint(
	t *testing.T,
	operator *GitOperator,
	changeSetID, checkoutID string,
	boundary turnchanges.CheckpointBoundary,
) turnchanges.CheckpointResult {
	t.Helper()
	result, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: boundary,
	})
	if err != nil {
		t.Fatalf("capture %s: %v", boundary, err)
	}
	return result
}

func compareHistoryEndpoints(
	t *testing.T,
	operator *GitOperator,
	changeSetID, checkoutID string,
) turnchanges.CheckpointComparison {
	t.Helper()
	request := acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, "")
	request.ComparisonVersion = turnchanges.ComparisonVersion2
	comparison, err := operator.CompareTurnCheckpoints(context.Background(), request)
	if err != nil {
		t.Fatalf("compare history endpoints: %v", err)
	}
	return comparison
}

func TestTurnCheckpointStoredBaseCommitAndAmendDirect(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetBaseBranch("main")
	operator := NewGitOperator(repoDir, newTestLogger(t), tracker)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
	if start.Provenance == nil || !start.Provenance.TargetConfigured || start.Provenance.TargetStatus != turnCheckpointTargetReady {
		t.Fatalf("stored-base provenance = %+v", start.Provenance)
	}
	writeFile(t, repoDir, "turn.txt", "committed during turn\n")
	runGit(t, repoDir, "add", "turn.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "turn commit")
	runGit(t, repoDir, "commit", "-q", "--amend", "-m", "amended turn commit")
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonDirect ||
		comparison.FileCount != 1 || string(comparison.Files[0].PathBytes) != "turn.txt" {
		t.Fatalf("stored-base comparison = %+v", comparison)
	}
}

func TestTurnCheckpointStoredBaseAdvancingRebaseAdjusted(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetBaseBranch("main")
	operator := NewGitOperator(repoDir, newTestLogger(t), tracker)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	writeFile(t, repoDir, "local.txt", "dirty start\n")
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)

	runGit(t, repoDir, "checkout", "-q", "--detach", base)
	writeFile(t, repoDir, "upstream.txt", "upstream\n")
	runGit(t, repoDir, "add", "upstream.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "upstream advance")
	upstream := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", upstream)
	runGit(t, repoDir, "checkout", "-q", "-B", "main", upstream)
	writeFile(t, repoDir, "turn.txt", "turn edit\n")
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonUpstreamAdjusted {
		t.Fatalf("stored-base rebase comparison = %+v", comparison)
	}
	paths := map[string]bool{}
	for _, file := range comparison.Files {
		paths[string(file.PathBytes)] = true
	}
	if !paths["turn.txt"] || paths["upstream.txt"] {
		t.Fatalf("stored-base rebase paths = %v", paths)
	}
}

func TestTurnCheckpointExplicitUnavailableTargetHasNoStoredFallback(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetBaseBranch("main")
	target := comparisonTargetProcessTestTarget()
	tracker.SetComparisonTarget(&target)
	operator := NewGitOperator(repoDir, newTestLogger(t), tracker)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
	if start.Provenance == nil || !start.Provenance.TargetConfigured ||
		start.Provenance.TargetStatus != turnCheckpointTargetUnavailable ||
		start.Provenance.TargetCommitOID != "" || start.Provenance.MergeBaseOID != "" {
		t.Fatalf("unavailable explicit target fell back to stored base: %+v", start.Provenance)
	}
}

func TestTurnCheckpointHistoryUnbornFirstRootIsDirect(t *testing.T) {
	repoDir := newUnbornTestRepo(t)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start := captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
	if start.Provenance == nil || !start.Provenance.Unborn {
		t.Fatalf("unborn start provenance = %+v", start.Provenance)
	}
	writeFile(t, repoDir, "first.txt", "first root\n")
	runGit(t, repoDir, "add", "first.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "first root")
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)
	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonDirect || comparison.FileCount != 1 {
		t.Fatalf("unborn first-root comparison = %+v", comparison)
	}
}

func TestTurnCheckpointHistoryUnbornImportedHistoryUnavailable(t *testing.T) {
	repoDir := newUnbornTestRepo(t)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)
	writeFile(t, repoDir, "imported.txt", "imported\n")
	runGit(t, repoDir, "add", "imported.txt")
	runGitWithEnvironment(t, repoDir, map[string]string{
		"GIT_AUTHOR_DATE": "2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE": "2000-01-01T00:00:00Z",
	}, "commit", "-q", "-m", "imported root")
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonHistoryUnavailable {
		t.Fatalf("imported unborn comparison = %+v", comparison)
	}
}

func runGitWithEnvironment(t *testing.T, dir string, overrides map[string]string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Env = append(hermeticTestGitEnv(os.Environ()), "GIT_AUTHOR_DATE="+overrides["GIT_AUTHOR_DATE"], "GIT_COMMITTER_DATE="+overrides["GIT_COMMITTER_DATE"])
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func newUnbornTestRepo(t *testing.T) string {
	t.Helper()
	isolateTestGitEnv(t)
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--initial-branch=main")
	runGit(t, repoDir, "config", "user.email", "test@test.com")
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "config", "core.hooksPath", "/dev/null")
	return repoDir
}

func TestTurnCheckpointHistorySHA256RebaseAndExport(t *testing.T) {
	repoDir := newSHA256TestRepo(t)
	_, tracker, operator, ref := setupHistoryTarget(t, repoDir)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "checkout", "-q", "-b", "feature/sha256")
	writeFile(t, repoDir, "local.txt", "local start\n")
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointStart)

	runGit(t, repoDir, "checkout", "-q", "--detach", base)
	writeFile(t, repoDir, "upstream.txt", "upstream\n")
	runGit(t, repoDir, "add", "upstream.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "upstream")
	upstream := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", ref, upstream)
	tracker.SetComparisonTargetReady(turnChangeTargetPointer(), ref)
	runGit(t, repoDir, "checkout", "-q", "-B", "feature/sha256", upstream)
	writeFile(t, repoDir, "turn.txt", "turn\n")
	captureHistoryEndpoint(t, operator, changeSetID, checkoutID, turnchanges.CheckpointEnd)

	comparison := compareHistoryEndpoints(t, operator, changeSetID, checkoutID)
	if comparison.ComparisonBasis != turnchanges.ComparisonUpstreamAdjusted ||
		comparison.StartCommitOID == "" || len(comparison.StartCommitOID) != 64 {
		t.Fatalf("sha256 adjusted comparison = %+v", comparison)
	}
	request := turnchanges.ExportRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, HashAlgorithm: comparison.HashAlgorithm,
		StartCommitOID: comparison.StartCommitOID, StartTreeOID: comparison.StartTreeOID,
		EndCommitOID: comparison.EndCommitOID, EndTreeOID: comparison.EndTreeOID,
		ComparisonVersion: turnchanges.ComparisonVersion2, ComparisonBasis: comparison.ComparisonBasis,
		EffectiveStartCommitOID: comparison.EffectiveStartCommitOID,
		EffectiveStartTreeOID:   comparison.EffectiveStartTreeOID,
		EffectiveEndCommitOID:   comparison.EffectiveEndCommitOID,
		EffectiveEndTreeOID:     comparison.EffectiveEndTreeOID,
		ComparisonRef:           comparison.ComparisonRef,
		ExportBatchVersion:      turnchanges.ExportBatchVersion1, MaxEntries: 25,
	}
	export, err := operator.ExportTurnCheckpoint(context.Background(), request)
	if err != nil {
		t.Fatalf("sha256 export: %v", err)
	}
	if export.ComparisonVersion != turnchanges.ComparisonVersion2 || len(export.Files) != 1 ||
		string(export.Files[0].File.PathBytes) != "turn.txt" {
		t.Fatalf("sha256 export = %+v", export)
	}
}

func newSHA256TestRepo(t *testing.T) string {
	t.Helper()
	isolateTestGitEnv(t)
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--object-format=sha256", "--initial-branch=main")
	runGit(t, repoDir, "config", "user.email", "test@test.com")
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "config", "core.hooksPath", "/dev/null")
	writeFile(t, repoDir, "README.md", "base\n")
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-q", "-m", "base")
	return repoDir
}

func TestTurnCheckpointCaptureRejectsHistoryMoveDuringSnapshot(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	runGit(t, repoDir, "checkout", "-q", "-b", "side")
	writeFile(t, repoDir, "moved.txt", "moved\n")
	runGit(t, repoDir, "add", "moved.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "move main during snapshot")
	movedHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "checkout", "-q", "main")

	hookDir := t.TempDir()
	marker := filepath.Join(repoDir, ".git", "snapshot-hook-fired")
	hook := fmt.Sprintf("#!/bin/sh\nif [ -f %q ]; then exit 0; fi\n: > %q\ngit -C %q update-ref refs/heads/main %s\n",
		marker, marker, repoDir, movedHead)
	if err := os.WriteFile(filepath.Join(hookDir, "post-index-change"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "config", "core.hooksPath", hookDir)
	t.Cleanup(func() { runGit(t, repoDir, "config", "core.hooksPath", "/dev/null") })

	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	_, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	})
	if err == nil || !strings.Contains(err.Error(), "history_provenance_unavailable") {
		t.Fatalf("capture with moving history = %v, want history_provenance_unavailable", err)
	}
	if refs := strings.TrimSpace(runGit(t, repoDir, "show-ref")); strings.Contains(refs, changeSetID) {
		t.Fatalf("failed capture retained a checkpoint ref: %s", refs)
	}
}

func TestTurnCheckpointSyntheticTreeCommitWrapsTree(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	treeOID := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD^{tree}"))
	commitOID, err := operator.turnCheckpointSyntheticTreeCommit(context.Background(), treeOID, turnCheckpointSHA1)
	if err != nil {
		t.Fatalf("synthetic comparison commit: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "cat-file", "-t", commitOID)); got != "commit" {
		t.Fatalf("object type = %q, want commit", got)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", commitOID+"^{tree}")); got != treeOID {
		t.Fatalf("commit tree = %q, want %q", got, treeOID)
	}
}
