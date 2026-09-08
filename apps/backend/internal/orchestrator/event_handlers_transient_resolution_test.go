package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

type transientRetryMessageServiceStub struct {
	messages  []*models.Message
	listErr   error
	deleteErr error
	listCalls int
	deleted   []string
}

func (s *transientRetryMessageServiceStub) ListMessages(context.Context, string) ([]*models.Message, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.messages, nil
}

func (s *transientRetryMessageServiceStub) DeleteMessage(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return s.deleteErr
}

func newPersistentTransientRetryTestService(t *testing.T) (*Service, *taskservice.Service, *recordingEventBus) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eventBus := &recordingEventBus{}
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            repo,
		Sessions:         repo,
		GitSnapshots:     repo,
		RepoEntities:     repo,
		Executors:        repo,
		Environments:     repo,
		TaskEnvironments: repo,
		Reviews:          repo,
	}, eventBus, testLogger(), taskservice.RepositoryDiscoveryConfig{})
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.messageCreator = &serviceBackedMessageCreator{svc: taskSvc}
	svc.SetTransientRetryMessageService(taskSvc)
	return svc, taskSvc, eventBus
}

func createPersistedTransientRetryNotice(t *testing.T, svc *Service) {
	t.Helper()
	createPersistedTransientRetryNoticeAttempt(t, svc, 1)
}

func createPersistedTransientRetryNoticeAttempt(t *testing.T, svc *Service, attempt int) {
	t.Helper()
	svc.createTransientRetryStatusMessage(
		context.Background(),
		watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentID: "auggie"},
		nil,
		attempt,
		5*time.Second,
		time.Now().UTC().Add(5*time.Second),
	)
}

func retryingMessages(t *testing.T, taskSvc *taskservice.Service) []*models.Message {
	t.Helper()
	messages, err := taskSvc.ListMessages(context.Background(), "s1")
	require.NoError(t, err)
	result := make([]*models.Message, 0, len(messages))
	for _, message := range messages {
		if message.Metadata["retrying"] == true {
			result = append(result, message)
		}
	}
	return result
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.10
func TestResetTransientRetry_ResolvesPersistedNotices(t *testing.T) {
	svc, taskSvc, eventBus := newPersistentTransientRetryTestService(t)
	createPersistedTransientRetryNotice(t, svc)
	createPersistedTransientRetryNoticeAttempt(t, svc, 2)
	svc.scheduleTransientRetry("t1", "s1", "", 2, time.Hour)
	_, err := taskSvc.CreateMessage(context.Background(), &taskservice.CreateMessageRequest{
		TaskSessionID: "s1",
		TaskID:        "t1",
		Content:       "unrelated status",
		AuthorType:    "agent",
		Type:          string(models.MessageTypeStatus),
		Metadata:      map[string]interface{}{"retrying": false},
	})
	require.NoError(t, err)
	require.Len(t, retryingMessages(t, taskSvc), 2)

	svc.resetTransientRetry("s1")

	require.Empty(t, retryingMessages(t, taskSvc))
	messages, err := taskSvc.ListMessages(context.Background(), "s1")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	var deleted int
	for _, recorded := range eventBus.events {
		if recorded.subject == events.MessageDeleted {
			deleted++
		}
	}
	require.Equal(t, 2, deleted)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.10
func TestResetTransientRetry_WithoutActiveLoopSkipsTranscriptScan(t *testing.T) {
	store := &transientRetryMessageServiceStub{}
	svc := &Service{logger: testLogger(), transientRetryMessages: store}

	svc.resetTransientRetry("s1")

	require.Zero(t, store.listCalls)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.11
func TestCancelTransientRetry_NoActiveLoopResolvesPersistedNotice(t *testing.T) {
	svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
	createPersistedTransientRetryNotice(t, svc)

	require.False(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
	require.Empty(t, retryingMessages(t, taskSvc))
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.11
func TestCancelTransientRetry_ActiveLoopResolvesNoticeAndShowsRecovery(t *testing.T) {
	svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
	createPersistedTransientRetryNotice(t, svc)
	svc.scheduleTransientRetry("t1", "s1", "", 1, time.Hour)

	require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
	require.Empty(t, retryingMessages(t, taskSvc))

	messages, err := taskSvc.ListMessages(context.Background(), "s1")
	require.NoError(t, err)
	var recoveryFound bool
	for _, message := range messages {
		if message.Metadata["recovery_actions"] == true {
			recoveryFound = true
		}
	}
	require.True(t, recoveryFound)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.10
func TestNextTransientAttempt_DoesNotResolveCurrentNotice(t *testing.T) {
	svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
	createPersistedTransientRetryNotice(t, svc)
	svc.scheduleTransientRetry("t1", "s1", "", 1, time.Hour)

	require.Equal(t, 2, svc.nextTransientAttempt("s1"))
	require.Len(t, retryingMessages(t, taskSvc), 1)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.10
func TestResolveTransientRetryMessages_SwallowsStoreErrors(t *testing.T) {
	listErr := errors.New("list failed")
	listStub := &transientRetryMessageServiceStub{listErr: listErr}
	svc := &Service{logger: testLogger(), transientRetryMessages: listStub}
	svc.resolveTransientRetryMessages(context.Background(), "s1")
	require.Empty(t, listStub.deleted)

	deleteErr := errors.New("delete failed")
	deleteStub := &transientRetryMessageServiceStub{
		messages: []*models.Message{
			{ID: "retry-1", TaskSessionID: "s1", Metadata: map[string]interface{}{"retrying": true}},
			{ID: "retry-2", TaskSessionID: "s1", Metadata: map[string]interface{}{"retrying": true}},
		},
		deleteErr: deleteErr,
	}
	svc.transientRetryMessages = deleteStub
	svc.resolveTransientRetryMessages(context.Background(), "s1")
	require.ElementsMatch(t, []string{"retry-1", "retry-2"}, deleteStub.deleted)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.11
func TestCancelTransientRetry_DeniedPairDoesNotResolveNotice(t *testing.T) {
	store := &transientRetryMessageServiceStub{
		messages: []*models.Message{{
			ID:            "retry-1",
			TaskSessionID: "s1",
			Metadata:      map[string]interface{}{"retrying": true},
		}},
	}
	svc := &Service{
		logger:                 testLogger(),
		transientRetryMessages: store,
		sessionAccessCheck: func(context.Context, string) error {
			return errors.New("denied")
		},
	}

	require.False(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
	require.Empty(t, store.deleted)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.10
func TestStopSession_ResolvesPersistedNotice(t *testing.T) {
	svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
	repo, ok := svc.repo.(*sqliterepo.Repository)
	require.True(t, ok)
	seedExecutorRunning(t, repo, "s1", "t1", "exec-stop")
	createPersistedTransientRetryNotice(t, svc)

	require.NoError(t, svc.StopSession(context.Background(), "s1", "operator stopped", false))
	require.Empty(t, retryingMessages(t, taskSvc))
}
