package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	releaseWorkspaceID = "ws-wip-release"
	releaseWorkflowID  = "wf-wip-release"
	releaseStepID      = "step-wip-release"
)

type releaseFixture struct {
	t    *testing.T
	repo *Repository
	ctx  context.Context
	now  time.Time
}

// newReleaseFixture seeds one workspace, one workflow, and one WIP-1 step.
func newReleaseFixture(t *testing.T, enabled bool) *releaseFixture {
	t.Helper()
	return newReleaseFixtureWithRepo(t, newRepoForEntityTests(t), enabled)
}

// newReleaseFixtureWithRepo seeds the same step in repo, so the PostgreSQL
// tests run the scenarios against that dialect.
func newReleaseFixtureWithRepo(t *testing.T, repo *Repository, enabled bool) *releaseFixture {
	t.Helper()
	f := &releaseFixture{t: t, repo: repo, ctx: context.Background(), now: time.Now().UTC()}
	if err := f.repo.CreateWorkspace(f.ctx, &models.Workspace{
		ID: releaseWorkspaceID, Name: "Release", WIPSessionlessReleaseEnabled: enabled,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := f.repo.CreateWorkflow(f.ctx, &models.Workflow{ID: releaseWorkflowID, WorkspaceID: releaseWorkspaceID, Name: "Workflow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	f.exec(`INSERT INTO workflow_steps (id, workflow_id, name, position, wip_limit, created_at, updated_at)
		VALUES (?, ?, ?, 0, 1, ?, ?)`, releaseStepID, releaseWorkflowID, "Execution ready", f.now, f.now)
	return f
}

func (f *releaseFixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.repo.db.Exec(f.repo.db.Rebind(query), args...); err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

// occupant creates an admitted task whose last change is idle ago.
func (f *releaseFixture) occupant(id string, state v1.TaskState, idle time.Duration) {
	f.t.Helper()
	if err := f.repo.CreateTask(f.ctx, &models.Task{
		ID: id, WorkspaceID: releaseWorkspaceID, WorkflowID: releaseWorkflowID,
		WorkflowStepID: releaseStepID, Title: id, State: state, WIPAdmitted: true,
	}); err != nil {
		f.t.Fatalf("create task %s: %v", id, err)
	}
	f.exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`, f.now.Add(-idle), id)
}

func (f *releaseFixture) session(id, taskID string, state models.TaskSessionState, idle time.Duration) {
	f.t.Helper()
	at := f.now.Add(-idle)
	f.exec(`INSERT INTO task_sessions (id, task_id, state, started_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, taskID, string(state), at, at)
}

func (f *releaseFixture) queued(id string) *models.Task {
	f.t.Helper()
	queuedAt := f.now
	task := &models.Task{
		ID: id, WorkspaceID: releaseWorkspaceID, WorkflowID: releaseWorkflowID,
		WorkflowStepID: releaseStepID, Title: id, State: v1.TaskStateTODO,
		QueuedForStepID: releaseStepID, QueuedAt: &queuedAt,
	}
	if err := f.repo.CreateTask(f.ctx, task); err != nil {
		f.t.Fatalf("create queued task %s: %v", id, err)
	}
	return task
}

func (f *releaseFixture) counted() int {
	f.t.Helper()
	count, err := f.repo.CountAdmittedTasksByWorkflowStep(f.ctx, releaseStepID)
	if err != nil {
		f.t.Fatalf("count admitted: %v", err)
	}
	excluding, err := f.repo.CountTasksByWorkflowStepExcludingTask(f.ctx, releaseStepID, "")
	if err != nil {
		f.t.Fatalf("count admitted excluding: %v", err)
	}
	if count != excluding {
		f.t.Fatalf("admitted counts disagree: %d vs %d", count, excluding)
	}
	return count
}

func (f *releaseFixture) promote(task *models.Task) bool {
	f.t.Helper()
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.QueuedAt = nil
	claimed, err := f.repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(f.ctx, task, releaseStepID, releaseStepID, 1)
	if err != nil {
		f.t.Fatalf("promote %s: %v", task.ID, err)
	}
	return claimed
}

func TestWIPSessionlessRelease_OffKeepsSlot(t *testing.T) {
	f := newReleaseFixture(t, false)
	f.occupant("failed-no-session", v1.TaskStateFailed, time.Hour)

	if got := f.counted(); got != 1 {
		t.Fatalf("counted = %d, want 1 with the policy off", got)
	}
	if f.promote(f.queued("queued-next")) {
		t.Fatal("queued task was promoted into a full step with the policy off")
	}
}

func TestWIPSessionlessRelease_OnReleasesAfterGrace(t *testing.T) {
	for _, state := range []v1.TaskState{v1.TaskStateFailed, v1.TaskStateInProgress} {
		t.Run(string(state), func(t *testing.T) {
			f := newReleaseFixture(t, true)
			f.occupant("stuck", state, time.Hour)
			f.session("stuck-cancelled", "stuck", models.TaskSessionStateCancelled, 30*time.Minute)

			if got := f.counted(); got != 0 {
				t.Fatalf("counted = %d, want 0 once the grace passed", got)
			}
			if !f.promote(f.queued("queued-next")) {
				t.Fatal("queued task was not promoted into the released slot")
			}
			stuck, err := f.repo.GetTask(f.ctx, "stuck")
			if err != nil {
				t.Fatal(err)
			}
			if stuck.State != state || stuck.WorkflowStepID != releaseStepID || !stuck.WIPAdmitted {
				t.Fatalf("released task was changed: %+v", stuck)
			}
			if got := f.counted(); got != 1 {
				t.Fatalf("counted after pull = %d, want 1 (only the pulled task)", got)
			}
		})
	}
}

func TestWIPSessionlessRelease_HoldsDuringGrace(t *testing.T) {
	f := newReleaseFixture(t, true)
	f.occupant("recent", v1.TaskStateFailed, time.Hour)
	f.session("recent-failed", "recent", models.TaskSessionStateFailed, 2*time.Minute)

	if got := f.counted(); got != 1 {
		t.Fatalf("counted = %d, want 1 inside the grace", got)
	}
}

func TestWIPSessionlessRelease_LiveSessionHoldsSlot(t *testing.T) {
	f := newReleaseFixture(t, true)
	f.occupant("live", v1.TaskStateInProgress, time.Hour)
	f.session("live-old", "live", models.TaskSessionStateCompleted, time.Hour)
	f.session("live-waiting", "live", models.TaskSessionStateWaitingForInput, time.Hour)

	if got := f.counted(); got != 1 {
		t.Fatalf("counted = %d, want 1 while a session is live", got)
	}
}

func TestWIPSessionlessRelease_ReentryCountsAgain(t *testing.T) {
	f := newReleaseFixture(t, true)
	f.occupant("returning", v1.TaskStateInProgress, time.Hour)
	if got := f.counted(); got != 0 {
		t.Fatalf("counted = %d, want 0 for a sessionless task past its grace", got)
	}
	f.session("returning-new", "returning", models.TaskSessionStateRunning, 0)
	if got := f.counted(); got != 1 {
		t.Fatalf("counted = %d, want 1 after the task re-entered a live session", got)
	}
}

func TestWIPSessionlessRelease_OtherStatesKeepSlot(t *testing.T) {
	f := newReleaseFixture(t, true)
	f.occupant("in-review", v1.TaskStateReview, time.Hour)

	if got := f.counted(); got != 1 {
		t.Fatalf("counted = %d, want 1 for a state the policy does not cover", got)
	}
}

func TestWIPSessionlessRelease_ListsOnlyEnabledSteps(t *testing.T) {
	off := newReleaseFixture(t, false)
	off.occupant("off-failed", v1.TaskStateFailed, time.Hour)
	steps, err := off.repo.ListSessionlessWIPReleaseSteps(off.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 0 {
		t.Fatalf("steps = %v, want none with the policy off", steps)
	}

	on := newReleaseFixture(t, true)
	on.occupant("on-failed", v1.TaskStateFailed, time.Minute)
	on.occupant("on-live", v1.TaskStateInProgress, time.Hour)
	on.session("on-live-running", "on-live", models.TaskSessionStateRunning, time.Hour)
	steps, err = on.repo.ListSessionlessWIPReleaseSteps(on.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0] != releaseStepID {
		t.Fatalf("steps = %v, want [%s]", steps, releaseStepID)
	}
}

func TestWIPStepOccupancy_ExplainsEachHolder(t *testing.T) {
	f := newReleaseFixture(t, true)
	f.occupant("a-live", v1.TaskStateInProgress, time.Hour)
	f.session("a-live-running", "a-live", models.TaskSessionStateRunning, time.Hour)
	f.occupant("b-released", v1.TaskStateFailed, time.Hour)
	f.occupant("c-pending", v1.TaskStateFailed, time.Hour)
	f.session("c-pending-failed", "c-pending", models.TaskSessionStateFailed, 3*time.Minute)
	f.occupant("d-review", v1.TaskStateReview, time.Hour)

	occupancy, err := f.repo.GetWorkflowStepWIPOccupancy(f.ctx, releaseStepID)
	if err != nil {
		t.Fatal(err)
	}
	if occupancy.WorkspaceID != releaseWorkspaceID || occupancy.WIPLimit != 1 ||
		!occupancy.ReleaseEnabled || occupancy.ReleaseGraceMinutes != models.DefaultWIPSessionlessReleaseGraceMinutes {
		t.Fatalf("occupancy header = %+v", occupancy)
	}
	want := map[string]models.WIPOccupantReason{
		"a-live":     models.WIPOccupantReasonLiveSession,
		"b-released": models.WIPOccupantReasonReleasedNoLiveSession,
		"c-pending":  models.WIPOccupantReasonReleaseGracePending,
		"d-review":   models.WIPOccupantReasonNoLiveSession,
	}
	if len(occupancy.Occupants) != len(want) {
		t.Fatalf("occupants = %+v", occupancy.Occupants)
	}
	for _, occupant := range occupancy.Occupants {
		if occupant.Reason != want[occupant.TaskID] {
			t.Errorf("%s reason = %s, want %s", occupant.TaskID, occupant.Reason, want[occupant.TaskID])
		}
		if occupant.HoldsSlot == (occupant.Reason == models.WIPOccupantReasonReleasedNoLiveSession) {
			t.Errorf("%s holds_slot = %v for reason %s", occupant.TaskID, occupant.HoldsSlot, occupant.Reason)
		}
	}
	pending := occupancy.Occupants[2]
	if pending.ReleaseAt == nil || pending.ReleaseAt.Before(f.now) || pending.LiveSessionCount != 0 || pending.SessionCount != 1 {
		t.Fatalf("pending occupant = %+v", pending)
	}
	if got := f.counted(); occupancy.Counted != got || got != 3 {
		t.Fatalf("occupancy counted = %d, admission counted = %d, want 3", occupancy.Counted, got)
	}
}

func TestWIPStepOccupancy_OffExplainsNoLiveSession(t *testing.T) {
	f := newReleaseFixture(t, false)
	f.occupant("stuck", v1.TaskStateFailed, time.Hour)

	occupancy, err := f.repo.GetWorkflowStepWIPOccupancy(f.ctx, releaseStepID)
	if err != nil {
		t.Fatal(err)
	}
	if occupancy.Counted != 1 || len(occupancy.Occupants) != 1 ||
		occupancy.Occupants[0].Reason != models.WIPOccupantReasonNoLiveSession || !occupancy.Occupants[0].HoldsSlot {
		t.Fatalf("occupancy = %+v", occupancy)
	}
}

func TestWIPStepOccupancy_UnknownStep(t *testing.T) {
	f := newReleaseFixture(t, false)
	if _, err := f.repo.GetWorkflowStepWIPOccupancy(f.ctx, "missing-step"); err == nil {
		t.Fatal("unknown step returned an occupancy")
	}
}
