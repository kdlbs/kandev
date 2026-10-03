package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newManagedAgentTestRepo(t *testing.T) (*Repository, *sqlx.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "managed-agent.db")
	conn, err := dbutil.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(conn, "sqlite3")
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		_ = db.Close()
		t.Fatalf("initialize repository: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return repo, db, path
}

func seedManagedAgentSession(t *testing.T, repo *Repository, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-1")
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-1", Title: "Managed agent task"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
}

func managedAgentRequestSnapshot(prompt string) models.ManagedAgentRequestSnapshot {
	return models.ManagedAgentRequestSnapshot{
		Prompt: prompt, RepositoryURL: "https://github.com/acme/repo", StartingRef: "main",
		Model: "model-1", CallbackURL: "https://kandev.example",
	}
}

func transitionManagedAgentOperation(
	t *testing.T,
	repo *Repository,
	operation *models.ManagedAgentOperation,
	bindingRevision int64,
	leaseOwner string,
	state models.ManagedAgentSubmissionState,
	remoteRunID, sanitizedError string,
) *models.ManagedAgentOperation {
	t.Helper()
	updated, err := repo.CompareAndSwapManagedAgentOperation(context.Background(), models.ManagedAgentOperationUpdate{
		OperationID: operation.ID, ExpectedRevision: operation.Revision,
		ExpectedBindingRevision: bindingRevision, LeaseOwner: leaseOwner,
		State: state, RemoteRunID: remoteRunID, SanitizedError: sanitizedError,
	})
	if err != nil {
		t.Fatalf("transition managed agent operation to %s: %v", state, err)
	}
	return updated
}

func reserveManagedAgentStart(
	t *testing.T,
	repo *Repository,
	sessionID, turnID string,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation) {
	t.Helper()
	now := time.Now().UTC()
	binding := &models.ManagedAgentBinding{
		ID: "binding-" + sessionID, SessionID: sessionID, TaskID: "task-" + sessionID,
		WorkspaceID: "workspace-1", UserID: "user-1", ExecutionID: "execution-1",
		ProviderKind: "cursor_cloud", ExecutorID: "cursor-cloud", ExecutorProfileID: "profile-1",
		CredentialRef: "secret-ref-1", RemoteAgentID: "bc-11111111-1111-4111-8111-111111111111",
		Lifecycle: models.ManagedAgentBindingCreating,
		Launch: models.ManagedAgentLaunchSnapshot{
			RepositoryID: "repo-1", RepositoryURL: "https://github.com/acme/repo",
			StartingRef: "main", Model: "model-1", CallbackURL: "https://kandev.example",
		},
	}
	operation := &models.ManagedAgentOperation{
		ID: "operation-" + turnID, BindingID: binding.ID, PromptTurnID: turnID,
		Kind: models.ManagedAgentOperationCreate, RequestDigest: "digest-" + turnID,
		RequestSnapshot: managedAgentRequestSnapshot("first"),
		State:           models.ManagedAgentSubmissionReserved, CreatedAt: now,
	}
	reservedBinding, reservedOperation, replayed, err := repo.ReserveManagedAgentStart(
		context.Background(), binding, operation, "worker-1", now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("reserve managed agent start: %v", err)
	}
	if replayed {
		t.Fatal("first start reservation unexpectedly replayed")
	}
	return reservedBinding, reservedOperation
}

func TestManagedOperationSingleWriter(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-single-writer"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, first := reserveManagedAgentStart(t, repo, sessionID, "turn-initial")
	var err error
	accepted := transitionManagedAgentOperation(t, repo, first, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-initial", "")
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get binding: %v", err)
	}
	transitionManagedAgentOperation(t, repo, accepted, binding.Revision, "",
		models.ManagedAgentSubmissionSucceeded, accepted.RemoteRunID, "")
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload binding after settled operation: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var admitted int
	var conflicts int
	var unexpected []error
	for i := range 8 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			now := time.Now().UTC()
			operation := &models.ManagedAgentOperation{
				ID: fmt.Sprintf("operation-followup-%d", index), BindingID: binding.ID,
				PromptTurnID: fmt.Sprintf("turn-followup-%d", index),
				Kind:         models.ManagedAgentOperationFollowup, RequestDigest: fmt.Sprintf("digest-%d", index),
				RequestSnapshot: managedAgentRequestSnapshot("follow up"),
			}
			_, _, replayed, err := repo.ReserveManagedAgentOperation(
				ctx, operation, binding.Revision, "worker-followup", now.Add(time.Minute),
			)
			mu.Lock()
			defer mu.Unlock()
			if err == nil && !replayed {
				admitted++
				return
			}
			if errors.Is(err, ErrManagedAgentRevisionConflict) || errors.Is(err, ErrManagedAgentActiveOperation) {
				conflicts++
				return
			}
			unexpected = append(unexpected, err)
		}(i)
	}
	close(start)
	wg.Wait()
	if admitted != 1 || conflicts != 7 || len(unexpected) != 0 {
		t.Fatalf("reservations admitted=%d conflicts=%d unexpected=%v, want 1/7/0", admitted, conflicts, unexpected)
	}
	var active int
	if err := repo.db.GetContext(ctx, &active, `SELECT COUNT(*) FROM managed_agent_operations WHERE binding_id = ? AND submission_state IN ('reserved', 'submitting', 'accepted', 'unknown')`, binding.ID); err != nil {
		t.Fatalf("count active operations: %v", err)
	}
	if active != 1 {
		t.Fatalf("active operation count = %d, want 1", active)
	}
}

func TestManagedOperationUnknown(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-unknown"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-unknown")
	var err error
	unknown := transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionUnknown, "", "provider submission outcome is unknown")
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload binding: %v", err)
	}

	newOperation := &models.ManagedAgentOperation{
		ID: "operation-blocked", BindingID: binding.ID, PromptTurnID: "turn-blocked",
		Kind: models.ManagedAgentOperationFollowup, RequestDigest: "digest-blocked",
		RequestSnapshot: managedAgentRequestSnapshot("another turn"),
	}
	if _, _, _, err := repo.ReserveManagedAgentOperation(
		ctx, newOperation, binding.Revision, "worker-2", time.Now().Add(time.Minute),
	); !errors.Is(err, ErrManagedAgentActiveOperation) {
		t.Fatalf("reserve while outcome unknown error = %v, want ErrManagedAgentActiveOperation", err)
	}

	retry := *operation
	retry.ID = "operation-retry-id"
	retry.State = ""
	_, replay, replayed, err := repo.ReserveManagedAgentStart(
		ctx, binding, &retry, "worker-2", time.Now().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("lookup repeated prompt turn: %v", err)
	}
	if !replayed || replay.State != models.ManagedAgentSubmissionUnknown || replay.ID != unknown.ID {
		t.Fatalf("replayed operation = %+v, replayed=%t; want existing unknown operation %+v", replay, replayed, unknown)
	}
}

func TestManagedCompletionOutboxSurvivesTerminalSettlementUntilAcknowledged(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-completion-outbox"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-completion-outbox")
	operation = transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionSubmitting, "", "")
	binding, _ = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	operation = transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-completion-outbox", "")
	binding, _ = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	pending := true
	settled, err := repo.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: operation.ID, ExpectedRevision: operation.Revision, ExpectedBindingRevision: binding.Revision,
		State: models.ManagedAgentSubmissionSucceeded, CompletionPending: &pending,
	})
	if err != nil {
		t.Fatalf("settle with completion outbox: %v", err)
	}
	if !settled.CompletionPending {
		t.Fatal("terminal settlement did not durably retain its completion")
	}
	bindings, err := repo.ListActiveManagedAgentBindings(ctx)
	if err != nil || len(bindings) != 1 || bindings[0].ID != binding.ID {
		t.Fatalf("recovery bindings before acknowledgement = %#v, %v; want the outbox binding", bindings, err)
	}
	reloaded, err := repo.GetManagedAgentOperation(ctx, operation.ID)
	if err != nil || !reloaded.CompletionPending {
		t.Fatalf("reloaded completion outbox = %#v, %v; want pending", reloaded, err)
	}
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload binding with pending completion: %v", err)
	}
	followup := &models.ManagedAgentOperation{
		ID: "operation-after-outbox", BindingID: binding.ID, PromptTurnID: "turn-after-outbox",
		Kind: models.ManagedAgentOperationFollowup, RequestDigest: "digest-after-outbox",
		RequestSnapshot: managedAgentRequestSnapshot("follow up"),
	}
	if _, _, _, err := repo.ReserveManagedAgentOperation(ctx, followup, binding.Revision, "worker-2", time.Now().Add(time.Minute)); !errors.Is(err, ErrManagedAgentActiveOperation) {
		t.Fatalf("follow-up before completion acknowledgement = %v, want pending receipt to block dispatch", err)
	}
	if err := repo.AcknowledgeManagedAgentCompletion(ctx, operation.ID); err != nil {
		t.Fatalf("acknowledge completion outbox: %v", err)
	}
	if err := repo.AcknowledgeManagedAgentCompletion(ctx, operation.ID); err != nil {
		t.Fatalf("repeat completion acknowledgement: %v", err)
	}
	bindings, err = repo.ListActiveManagedAgentBindings(ctx)
	if err != nil || len(bindings) != 0 {
		t.Fatalf("recovery bindings after acknowledgement = %#v, %v; want none", bindings, err)
	}
	reloaded, err = repo.GetManagedAgentOperation(ctx, operation.ID)
	if err != nil || reloaded.CompletionPending {
		t.Fatalf("completion outbox after acknowledgement = %#v, %v; want cleared", reloaded, err)
	}
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload binding after completion acknowledgement: %v", err)
	}
	if _, _, replayed, err := repo.ReserveManagedAgentOperation(ctx, followup, binding.Revision, "worker-2", time.Now().Add(time.Minute)); err != nil || replayed {
		t.Fatalf("follow-up after completion acknowledgement = replayed %t err %v, want admitted", replayed, err)
	}
}

func TestManagedOperationPersistsResultSnapshot(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-result-snapshot"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-result-snapshot")
	accepted := transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-result", "")
	binding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload binding: %v", err)
	}
	result := models.ManagedAgentResultSnapshot{
		RepositoryID: "repo-1", Branch: "cursor/fix", PullRequestURL: "https://github.com/acme/repo/pull/9",
		AgentURL: "https://cursor.com/agents/bc-1",
	}
	_, err = repo.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: accepted.ID, ExpectedRevision: accepted.Revision,
		ExpectedBindingRevision: binding.Revision, State: models.ManagedAgentSubmissionSucceeded,
		ResultSnapshot: &result,
	})
	if err != nil {
		t.Fatalf("settle operation with result: %v", err)
	}
	stored, err := repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		t.Fatalf("reload operation: %v", err)
	}
	if stored.ResultSnapshot != result {
		t.Fatalf("stored result = %+v, want %+v", stored.ResultSnapshot, result)
	}
}

func TestManagedAgentTerminationRevokesGrantsAndBlocksDispatch(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-termination"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-termination")
	accepted := transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-termination", "")
	binding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Lifecycle != models.ManagedAgentBindingReady {
		t.Fatalf("accepted create lifecycle = %s, want ready", binding.Lifecycle)
	}
	if err := repo.CreateManagedAgentToolGrant(ctx, &models.ManagedAgentToolGrant{
		ID: "grant-termination", BindingID: binding.ID, OperationID: accepted.ID,
		TokenHash: "hash-termination", Scope: "{}", Generation: 1,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.BeginManagedAgentTermination(ctx, binding.ID, time.Now().UTC()); err != nil {
		t.Fatalf("begin termination: %v", err)
	}
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Lifecycle != models.ManagedAgentBindingTerminationPending {
		t.Fatalf("termination lifecycle = %s, want pending", binding.Lifecycle)
	}
	grant, err := repo.GetManagedAgentToolGrantByHash(ctx, "hash-termination")
	if err != nil || grant.RevokedAt == nil {
		t.Fatalf("grant after termination intent = %+v, %v; want revoked", grant, err)
	}
	followup := &models.ManagedAgentOperation{
		ID: "operation-after-archive", BindingID: binding.ID, PromptTurnID: "turn-after-archive",
		Kind: models.ManagedAgentOperationFollowup, RequestDigest: "digest-after-archive",
		RequestSnapshot: managedAgentRequestSnapshot("must not dispatch"),
	}
	if _, _, _, err := repo.ReserveManagedAgentOperation(ctx, followup, binding.Revision, "worker-2", time.Now().Add(time.Minute)); !errors.Is(err, ErrManagedAgentBindingConflict) {
		t.Fatalf("follow-up after termination intent error = %v, want binding conflict", err)
	}
	latest, err := repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	terminal := transitionManagedAgentOperation(t, repo, latest, binding.Revision, "",
		models.ManagedAgentSubmissionSucceeded, latest.RemoteRunID, "")
	if terminal.State != models.ManagedAgentSubmissionSucceeded {
		t.Fatalf("terminal state = %s, want succeeded", terminal.State)
	}
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Lifecycle != models.ManagedAgentBindingArchived {
		t.Fatalf("confirmed termination lifecycle = %s, want archived", binding.Lifecycle)
	}
}

func TestManagedAgentDispatchReservationRejectsArchivedTask(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-archived-reservation"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	if err := repo.ArchiveTask(ctx, "task-"+sessionID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	now := time.Now().UTC()
	binding := &models.ManagedAgentBinding{
		ID: "binding-" + sessionID, SessionID: sessionID, TaskID: "task-" + sessionID,
		WorkspaceID: "workspace-1", UserID: "user-1", ExecutionID: "execution-1",
		ProviderKind: "cursor_cloud", ExecutorID: "cursor-cloud", ExecutorProfileID: "profile-1",
		CredentialRef: "secret-ref-1", RemoteAgentID: "bc-11111111-1111-4111-8111-111111111111",
		Lifecycle: models.ManagedAgentBindingCreating,
		Launch: models.ManagedAgentLaunchSnapshot{
			RepositoryID: "repo-1", RepositoryURL: "https://github.com/acme/repo",
			StartingRef: "main", Model: "model-1", CallbackURL: "https://kandev.example",
		},
	}
	operation := &models.ManagedAgentOperation{
		ID: "operation-archived-reservation", BindingID: binding.ID, PromptTurnID: "turn-archived-reservation",
		Kind: models.ManagedAgentOperationCreate, RequestDigest: "digest-archived-reservation",
		RequestSnapshot: managedAgentRequestSnapshot("must not create"),
	}
	if _, _, _, err := repo.ReserveManagedAgentStart(ctx, binding, operation, "worker", now.Add(time.Minute)); !errors.Is(err, ErrManagedAgentBindingConflict) {
		t.Fatalf("archived task reservation error = %v, want binding conflict", err)
	}
}

func TestManagedAgentLeaseClaimRejectsReservedCreateAfterArchiveIntent(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-archive-before-dispatch"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-archive-before-dispatch")
	if err := repo.ArchiveTask(ctx, binding.TaskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if err := repo.BeginManagedAgentTermination(ctx, binding.ID, time.Now().UTC()); err != nil {
		t.Fatalf("begin termination: %v", err)
	}
	binding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ClaimManagedAgentDispatchLease(ctx, binding.ID, operation.ID, binding.Revision, "worker-after-archive", time.Now().Add(time.Minute)); !errors.Is(err, ErrManagedAgentBindingConflict) {
		t.Fatalf("post-archive lease claim error = %v, want binding conflict", err)
	}
}

func TestManagedAgentCancellationLeaseCanDrainArchivedRun(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-cancel-archived-run"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-cancel-archived-run")
	accepted := transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-1", "")
	if err := repo.ArchiveTask(ctx, binding.TaskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if err := repo.BeginManagedAgentTermination(ctx, binding.ID, time.Now().UTC()); err != nil {
		t.Fatalf("begin binding termination: %v", err)
	}
	binding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload terminating binding: %v", err)
	}
	if _, err := repo.ClaimManagedAgentDispatchLease(ctx, binding.ID, accepted.ID, binding.Revision,
		"dispatch-after-archive", time.Now().Add(time.Minute)); !errors.Is(err, ErrManagedAgentBindingConflict) {
		t.Fatalf("dispatch lease claim after archive = %v, want binding conflict", err)
	}
	claimed, err := repo.ClaimManagedAgentCancellationLease(ctx, binding.ID, accepted.ID, binding.Revision,
		"cancel-after-archive", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("cancellation lease claim after archive: %v", err)
	}
	if claimed.DispatchOwner != "cancel-after-archive" || claimed.Lifecycle != models.ManagedAgentBindingTerminationPending {
		t.Fatalf("cancellation claim binding = %+v, want lease without reopening dispatch", claimed)
	}
}

func TestUnarchiveLeavesConfirmedCloudBindingIdleUntilExplicitFollowup(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-unarchive-cloud"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-unarchive-cloud")
	accepted := transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-unarchive", "")
	if err := repo.ArchiveTask(ctx, binding.TaskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if err := repo.BeginManagedAgentTermination(ctx, binding.ID, time.Now().UTC()); err != nil {
		t.Fatalf("begin termination: %v", err)
	}
	binding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	transitionManagedAgentOperation(t, repo, accepted, binding.Revision, "",
		models.ManagedAgentSubmissionSucceeded, accepted.RemoteRunID, "")
	unarchived, err := repo.UnarchiveTask(ctx, binding.TaskID)
	if err != nil || !unarchived {
		t.Fatalf("unarchive task = %t, %v; want restored task", unarchived, err)
	}
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Lifecycle != models.ManagedAgentBindingArchived {
		t.Fatalf("binding after unarchive = %s, want archived until explicit follow-up", binding.Lifecycle)
	}
	if err := repo.ResolveManagedAgentTermination(ctx, binding.ID, models.ManagedAgentBindingReady, time.Now().UTC()); err != nil {
		t.Fatalf("explicitly restore terminal conversation: %v", err)
	}
	var active int
	if err := repo.db.GetContext(ctx, &active, `SELECT COUNT(*) FROM managed_agent_operations WHERE binding_id = ? AND submission_state IN ('reserved', 'submitting', 'accepted', 'cancelling', 'unknown')`, binding.ID); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("unarchive started %d remote operations, want no automatic dispatch", active)
	}
}

func TestManagedUnknownRetryAcknowledgmentAndReservationAreAtomic(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-explicit-retry"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, initial := reserveManagedAgentStart(t, repo, sessionID, "turn-initial-retry")
	accepted := transitionManagedAgentOperation(t, repo, initial, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-initial", "")
	binding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	settled := transitionManagedAgentOperation(t, repo, accepted, binding.Revision, "",
		models.ManagedAgentSubmissionSucceeded, accepted.RemoteRunID, "")
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	unknownCandidate := &models.ManagedAgentOperation{
		ID: "operation-unknown-followup", BindingID: binding.ID, PromptTurnID: "turn-unknown-followup",
		Kind: models.ManagedAgentOperationFollowup, RequestDigest: "digest-unknown-followup",
		RequestSnapshot: managedAgentRequestSnapshot("retry this prompt"),
	}
	reservedBinding, unknown, _, err := repo.ReserveManagedAgentOperation(ctx, unknownCandidate, binding.Revision, "worker-2", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("reserve unknown candidate: %v", err)
	}
	unknown = transitionManagedAgentOperation(t, repo, unknown, reservedBinding.Revision, "worker-2",
		models.ManagedAgentSubmissionSubmitting, "", "")
	unknown = transitionManagedAgentOperation(t, repo, unknown, reservedBinding.Revision, "worker-2",
		models.ManagedAgentSubmissionUnknown, "", "response timed out")
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	retry := &models.ManagedAgentOperation{
		ID: "operation-explicit-retry", BindingID: binding.ID, PromptTurnID: "turn-explicit-retry",
		Kind: models.ManagedAgentOperationFollowup, RequestDigest: "digest-explicit-retry",
		RequestSnapshot:              managedAgentRequestSnapshot("retry this prompt"),
		RetryAcknowledgesOperationID: unknown.ID, DuplicationRiskAcknowledged: true,
	}
	newBinding, retried, replayed, err := repo.ReserveManagedAgentOperation(ctx, retry, binding.Revision, "worker-3", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("reserve acknowledged retry: %v", err)
	}
	if replayed || retried.State != models.ManagedAgentSubmissionReserved || newBinding.DispatchGeneration != binding.DispatchGeneration+1 {
		t.Fatalf("reservation binding=%+v operation=%+v replayed=%t, want one fresh generation", newBinding, retried, replayed)
	}
	storedUnknown, err := repo.GetManagedAgentOperationByPromptTurnID(ctx, unknown.PromptTurnID)
	if err != nil {
		t.Fatal(err)
	}
	if storedUnknown.State != models.ManagedAgentSubmissionRetryAcked || storedUnknown.SanitizedError == "" {
		t.Fatalf("prior uncertain operation = %+v, want explicit retry acknowledgment", storedUnknown)
	}
	if _, err := repo.GetManagedAgentOperationByPromptTurnID(ctx, settled.PromptTurnID); err != nil {
		t.Fatal(err)
	}
}

func TestManagedCreateUnknownRetryAcknowledgmentIsAtomic(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-create-explicit-retry"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, initial := reserveManagedAgentStart(t, repo, sessionID, "turn-create-initial")
	submitting := transitionManagedAgentOperation(t, repo, initial, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionSubmitting, "", "")
	unknown := transitionManagedAgentOperation(t, repo, submitting, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionUnknown, "", "response timed out")
	binding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	retrySnapshot := unknown.RequestSnapshot
	retry := &models.ManagedAgentOperation{
		ID: "operation-create-retry", BindingID: binding.ID, PromptTurnID: "turn-create-retry",
		Kind: models.ManagedAgentOperationCreate, RequestDigest: "digest-create-retry",
		RequestSnapshot: retrySnapshot, RetryAcknowledgesOperationID: unknown.ID,
		DuplicationRiskAcknowledged: true,
	}
	reservedBinding, reserved, replayed, err := repo.ReserveManagedAgentOperation(
		ctx, retry, binding.Revision, "worker-2", time.Now().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("reserve acknowledged create retry: %v", err)
	}
	if replayed || reserved.State != models.ManagedAgentSubmissionReserved ||
		reservedBinding.Lifecycle != models.ManagedAgentBindingCreating {
		t.Fatalf("create retry reservation binding=%+v operation=%+v replayed=%t, want creating binding and reserved operation", reservedBinding, reserved, replayed)
	}
	storedUnknown, err := repo.GetManagedAgentOperation(ctx, unknown.ID)
	if err != nil || storedUnknown.State != models.ManagedAgentSubmissionRetryAcked {
		t.Fatalf("original create operation = %+v, %v; want explicit retry acknowledgment", storedUnknown, err)
	}
	started := transitionManagedAgentOperation(t, repo, reserved, reservedBinding.Revision, "worker-2",
		models.ManagedAgentSubmissionSubmitting, "", "")
	accepted := transitionManagedAgentOperation(t, repo, started, reservedBinding.Revision, "worker-2",
		models.ManagedAgentSubmissionAccepted, "run-create-retry", "")
	finalBinding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil || finalBinding.Lifecycle != models.ManagedAgentBindingReady || accepted.RemoteRunID != "run-create-retry" {
		t.Fatalf("accepted create retry binding=%+v operation=%+v error=%v, want ready binding", finalBinding, accepted, err)
	}
}

func TestManagedBindingDeletionRequiresConfirmedTerminalOperation(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-binding-delete"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-binding-delete")
	if err := repo.DeleteManagedAgentBindingIfTerminal(ctx, binding.ID); !errors.Is(err, ErrManagedAgentActiveOperation) {
		t.Fatalf("delete active binding error = %v, want ErrManagedAgentActiveOperation", err)
	}
	submitting := transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionSubmitting, "", "")
	accepted := transitionManagedAgentOperation(t, repo, submitting, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-1", "")
	binding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload accepted binding: %v", err)
	}
	transitionManagedAgentOperation(t, repo, accepted, binding.Revision, "",
		models.ManagedAgentSubmissionSucceeded, "run-1", "")
	if err := repo.DeleteManagedAgentBindingIfTerminal(ctx, binding.ID); err != nil {
		t.Fatalf("delete confirmed terminal binding: %v", err)
	}
	if _, err := repo.GetManagedAgentBindingBySession(ctx, sessionID); !errors.Is(err, ErrManagedAgentBindingNotFound) {
		t.Fatalf("binding after terminal delete error = %v, want not found", err)
	}
}

func TestManagedDispatchLeaseExpiryFencesStaleWriter(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-expired-lease"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-expired-lease")
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE managed_agent_bindings SET dispatch_lease_until = ? WHERE id = ?
	`), time.Now().Add(-time.Minute), binding.ID); err != nil {
		t.Fatalf("expire initial lease: %v", err)
	}
	claimed, err := repo.ClaimManagedAgentDispatchLease(ctx, binding.ID, operation.ID,
		binding.Revision, "worker-2", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("claim expired lease: %v", err)
	}
	_, err = repo.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: operation.ID, ExpectedRevision: operation.Revision,
		ExpectedBindingRevision: binding.Revision, LeaseOwner: "worker-1",
		State: models.ManagedAgentSubmissionSubmitting,
	})
	if !errors.Is(err, ErrManagedAgentRevisionConflict) {
		t.Fatalf("stale worker transition error = %v, want revision conflict", err)
	}
	updated, err := repo.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: operation.ID, ExpectedRevision: operation.Revision,
		ExpectedBindingRevision: claimed.Revision, LeaseOwner: "worker-2",
		State: models.ManagedAgentSubmissionSubmitting,
	})
	if err != nil || updated.State != models.ManagedAgentSubmissionSubmitting || updated.DispatchStartedAt == nil {
		t.Fatalf("current lease transition = %+v err=%v", updated, err)
	}
}
