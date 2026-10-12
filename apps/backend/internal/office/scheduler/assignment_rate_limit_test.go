package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

// TestClassifyAssignmentWake covers the three-step predicate in the
// system design's "## The predicate": reason == task_assigned,
// actor_type == "agent", the wake names a task. Table-driven per the
// design's non-symmetric failure-shape table.
func TestClassifyAssignmentWake(t *testing.T) {
	tests := []struct {
		name           string
		reason         string
		payload        string
		wantInScope    bool
		wantAttributed bool
		wantTaskID     string
		wantActorID    string
	}{
		{
			name:    "wrong reason is out of scope",
			reason:  RunReasonTaskComment,
			payload: `{"task_id":"t1","actor_type":"agent"}`,
		},
		{
			name:    "actor_type user is out of scope",
			reason:  RunReasonTaskAssigned,
			payload: `{"task_id":"t1","actor_type":"user"}`,
		},
		{
			name:    "actor_type absent is out of scope",
			reason:  RunReasonTaskAssigned,
			payload: `{"task_id":"t1"}`,
		},
		{
			name:    "actor_type other value is out of scope",
			reason:  RunReasonTaskAssigned,
			payload: `{"task_id":"t1","actor_type":"system"}`,
		},
		{
			name:    "unparseable payload is out of scope, never unattributed",
			reason:  RunReasonTaskAssigned,
			payload: `not json`,
		},
		{
			name:    "both actor_type and task_id absent is out of scope on the actor step",
			reason:  RunReasonTaskAssigned,
			payload: `{}`,
		},
		{
			name:        "task_id absent is in scope, unattributed",
			reason:      RunReasonTaskAssigned,
			payload:     `{"actor_type":"agent","actor_id":"a1"}`,
			wantInScope: true,
			wantActorID: "a1",
		},
		{
			name:        "task_id null is in scope, unattributed",
			reason:      RunReasonTaskAssigned,
			payload:     `{"task_id":null,"actor_type":"agent"}`,
			wantInScope: true,
		},
		{
			name:        "task_id empty string is in scope, unattributed",
			reason:      RunReasonTaskAssigned,
			payload:     `{"task_id":"","actor_type":"agent"}`,
			wantInScope: true,
		},
		{
			name:        "task_id non-string is in scope, unattributed",
			reason:      RunReasonTaskAssigned,
			payload:     `{"task_id":42,"actor_type":"agent"}`,
			wantInScope: true,
		},
		{
			name:           "task_id valid string is in scope, attributed",
			reason:         RunReasonTaskAssigned,
			payload:        `{"task_id":"t1","actor_type":"agent","actor_id":"a1"}`,
			wantInScope:    true,
			wantAttributed: true,
			wantTaskID:     "t1",
			wantActorID:    "a1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyAssignmentWake(tc.reason, tc.payload)
			if got.inScope != tc.wantInScope {
				t.Fatalf("inScope = %v, want %v", got.inScope, tc.wantInScope)
			}
			if got.taskAttributed != tc.wantAttributed {
				t.Fatalf("taskAttributed = %v, want %v", got.taskAttributed, tc.wantAttributed)
			}
			if got.taskID != tc.wantTaskID {
				t.Fatalf("taskID = %q, want %q", got.taskID, tc.wantTaskID)
			}
			if got.actorID != tc.wantActorID {
				t.Fatalf("actorID = %q, want %q", got.actorID, tc.wantActorID)
			}
		})
	}
}

// TestCheckAssignmentWakeAllowance_OutOfScopeNeverTouchesDB proves an
// out-of-scope wake short-circuits before any repo call by using a
// scheduler whose repo is nil — a DB touch would panic.
func TestCheckAssignmentWakeAllowance_OutOfScopeNeverTouchesDB(t *testing.T) {
	ss := &SchedulerService{}
	refused := ss.checkAssignmentWakeAllowance(context.Background(), "agent-1", RunReasonTaskComment, `{"task_id":"t1","actor_type":"agent"}`)
	if refused {
		t.Fatal("out-of-scope wake must never be refused")
	}
}

// TestCheckAssignmentWakeAllowance_AdmitsUnderAllowance covers the silent
// admit path: an in-scope wake with room left in the allowance is
// admitted with no counter movement.
func TestCheckAssignmentWakeAllowance_AdmitsUnderAllowance(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newReactivityTestScheduler(t, repo)
	ctx := context.Background()

	for i := 0; i < AssignmentWakeAllowanceN-1; i++ {
		createAssignmentWakeRun(t, repo, "under-allowance-task", time.Now().UTC())
	}

	refused := ss.checkAssignmentWakeAllowance(ctx, "agent-1", RunReasonTaskAssigned,
		`{"task_id":"under-allowance-task","actor_type":"agent"}`)
	if refused {
		t.Fatal("wake under the allowance must be admitted")
	}
}

// TestCheckAssignmentWakeAllowance_RefusesAtAllowance covers the refusal
// path: N admitted wakes already exist in the window, so the N+1th must
// be refused.
func TestCheckAssignmentWakeAllowance_RefusesAtAllowance(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newReactivityTestScheduler(t, repo)
	ctx := context.Background()

	for i := 0; i < AssignmentWakeAllowanceN; i++ {
		createAssignmentWakeRun(t, repo, "at-allowance-task", time.Now().UTC())
	}

	refused := ss.checkAssignmentWakeAllowance(ctx, "agent-1", RunReasonTaskAssigned,
		`{"task_id":"at-allowance-task","actor_type":"agent"}`)
	if !refused {
		t.Fatal("wake at the allowance must be refused")
	}
}

// TestCheckAssignmentWakeAllowance_TaskUnattributedAdmits covers
// AC-OFFICE-ASSIGN-RATE-002.3: an otherwise in-scope wake whose task
// cannot be determined is admitted, not refused.
func TestCheckAssignmentWakeAllowance_TaskUnattributedAdmits(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newReactivityTestScheduler(t, repo)
	ctx := context.Background()

	refused := ss.checkAssignmentWakeAllowance(ctx, "agent-1", RunReasonTaskAssigned,
		`{"actor_type":"agent"}`)
	if refused {
		t.Fatal("a wake whose task cannot be determined must fail open (admit)")
	}
}

// createAssignmentWakeRun inserts an admitted agent-initiated
// task_assigned run for taskID, backdated to requestedAt, using the
// established repo pattern (SetRunRequestedAtForTest) instead of
// time.Sleep.
func createAssignmentWakeRun(t *testing.T, repo interface {
	CreateRun(ctx context.Context, r *models.Run) error
	SetRunRequestedAtForTest(ctx context.Context, runID string, ts time.Time) error
}, taskID string, requestedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	r := &models.Run{
		ID:             uuid.New().String(),
		AgentProfileID: "agent-1",
		Reason:         RunReasonTaskAssigned,
		Payload:        `{"task_id":"` + taskID + `","actor_type":"agent"}`,
		Status:         RunStatusQueued,
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, r); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := repo.SetRunRequestedAtForTest(ctx, r.ID, requestedAt); err != nil {
		t.Fatalf("set requested_at: %v", err)
	}
}
