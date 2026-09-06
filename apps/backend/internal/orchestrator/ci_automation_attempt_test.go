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

func (s *startupCIAutoFixAttemptService) GetTaskCIPRState(
	_ context.Context, taskID, repositoryID string, prNumber int,
) (*github.TaskCIPRAutomationState, error) {
	for _, state := range s.states {
		if state != nil && state.TaskID == taskID && state.RepositoryID == repositoryID && state.PRNumber == prNumber {
			return state, nil
		}
	}
	return nil, nil
}

type startupOrderCIAutoFixGitHubService struct {
	*mockGitHubService
	*startupCIAutoFixAttemptService
}

func (s *startupOrderCIAutoFixGitHubService) GetTaskCIPRState(
	ctx context.Context, taskID, repositoryID string, prNumber int,
) (*github.TaskCIPRAutomationState, error) {
	return s.mockGitHubService.GetTaskCIPRState(ctx, taskID, repositoryID, prNumber)
}

type cancellationCIAutoFixGitHubService struct {
	*mockGitHubService
	completionCalls int
}

func (s *cancellationCIAutoFixGitHubService) BindTaskCIAutoFixAttemptTurn(
	context.Context, github.TaskCIAutoFixAttemptBinding,
) error {
	return nil
}

func (s *cancellationCIAutoFixGitHubService) ReportTaskCIAutoFixOutcome(
	context.Context, github.TaskCIAutoFixOutcomeReport,
) error {
	return nil
}

func (s *cancellationCIAutoFixGitHubService) ReconcileTaskCIAutoFixTurnCompletion(
	context.Context, string, string, string,
) error {
	s.completionCalls++
	return nil
}

func (s *cancellationCIAutoFixGitHubService) ReconcileTaskCIAutoFixQueuedDispatchFailure(
	context.Context, github.TaskCIAutoFixAttemptBinding,
) error {
	return nil
}

func (s *cancellationCIAutoFixGitHubService) ReconcileTaskCIAutoFixProviderProgress(
	context.Context, github.TaskCIAutoFixProviderProgress,
) error {
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

func TestReconcileCIAutoFixAttemptsAfterSessionStartupReconciliation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")
	seedExecutorRunning(t, repo, "session1", "task1", "execution-startup")

	turn := &models.Turn{
		ID:            "turn-startup",
		TaskID:        "task1",
		TaskSessionID: "session1",
		StartedAt:     time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("seed open turn: %v", err)
	}
	attempts := &startupCIAutoFixAttemptService{states: []*github.TaskCIPRAutomationState{{
		TaskID:                  "task1",
		RepositoryID:            "repo-startup",
		PRNumber:                42,
		AutoFixAttemptState:     github.TaskCIAutoFixAttemptRunning,
		AutoFixAttemptSessionID: "session1",
		AutoFixAttemptTurnID:    "turn-startup",
	}}}
	ghSvc := &startupOrderCIAutoFixGitHubService{
		mockGitHubService:              &mockGitHubService{},
		startupCIAutoFixAttemptService: attempts,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.turnService = &repoTurnService{repo: repo}
	svc.SetGitHubService(ghSvc)

	// Mirror Service.Start: the first sweep sees the still-open pre-crash turn.
	svc.reconcileCIAutoFixAttemptsOnStartup(ctx)
	if len(attempts.reconciled) != 0 {
		t.Fatalf("startup sweep reconciled an open turn: %v", attempts.reconciled)
	}
	svc.reconcileExecutorSessionsOnStartup(ctx)
	// The second sweep must run after the executor pass abandons that turn.
	svc.reconcileCIAutoFixAttemptsOnStartup(ctx)

	require.Equal(t, []string{"task1/session1/turn-startup"}, attempts.reconciled)
	open := openTurnCount(t, repo, "session1")
	require.Zero(t, open, "startup reconciliation must close the pre-crash turn")
}

func TestReconcileCIAutoFixQueueAdmissionGapOnStartup(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-queue-recovery", "session-queue-recovery", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-queue-recovery"
	pr := &github.TaskPR{
		TaskID: "task-queue-recovery", RepositoryID: "repo-queue-recovery", PRNumber: 42,
	}
	metadata := ciAutomationMessageMetadataForPR(pr, "feedback-queue-recovery")
	queued, _, _, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, sessionID, pr.TaskID, "@ci-auto-fix\n\ncrash gap", "", "workflow", false,
		nil, metadata, ciAutomationCoalesceKey(pr), true,
	)
	require.NoError(t, err)
	require.NotNil(t, queued)

	// Simulate a process crash after the queue transaction committed but before
	// the attempt transaction ran: no matching state exists on restart.
	attempts := &startupCIAutoFixAttemptService{}
	require.NoError(t, svc.reconcileOrphanedCIAutoFixQueueEntries(ctx, attempts))
	require.Zero(t, svc.messageQueue.GetStatus(ctx, sessionID).Count)

	matching, _, _, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, sessionID, pr.TaskID, "@ci-auto-fix\n\nvalid reservation", "", "workflow", false,
		nil, metadata, ciAutomationCoalesceKey(pr), true,
	)
	require.NoError(t, err)
	attempts.states = []*github.TaskCIPRAutomationState{{
		TaskID:                     pr.TaskID,
		RepositoryID:               pr.RepositoryID,
		PRNumber:                   pr.PRNumber,
		AutoFixAttemptState:        github.TaskCIAutoFixAttemptQueued,
		AutoFixAttemptQueueEntryID: matching.ID,
		AutoFixAttemptSessionID:    sessionID,
		AutoFixAttemptSignature:    metadata["feedback_signature"].(string),
	}}
	require.NoError(t, svc.reconcileOrphanedCIAutoFixQueueEntries(ctx, attempts))
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, sessionID).Count)
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
