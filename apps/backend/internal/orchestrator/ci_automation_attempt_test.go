package orchestrator

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type startupCIAutoFixAttemptService struct {
	states     []*github.TaskCIPRAutomationState
	reconciled []string
}

func (s *startupCIAutoFixAttemptService) ListTaskCIAutoFixStates(context.Context) ([]*github.TaskCIPRAutomationState, error) {
	return s.states, nil
}

func (s *startupCIAutoFixAttemptService) ReconcileTaskCIAutoFixTurnCompletion(
	_ context.Context, taskID, sessionID, turnID string,
) error {
	s.reconciled = append(s.reconciled, taskID+"/"+sessionID+"/"+turnID)
	return nil
}

type startupCIAutoFixTurnService struct {
	TurnService
	turns map[string]*models.Turn
	errs  map[string]error
}

func (s *startupCIAutoFixTurnService) GetTurn(_ context.Context, turnID string) (*models.Turn, error) {
	if err := s.errs[turnID]; err != nil {
		return nil, err
	}
	return s.turns[turnID], nil
}

func TestReconcileCIAutoFixAttemptStatesOnStartup(t *testing.T) {
	completedAt := time.Now().UTC()
	attempts := &startupCIAutoFixAttemptService{states: []*github.TaskCIPRAutomationState{
		{TaskID: "task-terminal", AutoFixAttemptState: github.TaskCIAutoFixAttemptRunning, AutoFixAttemptSessionID: "session-terminal", AutoFixAttemptTurnID: "turn-terminal"},
		{TaskID: "task-active", AutoFixAttemptState: github.TaskCIAutoFixAttemptRunning, AutoFixAttemptSessionID: "session-active", AutoFixAttemptTurnID: "turn-active"},
		{TaskID: "task-queued", AutoFixAttemptState: github.TaskCIAutoFixAttemptQueued, AutoFixAttemptSessionID: "session-queued", AutoFixAttemptTurnID: ""},
		{TaskID: "task-missing", AutoFixAttemptState: github.TaskCIAutoFixAttemptRunning, AutoFixAttemptSessionID: "session-missing", AutoFixAttemptTurnID: "turn-missing"},
		{TaskID: "task-stale", AutoFixAttemptState: github.TaskCIAutoFixAttemptRunning, AutoFixAttemptSessionID: "session-stale", AutoFixAttemptTurnID: "turn-stale"},
	}}
	turns := &startupCIAutoFixTurnService{
		turns: map[string]*models.Turn{
			"turn-terminal": {ID: "turn-terminal", CompletedAt: &completedAt},
			"turn-active":   {ID: "turn-active", TaskSessionID: "session-active", TaskID: "task-active"},
			"turn-stale":    {ID: "turn-stale", TaskSessionID: "other-session", TaskID: "other-task"},
		},
		errs: map[string]error{"turn-missing": sql.ErrNoRows},
	}

	reconciled, err := reconcileCIAutoFixAttemptStates(context.Background(), turns, attempts)
	require.NoError(t, err)
	require.Equal(t, 3, reconciled)
	require.ElementsMatch(t, []string{
		"task-terminal/session-terminal/turn-terminal",
		"task-missing/session-missing/turn-missing",
		"task-stale/session-stale/turn-stale",
	}, attempts.reconciled)
}
