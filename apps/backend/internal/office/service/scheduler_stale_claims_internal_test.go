package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type staleClaimProbeStarter struct {
	live  map[string]bool
	fail  map[string]bool
	calls []string
}

func (s *staleClaimProbeStarter) StartTask(context.Context, string, string, string, string, string,
	string, string, bool, []v1.MessageAttachment) error {
	return errors.New("unexpected launch")
}

func (s *staleClaimProbeStarter) TaskRunExecutionLive(
	_ context.Context, taskID, agentProfileID, sessionID string,
) (bool, error) {
	s.calls = append(s.calls, taskID+"/"+agentProfileID+"/"+sessionID)
	if s.fail[taskID] {
		return false, errors.New("probe failed")
	}
	return s.live[taskID], nil
}

func seedStaleClaimedRun(t *testing.T, si *SchedulerIntegration, id, agentID, payload string, claimedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	run := &runsmodels.Run{
		ID: id, AgentProfileID: agentID, WorkspaceID: "ws-1",
		Reason: "task_assigned", Payload: payload, Status: "queued", CoalescedCount: 1,
	}
	if err := si.svc.repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run %s: %v", id, err)
	}
	if err := si.svc.repo.SetRunStatusForTest(ctx, id, "claimed", &claimedAt, nil); err != nil {
		t.Fatalf("claim run %s: %v", id, err)
	}
}

func TestRecoverStaleClaimedRuns_KeepsClaimsWhoseExecutionIsLive(t *testing.T) {
	ctx := context.Background()
	repo := newClearAutoPauseTestRepo(t)
	starter := &staleClaimProbeStarter{
		live: map[string]bool{"task-live": true},
		fail: map[string]bool{"task-error": true},
	}
	si := &SchedulerIntegration{
		svc:    &Service{repo: repo, taskStarter: starter},
		logger: logger.Default(),
	}
	stale := time.Now().UTC().Add(-staleClaimedRunAge - time.Minute)
	seedStaleClaimedRun(t, si, "run-live", "agent-a", `{"task_id":"task-live"}`, stale)
	seedStaleClaimedRun(t, si, "run-ended", "agent-a", `{"task_id":"task-ended"}`, stale)
	seedStaleClaimedRun(t, si, "run-error", "agent-a", `{"task_id":"task-error"}`, stale)
	seedStaleClaimedRun(t, si, "run-taskless", "agent-a", `{}`, stale)
	if _, err := repo.SetRunSessionID(ctx, "run-live", "session-live"); err != nil {
		t.Fatalf("set session: %v", err)
	}

	si.recoverStaleClaimedRuns(ctx)

	live, err := repo.GetRunByID(ctx, "run-live")
	if err != nil {
		t.Fatalf("get live run: %v", err)
	}
	if live.Status != "claimed" || live.ClaimedAt == nil || !live.ClaimedAt.After(stale) {
		t.Errorf("live run = status %q claimed_at %v, want a renewed claim", live.Status, live.ClaimedAt)
	}
	for _, id := range []string{"run-ended", "run-error", "run-taskless"} {
		got, err := repo.GetRunByID(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if got.Status != "queued" || got.ClaimedAt != nil {
			t.Errorf("%s = status %q claimed_at %v, want requeued", id, got.Status, got.ClaimedAt)
		}
	}
	if len(starter.calls) != 3 {
		t.Errorf("probe calls = %v, want one per stale task-bound run", starter.calls)
	}
	for _, call := range starter.calls {
		if call == "task-live/agent-a/session-live" {
			return
		}
	}
	t.Errorf("probe calls = %v, want the live run's recorded session", starter.calls)
}

func TestRecoverStaleClaimedRuns_WithoutProbeRequeuesAsBefore(t *testing.T) {
	ctx := context.Background()
	repo := newClearAutoPauseTestRepo(t)
	si := &SchedulerIntegration{svc: &Service{repo: repo}, logger: logger.Default()}
	stale := time.Now().UTC().Add(-staleClaimedRunAge - time.Minute)
	seedStaleClaimedRun(t, si, "run-task", "agent-a", `{"task_id":"task-1"}`, stale)

	si.recoverStaleClaimedRuns(ctx)

	got, err := repo.GetRunByID(ctx, "run-task")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "queued" {
		t.Errorf("status = %q, want queued", got.Status)
	}
}
