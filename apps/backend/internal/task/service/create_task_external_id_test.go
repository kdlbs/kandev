package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func seedWorkspaceAndWorkflowForCreate(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, wsID string) string {
	t.Helper()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: wsID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wfID := wsID + "-wf"
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: wfID, WorkspaceID: wsID, Name: "WF"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	return wfID
}

func mustSettle(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID, externalID string) {
	t.Helper()
	ok, err := repo.SettleTaskExternalID(ctx, taskID, externalID, time.Now().UTC())
	if err != nil {
		t.Fatalf("settle %s/%s: %v", taskID, externalID, err)
	}
	if !ok {
		t.Fatalf("settle %s/%s affected zero rows", taskID, externalID)
	}
}

func countTasksHoldingExternalID(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, workspaceID, externalID string) int {
	t.Helper()
	if _, err := repo.GetTaskByExternalID(ctx, workspaceID, externalID); err != nil {
		return 0
	}
	return 1
}

// TestCreateTaskWithExternalIDGoldenPath covers the spec's golden path: a
// create carrying a fresh external_id makes a new task, reports Created, and
// leaves the row unsettled — settlement is the handler's job, not the
// service's, per the create-sequence contract.
func TestCreateTaskWithExternalIDGoldenPath(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-golden")

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-golden",
		WorkflowID:  wfID,
		Title:       "Task",
		ExternalID:  "ext-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if result.Outcome != CreateTaskOutcomeCreated {
		t.Fatalf("outcome = %v, want Created", result.Outcome)
	}
	if result.Task.ExternalID != "ext-1" {
		t.Fatalf("task.ExternalID = %q, want ext-1", result.Task.ExternalID)
	}
	if result.Task.ExternalIDSettledAt != nil {
		t.Fatal("task should be unsettled immediately after Service.CreateTask — settlement is the handler's job")
	}
	published := eventBus.GetPublishedEvents()
	if len(published) != 1 || published[0].Type != "task.created" {
		t.Fatalf("events = %#v, want exactly one task.created", published)
	}
}

// TestCreateTaskWithExternalIDFoundSettled covers the dedupe hit on a settled
// task: no new row, no event, deduplicated outcome, existing task returned
// unchanged even though the retry payload differs.
func TestCreateTaskWithExternalIDFoundSettled(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-found-settled")

	first, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-found-settled", WorkflowID: wfID, Title: "Original", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	mustSettle(t, ctx, repo, first.Task.ID, "ext-1")

	retry, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-found-settled", WorkflowID: wfID, Title: "Changed", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create retry: %v", err)
	}
	if retry.Outcome != CreateTaskOutcomeFoundSettled {
		t.Fatalf("outcome = %v, want FoundSettled", retry.Outcome)
	}
	if retry.Task.ID != first.Task.ID {
		t.Fatalf("retry task id = %s, want %s", retry.Task.ID, first.Task.ID)
	}
	if retry.Task.Title != "Original" {
		t.Fatalf("retry task title = %q, want unchanged Original", retry.Task.Title)
	}
	published := eventBus.GetPublishedEvents()
	if len(published) != 1 {
		t.Fatalf("events = %#v, want exactly one (from the first create only)", published)
	}

	if count := countTasksHoldingExternalID(t, ctx, repo, "ws-found-settled", "ext-1"); count != 1 {
		t.Fatalf("tasks holding ext-1 = %d, want 1", count)
	}
}

// TestCreateTaskWithExternalIDFoundUnsettled covers the diagnostic tuple:
// deduplicated + creation_complete:false — a retry against a task whose
// create has not finished must not create anything, and must return the
// unsettled task unmodified, repeatedly, with no timeout-based change.
func TestCreateTaskWithExternalIDFoundUnsettled(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-unsettled")

	first, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-unsettled", WorkflowID: wfID, Title: "Task", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	for i := 0; i < 3; i++ {
		retry, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-unsettled", WorkflowID: wfID, Title: "Task", ExternalID: "ext-1",
		})
		if err != nil {
			t.Fatalf("retry %d: %v", i, err)
		}
		if retry.Outcome != CreateTaskOutcomeFoundUnsettled {
			t.Fatalf("retry %d outcome = %v, want FoundUnsettled", i, retry.Outcome)
		}
		if retry.Task.ID != first.Task.ID {
			t.Fatalf("retry %d task id = %s, want %s", i, retry.Task.ID, first.Task.ID)
		}
	}

	if count := countTasksHoldingExternalID(t, ctx, repo, "ws-unsettled", "ext-1"); count != 1 {
		t.Fatalf("tasks holding ext-1 = %d, want 1 (no duplicates from repeated retries)", count)
	}
}

// TestCreateTaskWithExternalIDLookupPrecedesIdentifierAllocation is the
// office-task variant of the lookup-before-write ordering requirement:
// resolving a Found outcome via the step-3 lookup must not burn a
// task_sequence number.
func TestCreateTaskWithExternalIDLookupPrecedesIdentifierAllocation(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wsID := "ws-office-seq"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: wsID, TaskPrefix: "KAN"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := repo.EnsureOfficeWorkflow(ctx, wsID); err != nil {
		t.Fatalf("ensure office workflow: %v", err)
	}

	first, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: wsID, Title: "Office task", ExternalID: "ext-1",
		Origin: models.TaskOriginAgentCreated, ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("create first office task: %v", err)
	}
	if first.Task.Identifier == "" {
		t.Fatal("first office create should have assigned an identifier")
	}
	mustSettle(t, ctx, repo, first.Task.ID, "ext-1")

	ws, err := repo.GetWorkspace(ctx, wsID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	seqBefore := ws.TaskSequence

	retry, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: wsID, Title: "Office task retry", ExternalID: "ext-1",
		Origin: models.TaskOriginAgentCreated, ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("retry office task: %v", err)
	}
	if retry.Outcome != CreateTaskOutcomeFoundSettled {
		t.Fatalf("outcome = %v, want FoundSettled", retry.Outcome)
	}

	wsAfter, err := repo.GetWorkspace(ctx, wsID)
	if err != nil {
		t.Fatalf("get workspace after retry: %v", err)
	}
	if wsAfter.TaskSequence != seqBefore {
		t.Fatalf("task_sequence = %d, want unchanged %d — the step-3 lookup must resolve before identifier allocation", wsAfter.TaskSequence, seqBefore)
	}
}

// TestCreateTaskWithExternalIDConcurrentRaceYieldsOneWinner exercises the
// TOCTOU backstop: two creates for the same never-before-seen external_id,
// racing past the step-3 lookup, must produce exactly one Created + one
// Found outcome, no orphan row, and no 5xx-shaped error from either caller.
func TestCreateTaskWithExternalIDConcurrentRaceYieldsOneWinner(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-race")

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]CreateTaskResult, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = svc.CreateTask(ctx, &CreateTaskRequest{
				WorkspaceID: "ws-race", WorkflowID: wfID, Title: "Race", ExternalID: "ext-race",
			})
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d returned an error instead of a Found outcome: %v", i, err)
		}
	}

	createdCount, foundCount := 0, 0
	for _, r := range results {
		switch r.Outcome {
		case CreateTaskOutcomeCreated:
			createdCount++
		case CreateTaskOutcomeFoundSettled, CreateTaskOutcomeFoundUnsettled:
			foundCount++
		}
	}
	if createdCount != 1 || foundCount != 1 {
		t.Fatalf("outcomes = %#v, want exactly one Created and one Found", results)
	}
	if results[0].Task.ID != results[1].Task.ID {
		t.Fatalf("both callers must observe the same winning task: %s vs %s", results[0].Task.ID, results[1].Task.ID)
	}

	if count := countTasksHoldingExternalID(t, ctx, repo, "ws-race", "ext-race"); count != 1 {
		t.Fatalf("tasks holding ext-race = %d, want exactly 1 (no orphan from the loser)", count)
	}
}

// TestCreateTaskWithoutExternalIDUnaffected pins the "omitting external_id
// leaves task creation behaving exactly as it does today" requirement.
func TestCreateTaskWithoutExternalIDUnaffected(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-no-ext")

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-no-ext", WorkflowID: wfID, Title: "Plain task",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if result.Outcome != CreateTaskOutcomeCreated {
		t.Fatalf("outcome = %v, want Created", result.Outcome)
	}
	if result.Task.ExternalID != "" {
		t.Fatalf("task.ExternalID = %q, want empty", result.Task.ExternalID)
	}

	second, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-no-ext", WorkflowID: wfID, Title: "Plain task",
	})
	if err != nil {
		t.Fatalf("create second task: %v", err)
	}
	if second.Task.ID == result.Task.ID {
		t.Fatal("two creates without external_id must never collide")
	}
}

// TestCreateTaskWithInvalidExternalIDFailsValidation covers a malformed
// external_id short-circuiting before any task is created.
func TestCreateTaskWithInvalidExternalIDFailsValidation(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-invalid-ext")

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-invalid-ext", WorkflowID: wfID, Title: "Task", ExternalID: "ext-1\n",
	})
	if err == nil {
		t.Fatal("expected a validation error for a control character in external_id")
	}
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatal("no task should be created when external_id validation fails")
	}
}

// seedAdmissionPreemptionWorkflow wires a target step with a WIP limit of 1
// and a feeder step that is already saturated. Docs/specs/tasks/external-id-
// idempotency/spec.md's "Lookup-before-write ordering" section: with no
// feeder configured, a loser that misses the target's last slot is only
// QUEUED (not rejected) by internal/task/repository/sqlite/task.go's
// applyAdmissionPlacement, and its insert then fails on the unique index
// instead — already covered by
// TestCreateTaskWithExternalIDConcurrentRaceYieldsOneWinner. The genuinely
// untested path is admission rejecting BEFORE the insert is even attempted,
// which this implementation only does when the feeder is configured and is
// itself also at capacity (applyAdmissionPlacement's default branch,
// task.go:258-265). That is the configuration exercised here.
func seedAdmissionPreemptionWorkflow(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, svc *Service) (workspaceID, targetStepID string) {
	t.Helper()
	workspaceID = "ws-admission-preempt"
	workflowID := "wf-admission-preempt"
	feederStepID := "feeder-step"
	targetStepID = "target-step"

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "WF"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "feeder-occupant", WorkspaceID: workspaceID, WorkflowID: workflowID,
		WorkflowStepID: feederStepID, WIPAdmitted: true, Title: "Feeder occupant",
	}); err != nil {
		t.Fatalf("seed feeder occupant: %v", err)
	}

	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		feederStepID: {ID: feederStepID, WorkflowID: workflowID, Name: "Feeder", WIPLimit: 1},
		targetStepID: {ID: targetStepID, WorkflowID: workflowID, Name: "Target", WIPLimit: 1, PullFromStepID: feederStepID},
	}})
	return workspaceID, targetStepID
}

// TestCreateTaskWithExternalIDAdmissionPreemptionGuard is the spec's
// "load-bearing" admission-preemption scenario (LO4): a target step with
// exactly one remaining WIP slot, two creates race past a step-3 miss, the
// winner consumes the slot and inserts, and the loser is rejected by WIP
// admission BEFORE reaching the insert (because its feeder is also
// saturated, so applyAdmissionPlacement returns before insertTaskTx runs).
// Without recoverFoundTaskAfterInsertFailure re-reading on that failure, the
// loser would surface a raw capacity error instead of the winner's task.
func TestCreateTaskWithExternalIDAdmissionPreemptionGuard(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	workspaceID, targetStepID := seedAdmissionPreemptionWorkflow(t, ctx, repo, svc)

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]CreateTaskResult, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = svc.CreateTask(ctx, &CreateTaskRequest{
				WorkspaceID: workspaceID, WorkflowID: "wf-admission-preempt", WorkflowStepID: targetStepID, Title: "Preempt", ExternalID: "ext-preempt",
			})
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d returned an error instead of a Found outcome — the admission-preemption guard did not fire: %v", i, err)
		}
	}

	createdCount, foundCount := 0, 0
	for _, r := range results {
		switch r.Outcome {
		case CreateTaskOutcomeCreated:
			createdCount++
		case CreateTaskOutcomeFoundSettled, CreateTaskOutcomeFoundUnsettled:
			foundCount++
		}
	}
	if createdCount != 1 || foundCount != 1 {
		t.Fatalf("outcomes = %#v, want exactly one Created and one Found", results)
	}
	if results[0].Task.ID != results[1].Task.ID {
		t.Fatalf("both callers must observe the same winning task: %s vs %s", results[0].Task.ID, results[1].Task.ID)
	}
	if count := countTasksHoldingExternalID(t, ctx, repo, workspaceID, "ext-preempt"); count != 1 {
		t.Fatalf("tasks holding ext-preempt = %d, want exactly 1 (no orphan from the loser)", count)
	}
}

// TestCreateTaskWithExternalIDAdmissionRejectionSurfacesWithoutWinner is the
// spec's LO5 sibling scenario: the same saturated step and feeder, but a
// single request for an external_id nobody holds. The admission-preemption
// guard's re-read finds nothing, so it must not swallow the genuine capacity
// failure — the original WIP-limit error surfaces unchanged.
func TestCreateTaskWithExternalIDAdmissionRejectionSurfacesWithoutWinner(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	workspaceID, targetStepID := seedAdmissionPreemptionWorkflow(t, ctx, repo, svc)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "target-occupant", WorkspaceID: workspaceID, WorkflowID: "wf-admission-preempt",
		WorkflowStepID: targetStepID, WIPAdmitted: true, Title: "Target occupant",
	}); err != nil {
		t.Fatalf("seed target occupant: %v", err)
	}

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: workspaceID, WorkflowID: "wf-admission-preempt", WorkflowStepID: targetStepID, Title: "No winner", ExternalID: "ext-no-winner",
	})
	if err == nil {
		t.Fatal("expected the original capacity error to surface — nothing holds ext-no-winner for the re-read to find")
	}
	if !errors.Is(err, wfmodels.ErrWIPLimitExceeded) {
		t.Fatalf("err = %v, want a WIP-limit error, not swallowed or replaced", err)
	}
	if count := countTasksHoldingExternalID(t, ctx, repo, workspaceID, "ext-no-winner"); count != 0 {
		t.Fatalf("tasks holding ext-no-winner = %d, want 0 — the rejected create must not have persisted anything", count)
	}
}

// raceInjectingTaskRepo wraps the real repository and, on the first call to
// GetWorkspaceTaskPrefix, runs an arbitrary side effect before failing that
// call. assignIdentifier (called from prepareTaskForCreation, only for
// office-shaped requests) is the sole caller of GetWorkspaceTaskPrefix once a
// request already carries an explicit WorkflowID, so this lets a test
// synchronously simulate "a concurrent creator committed the winning task in
// the window between this request's step-3 lookup and its
// prepareTaskForCreation-stage failure" without real goroutines.
type raceInjectingTaskRepo struct {
	*sqliterepo.Repository
	inject func()
	once   sync.Once
}

func (r *raceInjectingTaskRepo) GetWorkspaceTaskPrefix(ctx context.Context, workspaceID string) (string, string, error) {
	r.once.Do(r.inject)
	return "", "", errors.New("injected failure: identifier prefix lookup failed")
}

// TestCreateTaskWithExternalIDPrepareFailureAfterStepThreeMissRecovers pins
// the spec's general "any pre-insert failure — capacity, admission, or
// otherwise — MUST trigger a re-read" requirement for failures inside
// prepareTaskForCreation specifically (step 4 validation and step 5's
// assignIdentifier), not just createTaskWithCapacity's admission/unique-
// violation failures. Without the recovery re-read on this branch, a
// concurrent winner that committed after this request's step-3 miss but
// before its own prepare-stage failure would be invisible, and this request
// would surface a raw error instead of the winner's task.
func TestCreateTaskWithExternalIDPrepareFailureAfterStepThreeMissRecovers(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-prepare-fail")

	const winnerID = "winner-task"
	svc.tasks = &raceInjectingTaskRepo{
		Repository: repo,
		inject: func() {
			if err := repo.CreateTask(ctx, &models.Task{
				ID: winnerID, WorkspaceID: "ws-prepare-fail", WorkflowID: wfID,
				Title: "Winner", ExternalID: "ext-prepare-fail",
			}); err != nil {
				t.Fatalf("seed concurrent winner: %v", err)
			}
			mustSettle(t, ctx, repo, winnerID, "ext-prepare-fail")
		},
	}

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-prepare-fail",
		WorkflowID:  wfID,
		Title:       "Loser",
		Origin:      models.TaskOriginAgentCreated,
		ExternalID:  "ext-prepare-fail",
	})
	if err != nil {
		t.Fatalf("expected the recovery re-read to return the concurrent winner as a Found outcome, got error instead: %v", err)
	}
	if result.Outcome != CreateTaskOutcomeFoundSettled {
		t.Fatalf("outcome = %v, want FoundSettled", result.Outcome)
	}
	if result.Task.ID != winnerID {
		t.Fatalf("task id = %s, want the concurrent winner %s", result.Task.ID, winnerID)
	}
	if count := countTasksHoldingExternalID(t, ctx, repo, "ws-prepare-fail", "ext-prepare-fail"); count != 1 {
		t.Fatalf("tasks holding ext-prepare-fail = %d, want exactly 1 (no orphan or duplicate from the losing request)", count)
	}
}

// TestCreateTaskSubtaskWithoutExplicitExternalIDDoesNotInheritParent pins the
// spec's explicit non-goal: "An external ID SHALL NOT be inherited by
// subtasks... or auto-generated by the system." A subtask created with no
// external_id of its own must hold none, even though its parent holds one.
func TestCreateTaskSubtaskWithoutExplicitExternalIDDoesNotInheritParent(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-subtask")

	parentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-subtask",
		WorkflowID:  wfID,
		Title:       "Parent",
		ExternalID:  "ext-parent",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	childResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-subtask",
		WorkflowID:  wfID,
		Title:       "Child",
		ParentID:    parentResult.Task.ID,
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if childResult.Task.ExternalID != "" {
		t.Fatalf("child.ExternalID = %q, want empty — a subtask must never inherit its parent's identity", childResult.Task.ExternalID)
	}
}
