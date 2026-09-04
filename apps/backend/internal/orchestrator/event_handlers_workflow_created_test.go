package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestHandleTaskCreated covers WO-36: a task created directly onto a step
// whose on_enter carries auto_start_agent (e.g. a materialized heavy
// routine run landing on the Routine workflow's start step) never got its
// on_enter evaluated, because every other autoStartTaskForStep caller
// represents a transition INTO a step (task.moved, task.queue_promoted,
// dependency resolution) — creation was never wired in.
//
// Subtests that expect NO launch attempt assert on stepGetter.GetStepCalls()
// rather than on the mock agent manager. autoStartTaskForStep calls GetStep
// synchronously before any launch work is handed off to a detached goroutine
// (see autoStartTaskForLoadedStep), so a zero call count is race-free proof
// that autoStartTaskForStep was never entered — unlike waiting on the agent
// manager, which races the test's own DB teardown against that goroutine.
func TestHandleTaskCreated(t *testing.T) {
	ctx := context.Background()

	t.Run("launches a session for a task created directly on an auto-start step", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

		// Mirrors what CreateOfficeTaskInWorkflow now stamps for a heavy
		// routine run: the positive create-time opt-in, plus the routine's
		// assignee carried as a task-level fallback since the Routine
		// workflow's start step pins no agent.
		metadata := map[string]interface{}{
			models.MetaKeyAutoStartOnCreate: true,
			models.MetaKeyAgentProfileID:    "routine-assignee",
		}
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID:             "t1",
			WorkspaceID:    "ws1",
			WorkflowID:     "wf1",
			WorkflowStepID: "step1",
			Title:          "Routine run",
			Description:    "prompt",
			State:          v1.TaskStateCreated,
			Metadata:       metadata,
			CreatedAt:      now,
			UpdatedAt:      now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Routine Start", Position: 0,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		taskRepo := newMockTaskRepo()
		taskRepo.tasks["t1"] = &v1.Task{
			ID:          "t1",
			WorkspaceID: "ws1",
			WorkflowID:  "wf1",
			Description: "prompt",
			State:       v1.TaskStateCreated,
			Metadata:    metadata,
		}
		launched := make(chan string, 1)
		agentMgr := &mockAgentManager{
			launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				agentProfileID, _ := req.Metadata[models.MetaKeyAgentProfileID].(string)
				launched <- agentProfileID
				return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
			},
		}
		svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

		svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t1"})

		select {
		case got := <-launched:
			if got != "routine-assignee" {
				t.Fatalf("AgentProfileID = %q, want routine-assignee", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for auto-start launch on task creation")
		}
	})

	t.Run("skips a queued task even when it carries the create-time opt-in", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

		// QueuedForStepID marks a task as WIP-blocked: it isn't really
		// entering step1 yet, it's waiting for capacity. autoStartTaskForStep
		// bails on this before doing anything else, so a task.created firing
		// for a queued task (e.g. via CreateTaskWithWorkflowStepAdmission
		// overflow placement) must not attempt a launch even though it
		// carries the create-time opt-in.
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID:              "t2",
			WorkspaceID:     "ws1",
			WorkflowID:      "wf1",
			WorkflowStepID:  "step1",
			QueuedForStepID: "step1",
			QueuedAt:        &now,
			Title:           "Queued create",
			Description:     "prompt",
			State:           v1.TaskStateCreated,
			Metadata:        map[string]interface{}{models.MetaKeyAutoStartOnCreate: true},
			CreatedAt:       now,
			UpdatedAt:       now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Start", Position: 0,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))

		svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t2"})

		if calls := stepGetter.GetStepCalls(); calls != 0 {
			t.Fatalf("GetStepCalls() = %d, want 0 (autoStartTaskForStep must bail on QueuedForStepID before loading the step)", calls)
		}
	})

	t.Run("skips a task created on an auto-start step without the create-time opt-in (R-1)", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

		// An ordinary task with no launch opinion at all: no
		// MetaKeyAutoStartOnCreate. A prior version of this guard launched
		// unless the task carried MetaKeyDeferredLaunch, treating every
		// other absence as "no launch opinion, please auto-start" — which
		// incorrectly launched every producer that simply never set it
		// (REST/MCP/WS creates without start_agent or prepare_session,
		// CreateChildTask, etc.). Landing on an auto-start step must not be
		// enough on its own.
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID:             "t3",
			WorkspaceID:    "ws1",
			WorkflowID:     "wf1",
			WorkflowStepID: "step1",
			Title:          "Ordinary create",
			Description:    "prompt",
			State:          v1.TaskStateCreated,
			CreatedAt:      now,
			UpdatedAt:      now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Start", Position: 0,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))

		svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t3"})

		if calls := stepGetter.GetStepCalls(); calls != 0 {
			t.Fatalf("GetStepCalls() = %d, want 0 (handleTaskCreated must not call autoStartTaskForStep without the create-time opt-in)", calls)
		}
	})

	t.Run("does not launch an office task even when it carries the create-time opt-in (R-2)", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
			ID: "ws1", Name: "Test", OfficeWorkflowID: "wf1", CreatedAt: now, UpdatedAt: now,
		}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Office WF", CreatedAt: now, UpdatedAt: now}))

		// task.IsFromOffice is a read-time projection: true because this
		// task's workflow matches the workspace's OfficeWorkflowID. The
		// office subscriber's own task.created handler already queues a run
		// for every office task; if handleTaskCreated also launched one here
		// (even though this task, hypothetically, carried the opt-in), the
		// two would race to double-queue the same task with different
		// idempotency keys. handleTaskCreated must defer to the office
		// subscriber entirely, regardless of the opt-in marker.
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID:             "t4",
			WorkspaceID:    "ws1",
			WorkflowID:     "wf1",
			WorkflowStepID: "step1",
			Title:          "Office task",
			Description:    "prompt",
			State:          v1.TaskStateCreated,
			Metadata:       map[string]interface{}{models.MetaKeyAutoStartOnCreate: true},
			CreatedAt:      now,
			UpdatedAt:      now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Start", Position: 0,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))

		svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t4"})

		if calls := stepGetter.GetStepCalls(); calls != 0 {
			t.Fatalf("GetStepCalls() = %d, want 0 (handleTaskCreated must never call autoStartTaskForStep for an office task)", calls)
		}
	})

	t.Run("claims the create-time opt-in so a duplicate task.created delivery cannot double-launch", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

		metadata := map[string]interface{}{
			models.MetaKeyAutoStartOnCreate: true,
			models.MetaKeyAgentProfileID:    "routine-assignee",
		}
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID:             "t5",
			WorkspaceID:    "ws1",
			WorkflowID:     "wf1",
			WorkflowStepID: "step1",
			Title:          "Routine run",
			Description:    "prompt",
			State:          v1.TaskStateCreated,
			Metadata:       metadata,
			CreatedAt:      now,
			UpdatedAt:      now,
		}))

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Routine Start", Position: 0,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterAutoStartAgent},
				},
			},
		}

		taskRepo := newMockTaskRepo()
		taskRepo.tasks["t5"] = &v1.Task{
			ID:          "t5",
			WorkspaceID: "ws1",
			WorkflowID:  "wf1",
			Description: "prompt",
			State:       v1.TaskStateCreated,
			Metadata:    metadata,
		}
		launched := make(chan string, 2)
		agentMgr := &mockAgentManager{
			launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				agentProfileID, _ := req.Metadata[models.MetaKeyAgentProfileID].(string)
				launched <- agentProfileID
				return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
			},
		}
		svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

		// First delivery: the opt-in is present, so it is claimed (removed)
		// synchronously — before the (async) launch even starts — and the
		// launch proceeds.
		svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t5"})

		reloaded, err := repo.GetTask(ctx, "t5")
		requireNoError(t, err)
		if models.HasAutoStartOnCreateIntent(reloaded.Metadata) {
			t.Fatal("MetaKeyAutoStartOnCreate still present on the task after the first delivery; a duplicate task.created delivery would double-launch")
		}

		select {
		case <-launched:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for the first delivery's launch")
		}

		// Second, duplicate delivery of the same event (e.g. a redelivered
		// task.created): the opt-in was already claimed by the first call,
		// so HasAutoStartOnCreateIntent is now false and this must return
		// before attempting a second launch.
		svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t5"})
		select {
		case got := <-launched:
			t.Fatalf("unexpected second launch for agent profile %q", got)
		case <-time.After(100 * time.Millisecond):
		}
	})

	t.Run("skips when the task cannot be found", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		// Should not panic — task not found is logged and returns.
		svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "nonexistent"})
	})
}

// TestReconcileTaskLifecycleTokensRecoversAutoStartOnCreate covers the
// greptile-flagged gap on PR #2967 (review thread PRRT_kwDOQ2-eWs6bh_fo,
// comment r3839544092): when handleTaskCreated's GetTask call fails
// transiently, it returns before ever reaching claimTaskEventMetadata, so
// MetaKeyAutoStartOnCreate is never claimed. task.created is a one-shot
// watcher event with no redelivery, so without this recovery path the task
// would carry the opt-in forever with no session and no way to retry.
//
// This test never calls handleTaskCreated directly — that IS the simulated
// lost delivery. It seeds a task carrying the opt-in exactly as
// CreateOfficeTaskInWorkflow would leave it if the live handler never ran,
// then drives recovery entirely through reconcileTaskLifecycleTokens, the
// startup sweep.
func TestReconcileTaskLifecycleTokensRecoversAutoStartOnCreate(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

	metadata := map[string]interface{}{
		models.MetaKeyAutoStartOnCreate: true,
		models.MetaKeyAgentProfileID:    "routine-assignee",
	}
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             "t-lost-delivery",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		Title:          "Routine run",
		Description:    "prompt",
		State:          v1.TaskStateCreated,
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Routine Start", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
			},
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t-lost-delivery"] = &v1.Task{
		ID:          "t-lost-delivery",
		WorkspaceID: "ws1",
		WorkflowID:  "wf1",
		Description: "prompt",
		State:       v1.TaskStateCreated,
		Metadata:    metadata,
	}
	launched := make(chan string, 1)
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			agentProfileID, _ := req.Metadata[models.MetaKeyAgentProfileID].(string)
			launched <- agentProfileID
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

	svc.reconcileTaskLifecycleTokens(ctx)

	select {
	case got := <-launched:
		if got != "routine-assignee" {
			t.Fatalf("AgentProfileID = %q, want routine-assignee", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the startup sweep to recover the lost task.created delivery")
	}

	reloaded, err := repo.GetTask(ctx, "t-lost-delivery")
	requireNoError(t, err)
	if models.HasAutoStartOnCreateIntent(reloaded.Metadata) {
		t.Fatal("MetaKeyAutoStartOnCreate still present after recovery; the next startup sweep would re-launch the same task")
	}
}

// TestReconcileTaskLifecycleTokensSkipsAutoStartWhenSessionExists covers the
// review finding that the create-time opt-in is NOT proof that no launch
// happened. MetaKeyAutoStartOnCreate is claimed by exactly one function,
// handleTaskCreated. Every other launch path — a manual StartTask, task.moved
// auto-start, handleTaskQueuePromoted — starts a task without touching the
// key. So once a task.created delivery is lost (the very failure this sweep
// exists to repair), an operator starting the task by hand leaves the token in
// place, and the next startup would replay it against a task that is already
// running or already finished. Neither autoStartTaskForStep nor startTask has
// an existing-session guard, so that second launch goes all the way through to
// a real agent that can write, commit, and open a PR.
//
// seedSession creates the task WITH a session, which is the state the operator
// workaround leaves behind. Asserting on GetStepCalls() (see this file's
// header) is the race-free way to prove autoStartTaskForStep was never entered.
func TestReconcileTaskLifecycleTokensSkipsAutoStartWhenSessionExists(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-already-started", "s-existing", "step1")

	metadata := map[string]interface{}{
		models.MetaKeyAutoStartOnCreate: true,
		models.MetaKeyAgentProfileID:    "routine-assignee",
	}
	task, err := repo.GetTask(ctx, "t-already-started")
	requireNoError(t, err)
	task.Metadata = metadata
	task.WorkflowStepID = "step1"
	requireNoError(t, repo.UpdateTask(ctx, task))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Routine Start", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
			},
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t-already-started"] = &v1.Task{
		ID:          "t-already-started",
		WorkspaceID: "ws1",
		WorkflowID:  "wf1",
		Description: "Test",
		State:       v1.TaskStateInProgress,
		Metadata:    metadata,
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, &mockAgentManager{})

	svc.reconcileTaskLifecycleTokens(ctx)

	if calls := stepGetter.GetStepCalls(); calls != 0 {
		t.Fatalf("GetStepCalls() = %d, want 0 (the sweep must not replay the opt-in on a task that already has a session)", calls)
	}
	reloaded, err := repo.GetTask(ctx, "t-already-started")
	requireNoError(t, err)
	if _, present := reloaded.Metadata[models.MetaKeyAutoStartOnCreate]; present {
		t.Fatal("MetaKeyAutoStartOnCreate survived the sweep; the token is spent once the task has a session, so leaving it makes every future startup re-scan this row forever")
	}
}

// TestRecoverTaskLifecycleAttemptDoesNotRetryUnactionableAutoStart covers the
// review finding that the sweep's notion of "pending" was broader than
// handleTaskCreated's notion of "actionable". The sweep lists by metadata-key
// EXISTENCE, but handleTaskCreated requires a positive bool AND a non-office
// task, and it returns BEFORE claiming the key in both of those cases.
//
// A row the handler refuses therefore kept its key forever, and
// recoverTaskLifecycleAttempt reported "still pending, retry" on every pass:
// the full attempt budget was burned on every startup, and the row was listed
// again on the next one, indefinitely. Every other key in this sweep is
// cleared by its handler, so this was the first token class that could never
// drain — the exact failure mode
// docs/specs/startup-listener-before-recovery/spec.md attributes a
// non-converging boot loop to.
//
// A false return means "do not retry". The key is deliberately left in place:
// the sweep declines to act on it, but discarding another subsystem's durable
// metadata is not this recovery path's call to make.
func TestRecoverTaskLifecycleAttemptDoesNotRetryUnactionableAutoStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

	cases := []struct {
		name      string
		taskID    string
		projectID string
		value     interface{}
	}{
		// IsFromOffice is a read-time projection that is true whenever
		// project_id is non-empty, and CreateOfficeTaskInWorkflow — the only
		// production setter of this key — writes a caller-supplied projectID
		// straight onto the task. Today's single caller passes "", so the
		// non-office property holds by convention, not by construction.
		{name: "office task", taskID: "t-office", projectID: "proj-1", value: true},
		// HasAutoStartOnCreateIntent requires an explicit true; a false value
		// is a row the handler will never act on.
		{name: "non-positive opt-in", taskID: "t-false", projectID: "", value: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := map[string]interface{}{
				models.MetaKeyAutoStartOnCreate: tc.value,
				models.MetaKeyAgentProfileID:    "routine-assignee",
			}
			requireNoError(t, repo.CreateTask(ctx, &models.Task{
				ID:             tc.taskID,
				WorkspaceID:    "ws1",
				WorkflowID:     "wf1",
				WorkflowStepID: "step1",
				ProjectID:      tc.projectID,
				Title:          "Routine run",
				Description:    "prompt",
				State:          v1.TaskStateCreated,
				Metadata:       metadata,
				CreatedAt:      now,
				UpdatedAt:      now,
			}))

			stepGetter := newMockStepGetter()
			stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
				ID: "step1", WorkflowID: "wf1", Name: "Routine Start", Position: 0,
				Events: wfmodels.StepEvents{
					OnEnter: []wfmodels.OnEnterAction{
						{Type: wfmodels.OnEnterAutoStartAgent},
					},
				},
			}
			taskRepo := newMockTaskRepo()
			taskRepo.tasks[tc.taskID] = &v1.Task{
				ID: tc.taskID, WorkspaceID: "ws1", WorkflowID: "wf1",
				Description: "prompt", State: v1.TaskStateCreated, Metadata: metadata,
			}
			svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, &mockAgentManager{})

			if svc.recoverTaskLifecycleAttempt(ctx, tc.taskID) {
				t.Fatal("recoverTaskLifecycleAttempt reported retry for a task handleTaskCreated will never act on; the token can never be cleared, so this burns the attempt budget on every startup and the row is re-listed on the next one, forever")
			}
			if calls := stepGetter.GetStepCalls(); calls != 0 {
				t.Fatalf("GetStepCalls() = %d, want 0 (recovery must not enter autoStartTaskForStep for an unactionable row)", calls)
			}
			reloaded, err := repo.GetTask(ctx, tc.taskID)
			requireNoError(t, err)
			if _, present := reloaded.Metadata[models.MetaKeyAutoStartOnCreate]; !present {
				t.Fatal("recovery discarded the opt-in; declining to act on a row must not silently delete another subsystem's durable metadata")
			}
		})
	}
}

// TestRecoverAutoStartOnCreatePreservesTokenOnSessionListError covers the
// ListTaskSessions error branch in recoverAutoStartOnCreate: a transient
// repo failure there must leave MetaKeyAutoStartOnCreate in place and report
// "retry" rather than either discarding the token (which would silently
// abandon the recovery) or falling through to handleTaskCreated (which could
// launch a second agent onto a task the failed lookup couldn't rule out as
// already running).
func TestRecoverAutoStartOnCreatePreservesTokenOnSessionListError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

	metadata := map[string]interface{}{
		models.MetaKeyAutoStartOnCreate: true,
		models.MetaKeyAgentProfileID:    "routine-assignee",
	}
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             "t-list-error",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		Title:          "Routine run",
		Description:    "prompt",
		State:          v1.TaskStateCreated,
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Routine Start", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
			},
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t-list-error"] = &v1.Task{
		ID: "t-list-error", WorkspaceID: "ws1", WorkflowID: "wf1",
		Description: "prompt", State: v1.TaskStateCreated, Metadata: metadata,
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, &mockAgentManager{})
	svc.repo = listTaskSessionsErrorRepo{sessionExecutorStore: svc.repo}

	if !svc.recoverTaskLifecycleAttempt(ctx, "t-list-error") {
		t.Fatal("recoverTaskLifecycleAttempt reported no retry after a ListTaskSessions error; a transient lookup failure must stay retryable, not be treated as resolved")
	}
	if calls := stepGetter.GetStepCalls(); calls != 0 {
		t.Fatalf("GetStepCalls() = %d, want 0 (a failed session lookup must not fall through to a launch attempt)", calls)
	}
	reloaded, err := repo.GetTask(ctx, "t-list-error")
	requireNoError(t, err)
	if _, present := reloaded.Metadata[models.MetaKeyAutoStartOnCreate]; !present {
		t.Fatal("MetaKeyAutoStartOnCreate was discarded despite the session lookup failing; a token that could not be verified as spent must survive for the next attempt")
	}
}
