package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestCaptureWorkflowStartPromptAdmission_BackfillsLegacyEntry(t *testing.T) {
	svc, repo, ctx, _, _ := newWorkflowStartPromptAttemptFixture(
		t, models.TaskSessionStateStarting, "workflow-fence-execution",
	)
	if _, err := repo.DB().ExecContext(ctx,
		`DELETE FROM task_step_transitions WHERE task_id = ?`, "workflow-fence-task"); err != nil {
		t.Fatalf("remove legacy task transition rows: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "workflow-fence-session")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	_, entry, required, err := svc.captureWorkflowStartPromptAdmission(
		ctx, "workflow-fence-task", session,
	)
	if err != nil {
		t.Fatalf("capture legacy workflow entry: %v", err)
	}
	if !required {
		t.Fatal("workflow entry admission = unsupported, want strict admission")
	}
	if entry.TransitionID <= 0 {
		t.Fatalf("backfilled transition id = %d, want positive identity", entry.TransitionID)
	}
	var rows int
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?`,
		"workflow-fence-task").Scan(&rows); err != nil {
		t.Fatalf("count backfilled transition rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("backfilled transition rows = %d, want 1", rows)
	}
}

type workflowStartPromptTransitionReadFailureRepository struct {
	*sqliterepo.Repository
	err error
}

func (r *workflowStartPromptTransitionReadFailureRepository) GetLatestTaskStepTransitionID(context.Context, string) (int64, error) {
	return 0, r.err
}

func TestAutoStartCreatedLaunch_AbortsWhenAdmissionCaptureFails(t *testing.T) {
	fixture := newWorkflowAsyncStartFailureFixture(t, errors.New("provider startup failed"))
	fixture.svc.repo = &workflowStartPromptTransitionReadFailureRepository{
		Repository: fixture.repo,
		err:        errors.New("transition ledger unavailable"),
	}

	err := fixture.svc.autoStartStepPrompt(
		context.Background(), fixture.taskID, fixture.session, fixture.step,
		fixture.prompt, false, true, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "read workflow launch entry") {
		t.Fatalf("auto-start error = %v, want admission capture failure", err)
	}
	select {
	case <-fixture.startEntered:
		t.Fatal("agent launch started after admission capture failed")
	case <-time.After(100 * time.Millisecond):
	}
	if got := fixture.svc.messageQueue.GetStatus(context.Background(), fixture.sessionID).Count; got != 0 {
		t.Fatalf("queue count after admission capture failure = %d, want 0", got)
	}
}

type workflowStartPromptSessionReadFailureRepository struct {
	*sqliterepo.Repository
	err error
}

func (r *workflowStartPromptSessionReadFailureRepository) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return nil, r.err
}

func TestDispatchTakenQueuedMessage_RetainsEntryWhenInputReadFails(t *testing.T) {
	svc, repo, ctx, _, _ := newWorkflowStartPromptAttemptFixture(
		t, models.TaskSessionStateStarting, "workflow-fence-execution",
	)
	svc.repo = &workflowStartPromptSessionReadFailureRepository{
		Repository: repo,
		err:        errors.New("session metadata temporarily unavailable"),
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "workflow-fence-session", "workflow-fence-task", "", "",
		messagequeue.QueuedByWorkflow, false, nil, nil,
	); err != nil {
		t.Fatalf("queue empty raw entry: %v", err)
	}
	identity, err := svc.messageQueue.ResolveSessionIdentity(
		ctx, "workflow-fence-task", "workflow-fence-session",
	)
	if err != nil {
		t.Fatalf("resolve session identity: %v", err)
	}
	queued, exists, autoRun, err := svc.messageQueue.
		ReserveQueuedWithAutoRunForSession(ctx, identity)
	if err != nil {
		t.Fatalf("reserve queued entry: %v", err)
	}
	if !exists || !autoRun || queued == nil {
		t.Fatalf("reservation = (%#v, %t, %t), want one auto-run entry", queued, exists, autoRun)
	}
	if dispatched := svc.dispatchTakenQueuedMessageForSession(ctx, identity, queued, true); dispatched {
		t.Fatal("dispatch returned success after input inspection failed")
	}
	if got := svc.messageQueue.GetStatus(ctx, identity.SessionID).Count; got != 1 {
		t.Fatalf("queue count after input inspection failure = %d, want retained entry", got)
	}
}
