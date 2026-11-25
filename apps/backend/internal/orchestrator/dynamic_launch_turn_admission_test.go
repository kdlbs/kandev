package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

type failedOfficeRelaunchTurnStart struct {
	TurnService
	err error
}

func (s failedOfficeRelaunchTurnStart) StartTurn(context.Context, string) (*models.Turn, error) {
	return nil, s.err
}

func officeRelaunchFailureEvent() watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID: officeRelaunchTaskID, SessionID: officeRelaunchSessionID,
		AgentExecutionID: officeRelaunchExecutionID, AgentProfileID: "reviewer",
	}
}

func TestOfficeDynamicRelaunchRejectsTurnAdmissionFailure(t *testing.T) {
	turnErr := errors.New("turn store unavailable")
	cases := []struct {
		name    string
		service func(TurnService) TurnService
		wantErr error
	}{
		{
			name: "lookup error",
			service: func(base TurnService) TurnService {
				return &failingActiveTurnLookup{TurnService: base, err: turnErr}
			},
			wantErr: turnErr,
		},
		{
			name: "write error",
			service: func(base TurnService) TurnService {
				return failedOfficeRelaunchTurnStart{TurnService: base, err: turnErr}
			},
			wantErr: turnErr,
		},
		{name: "turn service unavailable", service: func(TurnService) TurnService { return nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, captured := newOfficeRelaunchFixture(t)
			svc.turnService = tc.service(svc.turnService)

			outcome, err := svc.relaunchDynamicTaskAfterFailureOutcomeWithBinding(
				t.Context(), officeRelaunchFailureEvent(), "fallback-profile", launchOriginAutomatic, nil,
			)

			require.Equal(t, dynamicRelaunchFailed, outcome)
			require.Error(t, err)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
			require.Nil(t, *captured, "turn admission must succeed before provider dispatch")
		})
	}
}

func TestOfficeDynamicRelaunchFailedLaunchPreservesTurnOwnership(t *testing.T) {
	cases := []struct {
		name         string
		adoptTurn    bool
		cancelLaunch bool
	}{
		{name: "new turn"},
		{name: "new turn after cancellation", cancelLaunch: true},
		{name: "adopted turn", adoptTurn: true},
		{name: "adopted turn after cancellation", adoptTurn: true, cancelLaunch: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newOfficeRelaunchFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			data := officeRelaunchFailureEvent()
			task, session, prompt, mode, ok := svc.prepareDynamicRelaunchAfterFailure(ctx, data, launchOriginAutomatic)
			require.True(t, ok)
			require.Equal(t, dynamicRelaunchLaunchModePrepared, mode)
			var adoptedTurnID string
			if tc.adoptTurn {
				turn, err := svc.turnService.StartTurn(ctx, session.ID)
				require.NoError(t, err)
				adoptedTurnID = turn.ID
			}
			launchErr := errors.New("provider launch failed")
			var dispatchedTurnID string
			manager := svc.agentManager.(*mockAgentManager)
			manager.launchAgentFunc = func(
				_ context.Context, req *executor.LaunchAgentRequest,
			) (*executor.LaunchAgentResponse, error) {
				dispatchedTurnID = req.TurnID
				if tc.cancelLaunch {
					cancel()
				}
				return nil, launchErr
			}

			outcome, err := svc.launchPreparedDynamicRelaunch(
				ctx, data, task, session, prompt, "fallback-profile", mode, launchOriginAutomatic, nil,
			)

			require.Equal(t, dynamicRelaunchFailed, outcome)
			require.ErrorIs(t, err, launchErr)
			require.NotEmpty(t, dispatchedTurnID)
			active, err := svc.turnService.GetActiveTurn(t.Context(), session.ID)
			require.NoError(t, err)
			if tc.adoptTurn {
				require.NotNil(t, active)
				require.Equal(t, adoptedTurnID, active.ID)
				return
			}
			require.Nil(t, active, "a failed launch must close the turn it created")
			closed, err := repo.GetTurn(t.Context(), dispatchedTurnID)
			require.NoError(t, err)
			require.NotNil(t, closed.CompletedAt)
		})
	}
}
