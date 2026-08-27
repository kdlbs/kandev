package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newWorkflowStateTestService seeds a workspace/workflow plus one task and
// session so the guarded state writers have real rows to CAS against.
func newWorkflowStateTestService(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-state", Name: "State"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-state", WorkspaceID: "ws-state", Name: "flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-state", WorkspaceID: "ws-state", WorkflowID: "wf-state", WorkflowStepID: "step-1",
		Title: "State", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-state", TaskID: "task-state", State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	bus.ClearEvents()
	return svc, bus, repo
}

// stateChangedEvent returns the single task.state_changed payload, failing when
// the count is not exactly one.
func stateChangedEvent(t *testing.T, bus *MockEventBus) map[string]interface{} {
	t.Helper()
	published := bus.GetPublishedEvents()
	var matched []map[string]interface{}
	for _, event := range published {
		if event.Type != events.TaskStateChanged {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data = %T, want a map payload", event.Data)
		}
		matched = append(matched, data)
	}
	if len(matched) != 1 {
		t.Fatalf("published %v, want exactly one %s", eventTypes(published), events.TaskStateChanged)
	}
	return matched[0]
}

func TestUpdateTaskStateIfCurrentInWritesOnlyFromAllowedStates(t *testing.T) {
	svc, bus, repo := newWorkflowStateTestService(t)
	ctx := context.Background()

	// Current state is CREATED, which is not in the allowed set: no write, no event.
	updated, err := svc.UpdateTaskStateIfCurrentIn(ctx, "task-state", v1.TaskStateCompleted,
		[]v1.TaskState{v1.TaskStateInProgress})
	if err != nil {
		t.Fatalf("UpdateTaskStateIfCurrentIn: %v", err)
	}
	if updated {
		t.Fatal("state must not change from a disallowed current state")
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("no-op CAS published %v, want nothing", eventTypes(published))
	}
	stored, err := repo.GetTask(ctx, "task-state")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.State != v1.TaskStateCreated {
		t.Fatalf("state = %q, want it untouched", stored.State)
	}

	// Now the current state is allowed: the row changes and one event fires.
	updated, err = svc.UpdateTaskStateIfCurrentIn(ctx, "task-state", v1.TaskStateInProgress,
		[]v1.TaskState{v1.TaskStateCreated})
	if err != nil {
		t.Fatalf("UpdateTaskStateIfCurrentIn: %v", err)
	}
	if !updated {
		t.Fatal("state must change from an allowed current state")
	}
	stored, err = repo.GetTask(ctx, "task-state")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.State != v1.TaskStateInProgress {
		t.Fatalf("state = %q, want in_progress", stored.State)
	}
	data := stateChangedEvent(t, bus)
	if data["task_id"] != "task-state" {
		t.Fatalf("event task_id = %v", data["task_id"])
	}
	if data["state"] != string(v1.TaskStateInProgress) {
		t.Fatalf("event state = %v, want in_progress", data["state"])
	}
	if data["old_state"] != string(v1.TaskStateCreated) {
		t.Fatalf("event old_state = %v, want created", data["old_state"])
	}
}

func TestUpdateTaskStateIfCurrentInRejectsEmptyAllowedSet(t *testing.T) {
	svc, bus, _ := newWorkflowStateTestService(t)

	updated, err := svc.UpdateTaskStateIfCurrentIn(context.Background(), "task-state", v1.TaskStateCompleted, nil)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfCurrentIn: %v", err)
	}
	if updated {
		t.Fatal("an empty allowed set must never write")
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("published %v, want nothing", eventTypes(published))
	}
}

func TestUpdateTaskStateIfNotArchivedFreezesArchivedTasks(t *testing.T) {
	svc, bus, repo := newWorkflowStateTestService(t)
	ctx := context.Background()

	updated, err := svc.UpdateTaskStateIfNotArchived(ctx, "task-state", v1.TaskStateInProgress)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfNotArchived: %v", err)
	}
	if !updated {
		t.Fatal("an unarchived task must accept the write")
	}
	if data := stateChangedEvent(t, bus); data["old_state"] != string(v1.TaskStateCreated) {
		t.Fatalf("event old_state = %v, want created", data["old_state"])
	}

	// Writing the same state again is a no-op with no duplicate event.
	bus.ClearEvents()
	updated, err = svc.UpdateTaskStateIfNotArchived(ctx, "task-state", v1.TaskStateInProgress)
	if err != nil {
		t.Fatalf("repeat UpdateTaskStateIfNotArchived: %v", err)
	}
	if updated {
		t.Fatal("re-writing the same state must report no change")
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("same-state write published %v, want nothing", eventTypes(published))
	}

	// Archived tasks are frozen.
	if err := svc.ArchiveTask(ctx, "task-state"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	bus.ClearEvents()
	updated, err = svc.UpdateTaskStateIfNotArchived(ctx, "task-state", v1.TaskStateCompleted)
	if err != nil {
		t.Fatalf("archived UpdateTaskStateIfNotArchived: %v", err)
	}
	if updated {
		t.Fatal("an archived task must not accept a state write")
	}
	stored, err := repo.GetTask(ctx, "task-state")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.State != v1.TaskStateInProgress {
		t.Fatalf("archived task state = %q, want the pre-archive value", stored.State)
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("frozen write published %v, want nothing", eventTypes(published))
	}
}

func TestUpdateTaskStateIfSessionStateRequiresExpectedSessionState(t *testing.T) {
	svc, bus, repo := newWorkflowStateTestService(t)
	ctx := context.Background()

	// The session is RUNNING, so a COMPLETED expectation must not write.
	updated, err := svc.UpdateTaskStateIfSessionState(ctx, "task-state", "sess-state",
		models.TaskSessionStateCompleted, v1.TaskStateCompleted)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfSessionState: %v", err)
	}
	if updated {
		t.Fatal("a mismatched session state must block the write")
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("blocked write published %v, want nothing", eventTypes(published))
	}

	updated, err = svc.UpdateTaskStateIfSessionState(ctx, "task-state", "sess-state",
		models.TaskSessionStateRunning, v1.TaskStateInProgress)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfSessionState: %v", err)
	}
	if !updated {
		t.Fatal("a matching session state must allow the write")
	}
	stored, err := repo.GetTask(ctx, "task-state")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.State != v1.TaskStateInProgress {
		t.Fatalf("state = %q, want in_progress", stored.State)
	}
	if data := stateChangedEvent(t, bus); data["state"] != string(v1.TaskStateInProgress) {
		t.Fatalf("event state = %v", data["state"])
	}
}

func TestCountTasksByWorkflowAndStep(t *testing.T) {
	svc, _, repo := newWorkflowStateTestService(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-state-2", WorkspaceID: "ws-state", WorkflowID: "wf-state", WorkflowStepID: "step-2",
		Title: "Second", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create second task: %v", err)
	}

	workflowCount, err := svc.CountTasksByWorkflow(ctx, "wf-state")
	if err != nil {
		t.Fatalf("CountTasksByWorkflow: %v", err)
	}
	if workflowCount != 2 {
		t.Fatalf("workflow count = %d, want 2", workflowCount)
	}

	stepCount, err := svc.CountTasksByWorkflowStep(ctx, "step-1")
	if err != nil {
		t.Fatalf("CountTasksByWorkflowStep: %v", err)
	}
	if stepCount != 1 {
		t.Fatalf("step-1 count = %d, want 1", stepCount)
	}
	emptyCount, err := svc.CountTasksByWorkflowStep(ctx, "step-empty")
	if err != nil {
		t.Fatalf("CountTasksByWorkflowStep: %v", err)
	}
	if emptyCount != 0 {
		t.Fatalf("empty step count = %d, want 0", emptyCount)
	}
}

func TestQueuedTaskBeforeOrdersByPositionPriorityThenAge(t *testing.T) {
	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Hour)

	cases := []struct {
		name        string
		left, right *models.Task
		want        bool
	}{
		{
			name:  "lower position wins",
			left:  &models.Task{ID: "a", Position: 1, Priority: "low"},
			right: &models.Task{ID: "b", Position: 2, Priority: "critical"},
			want:  true,
		},
		{
			name:  "higher position loses",
			left:  &models.Task{ID: "a", Position: 3, Priority: "critical"},
			right: &models.Task{ID: "b", Position: 2, Priority: "low"},
			want:  false,
		},
		{
			name:  "critical beats high at equal position",
			left:  &models.Task{ID: "a", Position: 1, Priority: "critical"},
			right: &models.Task{ID: "b", Position: 1, Priority: "high"},
			want:  true,
		},
		{
			name:  "medium beats low",
			left:  &models.Task{ID: "a", Position: 1, Priority: priorityMedium},
			right: &models.Task{ID: "b", Position: 1, Priority: priorityLow},
			want:  true,
		},
		{
			name:  "known priority beats an unrecognized one",
			left:  &models.Task{ID: "a", Position: 1, Priority: priorityLow},
			right: &models.Task{ID: "b", Position: 1, Priority: "whenever"},
			want:  true,
		},
		{
			name:  "earlier queued_at wins at equal priority",
			left:  &models.Task{ID: "a", Position: 1, Priority: priorityMedium, QueuedAt: &earlier},
			right: &models.Task{ID: "b", Position: 1, Priority: priorityMedium, QueuedAt: &later},
			want:  true,
		},
		{
			name: "created_at breaks a queued_at tie",
			left: &models.Task{
				ID: "a", Position: 1, Priority: priorityMedium, QueuedAt: &earlier, CreatedAt: earlier,
			},
			right: &models.Task{
				ID: "b", Position: 1, Priority: priorityMedium, QueuedAt: &earlier, CreatedAt: later,
			},
			want: true,
		},
		{
			name: "id breaks a full tie",
			left: &models.Task{
				ID: "a", Position: 1, Priority: priorityMedium, QueuedAt: &earlier, CreatedAt: earlier,
			},
			right: &models.Task{
				ID: "b", Position: 1, Priority: priorityMedium, QueuedAt: &earlier, CreatedAt: earlier,
			},
			want: true,
		},
		{
			name: "identical tasks are not before each other",
			left: &models.Task{
				ID: "same", Position: 1, Priority: priorityMedium, QueuedAt: &earlier, CreatedAt: earlier,
			},
			right: &models.Task{
				ID: "same", Position: 1, Priority: priorityMedium, QueuedAt: &earlier, CreatedAt: earlier,
			},
			want: false,
		},
	}
	for _, tc := range cases {
		if got := queuedTaskBefore(tc.left, tc.right); got != tc.want {
			t.Fatalf("%s: queuedTaskBefore = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFindSameStepQueuedCandidatePicksFirstEligible(t *testing.T) {
	admitted := &models.Task{ID: "admitted", QueuedForStepID: "step-1", WIPAdmitted: true, Position: 0}
	otherStep := &models.Task{ID: "other-step", QueuedForStepID: "step-2", Position: 0}
	skippedTask := &models.Task{ID: "skipped", QueuedForStepID: "step-1", Position: 0}
	first := &models.Task{ID: "first", QueuedForStepID: "step-1", Position: 1, Priority: priorityMedium}
	second := &models.Task{ID: "second", QueuedForStepID: "step-1", Position: 2, Priority: "critical"}

	candidates := []*models.Task{nil, admitted, otherStep, skippedTask, second, first}
	skipped := map[string]struct{}{"skipped": {}}

	selected := findSameStepQueuedCandidate(candidates, "step-1", skipped)
	if selected == nil || selected.ID != "first" {
		t.Fatalf("selected = %+v, want the lowest-position unskipped queued task", selected)
	}

	// With every eligible candidate skipped there is nothing to promote.
	if got := findSameStepQueuedCandidate(candidates, "step-1", map[string]struct{}{
		"skipped": {}, "first": {}, "second": {},
	}); got != nil {
		t.Fatalf("selected = %+v, want nil when all candidates are skipped", got)
	}
}

func TestSkippedTaskIDsDropsEmptyEntries(t *testing.T) {
	ids := skippedTaskIDs(map[string]struct{}{"a": {}, "": {}})
	if len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("ids = %v, want just [a]", ids)
	}
	if got := skippedTaskIDs(nil); len(got) != 0 {
		t.Fatalf("ids = %v, want empty", got)
	}
}
