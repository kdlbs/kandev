package service

import (
	"context"
	"errors"
	"testing"
	"time"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// errBlockerRepo wraps a working repo and fails the forward read on demand, so
// the fail-closed contract can be proven by breaking the store rather than by
// asserting the happy path.
type errBlockerRepo struct {
	*mockBlockerRepo
	failList bool
}

func (e *errBlockerRepo) ListTaskBlockers(ctx context.Context, taskID string) ([]*orchmodels.TaskBlocker, error) {
	if e.failList {
		return nil, errors.New("boom")
	}
	return e.mockBlockerRepo.ListTaskBlockers(ctx, taskID)
}

func (e *errBlockerRepo) ListBlockersForTasks(ctx context.Context, ids []string) (map[string][]string, error) {
	if e.failList {
		return nil, errors.New("boom")
	}
	return e.mockBlockerRepo.ListBlockersForTasks(ctx, ids)
}

func setDependencyState(t *testing.T, svc *Service, taskID string, state v1.TaskState) {
	t.Helper()
	task, err := svc.tasks.GetTask(context.Background(), taskID)
	if err != nil || task == nil {
		t.Fatalf("load task %s: %v", taskID, err)
	}
	task.State = state
	if err := svc.tasks.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("update task %s: %v", taskID, err)
	}
}

func TestDependencyGate_PendingBlocksAndResolvedReleases(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()
	predecessor := mustSeedTask(t, svc, "Predecessor").ID
	dependent := mustSeedTask(t, svc, "Dependent").ID
	if err := svc.AddDependency(ctx, dependent, predecessor); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	blocked, reason, err := svc.DependencyGate(ctx, dependent)
	if err != nil || !blocked || reason != BlockedReasonPending {
		t.Fatalf("pending predecessor: blocked=%v reason=%q err=%v; want true/pending/nil", blocked, reason, err)
	}

	setDependencyState(t, svc, predecessor, v1.TaskStateCompleted)
	blocked, reason, err = svc.DependencyGate(ctx, dependent)
	if err != nil || blocked {
		t.Fatalf("resolved predecessor: blocked=%v reason=%q err=%v; want false", blocked, reason, err)
	}
}

// A FAILED predecessor must NOT resolve the edge. This is the deliberate
// divergence from on_children_completed, which counts FAILED as terminal.
func TestDependencyGate_FailedPredecessorKeepsTaskBlocked(t *testing.T) {
	for _, state := range []v1.TaskState{v1.TaskStateFailed, v1.TaskStateCancelled} {
		svc, _ := setupOfficeTest(t)
		svc.SetBlockerRepository(&mockBlockerRepo{})
		ctx := context.Background()
		predecessor := mustSeedTask(t, svc, "Predecessor").ID
		dependent := mustSeedTask(t, svc, "Dependent").ID
		if err := svc.AddDependency(ctx, dependent, predecessor); err != nil {
			t.Fatalf("AddDependency: %v", err)
		}
		setDependencyState(t, svc, predecessor, state)

		blocked, reason, err := svc.DependencyGate(ctx, dependent)
		if err != nil || !blocked || reason != BlockedReasonFailed {
			t.Errorf("%s predecessor: blocked=%v reason=%q err=%v; want true/failed/nil",
				state, blocked, reason, err)
		}
	}
}

// AND semantics: a task with two predecessors stays blocked until BOTH resolve.
func TestDependencyGate_WaitsForEveryPredecessor(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()
	first := mustSeedTask(t, svc, "First").ID
	second := mustSeedTask(t, svc, "Second").ID
	dependent := mustSeedTask(t, svc, "Dependent").ID
	for _, p := range []string{first, second} {
		if err := svc.AddDependency(ctx, dependent, p); err != nil {
			t.Fatalf("AddDependency: %v", err)
		}
	}

	setDependencyState(t, svc, first, v1.TaskStateCompleted)
	if blocked, _, _ := svc.DependencyGate(ctx, dependent); !blocked {
		t.Error("one of two predecessors resolved: want still blocked")
	}
	setDependencyState(t, svc, second, v1.TaskStateCompleted)
	if blocked, _, _ := svc.DependencyGate(ctx, dependent); blocked {
		t.Error("both predecessors resolved: want unblocked")
	}
}

// The gate must fail CLOSED. Proven by breaking the store, not by the happy path:
// failing open would launch work whose predecessor may never have run.
func TestDependencyGate_FailsClosedOnReadError(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	repo := &errBlockerRepo{mockBlockerRepo: &mockBlockerRepo{}, failList: true}
	svc.SetBlockerRepository(repo)

	blocked, reason, err := svc.DependencyGate(context.Background(), "any-task")
	if err == nil {
		t.Fatal("expected the read error to be returned")
	}
	if !blocked || reason != BlockedReasonUnknown {
		t.Errorf("read failure: blocked=%v reason=%q; want true/unknown", blocked, reason)
	}
}

func TestBuildDependencyViews_ReportsBothDirections(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()
	a := mustSeedTask(t, svc, "A")
	b := mustSeedTask(t, svc, "B")
	c := mustSeedTask(t, svc, "C")
	// B depends on A; C depends on B. B therefore has one of each direction.
	if err := svc.AddDependency(ctx, b.ID, a.ID); err != nil {
		t.Fatalf("AddDependency(b,a): %v", err)
	}
	if err := svc.AddDependency(ctx, c.ID, b.ID); err != nil {
		t.Fatalf("AddDependency(c,b): %v", err)
	}

	views := svc.BuildDependencyViews(ctx, []*models.Task{b})
	view := views[b.ID]
	if len(view.DependsOn) != 1 || view.DependsOn[0].ID != a.ID {
		t.Errorf("depends_on = %+v; want [%s]", view.DependsOn, a.ID)
	}
	if len(view.Blocks) != 1 || view.Blocks[0].ID != c.ID {
		t.Errorf("blocks = %+v; want [%s]", view.Blocks, c.ID)
	}
	if view.DependsOn[0].Title != "A" {
		t.Errorf("depends_on entry should carry the title for the chip, got %q", view.DependsOn[0].Title)
	}
	if !view.Blocked || view.BlockedReason != BlockedReasonPending {
		t.Errorf("blocked=%v reason=%q; want true/pending", view.Blocked, view.BlockedReason)
	}
}

// BuildDependencyViews must fail closed too: the board reads dependency state
// through it, and a task reported unblocked on a read failure could be launched.
func TestBuildDependencyViews_FailsClosedOnReadError(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	task := mustSeedTask(t, svc, "A")
	svc.SetBlockerRepository(&errBlockerRepo{mockBlockerRepo: &mockBlockerRepo{}, failList: true})

	views := svc.BuildDependencyViews(context.Background(), []*models.Task{task})
	view := views[task.ID]
	if !view.Blocked || view.BlockedReason != BlockedReasonUnknown {
		t.Errorf("read failure: blocked=%v reason=%q; want true/unknown", view.Blocked, view.BlockedReason)
	}
}

func TestAddDependency_RejectsSelfEdge(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	task := mustSeedTask(t, svc, "A")
	if err := svc.AddDependency(context.Background(), task.ID, task.ID); err == nil {
		t.Fatal("expected a self-edge to be rejected")
	}
}

func TestAddDependency_RejectsCrossWorkspaceEdge(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()
	local := mustSeedTask(t, svc, "Local")
	// Move a seeded task to another workspace at the repository layer: CreateTask
	// requires a provisioned workspace, and the point here is only that the
	// validator compares the two tasks' workspaces.
	foreign := mustSeedTask(t, svc, "Foreign")
	foreign.WorkspaceID = "ws-other"
	if err := svc.tasks.UpdateTask(ctx, foreign); err != nil {
		t.Fatalf("move foreign task: %v", err)
	}
	if err := svc.AddDependency(ctx, local.ID, foreign.ID); err == nil {
		t.Fatal("expected a cross-workspace edge to be rejected")
	}
}

func TestRemoveDependency_AbsentEdgeSucceeds(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	if err := svc.RemoveDependency(context.Background(), "a", "b"); err != nil {
		t.Fatalf("removing an absent edge should succeed, got %v", err)
	}
}

func TestResolveStartWhenUnblocked(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name string
		req  *CreateTaskRequest
		want bool
	}{
		{"no dependencies never defers", &CreateTaskRequest{}, false},
		{"no dependencies ignores an explicit true", &CreateTaskRequest{StartWhenUnblocked: &yes}, false},
		// The load-bearing default: agents pass start_agent=true by habit, so a
		// create WITH dependencies must record an intent rather than launch now,
		// or every step of an agent-built chain starts at once.
		{"dependencies default to deferring", &CreateTaskRequest{BlockedBy: []string{"a"}}, true},
		{"explicit false opts out", &CreateTaskRequest{BlockedBy: []string{"a"}, StartWhenUnblocked: &no}, false},
		{"explicit true defers", &CreateTaskRequest{BlockedBy: []string{"a"}, StartWhenUnblocked: &yes}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveStartWhenUnblocked(tc.req); got != tc.want {
				t.Errorf("ResolveStartWhenUnblocked() = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestDependencyStatusForTask_ArchivedIsPendingNotResolved(t *testing.T) {
	// Archival is neither success nor failure: an archived predecessor must not
	// release a dependent, and must not be reported as a failed chain either.
	if got := DependencyStatusForTask(&models.Task{State: v1.TaskStateCompleted}); got != DependencyResolved {
		t.Errorf("completed = %q; want %q", got, DependencyResolved)
	}
	archivedAt := time.Now().UTC()
	archived := &models.Task{State: v1.TaskStateCompleted, ArchivedAt: &archivedAt}
	if got := DependencyStatusForTask(archived); got != DependencyPending {
		t.Errorf("archived+completed = %q; want %q", got, DependencyPending)
	}
	if got := DependencyStatusForTask(nil); got != DependencyPending {
		t.Errorf("nil task = %q; want %q", got, DependencyPending)
	}
}
