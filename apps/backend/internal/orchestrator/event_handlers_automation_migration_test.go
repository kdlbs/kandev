package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
)

// storeBackedAutomationService serves the firing path out of a real
// automation.Store, so the variable in the test below is the stored
// execution_mode itself rather than a struct field a test typed in by hand.
// The three run-recording methods are stubs: this is a test about what a
// firing builds, not about run bookkeeping, which the sibling file covers.
type storeBackedAutomationService struct {
	store *automation.Store
}

func (s *storeBackedAutomationService) GetAutomation(
	ctx context.Context, id string,
) (*automation.Automation, error) {
	return s.store.GetAutomation(ctx, id)
}

func (s *storeBackedAutomationService) RecordRun(context.Context, *automation.AutomationRun) error {
	return nil
}

func (s *storeBackedAutomationService) MarkRunFailedByTaskID(context.Context, string, string) error {
	return nil
}

func (s *storeBackedAutomationService) MarkRunSucceededByTaskID(context.Context, string) error {
	return nil
}

// seedStoredAutomation writes one automation carrying the given stored
// execution_mode, the way an install running the pre-withdrawal code wrote
// it, and returns a service that reads it back through the real store.
func seedStoredAutomation(t *testing.T, executionMode string) *storeBackedAutomationService {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	store, err := automation.NewStore(db, db)
	require.NoError(t, err)

	now := time.Now().UTC()
	_, err = db.Exec(
		`INSERT INTO automations (id, workspace_id, name, description, workflow_id, workflow_step_id,
			agent_profile_id, executor_profile_id, prompt, task_title_template, execution_mode,
			enabled, max_concurrent_runs, webhook_secret, created_at, updated_at)
		VALUES ('a-1', 'ws-1', 'nightly sweep', '', 'wf-1', 'step-1', 'agent-1', 'exec-1',
			'sweep the logs', 'Nightly sweep', ?, 1, 1, 'secret', ?, ?)`,
		executionMode, now, now)
	require.NoError(t, err)

	return &storeBackedAutomationService{store: store}
}

// The single-destination invariant, stated as a comparison rather than as a
// list of properties: two automations differing ONLY in the execution_mode
// stored against them must produce byte-identical task requests. The old
// `task` mode is what put a card on the kanban, and honouring it again is the
// one thing withdrawing it was supposed to make impossible — so this asserts
// the whole request, not the fields a reviewer thought to name.
func TestCreateAutomationTask_IsIdenticalRegardlessOfStoredExecutionMode(t *testing.T) {
	built := map[string]*ReviewTaskRequest{}
	flags := map[string]bool{}

	for _, mode := range []string{"task", "run"} {
		repo := setupTestRepo(t)
		seedAutomationWorkspaceRepo(t, repo, "ws-1")
		autoSvc := seedStoredAutomation(t, mode)
		creator := &stubReviewTaskCreator{task: &models.Task{ID: "t-created"}}

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.SetAutomationService(autoSvc)
		svc.reviewTaskCreator = creator

		loaded, err := autoSvc.GetAutomation(context.Background(), "a-1")
		require.NoError(t, err)
		require.NotNil(t, loaded)
		flags[mode] = loaded.LegacyBoardCard

		svc.createAutomationTask(context.Background(), &automation.AutomationTriggeredEvent{
			AutomationID: "a-1", TriggerID: "trg-1", TriggerType: automation.TriggerTypeScheduled,
		})
		require.NotNil(t, creator.got, "the %s-mode automation created no task at all", mode)
		built[mode] = creator.got
	}

	// Without this the comparison could pass by both sides being blank: a
	// derivation that never fires makes every automation look alike, which is
	// agreement for the wrong reason. The two rows must genuinely differ in
	// what the migration notice reads before "and yet they fire alike" means
	// anything.
	require.True(t, flags["task"], "the task-mode row must be recognised as a pre-upgrade automation")
	require.False(t, flags["run"], "the run-mode row must not be")

	require.Equal(t, built["run"], built["task"],
		"a stored execution_mode must not reach the firing path; every automation has one destination")
	require.Equal(t, models.TaskOriginAutomationRun, built["task"].Origin,
		"and that destination is the hidden, origin-tagged automation run")
	require.False(t, built["task"].IsEphemeral)
}
