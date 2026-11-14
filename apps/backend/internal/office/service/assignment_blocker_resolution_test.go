package service_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/runs/dedupkeys"
	runmodels "github.com/kandev/kandev/internal/runs/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfservice "github.com/kandev/kandev/internal/workflow/service"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/engine_dispatcher"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/scheduler"
	"github.com/kandev/kandev/internal/office/service"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/adapters"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// The resolution workflow only queues a run. Reads use the real task/step stores;
// embedding the unused transition methods makes an unexpected move fail loudly.
type assignmentResolutionStore struct {
	engine.TransitionStore
	f       *assignmentBlockerFixture
	applied map[string]bool
}

func (s *assignmentResolutionStore) LoadState(ctx context.Context, taskID, sessionID string) (engine.MachineState, error) {
	task, err := s.f.tasks.GetTask(ctx, taskID)
	if err != nil {
		return engine.MachineState{}, err
	}
	return engine.MachineState{TaskID: taskID, SessionID: sessionID, WorkflowID: task.WorkflowID, CurrentStepID: task.WorkflowStepID}, nil
}
func (s *assignmentResolutionStore) LoadStep(ctx context.Context, _, stepID string) (engine.StepSpec, error) {
	step, err := s.f.workflows.GetStep(ctx, stepID)
	if err != nil {
		return engine.StepSpec{}, err
	}
	return engine.CompileStep(step), nil
}
func (s *assignmentResolutionStore) IsOperationApplied(_ context.Context, op string) (bool, error) {
	return s.applied[op], nil
}
func (s *assignmentResolutionStore) MarkOperationApplied(_ context.Context, op string) error {
	s.applied[op] = true
	return nil
}
func (s *assignmentResolutionStore) PersistData(context.Context, string, map[string]any) error {
	return nil
}

type assignmentResolutionQueue struct{ runs *runsservice.Service }

func (q assignmentResolutionQueue) QueueRun(ctx context.Context, req engine.QueueRunRequest) (engine.QueueOutcome, error) {
	outcome, err := q.runs.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: req.AgentProfileID, TaskID: req.TaskID, WorkflowStepID: req.WorkflowStepID,
		Reason: req.Reason, IdempotencyKey: req.IdempotencyKey, Payload: req.Payload,
	})
	return engine.QueueOutcome(outcome), err
}

func (f *assignmentBlockerFixture) wireResolutionEngine(t *testing.T) {
	t.Helper()
	step, err := f.workflows.GetStep(context.Background(), "assignment-work")
	blockerMust(t, err)
	step.Events.OnBlockerResolved = []wfmodels.GenericAction{{Type: wfmodels.GenericActionQueueRun,
		Config: map[string]any{"target": "primary", "task_id": "this", "reason": service.RunReasonTaskBlockersResolved}}}
	blockerMust(t, f.workflows.UpdateStep(context.Background(), step))
	eng := engine.New(&assignmentResolutionStore{f: f, applied: map[string]bool{}}, engine.MapRegistry{
		engine.ActionQueueRun: engine.QueueRunCallback{Adapter: assignmentResolutionQueue{runs: f.runs}, Primary: adapters.NewPrimaryAgentAdapter(f.workflows)},
	})
	f.svc.SetWorkflowEngineDispatcher(engine_dispatcher.New(eng, f.tasks, logger.Default()))
	blockerMust(t, f.tasks.CreateTaskSession(context.Background(), &taskmodels.TaskSession{
		ID: "dependent-session", TaskID: "dependent", AgentProfileID: "assignment-worker", State: taskmodels.TaskSessionStateWaitingForInput,
	}))
}

// @covers AC-OFFICE-SCHEDULER-002.7
func TestOfficeAssignmentBlockerResolution(t *testing.T) {
	for _, source := range []string{"service_event", "reactivity_without_session"} {
		t.Run(source, func(t *testing.T) {
			f := newAssignmentBlockerFixture(t)
			f.task(t, "dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
			f.blockers(t, "dependent", v1.TaskStateInProgress, v1.TaskStateInProgress)
			f.assignment(t, events.TaskCreated, "dependent", "assignment-worker")
			if got := len(f.taskRuns(t, "dependent", service.RunReasonTaskAssigned)); got != 0 {
				t.Errorf("assignment while pending=%d, want 0", got)
			}
			if source == "service_event" {
				f.wireResolutionEngine(t)
			} else {
				_, err := f.tasks.GetActiveTaskSessionByTaskID(context.Background(), "dependent")
				if !errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
					t.Fatalf("held task has session: %v", err)
				}
			}
			for i := 0; i < 2; i++ {
				id := fmt.Sprintf("dependent-prerequisite-%d", i)
				_, err := f.repo.UpdateTaskAssignee(context.Background(), id, "assignment-worker")
				blockerMust(t, err)
				blockerMust(t, f.tasks.UpdateTaskState(context.Background(), id, v1.TaskStateCompleted))
				if source == "service_event" {
					data := service.TaskMovedData{TaskID: id, WorkspaceID: "assignment-ws", AssigneeAgentProfileID: "assignment-worker", FromStepName: "Work", ToStepName: "Done"}
					blockerMust(t, f.eb.Publish(context.Background(), events.TaskMoved, bus.NewEvent(events.TaskMoved, "resolution-test", data)))
				} else {
					done := "done"
					_, err := f.scheduler.ApplyTaskMutation(context.Background(), &scheduler.TaskSnapshot{ID: id, WorkspaceID: "assignment-ws", State: "IN_PROGRESS"}, scheduler.TaskMutation{NewStatus: &done, ActorType: "user", ActorID: "operator"})
					blockerMust(t, err)
				}
				runs := f.taskRuns(t, "dependent", service.RunReasonTaskBlockersResolved)
				if len(runs) != i {
					t.Fatalf("after completion %d: resolution runs=%d, want %d", i+1, len(runs), i)
				}
				if len(runs) == 1 && (runs[0].IdempotencyKey == nil || *runs[0].IdempotencyKey == "") {
					t.Fatal("resolution lost its key")
				}
			}
		})
	}
}

func (f *assignmentBlockerFixture) completionService(t *testing.T) *taskservice.Service {
	t.Helper()
	log := logger.Default()
	ts := taskservice.NewService(taskservice.Repos{Workspaces: f.tasks, Tasks: f.tasks, TaskRepos: f.tasks, Workflows: f.tasks, Sessions: f.tasks, TaskEnvironments: f.tasks, ResourceCleanups: f.tasks, TaskActivity: f.tasks}, f.eb, log, taskservice.RepositoryDiscoveryConfig{})
	ws := wfservice.NewService(f.workflows, log)
	t.Cleanup(func() { blockerMust(t, ws.Close()) })
	ts.SetWorkflowStepGetter(ws)
	ts.SetBlockerRepository(f.repo)
	return ts
}

func (f *assignmentBlockerFixture) completionSteps(t *testing.T, kanban bool) (string, string) {
	t.Helper()
	ctx := context.Background()
	workflow, work, done := "assignment-workflow", "assignment-work", "assignment-done"
	if kanban {
		workflow, work, done = "kanban", "kanban-work", "kanban-done"
		blockerMust(t, f.tasks.CreateWorkflow(ctx, &taskmodels.Workflow{ID: workflow, WorkspaceID: "assignment-ws", Name: "Kanban"}))
		blockerMust(t, f.workflows.CreateStep(ctx, &wfmodels.WorkflowStep{ID: work, WorkflowID: workflow, Name: "Work"}))
	}
	blockerMust(t, f.workflows.CreateStep(ctx, &wfmodels.WorkflowStep{ID: done, WorkflowID: workflow, Name: "Done", Position: 2, CompleteTaskOnEnter: true}))
	return work, done
}

func (f *assignmentBlockerFixture) heldCompletionTask(t *testing.T, ts *taskservice.Service, work string) []string {
	t.Helper()
	ctx := context.Background()
	f.task(t, "dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
	// Creation timestamps have no mutation API; age only this fixture row.
	_, err := f.repo.ExecRaw(ctx, "UPDATE tasks SET created_at = ? WHERE id = ?", time.Now().UTC().Add(-48*time.Hour), "dependent")
	blockerMust(t, err)
	step, err := f.workflows.GetStep(ctx, work)
	blockerMust(t, err)
	ids := []string{"first", "final"}
	for _, id := range ids {
		blockerMust(t, f.tasks.CreateTask(ctx, &taskmodels.Task{ID: id, WorkspaceID: "assignment-ws", WorkflowID: step.WorkflowID, WorkflowStepID: work, Title: id, State: v1.TaskStateInProgress, Priority: "medium", AssigneeAgentProfileID: "assignment-worker"}))
		blockerMust(t, ts.AddDependency(ctx, "dependent", id))
	}
	f.assignment(t, events.TaskCreated, "dependent", "assignment-worker")
	if got := len(f.taskRuns(t, "dependent", service.RunReasonTaskAssigned)); got != 0 {
		t.Fatalf("held assignment=%d", got)
	}
	if _, err := f.tasks.GetActiveTaskSessionByTaskID(ctx, "dependent"); !errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		t.Fatalf("dependent session: %v", err)
	}
	dependent, err := f.tasks.GetTask(ctx, "dependent")
	blockerMust(t, err)
	if taskservice.HasStartWhenUnblockedIntent(dependent) {
		t.Fatal("unexpected launch intent")
	}
	rows, err := f.repo.ListUnstartedTasks(ctx, 24, 10)
	blockerMust(t, err)
	for _, row := range rows {
		if row.ID == dependent.ID {
			t.Fatal("aged dependent eligible for 24h recovery")
		}
	}
	return ids
}

func (f *assignmentBlockerFixture) completedStateEvent(t *testing.T, id string) {
	t.Helper()
	data := map[string]any{"task_id": id, "old_state": "IN_PROGRESS", "new_state": "COMPLETED"}
	blockerMust(t, f.eb.Publish(context.Background(), events.TaskStateChanged, bus.NewEvent(events.TaskStateChanged, "orchestrator", data)))
}

func (f *assignmentBlockerFixture) dashboardComplete(t *testing.T, id, done string) {
	t.Helper()
	ctx := context.Background()
	task, err := f.tasks.GetTask(ctx, id)
	blockerMust(t, err)
	task.WorkflowStepID = done
	blockerMust(t, f.tasks.UpdateTask(ctx, task))
	log := logger.Default()
	dash := dashboard.NewDashboardService(f.repo, log, shared.NewActivityLogger(f.repo, log), f.svc, nil)
	dash.SetTaskStateWriter(f.tasks)
	dash.SetReactivityApplier(scheduler.NewDashboardReactivityAdapter(f.scheduler))
	blockerMust(t, dash.UpdateTaskStatus(ctx, dashboard.TaskStatusUpdateRequest{TaskID: id, NewStatus: "done", SuppressStatusActivity: true}))
	// Canonical terminal completion publishes this state transition after Dashboard.
	f.completedStateEvent(t, id)
}

// @covers AC-OFFICE-SCHEDULER-002.7 AC-OFFICE-SCHEDULER-002.9
func TestOfficeAssignmentBlockerCommittedCompletion(t *testing.T) {
	for _, source := range []string{"move_office", "move_kanban_prerequisite", "dashboard_and_state_event"} {
		t.Run(source, func(t *testing.T) {
			f := newAssignmentBlockerFixture(t)
			blockerMust(t, f.scheduler.RegisterTaskCompletionSubscriber(f.eb))
			ts := f.completionService(t)
			work, done := f.completionSteps(t, source == "move_kanban_prerequisite")
			ids := f.heldCompletionTask(t, ts, work)
			generation := f.generation(t, "dependent")
			for i, id := range ids {
				if source == "dashboard_and_state_event" {
					f.dashboardComplete(t, id, done)
				} else {
					step, err := f.workflows.GetStep(context.Background(), done)
					blockerMust(t, err)
					_, err = ts.MoveTask(context.Background(), id, step.WorkflowID, done, 0)
					blockerMust(t, err)
				}
				stored, err := f.tasks.GetTask(context.Background(), id)
				blockerMust(t, err)
				if stored.State != v1.TaskStateCompleted {
					t.Fatalf("completion not persisted: %s", stored.State)
				}
				got := len(f.taskRuns(t, "dependent", service.RunReasonTaskBlockersResolved))
				if got != i {
					t.Errorf("completion %d persisted resolution runs=%d, want %d", i+1, got, i)
				}
				f.completedStateEvent(t, id)
				f.completedStateEvent(t, id)
				if got := len(f.taskRuns(t, "dependent", service.RunReasonTaskBlockersResolved)); got != i {
					t.Errorf("duplicate completion %d runs=%d, want %d", i+1, got, i)
				}
			}
			runs := f.taskRuns(t, "dependent", service.RunReasonTaskBlockersResolved)
			if len(runs) == 1 {
				assertCompletionRun(t, runs[0], ids)
			}
			dependent, err := f.tasks.GetTask(context.Background(), "dependent")
			blockerMust(t, err)
			if dependent.AssigneeAgentProfileID != "assignment-worker" || f.generation(t, "dependent") != generation {
				t.Fatal("completion mutated runner/generation")
			}
		})
	}
	t.Run("rejected_completion", testCompletionIsolation)
}

func assertCompletionRun(t *testing.T, run *runmodels.Run, ids []string) {
	t.Helper()
	want := fmt.Sprintf("%s:dependent:assignment-worker:%s", service.RunReasonTaskBlockersResolved, dedupkeys.BlockerDigest(ids))
	if run.IdempotencyKey == nil || *run.IdempotencyKey != want {
		t.Fatalf("resolution key=%v, want %s", run.IdempotencyKey, want)
	}
	if run.ActorKind != runmodels.ActorKindSystem || run.ParentRunID != "" || run.CausationDepth != 0 || run.ChainCausationID != run.ID {
		t.Fatalf("resolution actor/causation changed: %+v", run)
	}
}

func (f *assignmentBlockerFixture) completionIsolationCase(t *testing.T, name string) any {
	t.Helper()
	ctx := context.Background()
	data := map[string]any{"task_id": "prerequisite", "old_state": "IN_PROGRESS", "new_state": "COMPLETED"}
	switch name {
	case "kanban_dependent":
		work, _ := f.completionSteps(t, true)
		task, err := f.tasks.GetTask(ctx, "dependent")
		blockerMust(t, err)
		task.WorkflowID, task.WorkflowStepID = "kanban", work
		blockerMust(t, f.tasks.UpdateTask(ctx, task))
	case "wrong_workspace":
		blockerMust(t, f.tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "other-ws", Name: "Other", OfficeWorkflowID: "other-office"}))
		blockerMust(t, f.tasks.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "other-office", WorkspaceID: "other-ws", Name: "Office"}))
		blockerMust(t, f.workflows.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "other-work", WorkflowID: "other-office", Name: "Work"}))
		task, err := f.tasks.GetTask(ctx, "dependent")
		blockerMust(t, err)
		task.WorkspaceID, task.WorkflowID, task.WorkflowStepID = "other-ws", "other-office", "other-work"
		blockerMust(t, f.tasks.UpdateTask(ctx, task))
	case "failed", "cancelled":
		data["new_state"] = map[string]string{"failed": "FAILED", "cancelled": "CANCELLED"}[name]
	case "already_completed":
		data["old_state"] = "COMPLETED"
	case "missing_old":
		delete(data, "old_state")
	case "missing_source":
		data["task_id"] = "missing"
	case "missing_new":
		delete(data, "new_state")
	case "stale_reopened":
		blockerMust(t, f.tasks.UpdateTaskState(ctx, "prerequisite", v1.TaskStateTODO))
	case "malformed":
		return []string{"not a task transition"}
	}
	return data
}

func testCompletionIsolation(t *testing.T) {
	t.Helper()
	for _, name := range []string{"kanban_dependent", "wrong_workspace", "failed", "cancelled", "already_completed", "missing_old", "missing_source", "missing_new", "stale_reopened", "malformed"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			f := newAssignmentBlockerFixture(t)
			blockerMust(t, f.scheduler.RegisterTaskCompletionSubscriber(f.eb))
			f.task(t, "dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
			f.task(t, "prerequisite", "assignment-work", v1.TaskStateCompleted, "")
			blockerMust(t, f.repo.CreateTaskBlocker(ctx, &models.TaskBlocker{TaskID: "dependent", BlockerTaskID: "prerequisite"}))
			data := f.completionIsolationCase(t, name)
			blockerMust(t, f.eb.Publish(ctx, events.TaskStateChanged, bus.NewEvent(events.TaskStateChanged, "isolation-test", data)))
			if got := len(f.taskRuns(t, "dependent", service.RunReasonTaskBlockersResolved)); got != 0 {
				t.Fatalf("rejected completion admitted %d runs", got)
			}
			dependent, err := f.tasks.GetTask(ctx, "dependent")
			blockerMust(t, err)
			dependent.WorkspaceID, dependent.WorkflowID, dependent.WorkflowStepID = "assignment-ws", "assignment-workflow", "assignment-work"
			blockerMust(t, f.tasks.UpdateTask(ctx, dependent))
			blockerMust(t, f.tasks.UpdateTaskState(ctx, "prerequisite", v1.TaskStateCompleted))
			f.completedStateEvent(t, "prerequisite")
			if got := len(f.taskRuns(t, "dependent", service.RunReasonTaskBlockersResolved)); got != 1 {
				t.Fatalf("same-fixture positive control admitted %d runs, want 1", got)
			}
		})
	}
}
