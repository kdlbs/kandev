package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func newManagedAgentPostgresRepo(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres repository: %v", err)
	}
	return repo
}

func TestPostgresManagedSingleWriter(t *testing.T) {
	repo := newManagedAgentPostgresRepo(t)
	ctx := context.Background()
	const sessionID = "session-pg-single-writer"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, create := reserveManagedAgentStart(t, repo, sessionID, "turn-pg-create")
	var err error
	accepted := transitionManagedAgentOperation(t, repo, create, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-pg", "")
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get binding: %v", err)
	}
	transitionManagedAgentOperation(t, repo, accepted, binding.Revision, "",
		models.ManagedAgentSubmissionSucceeded, accepted.RemoteRunID, "")
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload binding: %v", err)
	}
	now := time.Now().UTC()
	operation := &models.ManagedAgentOperation{
		ID: "operation-pg-followup", BindingID: binding.ID, PromptTurnID: "turn-pg-followup",
		Kind: models.ManagedAgentOperationFollowup, RequestDigest: "digest-pg-followup",
		RequestSnapshot: managedAgentRequestSnapshot("follow up"),
	}
	if _, _, replayed, err := repo.ReserveManagedAgentOperation(
		ctx, operation, binding.Revision, "worker-pg", now.Add(time.Minute),
	); err != nil || replayed {
		t.Fatalf("reserve postgres follow-up: replayed=%t err=%v", replayed, err)
	}
	if _, _, _, err := repo.ReserveManagedAgentOperation(
		ctx, &models.ManagedAgentOperation{
			ID: "operation-pg-second", BindingID: binding.ID, PromptTurnID: "turn-pg-second",
			Kind: models.ManagedAgentOperationFollowup, RequestDigest: "digest-pg-second",
			RequestSnapshot: managedAgentRequestSnapshot("second"),
		}, binding.Revision+1, "worker-pg-second", now.Add(time.Minute),
	); !errors.Is(err, ErrManagedAgentActiveOperation) && !errors.Is(err, ErrManagedAgentRevisionConflict) {
		t.Fatalf("second postgres writer error = %v, want active-operation or revision conflict", err)
	}
}

func TestPostgresManagedRecovery(t *testing.T) {
	repo := newManagedAgentPostgresRepo(t)
	ctx := context.Background()
	const sessionID = "session-pg-recovery"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-pg-recovery")
	var err error
	accepted := transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-pg-recovery", "")
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload binding: %v", err)
	}
	unknown := transitionManagedAgentOperation(t, repo, accepted, binding.Revision, "",
		models.ManagedAgentSubmissionUnknown, accepted.RemoteRunID, "provider response lost")
	if applied, err := repo.CommitManagedAgentStreamEvent(ctx, models.ManagedAgentStreamEvent{
		BindingID: binding.ID, OperationID: unknown.ID, RemoteRunID: unknown.RemoteRunID,
		EventID: "pg-event-1", EventType: "run_status", Cursor: "pg-cursor-1",
		DispatchGeneration: unknown.DispatchGeneration,
	}); err != nil || !applied {
		t.Fatalf("commit postgres checkpoint: applied=%t err=%v", applied, err)
	}
	if err := repo.initManagedAgentSchema(); err != nil {
		t.Fatalf("replay managed agent migration: %v", err)
	}
	storedBinding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil || storedBinding.RemoteAgentID != binding.RemoteAgentID {
		t.Fatalf("postgres binding after replay = %+v err=%v", storedBinding, err)
	}
	storedOperation, err := repo.GetManagedAgentOperationByPromptTurnID(ctx, operation.PromptTurnID)
	if err != nil || storedOperation.State != models.ManagedAgentSubmissionUnknown {
		t.Fatalf("postgres unknown operation after replay = %+v err=%v", storedOperation, err)
	}
	checkpoint, err := repo.GetManagedAgentStreamCheckpoint(ctx, binding.ID, unknown.RemoteRunID)
	if err != nil || checkpoint.Cursor != "pg-cursor-1" {
		t.Fatalf("postgres checkpoint after replay = %+v err=%v", checkpoint, err)
	}
}
