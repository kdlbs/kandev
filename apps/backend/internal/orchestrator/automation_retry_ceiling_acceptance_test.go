package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type retryCeilingOutcomeService struct {
	*stubAutomationService
	err    error
	result automation.RunDispatch
}

func (s *retryCeilingOutcomeService) DispatchRun(ctx context.Context, _ string, _ automation.ThreadAction, _ string, dispatch func() (automation.RunDispatch, error)) error {
	var err error
	s.result, err = dispatch()
	return errors.Join(s.err, err)
}

func TestQueuedRetryUncertainAcceptanceRetainsExecution(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		stop bool
	}{
		{"uncertain acceptance", fmt.Errorf("acceptance unresolved: %w", automation.ErrRetryLaunchUncertain), false},
		{"ordinary binding failure", errors.New("ordinary binding failed"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			base := setupAutomationRetentionFixture(t)
			taskID, sessionID := "queued-owner-task", "queued-owner-session"
			seedAutomationTask(t, base.repo, taskID, models.TaskOriginAutomationRun, false)
			require.NoError(t, base.repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, State: models.TaskSessionStateRunning, AgentExecutionID: "queued-execution"}))
			tasks := newMockTaskRepo()
			seedMockTaskState(tasks, taskID, v1.TaskStateInProgress)
			stopped := make(chan string, 1)
			manager := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "queued-execution", nil },
				stopAgentWithReasonFunc:      func(_ context.Context, executionID, _ string, _ bool) error { stopped <- executionID; return nil },
			}
			svc := createTestServiceWithScheduler(base.repo, newMockStepGetter(), tasks, manager)
			svc.executor.SetOnSessionStateTransition(svc.transitionTaskSessionState)
			svc.SetTurnService(&repoTurnService{repo: base.repo})
			dispatch := &retryCeilingOutcomeService{stubAutomationService: &stubAutomationService{}, err: tc.err}
			svc.SetAutomationService(dispatch)
			execution := &executor.TaskExecution{TaskID: taskID, SessionID: sessionID, TurnID: "queued-turn", AgentExecutionID: "queued-execution"}
			starts := 0
			actual, err := svc.startQueuedAutomationRun(ctx, taskID, &automationRunLaunch{RunID: "queued-run", ThreadAction: automation.ThreadActionCreated}, func() (*executor.TaskExecution, error) { starts++; return execution, nil })
			require.ErrorIs(t, err, tc.err)
			require.Same(t, execution, actual)
			require.Equal(t, 1, starts)
			require.Equal(t, automation.RunDispatch{TaskID: taskID, SessionID: sessionID, TurnID: "queued-turn"}, dispatch.result)
			session, err := base.repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			if tc.stop {
				require.Equal(t, models.TaskSessionStateCancelled, session.State)
				select {
				case id := <-stopped:
					require.Equal(t, "queued-execution", id)
				case <-time.After(2 * time.Second):
					t.Fatal("ordinary binding failure did not stop its execution")
				}
			} else {
				require.Equal(t, models.TaskSessionStateRunning, session.State)
				manager.mu.Lock()
				calls := len(manager.stopAgentWithReasonArgs)
				manager.mu.Unlock()
				require.Zero(t, calls)
			}
		})
	}
}
