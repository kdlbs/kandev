package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	agentapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

type settlingBackgroundWorkObserver struct {
	mockBackgroundWorkObserver
	mu      sync.Mutex
	settled []string
	open    []string
}

func (o *settlingBackgroundWorkObserver) SettleOpenBackgroundWorkloads(
	_ context.Context,
	sessionID string,
) ([]streams.WorkloadRunObservation, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.settled = append(o.settled, sessionID)
	return []streams.WorkloadRunObservation{{
		SessionID: sessionID,
		WorkID:    "work-" + sessionID,
		Kind:      streams.WorkloadKindShell,
		State:     streams.RunStateEnded,
		Revision:  2,
	}}, nil
}

func (o *settlingBackgroundWorkObserver) ListSessionIDsWithOpenBackgroundWork(context.Context) ([]string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.open...), nil
}

func (o *settlingBackgroundWorkObserver) settledSessions() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.settled...)
}

func backgroundWorkUpdates(recorded *recordingEventBus) []streams.WorkloadRunObservation {
	var out []streams.WorkloadRunObservation
	for _, record := range recorded.events {
		if record.event.Type != events.BackgroundWorkUpdated {
			continue
		}
		if obs, ok := record.event.Data.(*streams.WorkloadRunObservation); ok {
			out = append(out, *obs)
		}
	}
	return out
}

func TestTerminalSessionStateSettlesOpenBackgroundWork(t *testing.T) {
	for _, terminalState := range []models.TaskSessionState{
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
	} {
		t.Run(string(terminalState), func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			observer := &settlingBackgroundWorkObserver{}
			svc.SetBackgroundWorkObserver(observer)
			recorded := &recordingEventBus{}
			svc.eventBus = recorded

			svc.updateTaskSessionState(ctx, "t1", "s1", terminalState, "", false)

			require.Equal(t, []string{"s1"}, observer.settledSessions())
			updates := backgroundWorkUpdates(recorded)
			require.Len(t, updates, 1)
			require.Equal(t, streams.RunStateEnded, updates[0].State)
		})
	}
}

func TestWaitingForInputKeepsBackgroundWorkOpen(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

	require.Empty(t, observer.settledSessions())
}

func TestStartupSettlesBackgroundWorkWithoutLiveAgent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "live", "step1")
	agentManager := &mockAgentManager{
		isAgentRunningFn: func(_ context.Context, sessionID string) bool { return sessionID == "live" },
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	observer := &settlingBackgroundWorkObserver{open: []string{"live", "dead", "missing"}}
	svc.SetBackgroundWorkObserver(observer)
	recorded := &recordingEventBus{}
	svc.eventBus = recorded

	svc.settleOrphanedBackgroundWorkOnStartup(ctx)

	require.ElementsMatch(t, []string{"dead", "missing"}, observer.settledSessions())
	require.Len(t, backgroundWorkUpdates(recorded), 2)
}

func TestStartupKeepsBackgroundWorkOpenWhenLivenessProbeFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	agentManager := &failingAgentLivenessProbe{mockAgentManager: &mockAgentManager{}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	observer := &settlingBackgroundWorkObserver{open: []string{"unknown"}}
	svc.SetBackgroundWorkObserver(observer)

	svc.settleOrphanedBackgroundWorkOnStartup(ctx)

	require.Empty(t, observer.settledSessions())
}

func TestSettleWithoutSettlerIsNoop(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetBackgroundWorkObserver(&mockBackgroundWorkObserver{})

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateCompleted, "", false)
	svc.settleOrphanedBackgroundWorkOnStartup(ctx)
}

func TestIdleSuspensionSettlesOpenBackgroundWork(t *testing.T) {
	svc, agent, _ := newIdleParkingFixture(t, true, 1, "claude-acp")
	agent.suspendFunc = func(context.Context, agentapi.IdleSuspensionIdentity) error {
		agent.isAgentRunning = false
		return nil
	}

	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)

	svc.suspendWorkspaceIdleSessionsOnce(context.Background())

	require.Len(t, agent.suspensionCalls(), 1)
	require.Equal(t, []string{"session-idle-parking"}, observer.settledSessions())
}

func TestFailedIdleSuspensionKeepsBackgroundWorkOpen(t *testing.T) {
	svc, agent, _ := newIdleParkingFixture(t, true, 1, "claude-acp")
	agent.suspendFunc = func(context.Context, agentapi.IdleSuspensionIdentity) error {
		return errors.New("stop rejected")
	}
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)

	svc.suspendWorkspaceIdleSessionsOnce(context.Background())

	require.Empty(t, observer.settledSessions())
}

func TestArchiveCancellationSettlesOpenBackgroundWork(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	observer := &settlingBackgroundWorkObserver{}
	svc.SetBackgroundWorkObserver(observer)

	svc.ClearParkedProjectionOnSessionTerminated(ctx, "t1", "s1", models.TaskSessionStateCancelled)
	svc.ClearParkedProjectionOnSessionTerminated(ctx, "t1", "deleted", "")

	require.Equal(t, []string{"s1"}, observer.settledSessions())
}
