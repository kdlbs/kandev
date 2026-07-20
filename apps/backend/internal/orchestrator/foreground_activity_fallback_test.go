package orchestrator

import (
	"testing"

	taskdto "github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// The orchestrator's in-memory activity tracker (ADR-0047) is best-effort and
// never persisted: after a backend restart every session's fine-grained substate
// is UNKNOWN. §spec:live-propagation-fallback makes one guarantee absolute — an
// unknown substate must NEVER resolve to "done" while the turn is still open.
// TestForegroundActivity_ExportedValue already locks the per-session seam; these
// tests wire the REAL orchestrator provider through the exact task-level
// serialization primitives the boot payload and every task surface consume, so
// the two halves can never drift into a false "done" between them.

// TestForegroundActivity_UnknownSubstate_TaskAggregateReadsNotDone proves an
// in-flight RUNNING session whose substate the tracker has never seen aggregates
// to "generating" at the task level — a not-done reading — rather than the empty
// aggregate that would fall through to a done coarse state.
func TestForegroundActivity_UnknownSubstate_TaskAggregateReadsNotDone(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-untracked"

	// Per-session seam: the untracked default is generating, never background/done.
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("untracked RUNNING session: got %q, want generating", got)
	}

	// Task-level seam: EnrichTaskForegroundActivity is exactly what boot_state and
	// the task DTO stamp onto the at-a-glance surfaces. A single untracked RUNNING
	// session must produce a non-empty "generating" aggregate — an empty aggregate
	// would fall through to the coarse task state and could read done.
	dto := &taskdto.TaskDTO{ID: "task-untracked"}
	sessions := []*models.TaskSession{{ID: sessionID, State: models.TaskSessionStateRunning}}
	taskdto.EnrichTaskForegroundActivity(dto, sessions, svc)
	if dto.ForegroundActivity != v1.ForegroundActivityGenerating {
		t.Fatalf("task aggregate over untracked RUNNING session: got %q, want generating (never done)",
			dto.ForegroundActivity)
	}
}

// TestForegroundActivity_UnknownSubstate_FinishedPrimaryDoesNotMaskUntrackedSecondary
// covers the multi-session most-active-wins path after a restart: a finished
// primary session sits alongside a still-RUNNING (but untracked) secondary. The
// aggregate must follow the live secondary — generating — not the finished
// primary's done reading.
func TestForegroundActivity_UnknownSubstate_FinishedPrimaryDoesNotMaskUntrackedSecondary(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	dto := &taskdto.TaskDTO{ID: "task-mixed"}
	sessions := []*models.TaskSession{
		{ID: "primary-done", State: models.TaskSessionStateCompleted},
		{ID: "secondary-untracked", State: models.TaskSessionStateRunning},
	}
	taskdto.EnrichTaskForegroundActivity(dto, sessions, svc)
	if dto.ForegroundActivity != v1.ForegroundActivityGenerating {
		t.Fatalf("aggregate with a finished primary + untracked live secondary: got %q, want generating",
			dto.ForegroundActivity)
	}
}
