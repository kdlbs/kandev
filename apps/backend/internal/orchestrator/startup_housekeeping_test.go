package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
)

type housekeepingTurns struct {
	countingTurnService
	reconciliations int
	err             error
}

func (h *housekeepingTurns) ReconcileUnpublishedPromptTurns(context.Context) (int, error) {
	h.reconciliations++
	return 0, h.err
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.2
func TestHousekeepingPreparationRunsOnceWithoutStartingRuntime(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	turns := &housekeepingTurns{}
	svc.SetTurnService(turns)
	reporter := startup.New(nil)
	reporter.Set(startup.RecoveringSessions)
	ctx := startup.WithReporter(t.Context(), reporter)
	for range 2 {
		if err := svc.PrepareStartup(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if turns.reconciliations != 1 || svc.running {
		t.Fatalf("preparation repeated or started runtime: calls=%d running=%t", turns.reconciliations, svc.running)
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.3
func TestHousekeepingPreparationFailureAndCancellationRemainRetryable(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	turns := &housekeepingTurns{err: errors.New("preparation unavailable")}
	svc.SetTurnService(turns)
	if err := svc.PrepareStartup(t.Context()); !errors.Is(err, turns.err) {
		t.Fatalf("preparation failure = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := svc.PrepareStartup(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preparation = %v", err)
	}
	turns.err = nil
	if err := svc.PrepareStartup(t.Context()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if turns.reconciliations != 2 {
		t.Fatalf("cancelled work admitted or retry omitted: %d", turns.reconciliations)
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-002.3
func TestHousekeepingRuntimeSelectionPreservesUnresolvedOwners(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  models.ExecutorRunning
		want bool
	}{
		{"stable stopped", models.ExecutorRunning{Status: models.ExecutorRunningStatusStopped, ResumeToken: "saved"}, false},
		{"stopped with local process", models.ExecutorRunning{Status: models.ExecutorRunningStatusStopped, LocalPID: 123}, true},
		{"running", models.ExecutorRunning{Status: models.ExecutorRunningStatusRunning}, true},
		{"unknown", models.ExecutorRunning{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := startupRuntimeNeedsReconciliation(&tc.row, models.TaskSessionStateWaitingForInput); got != tc.want {
				t.Fatalf("selection = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestHousekeepingBackgroundWorkDoesNotProbePendingRemoteOwner(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "remote", "step1")
	row := &models.ExecutorRunning{ID: "remote", SessionID: "remote", TaskID: "t1", AgentExecutionID: "remote-owner", Runtime: "ssh", Status: models.ExecutorRunningStatusRunning}
	if err := repo.UpsertExecutorRunning(t.Context(), row); err != nil {
		t.Fatal(err)
	}
	probes := 0
	agent := &mockAgentManager{isAgentRunningFn: func(context.Context, string) bool { probes++; return true }}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	observer := &settlingBackgroundWorkObserver{open: []string{"remote"}}
	svc.SetBackgroundWorkObserver(observer)
	svc.SetPendingRemoteRecoveryChecker(func(id, execution string) bool { return id == row.SessionID && execution == row.AgentExecutionID })
	svc.settleOrphanedBackgroundWorkOnStartup(t.Context())
	if probes != 0 {
		t.Fatalf("pending remote owner blocked housekeeping with %d probes", probes)
	}
	svc.SetPendingRemoteRecoveryChecker(func(string, string) bool { return false })
	svc.settleOrphanedBackgroundWorkOnStartup(t.Context())
	if probes == 0 {
		t.Fatal("positive control never reached the liveness probe")
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-002.3
func TestHousekeepingStoppedHandleStillSettlesAnActiveSession(t *testing.T) {
	for _, state := range []models.TaskSessionState{models.TaskSessionStateStarting, models.TaskSessionStateRunning} {
		t.Run(string(state), func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "stopped", "step1")
			if err := repo.UpdateTaskSessionState(t.Context(), "stopped", state, ""); err != nil {
				t.Fatal(err)
			}
			row := &models.ExecutorRunning{ID: "stopped", SessionID: "stopped", TaskID: "t1", Runtime: "standalone", Status: models.ExecutorRunningStatusStopped, Resumable: true, ResumeToken: "saved-conversation"}
			if err := repo.UpsertExecutorRunning(t.Context(), row); err != nil {
				t.Fatal(err)
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
			svc.reconcileExecutorSessionsOnStartup(t.Context())
			session, err := repo.GetTaskSession(t.Context(), row.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if session.State != models.TaskSessionStateWaitingForInput {
				t.Fatalf("stopped handle stranded active session in %s", session.State)
			}
			saved, err := repo.GetExecutorRunningBySessionID(t.Context(), row.SessionID)
			if err != nil || saved.ResumeToken != row.ResumeToken {
				t.Fatalf("conversation identity lost: row=%#v err=%v", saved, err)
			}
		})
	}
}
