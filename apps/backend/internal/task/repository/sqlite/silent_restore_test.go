package sqlite

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

func TestSilentRestoreCheckpointCASRoundTripSQLite(t *testing.T) {
	testSilentRestoreCheckpointCAS(t, newRepoForSessionTests(t))
}

func TestSilentRestoreCheckpointCASRoundTripPostgres(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("create PostgreSQL task repository: %v", err)
	}
	testSilentRestoreCheckpointCAS(t, repo)
}

func testSilentRestoreCheckpointCAS(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "cas", models.TaskSessionStateRunning)
	attempt := fixture.attempt("attempt-cas")
	_, created, err := repo.EnsureRestoreAttempt(ctx, attempt)
	if err != nil || !created {
		t.Fatalf("EnsureRestoreAttempt(first) = created %v, err %v", created, err)
	}
	stored, created, err := repo.EnsureRestoreAttempt(ctx, attempt)
	if err != nil || created {
		t.Fatalf("EnsureRestoreAttempt(retry) = created %v, err %v", created, err)
	}
	if !sameSilentRestoreCheckpoint(*stored.Checkpoint, *attempt.Checkpoint) {
		t.Fatalf("round-trip checkpoint = %+v, want %+v", stored.Checkpoint, attempt.Checkpoint)
	}

	conflict := *attempt
	conflictCheckpoint := *attempt.Checkpoint
	conflict.Checkpoint = &conflictCheckpoint
	conflict.Checkpoint.WorkspaceOwnerID = "different-owner"
	if _, _, err := repo.EnsureRestoreAttempt(ctx, &conflict); !errors.Is(err, repoerrors.ErrSilentRestoreAttemptConflict) {
		t.Fatalf("EnsureRestoreAttempt(conflicting identity) error = %v", err)
	}

	expected := *stored.Checkpoint
	allocated := expected
	allocated.Stage = models.SilentRestoreStageCandidateAllocated
	allocated.CandidateExecutionID = "candidate-a"
	changed, err := repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, expected, allocated)
	if err != nil || !changed {
		t.Fatalf("checkpoint CAS to first candidate = %v, %v", changed, err)
	}
	changed, err = repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, expected, allocated)
	if err != nil || changed {
		t.Fatalf("stale checkpoint CAS = %v, %v; want false", changed, err)
	}
	launching := allocated
	launching.Stage = models.SilentRestoreStageCandidateLaunching
	changed, err = repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, allocated, launching)
	if err != nil || !changed {
		t.Fatalf("checkpoint CAS before candidate launch = %v, %v", changed, err)
	}
	replacement := allocated
	replacement.CandidateExecutionID = "candidate-b"
	changed, err = repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, launching, replacement)
	if err != nil || changed {
		t.Fatalf("direct candidate rotation CAS = %v, %v; want false", changed, err)
	}
	dead := launching
	dead.Stage = models.SilentRestoreStageCandidateDead
	changed, err = repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, launching, dead)
	if err != nil || !changed {
		t.Fatalf("persist proven-dead candidate CAS = %v, %v", changed, err)
	}
	got, err := repo.GetRestoreAttempt(ctx, attempt.ID)
	if err != nil || got.Checkpoint == nil || !sameSilentRestoreCheckpoint(*got.Checkpoint, dead) {
		t.Fatalf("persisted dead-candidate checkpoint = %+v, %v; want %+v", got, err, dead)
	}
	sameCandidateRotation := dead
	sameCandidateRotation.Stage = models.SilentRestoreStageCandidateAllocated
	changed, err = repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, dead, sameCandidateRotation)
	if err != nil || changed {
		t.Fatalf("same candidate retry after death CAS = %v, %v; want false", changed, err)
	}
	changed, err = repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, dead, replacement)
	if err != nil || !changed {
		t.Fatalf("candidate replacement CAS = %v, %v", changed, err)
	}
	launchingReplacement := replacement
	launchingReplacement.Stage = models.SilentRestoreStageCandidateLaunching
	changed, err = repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, replacement, launchingReplacement)
	if err != nil || !changed {
		t.Fatalf("replacement launch CAS = %v, %v", changed, err)
	}
	got, err = repo.GetRestoreAttempt(ctx, attempt.ID)
	if err != nil {
		t.Fatalf("GetRestoreAttempt after candidate replacement: %v", err)
	}
	if got.Checkpoint == nil || !sameSilentRestoreCheckpoint(*got.Checkpoint, launchingReplacement) {
		t.Fatalf("persisted replacement checkpoint = %+v, want %+v", got.Checkpoint, launchingReplacement)
	}
}

func TestSilentRestoreCandidatePagingIncludesFailedAndSkipsMalformedMetadata(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	createSilentRestoreCandidate(t, repo, "session-a-malformed", "task-a-malformed", models.TaskSessionStateRunning, "not-json")
	createSilentRestoreCandidate(t, repo, "session-b-failed", "task-b-failed", models.TaskSessionStateFailed, `{"marker":"valid"}`)
	createSilentRestoreCandidate(t, repo, "session-c-ready", "task-c-ready", models.TaskSessionStateWaitingForInput, `{"marker":"last"}`)

	first, err := repo.ListSilentRestoreCandidates(ctx, "", 1)
	if err != nil || len(first) != 1 || first[0].SessionID != "session-a-malformed" || first[0].Metadata != nil {
		t.Fatalf("first candidate page = %+v, %v; malformed row should be isolated", first, err)
	}
	second, err := repo.ListSilentRestoreCandidates(ctx, first[0].SessionID, 1)
	if err != nil || len(second) != 1 || second[0].SessionID != "session-b-failed" || second[0].State != models.TaskSessionStateFailed {
		t.Fatalf("second candidate page = %+v, %v; failed session should be included", second, err)
	}
	third, err := repo.ListSilentRestoreCandidates(ctx, second[0].SessionID, 200)
	if err != nil || len(third) != 1 || third[0].SessionID != "session-c-ready" {
		t.Fatalf("third candidate page = %+v, %v", third, err)
	}
	for _, limit := range []int{0, 201} {
		if _, err := repo.ListSilentRestoreCandidates(ctx, "", limit); err == nil {
			t.Fatalf("ListSilentRestoreCandidates(limit=%d) unexpectedly succeeded", limit)
		}
	}
}

func TestSilentRestoreCommitExactTransactionSQLite(t *testing.T) {
	testSilentRestoreCommitExactTransaction(t, newRepoForSessionTests(t))
}

func TestSilentRestoreCommitExactTransactionPostgres(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("create PostgreSQL task repository: %v", err)
	}
	testSilentRestoreCommitExactTransaction(t, repo)
}

func TestSilentRestoreEmptyWorkspaceOwnerCheckpointAndCommitSQLite(t *testing.T) {
	testSilentRestoreEmptyWorkspaceOwnerCheckpointAndCommit(t, newRepoForSessionTests(t))
}

func TestSilentRestoreEmptyWorkspaceOwnerCheckpointAndCommitPostgres(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("create PostgreSQL task repository: %v", err)
	}
	testSilentRestoreEmptyWorkspaceOwnerCheckpointAndCommit(t, repo)
}

func testSilentRestoreEmptyWorkspaceOwnerCheckpointAndCommit(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	fixture := newSilentRestoreFixtureWithWorkspaceOwner(t, repo, "empty-owner-commit", models.TaskSessionStateRunning, "")
	attempt := fixture.attempt("attempt-empty-owner-success")
	stored, created, err := repo.EnsureRestoreAttempt(ctx, attempt)
	if err != nil || !created || stored.Checkpoint == nil || stored.Checkpoint.WorkspaceOwnerID != "" ||
		!sameSilentRestoreCheckpoint(*stored.Checkpoint, *attempt.Checkpoint) {
		t.Fatalf("empty-owner checkpoint round trip = %+v, created %v, err %v", stored, created, err)
	}
	_, commit := prepareSilentRestoreCandidate(t, repo, fixture, "empty-owner-success")
	if commit.Checkpoint.WorkspaceOwnerID != "" {
		t.Fatalf("candidate checkpoint owner = %q, want the pinned empty owner", commit.Checkpoint.WorkspaceOwnerID)
	}
	changed, err := repo.CommitSilentRestore(ctx, commit)
	if err != nil || !changed {
		t.Fatalf("CommitSilentRestore with empty workspace owner = %v, %v; want committed", changed, err)
	}

	claimed := newSilentRestoreFixtureWithWorkspaceOwner(t, repo, "empty-owner-claim", models.TaskSessionStateRunning, "")
	attemptID, claimCommit := prepareSilentRestoreCandidate(t, repo, claimed, "empty-owner-claim")
	claimAttempt, err := repo.GetRestoreAttempt(ctx, attemptID)
	if err != nil || claimAttempt.Checkpoint == nil || claimAttempt.Checkpoint.WorkspaceOwnerID != "" {
		t.Fatalf("checkpoint before empty-owner claim = %+v, %v", claimAttempt, err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE workspaces SET owner_id = ? WHERE id = ?`), "claimed-owner", claimed.workspace.ID); err != nil {
		t.Fatalf("claim previously empty workspace: %v", err)
	}
	changed, err = repo.CommitSilentRestore(ctx, claimCommit)
	if err != nil || changed {
		t.Fatalf("CommitSilentRestore after empty-owner claim = %v, %v; want fenced", changed, err)
	}
	claimAttemptAfter, err := repo.GetRestoreAttempt(ctx, attemptID)
	if err != nil || claimAttemptAfter.Outcome != claimAttempt.Outcome || claimAttemptAfter.Checkpoint == nil ||
		!sameSilentRestoreCheckpoint(*claimAttemptAfter.Checkpoint, *claimAttempt.Checkpoint) {
		t.Fatalf("attempt changed after empty-owner claim: %+v, %v", claimAttemptAfter, err)
	}
}

func testSilentRestoreCommitExactTransaction(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "commit", models.TaskSessionStateRunning)
	attempt := fixture.attempt("attempt-commit")
	stored, _, err := repo.EnsureRestoreAttempt(ctx, attempt)
	if err != nil {
		t.Fatalf("EnsureRestoreAttempt: %v", err)
	}
	allocated := *stored.Checkpoint
	allocated.Stage = models.SilentRestoreStageCandidateAllocated
	allocated.CandidateExecutionID = "candidate-commit"
	changed, err := repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, *stored.Checkpoint, allocated)
	if err != nil || !changed {
		t.Fatalf("allocate candidate: %v, %v", changed, err)
	}
	checkpoint := allocated
	checkpoint.Stage = models.SilentRestoreStageCandidateLaunching
	changed, err = repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, allocated, checkpoint)
	if err != nil || !changed {
		t.Fatalf("mark candidate launch boundary: %v, %v", changed, err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: fixture.session.ID, SessionID: fixture.session.ID, TaskID: fixture.task.ID,
		ExecutorID: "executor-silent-restore", AgentExecutionID: checkpoint.CandidateExecutionID, Status: "running",
	}); err != nil {
		t.Fatalf("persist candidate executor: %v", err)
	}

	commit := &models.SilentRestoreCommit{
		AttemptID: attempt.ID, Checkpoint: checkpoint,
		Generation: models.HarnessSessionGeneration{
			SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
			Generation: 2, PredecessorGeneration: 1, NativeSessionID: fixture.generation.NativeSessionID,
			AgentType: fixture.generation.AgentType, AdapterVersion: fixture.generation.AdapterVersion,
			OriginalWorkspace: fixture.generation.OriginalWorkspace, CurrentWorkspace: fixture.generation.CurrentWorkspace,
			NativeStateReference: fixture.generation.NativeStateReference, CreationReason: "silent_restart_restore",
		},
	}
	changed, err = repo.CommitSilentRestore(ctx, commit)
	if err != nil || !changed {
		t.Fatalf("CommitSilentRestore = %v, %v; want committed", changed, err)
	}
	changed, err = repo.CommitSilentRestore(ctx, commit)
	if err != nil || changed {
		t.Fatalf("repeated CommitSilentRestore = %v, %v; want false", changed, err)
	}

	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, fixture.session.ID, fixture.session.QueueIncarnationID)
	if err != nil || generation.Generation != 2 || generation.NativeSessionID != fixture.generation.NativeSessionID {
		t.Fatalf("current generation = %+v, %v", generation, err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
	if err != nil || block.State != models.RecoveryBlockResolved || block.AuthorizedAction != models.SilentRestoreAction {
		t.Fatalf("source block after commit = %+v, %v", block, err)
	}
	attemptAfter, err := repo.GetRestoreAttempt(ctx, attempt.ID)
	if err != nil || attemptAfter.Outcome != models.SilentRestoreOutcomeRestored || attemptAfter.CompletedAt == nil ||
		attemptAfter.Checkpoint == nil || attemptAfter.Checkpoint.Stage != models.SilentRestoreStageTerminal {
		t.Fatalf("attempt after commit = %+v, %v", attemptAfter, err)
	}
	submission, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
	if err != nil || submission.State != models.DeliverySubmissionInterruptedUnknown || string(submission.Payload) != "old immutable prompt" {
		t.Fatalf("old uncertain submission changed: %+v, %v", submission, err)
	}
	session, err := repo.GetTaskSession(ctx, fixture.session.ID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	recovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	if !ok || recovery.Phase != models.AgentDeliveryRecoveryRestored || recovery.Revision != fixture.recovery.Revision+1 {
		t.Fatalf("recovery after commit = %+v (ok=%v)", recovery, ok)
	}
	if _, exists := session.Metadata[models.SessionMetaKeyLastAgentError]; exists {
		t.Fatal("matching durable-delivery uncertainty warning remained after successful restore")
	}
}

func TestSilentRestoreCommitPreservesUnrelatedLastAgentError(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "unrelated-error", models.TaskSessionStateRunning)
	if err := repo.SetSessionMetadataKey(ctx, fixture.session.ID, models.SessionMetaKeyLastAgentError,
		models.LastAgentError{Message: "unrelated", Code: "EXECUTOR_START_FAILED", Details: "other"}); err != nil {
		t.Fatal(err)
	}
	commitSilentRestoreFixture(t, repo, fixture, "attempt-candidate-unrelated-error", "candidate-unrelated-error")
	session, err := repo.GetTaskSession(ctx, fixture.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	lastError, ok := models.LoadLastAgentError(session.Metadata)
	if !ok || lastError.Code != "EXECUTOR_START_FAILED" || lastError.Message != "unrelated" {
		t.Fatalf("unrelated last-agent error after restore = %+v (ok=%v)", lastError, ok)
	}
}

func TestSilentRestoreStructuredErrorIdentitySQLite(t *testing.T) {
	testSilentRestoreStructuredErrorIdentity(t, newRepoForSessionTests(t))
}

func TestSilentRestoreStructuredErrorIdentityPostgres(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("create PostgreSQL task repository: %v", err)
	}
	testSilentRestoreStructuredErrorIdentity(t, repo)
}

func testSilentRestoreStructuredErrorIdentity(t *testing.T, repo *Repository) {
	t.Helper()
	cases := []struct {
		name          string
		submissionID  string
		explicitEmpty bool
		malformed     bool
		wantCleared   bool
	}{
		{name: "matching structured identity", wantCleared: true},
		{name: "mismatched structured identity", submissionID: "newer-submission"},
		{name: "present empty structured identity", explicitEmpty: true},
		{name: "malformed structured identity", malformed: true},
		{name: "legacy exact details fallback", wantCleared: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newSilentRestoreFixture(t, repo, "structured-"+strings.ReplaceAll(test.name, " ", "-"), models.TaskSessionStateRunning)
			submissionID := fixture.recovery.SubmissionID
			lastError := map[string]interface{}{
				"message": "delivery outcome is uncertain", "occurred_at": time.Now().UTC(),
				"code": durableDeliveryUncertainErrorCode, "details": "redacted delivery details",
				"agent_execution_id": fixture.recovery.AgentExecutionID,
			}
			switch {
			case test.malformed:
				lastError["details"] = submissionID
				lastError["delivery_submission_id"] = 42
			case test.explicitEmpty:
				lastError["details"] = submissionID
				lastError["delivery_submission_id"] = ""
			case test.name == "legacy exact details fallback":
				lastError["details"] = submissionID
			case test.submissionID != "":
				lastError["details"] = submissionID
				lastError["delivery_submission_id"] = test.submissionID
			default:
				lastError["delivery_submission_id"] = submissionID
			}
			if err := repo.SetSessionMetadataKey(ctx, fixture.session.ID, models.SessionMetaKeyLastAgentError, lastError); err != nil {
				t.Fatalf("SetSessionMetadataKey(last error): %v", err)
			}
			_, commit := prepareSilentRestoreCandidate(t, repo, fixture, "candidate-structured-"+strings.ReplaceAll(test.name, " ", "-"))
			changed, err := repo.CommitSilentRestore(ctx, commit)
			if err != nil || !changed {
				t.Fatalf("CommitSilentRestore = %v, %v; want committed", changed, err)
			}
			stored, err := repo.GetTaskSession(ctx, fixture.session.ID)
			if err != nil {
				t.Fatalf("GetTaskSession: %v", err)
			}
			_, present := stored.Metadata[models.SessionMetaKeyLastAgentError]
			if present == test.wantCleared {
				t.Fatalf("last-agent-error present = %v, want cleared %v", present, test.wantCleared)
			}
		})
	}
}

func TestSilentRestoreCommitRejectsChangedOwnershipAndIndependentBlock(t *testing.T) {
	testSilentRestoreCommitRejected(t, "transfer", func(t *testing.T, repo *Repository, fixture *silentRestoreFixture) {
		_, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`UPDATE workspaces SET owner_id = ? WHERE id = ?`), "new-owner", fixture.workspace.ID)
		if err != nil {
			t.Fatal(err)
		}
	})
	testSilentRestoreCommitRejected(t, "archive", func(t *testing.T, repo *Repository, fixture *silentRestoreFixture) {
		_, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`UPDATE tasks SET archived_at = ? WHERE id = ?`), time.Now().UTC(), fixture.task.ID)
		if err != nil {
			t.Fatal(err)
		}
	})
	testSilentRestoreCommitRejected(t, "independent-block", func(t *testing.T, repo *Repository, fixture *silentRestoreFixture) {
		if err := repo.UpsertSessionRecoveryBlock(context.Background(), &models.SessionRecoveryBlock{
			ID: "independent-" + fixture.session.ID, SessionID: fixture.session.ID,
			IncarnationID: fixture.session.QueueIncarnationID, ExpectedGeneration: 1,
			Reason: "independent", State: models.RecoveryBlockOpen,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func testSilentRestoreCommitRejected(t *testing.T, suffix string, mutate func(*testing.T, *Repository, *silentRestoreFixture)) {
	t.Helper()
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, suffix, models.TaskSessionStateRunning)
	attemptID, commit := prepareSilentRestoreCandidate(t, repo, fixture, "candidate-"+suffix)
	mutate(t, repo, fixture)
	generationBefore, err := repo.GetCurrentHarnessSessionGeneration(ctx, fixture.session.ID, fixture.session.QueueIncarnationID)
	if err != nil || generationBefore == nil {
		t.Fatalf("GetCurrentHarnessSessionGeneration before rejected %s commit: %v", suffix, err)
	}
	sessionBefore, err := repo.GetTaskSession(ctx, fixture.session.ID)
	if err != nil {
		t.Fatalf("GetTaskSession before rejected %s commit: %v", suffix, err)
	}
	attemptBefore, err := repo.GetRestoreAttempt(ctx, attemptID)
	if err != nil {
		t.Fatalf("GetRestoreAttempt before rejected %s commit: %v", suffix, err)
	}
	blockBefore, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
	if err != nil {
		t.Fatalf("GetSessionRecoveryBlock before rejected %s commit: %v", suffix, err)
	}
	submissionBefore, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
	if err != nil {
		t.Fatalf("GetAgentDeliverySubmission before rejected %s commit: %v", suffix, err)
	}
	executorBefore, err := repo.GetExecutorRunningBySessionID(ctx, fixture.session.ID)
	executorMissing := errors.Is(err, models.ErrExecutorRunningNotFound)
	if err != nil && !executorMissing {
		t.Fatalf("GetExecutorRunning before rejected %s commit: %v", suffix, err)
	}
	if executorMissing {
		executorBefore = nil
	}
	changed, err := repo.CommitSilentRestore(ctx, commit)
	if err != nil || changed {
		t.Fatalf("CommitSilentRestore after %s = %v, %v; want fenced", suffix, changed, err)
	}
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, fixture.session.ID, fixture.session.QueueIncarnationID)
	if err != nil || generation == nil || !sameHarnessSessionGeneration(*generation, *generationBefore) {
		t.Fatalf("generation after rejected %s commit = %+v, %v", suffix, generation, err)
	}
	sessionAfter, err := repo.GetTaskSession(ctx, fixture.session.ID)
	if err != nil || !reflect.DeepEqual(sessionAfter.Metadata, sessionBefore.Metadata) {
		t.Fatalf("session metadata changed after rejected %s commit", suffix)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
	if err != nil || !sameSessionRecoveryBlock(*block, *blockBefore) {
		t.Fatalf("source block after rejected %s commit = %+v, %v", suffix, block, err)
	}
	attempt, err := repo.GetRestoreAttempt(ctx, attemptID)
	if err != nil || attempt.Outcome != attemptBefore.Outcome || attempt.CompletedAt != nil ||
		attempt.Checkpoint == nil || attemptBefore.Checkpoint == nil ||
		!sameSilentRestoreCheckpoint(*attempt.Checkpoint, *attemptBefore.Checkpoint) {
		t.Fatalf("attempt after rejected %s commit = %+v, %v", suffix, attempt, err)
	}
	submissionAfter, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
	if err != nil || !sameAgentDeliverySubmission(submissionAfter, submissionBefore) {
		t.Fatalf("old uncertain submission changed after rejected %s commit: %+v, %v", suffix, submissionAfter, err)
	}
	executorAfter, err := repo.GetExecutorRunningBySessionID(ctx, fixture.session.ID)
	if executorBefore == nil {
		if !errors.Is(err, models.ErrExecutorRunningNotFound) {
			t.Fatalf("executor row after rejected %s commit = %+v, %v; want unchanged absence", suffix, executorAfter, err)
		}
	} else if err != nil || executorAfter == nil || executorAfter.ID != executorBefore.ID ||
		executorAfter.TaskID != executorBefore.TaskID || executorAfter.AgentExecutionID != executorBefore.AgentExecutionID ||
		executorAfter.Status != executorBefore.Status {
		t.Fatalf("executor identity after rejected %s commit = %+v, %v", suffix, executorAfter, err)
	}
}

func sameAgentDeliverySubmission(a, b *models.AgentDeliverySubmission) bool {
	return a != nil && b != nil && a.ID == b.ID && a.SessionID == b.SessionID &&
		a.IncarnationID == b.IncarnationID && a.HarnessGeneration == b.HarnessGeneration &&
		a.OwnerGeneration == b.OwnerGeneration && a.DispatchAttemptID == b.DispatchAttemptID &&
		a.PayloadHash == b.PayloadHash && string(a.Payload) == string(b.Payload) &&
		a.State == b.State && a.Outcome == b.Outcome && a.CreatedAt.Equal(b.CreatedAt) &&
		a.UpdatedAt.Equal(b.UpdatedAt)
}

func TestSilentRestoreCommitRejectsStaleEvidenceAndMissingCandidate(t *testing.T) {
	testSilentRestoreCommitRejected(t, "stale-recovery", func(t *testing.T, repo *Repository, fixture *silentRestoreFixture) {
		ctx := context.Background()
		session, err := repo.GetTaskSession(ctx, fixture.session.ID)
		if err != nil {
			t.Fatal(err)
		}
		recovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
		if !ok {
			t.Fatal("current delivery recovery is missing")
		}
		recovery.Revision++
		if err := repo.SetSessionMetadataKey(ctx, fixture.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, recovery); err != nil {
			t.Fatal(err)
		}
	})
	testSilentRestoreCommitRejected(t, "stale-generation", func(t *testing.T, repo *Repository, fixture *silentRestoreFixture) {
		if err := repo.CreateHarnessSessionGeneration(context.Background(), &models.HarnessSessionGeneration{
			SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
			Generation: 2, PredecessorGeneration: 1, NativeSessionID: "replacement-native", CreationReason: "external-change",
		}); err != nil {
			t.Fatal(err)
		}
	})
	testSilentRestoreCommitRejected(t, "missing-candidate", func(t *testing.T, repo *Repository, fixture *silentRestoreFixture) {
		if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`DELETE FROM executors_running WHERE session_id = ?`), fixture.session.ID); err != nil {
			t.Fatal(err)
		}
	})
	testSilentRestoreCommitRejected(t, "foreign-candidate", func(t *testing.T, repo *Repository, fixture *silentRestoreFixture) {
		if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(
			`UPDATE executors_running SET agent_execution_id = ? WHERE session_id = ?`), "foreign-execution", fixture.session.ID); err != nil {
			t.Fatal(err)
		}
	})
	testSilentRestoreCommitRejected(t, "passthrough", func(t *testing.T, repo *Repository, fixture *silentRestoreFixture) {
		if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(
			`UPDATE task_sessions SET is_passthrough = 1 WHERE id = ?`), fixture.session.ID); err != nil {
			t.Fatal(err)
		}
	})
	testSilentRestoreCommitRejected(t, "inactive-route", func(t *testing.T, repo *Repository, fixture *silentRestoreFixture) {
		if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(
			`UPDATE task_sessions SET route_state = 'waiting' WHERE id = ?`), fixture.session.ID); err != nil {
			t.Fatal(err)
		}
	})
	testSilentRestoreCommitRejected(t, "automation-origin", func(t *testing.T, repo *Repository, fixture *silentRestoreFixture) {
		if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(
			`UPDATE tasks SET origin = ? WHERE id = ?`), models.TaskOriginAutomationTask, fixture.task.ID); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSilentRestoreCommitRollbackIsAtomicSQLite(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "rollback", models.TaskSessionStateRunning)
	attemptID, commit := prepareSilentRestoreCandidate(t, repo, fixture, "candidate-rollback")
	_, err := repo.db.ExecContext(ctx, `CREATE TRIGGER reject_silent_restore_terminal
		BEFORE UPDATE ON session_restore_attempts WHEN NEW.outcome = 'restored'
		BEGIN SELECT RAISE(ABORT, 'reject terminal checkpoint'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := repo.CommitSilentRestore(ctx, commit); err == nil || changed {
		t.Fatalf("CommitSilentRestore with rejecting trigger = %v, %v; want rollback error", changed, err)
	}
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, fixture.session.ID, fixture.session.QueueIncarnationID)
	if err != nil || generation.Generation != 1 {
		t.Fatalf("generation after rollback = %+v, %v", generation, err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
	if err != nil || block.State != models.RecoveryBlockOpen {
		t.Fatalf("block after rollback = %+v, %v", block, err)
	}
	attempt, err := repo.GetRestoreAttempt(ctx, attemptID)
	if err != nil || attempt.Outcome != models.SilentRestoreOutcomePending {
		t.Fatalf("attempt after rollback = %+v, %v", attempt, err)
	}
}

func TestSessionRestoreAttemptCheckpointMigrationReplaySQLite(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "migration-task", Title: "Migration"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "migration-session", TaskID: "migration-task"}); err != nil {
		t.Fatal(err)
	}
	legacy := &models.RestoreAttempt{ID: "legacy-attempt", SessionID: "migration-session", IncarnationID: "old", Action: "old_action", Outcome: "pending"}
	if err := repo.CreateRestoreAttempt(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `ALTER TABLE session_restore_attempts DROP COLUMN checkpoint_json`); err != nil {
		t.Fatalf("simulate pre-checkpoint schema: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("replay repository migrations: %v", err)
	}
	got, err := repo.GetRestoreAttempt(ctx, legacy.ID)
	if err != nil || got.Checkpoint != nil {
		t.Fatalf("legacy attempt after migration = %+v, %v", got, err)
	}
}

type silentRestoreFixture struct {
	workspace  *models.Workspace
	task       *models.Task
	session    *models.TaskSession
	generation models.HarnessSessionGeneration
	recovery   models.AgentDeliveryRecovery
	block      models.SessionRecoveryBlock
}

func newSilentRestoreFixture(t *testing.T, repo *Repository, suffix string, state models.TaskSessionState) *silentRestoreFixture {
	return newSilentRestoreFixtureWithWorkspaceOwner(t, repo, suffix, state, "owner-"+suffix)
}

func newSilentRestoreFixtureWithWorkspaceOwner(
	t *testing.T,
	repo *Repository,
	suffix string,
	state models.TaskSessionState,
	ownerID string,
) *silentRestoreFixture {
	t.Helper()
	ctx := context.Background()
	fixture := &silentRestoreFixture{
		workspace: &models.Workspace{ID: "workspace-silent-" + suffix, Name: "Silent restore", OwnerID: ownerID, OrgID: "org-" + suffix},
		task:      &models.Task{ID: "task-silent-" + suffix, Title: "Silent restore", WorkspaceID: "workspace-silent-" + suffix},
		session:   &models.TaskSession{ID: "session-silent-" + suffix, TaskID: "task-silent-" + suffix, State: state},
	}
	if err := repo.CreateWorkspace(ctx, fixture.workspace); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, fixture.task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, fixture.session); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		Generation: 1, NativeSessionID: "native-" + suffix, AgentType: "test", CreationReason: "initial",
	}); err != nil {
		t.Fatalf("CreateHarnessSessionGeneration: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: fixture.session.ID, SessionID: fixture.session.ID, TaskID: fixture.task.ID,
		ExecutorID: "executor-silent-restore", AgentExecutionID: "old-execution-" + suffix, Status: "running",
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(source): %v", err)
	}
	submissionID := "submission-silent-" + suffix
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "payload-hash-" + suffix,
		Payload: []byte("old immutable prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	}); err != nil {
		t.Fatalf("PrepareAgentDeliverySubmission: %v", err)
	}
	fixture.recovery = models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryUncertain, SessionID: fixture.session.ID,
		AgentExecutionID: "old-execution-" + suffix, SubmissionID: submissionID,
		StreamID: "stream-silent-" + suffix, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: 1, PromptGeneration: 1, UpdatedAt: time.Now().UTC(),
	}
	fixture.block = *deliveryRecoveryBlock(fixture.recovery)
	stored, err := repo.UpsertAgentDeliveryRecovery(ctx, &fixture.recovery, &fixture.block)
	if err != nil || !stored {
		t.Fatalf("UpsertAgentDeliveryRecovery = %v, %v", stored, err)
	}
	if err := repo.SetSessionMetadataKey(ctx, fixture.session.ID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "delivery outcome is uncertain", Code: durableDeliveryUncertainErrorCode,
		Details: submissionID, AgentExecutionID: fixture.recovery.AgentExecutionID,
	}); err != nil {
		t.Fatalf("SetSessionMetadataKey(last error): %v", err)
	}
	fixture.session, err = repo.GetTaskSession(ctx, fixture.session.ID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	block, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
	if err != nil {
		t.Fatalf("GetSessionRecoveryBlock: %v", err)
	}
	fixture.block = *block
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, fixture.session.ID, fixture.session.QueueIncarnationID)
	if err != nil {
		t.Fatalf("GetCurrentHarnessSessionGeneration: %v", err)
	}
	fixture.generation = *generation
	fixture.recovery, _ = models.LoadAgentDeliveryRecovery(fixture.session.Metadata)
	return fixture
}

func (f *silentRestoreFixture) attempt(id string) *models.RestoreAttempt {
	return &models.RestoreAttempt{
		ID: id, SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID,
		ExpectedGeneration: f.generation.Generation, Action: models.SilentRestoreAction,
		Outcome: models.SilentRestoreOutcomePending, TargetWorkspace: f.workspace.ID,
		CreatedAt: time.Now().UTC(), Checkpoint: &models.SilentRestoreCheckpoint{
			Version: 1, Stage: models.SilentRestoreStagePrepared,
			SourceRecovery: f.recovery, SourceGeneration: f.generation, SourceBlock: f.block,
			TaskID: f.task.ID, WorkspaceID: f.workspace.ID,
			WorkspaceOwnerID: f.workspace.OwnerID, WorkspaceOrgID: f.workspace.OrgID,
		},
	}
}

func commitSilentRestoreFixture(t *testing.T, repo *Repository, fixture *silentRestoreFixture, attemptID, candidateID string) {
	t.Helper()
	_, commit := prepareSilentRestoreCandidate(t, repo, fixture, candidateID)
	commit.AttemptID = attemptID
	changed, err := repo.CommitSilentRestore(context.Background(), commit)
	if err != nil || !changed {
		t.Fatalf("CommitSilentRestore = %v, %v", changed, err)
	}
}

func prepareSilentRestoreCandidate(t *testing.T, repo *Repository, fixture *silentRestoreFixture, candidateID string) (string, *models.SilentRestoreCommit) {
	t.Helper()
	ctx := context.Background()
	attemptID := "attempt-" + candidateID
	attempt := fixture.attempt(attemptID)
	stored, _, err := repo.EnsureRestoreAttempt(ctx, attempt)
	if err != nil {
		t.Fatalf("EnsureRestoreAttempt: %v", err)
	}
	allocated := *stored.Checkpoint
	allocated.Stage = models.SilentRestoreStageCandidateAllocated
	allocated.CandidateExecutionID = candidateID
	changed, err := repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attemptID, *stored.Checkpoint, allocated)
	if err != nil || !changed {
		t.Fatalf("CompareAndSwapSilentRestoreCheckpoint: %v, %v", changed, err)
	}
	launching := allocated
	launching.Stage = models.SilentRestoreStageCandidateLaunching
	changed, err = repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attemptID, allocated, launching)
	if err != nil || !changed {
		t.Fatalf("mark candidate launch boundary: %v, %v", changed, err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: fixture.session.ID, SessionID: fixture.session.ID, TaskID: fixture.task.ID,
		ExecutorID: "executor-silent-restore", AgentExecutionID: candidateID, Status: "running",
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(candidate): %v", err)
	}
	return attemptID, &models.SilentRestoreCommit{
		AttemptID: attemptID, Checkpoint: launching,
		Generation: models.HarnessSessionGeneration{
			SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
			Generation: fixture.generation.Generation + 1, PredecessorGeneration: fixture.generation.Generation,
			NativeSessionID: fixture.generation.NativeSessionID,
			AgentType:       fixture.generation.AgentType, AdapterVersion: fixture.generation.AdapterVersion,
			OriginalWorkspace: fixture.generation.OriginalWorkspace, CurrentWorkspace: fixture.generation.CurrentWorkspace,
			NativeStateReference: fixture.generation.NativeStateReference, CreationReason: models.SilentRestoreAction,
		},
	}
}

func createSilentRestoreCandidate(t *testing.T, repo *Repository, sessionID, taskID string, state models.TaskSessionState, metadata string) {
	t.Helper()
	ctx := context.Background()
	workspaceID := "workspace-" + taskID
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: taskID, OwnerID: "owner"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID, WorkspaceID: workspaceID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, State: state}); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_sessions SET metadata = ? WHERE id = ?`), metadata, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "block-" + sessionID, SessionID: sessionID, IncarnationID: session.QueueIncarnationID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: "submission-" + sessionID,
		DeliveryStreamID: "stream-" + sessionID,
	}); err != nil {
		t.Fatal(err)
	}
}
