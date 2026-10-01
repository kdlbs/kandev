package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
	"github.com/stretchr/testify/require"
)

type forkTestTurnService struct {
	TurnService
	turn   *models.Turn
	active *models.Turn
}

func (s *forkTestTurnService) GetTurn(context.Context, string) (*models.Turn, error) {
	return s.turn, nil
}

func (s *forkTestTurnService) GetActiveTurn(context.Context, string) (*models.Turn, error) {
	return s.active, nil
}

type forkTestAgentManager struct {
	*mockAgentManager
	forkCalls int
	forkID    string
	forkErr   error
}

type failProviderForkMetadataWrite struct {
	sessionExecutorStore
	failed bool
}

func (r *failProviderForkMetadataWrite) SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error {
	operation, ok := value.(map[string]interface{})
	if !r.failed && ok && operation[codexForkStatusKey] == codexForkStatusProviderReady {
		r.failed = true
		return errors.New("provider identity persistence failed")
	}
	return r.sessionExecutorStore.SetSessionMetadataKey(ctx, sessionID, key, value)
}

func (m *forkTestAgentManager) ForkSessionBySessionID(context.Context, string, string) (string, error) {
	m.forkCalls++
	return m.forkID, m.forkErr
}

func newForkTestService(t *testing.T, manager executor.AgentManagerClient, active *models.Turn) (*Service, *sqliterepo.Repository, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-fork", "session-fork", "step1")
	source, err := repo.GetTaskSession(ctx, "session-fork")
	require.NoError(t, err)
	source.State = models.TaskSessionStateWaitingForInput
	source.AgentProfileID = "profile-codex-native"
	source.AgentProfileSnapshot = map[string]interface{}{"agent_id": "codex-app-server"}
	source.ExecutorID = "executor-local"
	source.ExecutorProfileID = "executor-profile-local"
	require.NoError(t, repo.UpdateTaskSession(ctx, source))

	now := time.Now().UTC()
	completedAt := now
	turn := &models.Turn{
		ID:            "turn-fork",
		TaskSessionID: source.ID,
		TaskID:        source.TaskID,
		StartedAt:     now.Add(-time.Minute),
		CompletedAt:   &completedAt,
		Metadata: map[string]interface{}{
			"agent_type":                        "codex-app-server",
			models.TurnMetaKeyCodexNativeTurnID: "provider-turn-1",
		},
		CreatedAt: now.Add(-time.Minute),
		UpdatedAt: now,
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-fork"] = &v1.Task{ID: "task-fork"}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.config.CodexAppServerEnabled = true
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.queue = queue.NewTaskQueue(10)
	svc.scheduler = scheduler.NewScheduler(svc.queue, svc.executor, taskRepo, testLogger(), scheduler.DefaultSchedulerConfig())
	svc.SetTurnService(&forkTestTurnService{turn: turn, active: active})
	return svc, repo, source
}

func TestForkOwnershipAndIdempotency(t *testing.T) {
	ctx := context.Background()
	requestID := uuid.NewString()

	t.Run("disabled provider does not leave a request marker", func(t *testing.T) {
		manager := &forkTestAgentManager{mockAgentManager: &mockAgentManager{}, forkID: "fork-thread"}
		svc, repo, _ := newForkTestService(t, manager, nil)
		svc.config.CodexAppServerEnabled = false

		_, err := svc.ForkConversation(ctx, ForkConversationRequest{
			TaskID: "task-fork", SourceSessionID: "session-fork", TurnID: "turn-fork", RequestID: requestID,
		})
		require.ErrorIs(t, err, ErrCodexForkUnsupported)
		require.Zero(t, manager.forkCalls)
		source, err := repo.GetTaskSession(ctx, "session-fork")
		require.NoError(t, err)
		require.NotContains(t, source.Metadata, models.SessionMetaKeyCodexForkRequestPrefix+requestID)
	})

	t.Run("unavailable fork capability does not claim the request", func(t *testing.T) {
		svc, repo, _ := newForkTestService(t, &mockAgentManager{}, nil)
		_, err := svc.ForkConversation(ctx, ForkConversationRequest{
			TaskID: "task-fork", SourceSessionID: "session-fork", TurnID: "turn-fork", RequestID: requestID,
		})
		require.ErrorIs(t, err, ErrCodexForkUnsupported)
		source, err := repo.GetTaskSession(ctx, "session-fork")
		require.NoError(t, err)
		require.NotContains(t, source.Metadata, models.SessionMetaKeyCodexForkRequestPrefix+requestID)
	})

	t.Run("ambiguous provider result is not retried", func(t *testing.T) {
		manager := &forkTestAgentManager{
			mockAgentManager: &mockAgentManager{},
			forkErr:          errors.New("connection lost after request write"),
		}
		svc, repo, _ := newForkTestService(t, manager, nil)
		req := ForkConversationRequest{
			TaskID: "task-fork", SourceSessionID: "session-fork", TurnID: "turn-fork", RequestID: requestID,
		}
		_, firstErr := svc.ForkConversation(ctx, req)
		require.ErrorIs(t, firstErr, ErrCodexForkUncertain)
		_, secondErr := svc.ForkConversation(ctx, req)
		require.ErrorIs(t, secondErr, ErrCodexForkUncertain)
		require.Equal(t, 1, manager.forkCalls)

		source, err := repo.GetTaskSession(ctx, "session-fork")
		require.NoError(t, err)
		record := readCodexForkOperation(source.Metadata[models.SessionMetaKeyCodexForkRequestPrefix+requestID])
		require.Equal(t, codexForkStatusUncertain, record[codexForkStatusKey])
	})

	t.Run("provider success with failed local persistence is uncertain", func(t *testing.T) {
		manager := &forkTestAgentManager{mockAgentManager: &mockAgentManager{}, forkID: "fork-thread"}
		svc, repo, _ := newForkTestService(t, manager, nil)
		svc.repo = &failProviderForkMetadataWrite{sessionExecutorStore: svc.repo}
		req := ForkConversationRequest{
			TaskID: "task-fork", SourceSessionID: "session-fork", TurnID: "turn-fork", RequestID: requestID,
		}
		_, firstErr := svc.ForkConversation(ctx, req)
		require.ErrorContains(t, firstErr, "provider identity persistence failed")
		_, secondErr := svc.ForkConversation(ctx, req)
		require.ErrorIs(t, secondErr, ErrCodexForkUncertain)
		require.Equal(t, 1, manager.forkCalls)

		source, err := repo.GetTaskSession(ctx, "session-fork")
		require.NoError(t, err)
		record := readCodexForkOperation(source.Metadata[models.SessionMetaKeyCodexForkRequestPrefix+requestID])
		require.Equal(t, codexForkStatusUncertain, record[codexForkStatusKey])
	})

	t.Run("known pre-provider refusal releases the request for retry", func(t *testing.T) {
		manager := &forkTestAgentManager{
			mockAgentManager: &mockAgentManager{},
			forkErr:          protocol.ErrForkPrecondition,
		}
		svc, repo, _ := newForkTestService(t, manager, nil)
		req := ForkConversationRequest{
			TaskID: "task-fork", SourceSessionID: "session-fork", TurnID: "turn-fork", RequestID: requestID,
		}
		_, firstErr := svc.ForkConversation(ctx, req)
		if errors.Is(firstErr, ErrCodexForkUncertain) {
			t.Fatalf("pre-provider refusal was marked uncertain: %v", firstErr)
		}
		source, err := repo.GetTaskSession(ctx, "session-fork")
		require.NoError(t, err)
		require.NotContains(t, source.Metadata, models.SessionMetaKeyCodexForkRequestPrefix+requestID)

		manager.forkErr = nil
		manager.forkID = "fork-thread"
		second, err := svc.ForkConversation(ctx, req)
		require.NoError(t, err)
		require.NotEmpty(t, second.SessionID)
		require.Equal(t, 2, manager.forkCalls)
	})

	t.Run("source activity is rejected before provider fork", func(t *testing.T) {
		manager := &forkTestAgentManager{mockAgentManager: &mockAgentManager{}, forkID: "fork-thread"}
		active := &models.Turn{ID: "active-turn"}
		svc, repo, _ := newForkTestService(t, manager, active)
		_, err := svc.ForkConversation(ctx, ForkConversationRequest{
			TaskID: "task-fork", SourceSessionID: "session-fork", TurnID: "turn-fork", RequestID: requestID,
		})
		require.ErrorIs(t, err, ErrCodexForkActive)
		require.Zero(t, manager.forkCalls)
		source, err := repo.GetTaskSession(ctx, "session-fork")
		require.NoError(t, err)
		require.NotContains(t, source.Metadata, models.SessionMetaKeyCodexForkRequestPrefix+requestID)
	})

	t.Run("successful request reuses one persisted destination", func(t *testing.T) {
		manager := &forkTestAgentManager{mockAgentManager: &mockAgentManager{}, forkID: "fork-thread"}
		svc, repo, source := newForkTestService(t, manager, nil)
		req := ForkConversationRequest{
			TaskID: "task-fork", SourceSessionID: "session-fork", TurnID: "turn-fork", RequestID: requestID,
		}
		first, err := svc.ForkConversation(ctx, req)
		require.NoError(t, err)
		require.NotEmpty(t, first.SessionID)
		require.Equal(t, string(models.TaskSessionStateCreated), first.State)
		second, err := svc.ForkConversation(ctx, req)
		require.NoError(t, err)
		require.Equal(t, first.SessionID, second.SessionID)
		require.Equal(t, 1, manager.forkCalls)

		destination, err := repo.GetTaskSession(ctx, first.SessionID)
		require.NoError(t, err)
		require.Equal(t, source.AgentProfileID, destination.AgentProfileID)
		require.Equal(t, source.ExecutorID, destination.ExecutorID)
		require.Equal(t, source.ExecutorProfileID, destination.ExecutorProfileID)
		require.Equal(t, "fork-thread", destination.DownstreamACPSessionID)
		require.Equal(t, source.ID, destination.Metadata["codex_fork_source_session_id"])
		require.Equal(t, "turn-fork", destination.Metadata["codex_fork_source_turn_id"])
		all, err := repo.ListTaskSessions(ctx, "task-fork")
		require.NoError(t, err)
		require.Len(t, all, 2)
	})
}

func TestForkConversationRequiresSessionPromptScope(t *testing.T) {
	manager := &forkTestAgentManager{mockAgentManager: &mockAgentManager{}, forkID: "fork-thread"}
	svc, repo, _ := newForkTestService(t, manager, nil)
	svc.SetSessionAccessChecker(func(context.Context, string) error { return nil })
	svc.SetTaskAccessChecker(func(context.Context, string) error { return nil })
	denied := errors.New("session.prompt denied")
	svc.SetSessionPromptChecker(func(context.Context, string) error { return denied })
	requestID := uuid.NewString()

	_, err := svc.ForkConversation(context.Background(), ForkConversationRequest{
		TaskID: "task-fork", SourceSessionID: "session-fork", TurnID: "turn-fork", RequestID: requestID,
	})
	require.ErrorIs(t, err, denied)
	require.Zero(t, manager.forkCalls)
	source, err := repo.GetTaskSession(context.Background(), "session-fork")
	require.NoError(t, err)
	require.NotContains(t, source.Metadata, models.SessionMetaKeyCodexForkRequestPrefix+requestID)
}
