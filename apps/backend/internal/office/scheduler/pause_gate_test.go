package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// fakeSchedulerPauseGate is a scripted shared.PauseGate double with
// pop-front semantics, mirroring office/service's and office/routines'
// own gate-test fakes. calls records every workspaceID the gate was asked
// about, in order — AC-001.7 requires the gate be consulted with the
// derivation site's actual workspace, not just called at all.
type fakeSchedulerPauseGate struct {
	active []*models.WorkspacePause
	errs   []error
	calls  []string
}

func (f *fakeSchedulerPauseGate) PauseState(_ context.Context, workspaceID string) (*models.WorkspacePause, error) {
	f.calls = append(f.calls, workspaceID)
	var active *models.WorkspacePause
	if len(f.active) > 0 {
		active, f.active = f.active[0], f.active[1:]
	}
	var err error
	if len(f.errs) > 0 {
		err, f.errs = f.errs[0], f.errs[1:]
	}
	return active, err
}

// TestSchedulerQueueRun_BlockedByPause_ReturnsErrWorkspacePaused proves
// SchedulerService.QueueRun (gate site 3, the run-queue writer distinct
// from office/service.Service.QueueRun) is gated independently: it has
// its own guardAgentStatus copy and now its own SetPauseGate wiring.
func TestSchedulerQueueRun_BlockedByPause_ReturnsErrWorkspacePaused(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-paused-ws")
	gate := &fakeSchedulerPauseGate{active: []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}}}
	ss.SetPauseGate(gate)

	err := ss.QueueRun(context.Background(), "agent-paused-ws", RunReasonTaskAssigned, "{}", "")
	if !errors.Is(err, shared.ErrWorkspacePaused) {
		t.Fatalf("err = %v, want shared.ErrWorkspacePaused", err)
	}
	if len(gate.calls) != 1 || gate.calls[0] != "ws-1" {
		t.Fatalf("gate.calls = %v, want [ws-1] — the gate must be asked about the agent's own workspace", gate.calls)
	}

	runs, err := repo.ListRuns(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("len(runs) = %d, want 0 for a paused workspace", len(runs))
	}
}

// TestSchedulerQueueRun_PauseGateError_FailsClosed proves a gate-read
// error fails QueueRun closed (shared.ErrPauseGateUnavailable) and writes
// no row, matching office/service.Service.QueueRun's contract at the
// sibling gate site.
func TestSchedulerQueueRun_PauseGateError_FailsClosed(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-gate-error")
	ss.SetPauseGate(&fakeSchedulerPauseGate{errs: []error{errors.New("db unavailable")}})

	err := ss.QueueRun(context.Background(), "agent-gate-error", RunReasonTaskAssigned, "{}", "")
	if !errors.Is(err, shared.ErrPauseGateUnavailable) {
		t.Fatalf("err = %v, want shared.ErrPauseGateUnavailable", err)
	}

	runs, err := repo.ListRuns(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("len(runs) = %d, want 0 on a gate-read error", len(runs))
	}
}

// TestSchedulerQueueRun_NoPauseGateWired_Unaffected proves the nil-gate
// default keeps QueueRun dispatching exactly as before the kill switch
// existed (existing scheduler tests all rely on this).
func TestSchedulerQueueRun_NoPauseGateWired_Unaffected(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-no-gate")

	if err := ss.QueueRun(context.Background(), "agent-no-gate", RunReasonTaskAssigned, "{}", ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}

	runs, err := repo.ListRuns(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1 with no pause gate wired", len(runs))
	}
}
