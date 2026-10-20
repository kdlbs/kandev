package process

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

func TestTurnCheckpointCapturesExactIntervalWithoutMutatingUserGitState(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	writeFile(t, repoDir, "README.md", "# Dirty baseline\n")
	runGit(t, repoDir, "add", "README.md")
	writeFile(t, repoDir, "untouched.txt", "present before start\n")
	runGit(t, repoDir, "add", "untouched.txt")
	startIndex := readFile(t, filepath.Join(repoDir, ".git", "index"))
	startHead := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	startBranch := strings.TrimSpace(runGit(t, repoDir, "branch", "--show-current"))
	startStatus := runGit(t, repoDir, "status", "--porcelain", "-z")
	startConfig := runGit(t, repoDir, "config", "--local", "--list")
	startRefs := checkpointUserRefs(t, repoDir)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture start: %v", err)
	}
	assertUnchangedUserGitState(t, repoDir, startIndex, startHead, startBranch, startStatus)
	if got := runGit(t, repoDir, "config", "--local", "--list"); got != startConfig {
		t.Fatal("checkpoint changed repository configuration")
	}
	if got := checkpointUserRefs(t, repoDir); got != startRefs {
		t.Fatalf("checkpoint changed user refs: before=%q after=%q", startRefs, got)
	}
	if start.HashAlgorithm != "sha1" || start.CommitOID == "" || start.TreeOID == "" || start.ReachabilityRef == "" {
		t.Fatalf("start checkpoint = %+v", start)
	}
	retried, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	})
	if err != nil || !retried.Reused || retried.CommitOID != start.CommitOID || retried.TreeOID != start.TreeOID {
		t.Fatalf("repeated start = %+v, %v; want the immutable accepted endpoint", retried, err)
	}

	writeFile(t, repoDir, "README.md", "# Turn result\n")
	writeFile(t, repoDir, "generated.txt", "generated during turn\n")
	writeFile(t, repoDir, "reverted.txt", "temporary\n")
	runGit(t, repoDir, "add", "README.md", "generated.txt", "reverted.txt")
	runGit(t, repoDir, "commit", "-m", "finish turn changes")
	if err := os.Remove(filepath.Join(repoDir, "reverted.txt")); err != nil {
		t.Fatalf("remove reverted file: %v", err)
	}
	end, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd,
	})
	if err != nil {
		t.Fatalf("capture end: %v", err)
	}
	if end.TreeOID == start.TreeOID || end.CommitOID == start.CommitOID {
		t.Fatalf("start and end checkpoints unexpectedly match: start=%+v end=%+v", start, end)
	}
	comparison, err := operator.CompareTurnCheckpoints(context.Background(), acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, ""))
	if err != nil {
		t.Fatalf("compare checkpoints: %v", err)
	}
	if !comparison.Complete || comparison.FileCount != 2 || comparison.AddedLines == nil || *comparison.AddedLines != 2 ||
		comparison.DeletedLines == nil || *comparison.DeletedLines != 1 || comparison.BinaryCount != 0 {
		t.Fatalf("checkpoint comparison summary = %+v", comparison)
	}
	nameOracle := runGit(t, repoDir, "diff", "--name-only", "-z", start.TreeOID, end.TreeOID)
	if nameOracle != "README.md\x00generated.txt\x00" {
		t.Fatalf("built-in Git path oracle = %q, want README.md and generated.txt", nameOracle)
	}
	numstatOracle := runGit(t, repoDir, "diff", "--numstat", "-z", start.TreeOID, end.TreeOID)
	for _, record := range []string{"1\t1\tREADME.md\x00", "1\t0\tgenerated.txt\x00"} {
		if !strings.Contains(numstatOracle, record) {
			t.Errorf("built-in Git numstat oracle %q omitted %q", numstatOracle, record)
		}
	}
	if !bytes.Equal([]byte(strings.TrimSpace(runGit(t, repoDir, "show", start.CommitOID+":untouched.txt"))), []byte("present before start")) {
		t.Fatal("start checkpoint omitted the pre-existing staged file")
	}
}

func TestTurnCheckpointClassifiesNonGitDirectoryAsUnsupported(t *testing.T) {
	operator := NewGitOperator(t.TempDir(), newTestLogger(t), nil)
	_, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
	})
	var checkpointErr *TurnCheckpointError
	if !errors.As(err, &checkpointErr) || checkpointErr.Reason != turnchanges.ReasonNoGitRepository {
		t.Fatalf("non-Git checkout error = %v, want explicit no-Git reason", err)
	}
}

func TestTurnCheckpointDeletesOnlyOwnedRefsWithAcceptedOIDCAS(t *testing.T) {
	if err := validateGitCommandArgs([]string{"update-ref", "-d", "refs/heads/user-owned", strings.Repeat("a", 40)}); err == nil {
		t.Fatal("generic argument validation allowed deletion of a user ref")
	}
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture start: %v", err)
	}
	userRef := "refs/heads/user-owned"
	runGit(t, repoDir, "update-ref", userRef, strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")))
	wrong := start
	wrong.CommitOID = strings.Repeat("0", len(start.CommitOID))
	if err := operator.DeleteTurnCheckpoint(context.Background(), turnchanges.CheckpointDeleteRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart, CommitOID: wrong.CommitOID,
	}); err == nil {
		t.Fatal("mismatched accepted OID deleted a checkpoint ref")
	}
	ref := turnCheckpointRef(changeSetID, checkoutID, turnchanges.CheckpointStart)
	if got := strings.TrimSpace(runGit(t, repoDir, "show-ref", "--verify", "--hash", ref)); got != start.CommitOID {
		t.Fatalf("checkpoint after mismatched CAS = %q, want %q", got, start.CommitOID)
	}
	if err := operator.DeleteTurnCheckpoint(context.Background(), turnchanges.CheckpointDeleteRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart, CommitOID: start.CommitOID,
	}); err != nil {
		t.Fatalf("delete accepted checkpoint ref: %v", err)
	}
	if _, err := exec.Command("git", "-C", repoDir, "show-ref", "--verify", ref).Output(); err == nil {
		t.Fatal("accepted checkpoint ref still exists after deletion")
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "show-ref", "--verify", "--hash", userRef)); got == "" {
		t.Fatal("owned-ref cleanup deleted an unrelated user ref")
	}
	if err := operator.DeleteTurnCheckpoint(context.Background(), turnchanges.CheckpointDeleteRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart, CommitOID: start.CommitOID,
	}); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
}

func TestTurnCheckpointReportsGitChangeKindsAndNullableBinaryCounts(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	unixFeatures := setupTurnCheckpointChangeKinds(t, repoDir)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart}); err != nil {
		t.Fatalf("capture start: %v", err)
	}
	applyTurnCheckpointChangeKinds(t, repoDir, unixFeatures)
	if err := os.WriteFile(filepath.Join(repoDir, "binary.bin"), []byte{0, 1, 2, 0xff}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd}); err != nil {
		t.Fatalf("capture end: %v", err)
	}
	comparison, err := operator.CompareTurnCheckpoints(context.Background(), acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, ""))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !comparison.Complete || comparison.BinaryCount != 1 {
		t.Fatalf("comparison summary = %+v, want complete with one binary file", comparison)
	}
	byPath := make(map[string]turnchanges.CheckpointFile, len(comparison.Files))
	for _, file := range comparison.Files {
		byPath[string(file.PathBytes)] = file
	}
	if binary := byPath["binary.bin"]; !binary.Binary || binary.Added != nil || binary.Deleted != nil {
		t.Fatalf("binary metadata = %+v, want null line counts", binary)
	}
	if unixFeatures {
		assertTurnCheckpointUnixChangeKinds(t, byPath)
	}
}

func TestTurnCheckpointExportsImmutablePatchesAndRenderingBlobs(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "format.txt", "words  here\nold\n")
	runGit(t, repoDir, "add", "format.txt")
	runGit(t, repoDir, "commit", "-m", "export base")
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	request := turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart}
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), request); err != nil {
		t.Fatalf("capture start: %v", err)
	}
	writeFile(t, repoDir, "format.txt", "words here\nnew\n")
	if err := os.WriteFile(filepath.Join(repoDir, "logo.bin"), []byte{0, 1, 2, 0xff}, 0o644); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	request.Boundary = turnchanges.CheckpointEnd
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), request); err != nil {
		t.Fatalf("capture end: %v", err)
	}
	exportRequest := exportRequestForPair(acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, ""))
	export, err := operator.ExportTurnCheckpoint(context.Background(), exportRequest)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !export.Complete || len(export.Files) != 2 {
		t.Fatalf("export summary = %+v, want two complete entries", export)
	}
	byPath := make(map[string]turnchanges.CheckpointExportFile, len(export.Files))
	for _, file := range export.Files {
		byPath[string(file.File.PathBytes)] = file
	}
	textFile := byPath["format.txt"]
	if textFile.ContentAvailability != turnchanges.Ready || !bytes.Contains(textFile.CanonicalPatch, []byte("-words  here")) ||
		!bytes.Contains(textFile.FilteredPatch, []byte("-old")) || string(textFile.OldRendering) != "words  here\nold\n" ||
		string(textFile.NewRendering) != "words here\nnew\n" {
		t.Fatalf("text export = %+v, want canonical and filtered patches with immutable sides", textFile)
	}
	binaryFile := byPath["logo.bin"]
	if !binaryFile.File.Binary || len(binaryFile.OldRendering) != 0 || len(binaryFile.NewRendering) != 0 || len(binaryFile.CanonicalPatch) == 0 {
		t.Fatalf("binary export = %+v, want patch metadata without rendering blobs", binaryFile)
	}
	writeFile(t, repoDir, "format.txt", "changed after capture\n")
	replayed, err := operator.ExportTurnCheckpoint(context.Background(), exportRequest)
	if err != nil {
		t.Fatalf("re-export after executor checkout edit: %v", err)
	}
	for _, file := range replayed.Files {
		if string(file.File.PathBytes) == "format.txt" && string(file.NewRendering) != "words here\nnew\n" {
			t.Fatalf("historical new blob followed current workspace: %q", file.NewRendering)
		}
	}
}

func TestTurnCheckpointCompareAndExportIgnoreMovedOrRemovedReachabilityRefs(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture start: %v", err)
	}
	writeFile(t, repoDir, "accepted.txt", "accepted endpoint\n")
	end, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd,
	})
	if err != nil {
		t.Fatalf("capture end: %v", err)
	}
	compareRequest := turnchanges.CompareRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, HashAlgorithm: start.HashAlgorithm,
		StartCommitOID: start.CommitOID, StartTreeOID: start.TreeOID,
		EndCommitOID: end.CommitOID, EndTreeOID: end.TreeOID,
	}
	if _, err := exec.Command("git", "-C", repoDir, "update-ref", start.ReachabilityRef, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("move start reachability ref: %v", err)
	}
	if _, err := exec.Command("git", "-C", repoDir, "update-ref", "-d", end.ReachabilityRef).CombinedOutput(); err != nil {
		t.Fatalf("remove end reachability ref: %v", err)
	}
	comparison, err := operator.CompareTurnCheckpoints(context.Background(), compareRequest)
	if err != nil {
		t.Fatalf("compare accepted object IDs after ref changes: %v", err)
	}
	if !comparison.Complete || comparison.FileCount != 1 || string(comparison.Files[0].PathBytes) != "accepted.txt" {
		t.Fatalf("comparison followed mutable refs: %+v", comparison)
	}
	exportRequest := turnchanges.ExportRequest{
		ChangeSetID: compareRequest.ChangeSetID, CheckoutID: compareRequest.CheckoutID,
		HashAlgorithm: compareRequest.HashAlgorithm, StartCommitOID: compareRequest.StartCommitOID,
		StartTreeOID: compareRequest.StartTreeOID, EndCommitOID: compareRequest.EndCommitOID, EndTreeOID: compareRequest.EndTreeOID,
	}
	export, err := operator.ExportTurnCheckpoint(context.Background(), exportRequest)
	if err != nil {
		t.Fatalf("export accepted object IDs after ref changes: %v", err)
	}
	if len(export.Files) != len(comparison.Files) || string(export.Files[0].File.PathBytes) != string(comparison.Files[0].PathBytes) ||
		!bytes.Contains(export.Files[0].CanonicalPatch, []byte("accepted endpoint")) {
		t.Fatalf("export did not use comparison's accepted object pair: %+v", export)
	}
}

func setupTurnCheckpointChangeKinds(t *testing.T, repoDir string) bool {
	t.Helper()
	if runtime.GOOS == "windows" {
		return false
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repoDir, "fixture/gone.txt", "gone\n")
	writeFile(t, repoDir, "fixture/rename-old.txt", "renamed\n")
	writeFile(t, repoDir, "fixture/copy-source.txt", "copied\n")
	writeFile(t, repoDir, "fixture/type.txt", "type\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "change kind fixture")
	return true
}

func applyTurnCheckpointChangeKinds(t *testing.T, repoDir string, unixFeatures bool) {
	t.Helper()
	if !unixFeatures {
		return
	}
	if err := os.Remove(filepath.Join(repoDir, "fixture/gone.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(repoDir, "fixture/rename-old.txt"), filepath.Join(repoDir, "fixture/rename-new.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "fixture/copy-new.txt"), []byte("copied\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repoDir, "fixture/type.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../README.md", filepath.Join(repoDir, "fixture/type.txt")); err != nil {
		t.Fatalf("create symlink for type change: %v", err)
	}
	if err := os.Chmod(filepath.Join(repoDir, "README.md"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertTurnCheckpointUnixChangeKinds(t *testing.T, files map[string]turnchanges.CheckpointFile) {
	t.Helper()
	if got := files["fixture/gone.txt"]; got.Kind != "deleted" {
		t.Errorf("deleted file metadata = %+v", got)
	}
	if got := files["fixture/rename-new.txt"]; got.Kind != "renamed" || string(got.OldPathBytes) != "fixture/rename-old.txt" {
		t.Errorf("rename metadata = %+v", got)
	}
	if got := files["fixture/copy-new.txt"]; got.Kind != "copied" || string(got.OldPathBytes) != "fixture/copy-source.txt" {
		t.Errorf("copy metadata = %+v", got)
	}
	if got := files["fixture/type.txt"]; got.Kind != "type_changed" {
		t.Errorf("type-change metadata = %+v", got)
	}
	if got := files["README.md"]; got.Kind != "mode_changed" || got.Added == nil || *got.Added != 0 || got.Deleted == nil || *got.Deleted != 0 {
		t.Errorf("mode-only metadata = %+v", got)
	}
}

func TestTurnCheckpointRetainsUninitializedSubmoduleGitlink(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-index", "--add", "--cacheinfo", "160000,"+head+",nested")
	runGit(t, repoDir, "commit", "-m", "add uninitialized gitlink")
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart})
	if err != nil {
		t.Fatalf("capture uninitialized submodule: %v", err)
	}
	end, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd})
	if err != nil {
		t.Fatalf("capture end: %v", err)
	}
	comparison, err := operator.CompareTurnCheckpoints(context.Background(), acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, ""))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if start.TreeOID != end.TreeOID || comparison.FileCount != 0 || !comparison.Complete {
		t.Fatalf("uninitialized gitlink became a false change: start=%+v end=%+v diff=%+v", start, end, comparison)
	}
	treeEntry := runGit(t, repoDir, "ls-tree", start.TreeOID, "nested")
	if !strings.HasPrefix(treeEntry, "160000 commit ") {
		t.Fatalf("uninitialized gitlink mode was not retained: %q", treeEntry)
	}
}

func TestValidateTurnCheckpointCandidateBytesEnforcesEntryAndByteLimits(t *testing.T) {
	workDir := t.TempDir()
	path := filepath.Join(workDir, "large.bin")
	if err := os.WriteFile(path, []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	var checkpointErr *TurnCheckpointError
	err := validateTurnCheckpointCandidateBytes([]byte("large.bin\x00"), workDir, 3)
	if !errors.As(err, &checkpointErr) || checkpointErr.Reason != turnchanges.ReasonSizeLimit {
		t.Fatalf("byte-limit error = %v", err)
	}
	if err := validateTurnCheckpointCandidateBytes([]byte("large.bin\x00"), workDir, 4); err != nil {
		t.Fatalf("exact byte limit rejected: %v", err)
	}
	if err := validateTurnCheckpointCandidateBytes([]byte("../outside\x00"), workDir, 10); !errors.As(err, &checkpointErr) || checkpointErr.Reason != turnchanges.ReasonUnsafeGitState {
		t.Fatalf("unsafe path error = %v", err)
	}
	paths := bytes.Repeat([]byte("file\x00"), turnCheckpointEntryLimit+1)
	if err := validateTurnCheckpointCandidateBytes(paths, workDir, 10); !errors.As(err, &checkpointErr) || checkpointErr.Reason != turnchanges.ReasonEntryLimit {
		t.Fatalf("entry-limit error = %v", err)
	}
}

func TestTurnCheckpointConcurrentCaptureSharesOneImmutableEndpoint(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	request := turnchanges.CheckpointRequest{ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart}
	const callers = 6
	results := make(chan turnchanges.CheckpointResult, callers)
	failures := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	start := make(chan struct{})
	for range callers {
		go func() {
			ready.Done()
			<-start
			operator := NewGitOperator(repoDir, newTestLogger(t), nil)
			result, err := operator.CaptureTurnCheckpoint(context.Background(), request)
			if err != nil {
				failures <- err
				return
			}
			results <- result
		}()
	}
	ready.Wait()
	close(start)
	var first turnchanges.CheckpointResult
	for range callers {
		select {
		case err := <-failures:
			t.Fatalf("concurrent capture: %v", err)
		case result := <-results:
			if first.CommitOID == "" {
				first = result
			} else if result.CommitOID != first.CommitOID || result.TreeOID != first.TreeOID {
				t.Fatalf("concurrent endpoints disagree: first=%+v next=%+v", first, result)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".git", "kandev-turn-changes.capture.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("capture lock remains after concurrent calls: %v", err)
	}
}

func TestTurnCheckpointDetectsRapidSameSizeTrackedEdit(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart})
	if err != nil {
		t.Fatalf("capture start: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "show", start.CommitOID+":README.md")); got != "# Test Repo" {
		t.Fatalf("start endpoint content = %q, want committed baseline", got)
	}
	before := readFile(t, filepath.Join(repoDir, "README.md"))
	writeFile(t, repoDir, "README.md", "# Best Repo")
	if len(before) != len(readFile(t, filepath.Join(repoDir, "README.md"))) {
		t.Fatal("fixture must replace content with a same-size write")
	}
	end, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd})
	if err != nil {
		t.Fatalf("capture end: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "show", end.CommitOID+":README.md")); got != "# Best Repo" {
		t.Fatalf("end endpoint content = %q, want rapid same-size edit", got)
	}
	comparison, err := operator.CompareTurnCheckpoints(context.Background(), acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, ""))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if comparison.FileCount != 1 || comparison.AddedLines == nil || *comparison.AddedLines != 1 || comparison.DeletedLines == nil || *comparison.DeletedLines != 1 {
		t.Fatalf("same-size change summary = %+v", comparison)
	}
}

func TestTurnCheckpointKeepsSameRelativePathSeparateAcrossWorktrees(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	secondWorktree := filepath.Join(t.TempDir(), "second")
	runGit(t, repoDir, "worktree", "add", "-b", "second-checkout", secondWorktree)
	changeSetID, firstCheckoutID, secondCheckoutID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	first := NewGitOperator(repoDir, newTestLogger(t), nil)
	second := NewGitOperator(secondWorktree, newTestLogger(t), nil)
	for _, target := range []struct {
		operator *GitOperator
		checkout string
	}{{first, firstCheckoutID}, {second, secondCheckoutID}} {
		if _, err := target.operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: target.checkout, Boundary: turnchanges.CheckpointStart}); err != nil {
			t.Fatalf("capture %s start: %v", target.checkout, err)
		}
	}
	writeFile(t, repoDir, "README.md", "first checkout\n")
	writeFile(t, secondWorktree, "README.md", "second checkout\n")
	for _, target := range []struct {
		operator *GitOperator
		checkout string
	}{{first, firstCheckoutID}, {second, secondCheckoutID}} {
		if _, err := target.operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{ChangeSetID: changeSetID, CheckoutID: target.checkout, Boundary: turnchanges.CheckpointEnd}); err != nil {
			t.Fatalf("capture %s end: %v", target.checkout, err)
		}
		comparison, err := target.operator.CompareTurnCheckpoints(context.Background(), acceptedTurnCheckpointPair(t, target.operator, changeSetID, target.checkout, ""))
		if err != nil {
			t.Fatalf("compare %s: %v", target.checkout, err)
		}
		if comparison.FileCount != 1 || string(comparison.Files[0].PathBytes) != "README.md" {
			t.Fatalf("checkout %s summary = %+v, want its own README change", target.checkout, comparison)
		}
	}
}

func TestTurnCheckpointLockRecoversStaleOwnerAndHonorsCancellation(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	gitDir := filepath.Join(repoDir, ".git")
	lockPath := filepath.Join(gitDir, "kandev-turn-changes.capture.lock")
	if err := os.WriteFile(lockPath, []byte("dead owner\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-turnCheckpointLockStale - time.Second)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatal(err)
	}
	unlock, err := acquireTurnCheckpointLock(context.Background(), gitDir)
	if err != nil {
		t.Fatalf("acquire after stale owner: %v", err)
	}
	unlock()
	if err := os.WriteFile(lockPath, []byte("active owner\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()
	if _, err := acquireTurnCheckpointLock(ctx, gitDir); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting lock error = %v, want context deadline", err)
	}
}

func TestTurnCheckpointRemovesOnlyOwnedStalePrivateIndexes(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	gitDir := filepath.Join(repoDir, ".git")
	stalePath := filepath.Join(gitDir, "kandev-turn-changes-index-orphan")
	if err := os.WriteFile(stalePath, []byte("stale private index"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-turnCheckpointLockStale - time.Second)
	if err := os.Chtimes(stalePath, old, old); err != nil {
		t.Fatal(err)
	}
	foreignPath := filepath.Join(gitDir, "index.backup")
	if err := os.WriteFile(foreignPath, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	if _, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
	}); err != nil {
		t.Fatalf("capture: %v", err)
	}
	if _, err := os.Stat(stalePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale owned index remains: %v", err)
	}
	if _, err := os.Stat(foreignPath); err != nil {
		t.Fatalf("unowned file was removed: %v", err)
	}
}

func TestTurnCheckpointConeSparseIndexKeepsExcludedEntriesAndCapturesMaterializedFiles(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	if err := os.MkdirAll(filepath.Join(repoDir, "included"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "excluded"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repoDir, "included/file.txt", "included before\n")
	writeFile(t, repoDir, "excluded/file.txt", "excluded before\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "sparse fixture")
	runGit(t, repoDir, "sparse-checkout", "init", "--cone", "--sparse-index")
	runGit(t, repoDir, "sparse-checkout", "set", "included")
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	changeSetID, checkoutID := uuid.NewString(), uuid.NewString()
	start, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture sparse start: %v", err)
	}
	writeFile(t, repoDir, "included/file.txt", "included after\n")
	if err := os.MkdirAll(filepath.Join(repoDir, "excluded"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repoDir, "excluded/file.txt", "excluded after\n")
	end, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: turnchanges.CheckpointEnd,
	})
	if err != nil {
		t.Fatalf("capture sparse end: %v", err)
	}
	comparison, err := operator.CompareTurnCheckpoints(context.Background(), acceptedTurnCheckpointPair(t, operator, changeSetID, checkoutID, ""))
	if err != nil {
		t.Fatalf("compare sparse endpoints: %v", err)
	}
	if start.TreeOID == end.TreeOID || comparison.FileCount != 2 {
		t.Fatalf("sparse interval start=%s end=%s comparison=%+v", start.TreeOID, end.TreeOID, comparison)
	}
	paths := map[string]bool{}
	for _, file := range comparison.Files {
		paths[string(file.PathBytes)] = true
	}
	if !paths["included/file.txt"] || !paths["excluded/file.txt"] {
		t.Fatalf("sparse diff paths = %v", paths)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "show", start.CommitOID+":excluded/file.txt")); got != "excluded before" {
		t.Fatalf("start endpoint lost excluded sparse content: %q", got)
	}
}

func TestTurnCheckpointRejectsNonConeSparseCaptureWithTypedReason(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	runGit(t, repoDir, "sparse-checkout", "init", "--cone")
	runGit(t, repoDir, "config", "core.sparseCheckout", "true")
	runGit(t, repoDir, "config", "--worktree", "core.sparseCheckoutCone", "false")
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	if got := strings.TrimSpace(runGit(t, repoDir, "config", "--bool", "--get", "core.sparseCheckout")); got != "true" {
		t.Fatalf("sparseCheckout config = %q", got)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "config", "--bool", "--get", "core.sparseCheckoutCone")); got != "false" {
		t.Fatalf("sparseCheckoutCone config = %q", got)
	}
	_, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
	})
	var checkpointErr *TurnCheckpointError
	if !errors.As(err, &checkpointErr) || checkpointErr.Reason != turnchanges.ReasonSparseCaptureUnsupported {
		t.Fatalf("non-cone sparse error = %v, want typed unsupported-sparse error", err)
	}
}

func TestTurnCheckpointCapturesUnmergedWorkingContentWithoutChangingMergeState(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	runGit(t, repoDir, "checkout", "-b", "conflicting")
	writeFile(t, repoDir, "README.md", "incoming\n")
	runGit(t, repoDir, "commit", "-am", "incoming")
	runGit(t, repoDir, "checkout", "main")
	writeFile(t, repoDir, "README.md", "current\n")
	runGit(t, repoDir, "commit", "-am", "current")
	if output := runGitExpectedFailure(t, repoDir, "merge", "conflicting"); !strings.Contains(output, "CONFLICT") {
		t.Fatalf("merge output = %q, want conflict fixture", output)
	}
	mergeHead := readFile(t, filepath.Join(repoDir, ".git", "MERGE_HEAD"))
	index := readFile(t, filepath.Join(repoDir, ".git", "index"))
	status := runGit(t, repoDir, "status", "--porcelain", "-z")
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	checkpoint, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture merge conflict state: %v", err)
	}
	assertUnchangedUserGitState(t, repoDir, index, strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")), "main", status)
	if got := readFile(t, filepath.Join(repoDir, ".git", "MERGE_HEAD")); !bytes.Equal(got, mergeHead) {
		t.Fatal("checkpoint changed MERGE_HEAD")
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "show", checkpoint.CommitOID+":README.md")); !strings.Contains(got, "<<<<<<<") {
		t.Fatalf("checkpoint conflict content = %q, want working-tree markers", got)
	}
}

func TestTurnCheckpointUsesPrivateIndexWhenRealIndexIsMissing(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	indexPath := filepath.Join(repoDir, ".git", "index")
	if err := os.Remove(indexPath); err != nil {
		t.Fatal(err)
	}
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	checkpoint, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
	})
	if err != nil {
		t.Fatalf("capture with missing user index: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "show", checkpoint.CommitOID+":README.md")); got != "# Test Repo" {
		t.Fatalf("missing-index checkpoint omitted tracked content: %q", got)
	}
	if _, err := os.Stat(indexPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkpoint recreated user's missing index: %v", err)
	}
}

func TestTurnCheckpointRefreshesAssumeUnchangedAndSkipWorktreeInPrivateIndex(t *testing.T) {
	for _, flag := range []string{"--assume-unchanged", "--skip-worktree"} {
		t.Run(flag, func(t *testing.T) {
			repoDir, cleanup := setupTestRepo(t)
			t.Cleanup(cleanup)
			runGit(t, repoDir, "update-index", flag, "README.md")
			writeFile(t, repoDir, "README.md", "# Best Repo")
			beforeIndex := readFile(t, filepath.Join(repoDir, ".git", "index"))
			operator := NewGitOperator(repoDir, newTestLogger(t), nil)
			checkpoint, err := operator.CaptureTurnCheckpoint(context.Background(), turnchanges.CheckpointRequest{
				ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), Boundary: turnchanges.CheckpointStart,
			})
			if err != nil {
				t.Fatalf("capture checkpoint: %v", err)
			}
			if got := strings.TrimSpace(runGit(t, repoDir, "show", checkpoint.CommitOID+":README.md")); got != "# Best Repo" {
				t.Fatalf("captured README = %q, want changed same-size content", got)
			}
			if afterIndex := readFile(t, filepath.Join(repoDir, ".git", "index")); !bytes.Equal(beforeIndex, afterIndex) {
				t.Fatal("checkpoint changed the user's assume-unchanged or skip-worktree index bits")
			}
		})
	}
}

func assertUnchangedUserGitState(t *testing.T, repoDir string, index []byte, head, branch, status string) {
	t.Helper()
	if got := readFile(t, filepath.Join(repoDir, ".git", "index")); !bytes.Equal(got, index) {
		t.Fatal("checkpoint changed the user's index bytes")
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")); got != head {
		t.Fatalf("HEAD = %q, want %q", got, head)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "branch", "--show-current")); got != branch {
		t.Fatalf("branch = %q, want %q", got, branch)
	}
	if got := runGit(t, repoDir, "status", "--porcelain", "-z"); got != status {
		t.Fatalf("working status changed: before %q, after %q", status, got)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}
