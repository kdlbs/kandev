package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// @covers AC-TASKS-SESSION-STOP-SETTLEMENT-001.3
func TestStopSession_ExecutionLessSettlementRetiresDeadExecutionActivity(t *testing.T) {
	ctx := context.Background()
	svc, background, sessionID, _ := newExecutionLessStopBackgroundService(t, models.TaskSessionStateWaitingForInput)
	published := &recordingEventBus{}
	svc.eventBus = published

	for workID, kind := range map[string]streams.WorkloadKind{
		"dead-subagent": streams.WorkloadKindSubagent,
		"dead-shell":    streams.WorkloadKindShell,
	} {
		recordOpenBackgroundWorkload(t, background, sessionID, workID, kind)
	}
	svc.registerBackgroundWorkKind(
		sessionID, "subagent-call", "dead-exec", "dead-subagent", streams.BackgroundWorkKindSubagent,
	)
	svc.registerBackgroundWorkKind(
		sessionID, "shell-call", "dead-exec", "dead-shell", streams.BackgroundWorkKindShell,
	)
	svc.recordToolOwnership(sessionID, "tool-call", "tool-exec", toolOwnershipBackground)
	svc.markForegroundGenerating(sessionID, "claim-only-exec")
	require.Equal(t, 1, svc.ActiveSubagentCount(sessionID))

	require.NoError(t, svc.StopSession(ctx, sessionID, "execution already stopped", false))

	require.Zero(t, svc.ActiveSubagentCount(sessionID))
	activity := svc.lockTurnActivity(sessionID, false)
	if activity != nil {
		activity.mu.Unlock()
	}
	require.Nil(t, activity, "all activity owned by executions absent from the session must be retired")
	for _, workID := range []string{"dead-subagent", "dead-shell"} {
		workload, err := background.GetBackgroundWorkload(ctx, sessionID, workID)
		require.NoError(t, err)
		require.NotNil(t, workload)
		require.Equal(t, string(streams.RunStateEnded), workload.State)
		require.NotNil(t, workload.FinishedAt)
	}
	session, err := svc.repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)

	var publishedZero bool
	for _, record := range published.events {
		if record.subject != events.TaskSessionActivityChanged {
			continue
		}
		data, ok := record.event.Data.(map[string]interface{})
		if !ok {
			continue
		}
		if count, exists := data["active_subagent_count"].(int); exists && count == 0 {
			publishedZero = true
		}
	}
	require.True(t, publishedZero, "retirement publishes the cleared subagent count")
}

// @covers AC-TASKS-SESSION-STOP-SETTLEMENT-001.3
func TestStopSession_ExecutionLessSettlementSettlesOpenBackgroundRows(t *testing.T) {
	for _, probe := range []string{"dead", "live", "uncertain"} {
		t.Run(probe, func(t *testing.T) {
			ctx := context.Background()
			svc, background, sessionID, manager := newExecutionLessStopBackgroundService(
				t, models.TaskSessionStateWaitingForInput,
			)
			const workID = "work-retained"
			recordOpenBackgroundWorkload(t, background, sessionID, workID, streams.WorkloadKindSubagent)
			svc.registerBackgroundWorkKind(
				sessionID, "subagent-call", "dead-exec", workID, streams.BackgroundWorkKindSubagent,
			)
			switch probe {
			case "dead":
			case "live":
				manager.isAgentRunningFn = func(context.Context, string) bool { return true }
			case "uncertain":
				svc.agentManager = &failingAgentLivenessProbe{mockAgentManager: manager}
			}

			require.NoError(t, svc.StopSession(ctx, sessionID, "execution already stopped", false))
			require.Zero(t, svc.ActiveSubagentCount(sessionID))
			workload, err := background.GetBackgroundWorkload(ctx, sessionID, workID)
			require.NoError(t, err)
			require.NotNil(t, workload)
			wantState := streams.RunStateEnded
			if probe != "dead" {
				wantState = streams.RunStateRunning
			}
			require.Equal(t, string(wantState), workload.State,
				"only a confirmed dead process can settle the durable row")
		})
	}
}

// @covers AC-TASKS-SESSION-STOP-SETTLEMENT-001.3
func TestStopSession_ExecutionLessSettlementSettlesRowsAfterRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc, background, sessionID, _ := newExecutionLessStopBackgroundService(
		t, models.TaskSessionStateWaitingForInput,
	)
	const workID = "work-cancelled-request"
	recordOpenBackgroundWorkload(t, background, sessionID, workID, streams.WorkloadKindSubagent)
	svc.registerBackgroundWorkKind(
		sessionID, "subagent-call", "dead-exec", workID, streams.BackgroundWorkKindSubagent,
	)
	baseRepo, ok := svc.repo.(repoStore)
	require.True(t, ok, "test repository must support full repository operations")
	postCancellationReadBounded := false
	svc.repo = &coordinatorStopRepoHooks{
		repoStore: baseRepo,
		getSessionFunc: func(readCtx context.Context, id string) (*models.TaskSession, error) {
			if ctx.Err() != nil {
				_, hasDeadline := readCtx.Deadline()
				postCancellationReadBounded = hasDeadline && readCtx.Err() == nil
			}
			return baseRepo.GetTaskSession(readCtx, id)
		},
	}
	svc.eventBus = &recordingEventBus{}
	svc.turnService = &cancelAfterTurnClosure{TurnService: svc.turnService, cancel: cancel}

	require.NoError(t, svc.StopSession(ctx, sessionID, "execution already stopped", false))
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.True(t, postCancellationReadBounded,
		"activity publication must use cancellation-independent context with a deadline")
	workload, err := background.GetBackgroundWorkload(context.Background(), sessionID, workID)
	require.NoError(t, err)
	require.NotNil(t, workload)
	require.Equal(t, string(streams.RunStateEnded), workload.State,
		"durable settlement must finish after the request is canceled following turn closure")
}

// @covers AC-TASKS-SESSION-STOP-SETTLEMENT-001.3
func TestStopSession_ExecutionLessRefusalKeepsActivity(t *testing.T) {
	type configureRefusal func(
		*testing.T, *Service, *taskservice.Service, string, *mockAgentManager,
	) []string
	for _, scenario := range []struct {
		name  string
		state models.TaskSessionState
		setup configureRefusal
	}{
		{name: "idle state", state: models.TaskSessionStateIdle},
		{
			name: "lifecycle contention", state: models.TaskSessionStateWaitingForInput,
			setup: func(t *testing.T, svc *Service, _ *taskservice.Service, sessionID string, _ *mockAgentManager) []string {
				t.Cleanup(svc.acquireSessionLifecycleLock(sessionID))
				return nil
			},
		},
		{
			name: "new execution", state: models.TaskSessionStateWaitingForInput,
			setup: func(t *testing.T, svc *Service, background *taskservice.Service, sessionID string, manager *mockAgentManager) []string {
				const newWorkID = "work-new-execution"
				recordOpenBackgroundWorkload(t, background, sessionID, newWorkID, streams.WorkloadKindSubagent)
				reads := 0
				manager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
					reads++
					if reads == 1 {
						return "", lifecycle.ErrNoExecutionForSession
					}
					svc.registerBackgroundWorkKind(
						sessionID, "new-subagent-call", "successor-exec", newWorkID,
						streams.BackgroundWorkKindSubagent,
					)
					return "successor-exec", nil
				}
				return []string{newWorkID}
			},
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			svc, background, sessionID, manager := newExecutionLessStopBackgroundService(t, scenario.state)
			const oldWorkID = "work-old-execution"
			recordOpenBackgroundWorkload(t, background, sessionID, oldWorkID, streams.WorkloadKindSubagent)
			svc.registerBackgroundWorkKind(
				sessionID, "old-subagent-call", "dead-exec", oldWorkID, streams.BackgroundWorkKindSubagent,
			)
			workIDs := []string{oldWorkID}
			if scenario.setup != nil {
				workIDs = append(workIDs, scenario.setup(t, svc, background, sessionID, manager)...)
			}

			err := svc.StopSession(ctx, sessionID, "execution already stopped", false)
			require.ErrorIs(t, err, lifecycle.ErrNoExecutionForSession)
			require.Equal(t, len(workIDs), svc.ActiveSubagentCount(sessionID))
			session, lookupErr := svc.repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, lookupErr)
			require.Equal(t, scenario.state, session.State)
			for _, workID := range workIDs {
				workload, lookupErr := background.GetBackgroundWorkload(ctx, sessionID, workID)
				require.NoError(t, lookupErr)
				require.NotNil(t, workload)
				require.Equal(t, string(streams.RunStateRunning), workload.State)
			}
		})
	}
}

// @covers AC-TASKS-SESSION-STOP-SETTLEMENT-001.3
func TestExecutionIDsWithActivityIncludesDistinctOwnerSources(t *testing.T) {
	svc := &Service{foregroundActivity: map[string]*turnActivity{
		"session": {
			background: map[string]backgroundWork{
				"background": {executionID: "exec-background"},
				"unowned":    {},
			},
			tools: map[string]toolOwnershipRecord{
				"tool": {executionID: "exec-tool"},
			},
			executionClaims: map[string]struct{}{
				"exec-claim": {}, "exec-background": {},
			},
		},
	}}

	require.Equal(t, []string{"exec-background", "exec-claim", "exec-tool"}, svc.executionIDsWithActivity("session"))
}

// @covers AC-TASKS-SESSION-STOP-SETTLEMENT-001.3
func TestExecutionLessSettlementRetiresActivityWithoutSessionRow(t *testing.T) {
	svc, repo, _, sessionID, _ := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
	svc.registerBackgroundWorkKind(
		sessionID, "subagent-call", "dead-exec", "dead-subagent", streams.BackgroundWorkKindSubagent,
	)
	published := &recordingEventBus{}
	svc.eventBus = published
	svc.repo = &coordinatorStopRepoHooks{
		repoStore: repo,
		getSessionFunc: func(context.Context, string) (*models.TaskSession, error) {
			return nil, models.ErrTaskSessionNotFound
		},
	}

	settled, err := svc.settleExecutionLessSessionStop(context.Background(), sessionID)

	require.NoError(t, err)
	require.True(t, settled)
	require.Zero(t, svc.ActiveSubagentCount(sessionID))
	require.Empty(t, published.events, "without a session row there is no task ID for publication")
}

func newExecutionLessStopBackgroundService(
	t *testing.T,
	state models.TaskSessionState,
) (*Service, *taskservice.Service, string, *mockAgentManager) {
	t.Helper()
	svc, repo, _, sessionID, _ := newExecutionLessStopService(t, state)
	background := taskservice.NewService(
		taskservice.Repos{BackgroundWork: repo}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{},
	)
	svc.SetBackgroundWorkObserver(background)
	return svc, background, sessionID, svc.agentManager.(*mockAgentManager)
}

func recordOpenBackgroundWorkload(
	t *testing.T,
	background *taskservice.Service,
	sessionID, workID string,
	kind streams.WorkloadKind,
) {
	t.Helper()
	startedAt := time.Now().UTC()
	err := background.RecordBackgroundWorkloadObservation(context.Background(), streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: workID, Kind: kind, State: streams.RunStateRunning, StartedAt: &startedAt,
	}, "task-execution-less-stop", sessionID)
	require.NoError(t, err)
}

type cancelAfterTurnClosure struct {
	TurnService
	cancel    context.CancelFunc
	completed bool
}

func (s *cancelAfterTurnClosure) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn, err := s.TurnService.GetActiveTurn(ctx, sessionID)
	if s.completed && (isNoActiveTurnError(err) || err == nil && turn == nil) {
		s.cancel()
		s.completed = false
	}
	return turn, err
}

func (s *cancelAfterTurnClosure) CompleteTurn(ctx context.Context, turnID string) error {
	err := s.TurnService.CompleteTurn(ctx, turnID)
	if err == nil {
		s.completed = true
	}
	return err
}
