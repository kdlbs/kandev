package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type bindRetryCIAutoFixGitHubService struct {
	*mockGitHubService
	bindErrors       []error
	bindCalls        int
	queuedRecoveries int
}

func (s *bindRetryCIAutoFixGitHubService) BindTaskCIAutoFixAttemptTurn(
	_ context.Context, _ github.TaskCIAutoFixAttemptBinding,
) error {
	s.bindCalls++
	if len(s.bindErrors) == 0 {
		return nil
	}
	err := s.bindErrors[0]
	s.bindErrors = s.bindErrors[1:]
	return err
}

func (s *bindRetryCIAutoFixGitHubService) ReconcileTaskCIAutoFixQueuedDispatchFailure(
	_ context.Context, _ github.TaskCIAutoFixAttemptBinding,
) error {
	s.queuedRecoveries++
	return nil
}

func (s *bindRetryCIAutoFixGitHubService) ReportTaskCIAutoFixOutcome(
	context.Context, github.TaskCIAutoFixOutcomeReport,
) error {
	return nil
}

func (s *bindRetryCIAutoFixGitHubService) ReconcileTaskCIAutoFixTurnCompletion(
	context.Context, string, string, string,
) error {
	return nil
}

func (s *bindRetryCIAutoFixGitHubService) ReconcileTaskCIAutoFixProviderProgress(
	context.Context, github.TaskCIAutoFixProviderProgress,
) error {
	return nil
}

type startupCIAutoFixAttemptService struct {
	states             []*github.TaskCIPRAutomationState
	reconciled         []string
	providerReconciled []github.TaskCIAutoFixProviderProgress
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

func (s *startupCIAutoFixAttemptService) ReconcileTaskCIAutoFixProviderProgress(
	_ context.Context, progress github.TaskCIAutoFixProviderProgress,
) error {
	s.providerReconciled = append(s.providerReconciled, progress)
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
	expiredDeadline := completedAt.Add(-time.Minute)
	attempts := &startupCIAutoFixAttemptService{states: []*github.TaskCIPRAutomationState{
		{TaskID: "task-terminal", AutoFixAttemptState: github.TaskCIAutoFixAttemptRunning, AutoFixAttemptSessionID: "session-terminal", AutoFixAttemptTurnID: "turn-terminal"},
		{TaskID: "task-active", AutoFixAttemptState: github.TaskCIAutoFixAttemptRunning, AutoFixAttemptSessionID: "session-active", AutoFixAttemptTurnID: "turn-active"},
		{TaskID: "task-queued", AutoFixAttemptState: github.TaskCIAutoFixAttemptQueued, AutoFixAttemptSessionID: "session-queued", AutoFixAttemptTurnID: ""},
		{TaskID: "task-missing", AutoFixAttemptState: github.TaskCIAutoFixAttemptRunning, AutoFixAttemptSessionID: "session-missing", AutoFixAttemptTurnID: "turn-missing"},
		{TaskID: "task-stale", AutoFixAttemptState: github.TaskCIAutoFixAttemptRunning, AutoFixAttemptSessionID: "session-stale", AutoFixAttemptTurnID: "turn-stale"},
		{TaskID: "task-expired", RepositoryID: "repo-expired", PRNumber: 42, AutoFixAttemptState: github.TaskCIAutoFixAttemptAwaitingProviderProgress, AutoFixAttemptSignature: "sig-expired", AutoFixAttemptProviderGeneration: "generation-expired", AutoFixAttemptProgressDeadline: &expiredDeadline},
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
	require.Equal(t, 4, reconciled)
	require.ElementsMatch(t, []string{
		"task-terminal/session-terminal/turn-terminal",
		"task-missing/session-missing/turn-missing",
		"task-stale/session-stale/turn-stale",
	}, attempts.reconciled)
	require.Len(t, attempts.providerReconciled, 1)
	require.Equal(t, "task-expired", attempts.providerReconciled[0].TaskID)
	require.Equal(t, "repo-expired", attempts.providerReconciled[0].RepositoryID)
	require.Equal(t, 42, attempts.providerReconciled[0].PRNumber)
	require.Equal(t, "sig-expired", attempts.providerReconciled[0].Signature)
	require.Equal(t, "generation-expired", attempts.providerReconciled[0].ProviderGeneration)
}

func TestBindCIAutoFixAttemptTurnRetriesAndReleasesFailedQueueReservation(t *testing.T) {
	failure := errors.New("temporary store failure")
	service := &bindRetryCIAutoFixGitHubService{
		mockGitHubService: &mockGitHubService{},
		bindErrors:        []error{failure, failure, nil},
	}
	svc := &Service{githubService: service, logger: testLogger()}
	binding := github.TaskCIAutoFixAttemptBinding{
		TaskID: "task-bind", RepositoryID: "repo-bind", PRNumber: 7,
		SessionID: "session-bind", QueueEntryID: "queue-bind", Signature: "sig-bind", TurnID: "turn-bind",
	}

	svc.bindCIAutoFixAttemptTurnWithRecovery(context.Background(), binding)
	require.Equal(t, 3, service.bindCalls)
	assert.Zero(t, service.queuedRecoveries)

	service.bindErrors = []error{failure, failure, failure}
	svc.bindCIAutoFixAttemptTurnWithRecovery(context.Background(), binding)
	assert.Equal(t, 6, service.bindCalls)
	assert.Equal(t, 1, service.queuedRecoveries)
}
