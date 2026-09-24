package orchestrator

import (
	"context"
	"errors"
	"sync"
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
	updateErr error
	deleteErr error
	listCalls int
	updated   []string
	deleted   []string
}

func armRetiredTransientPromptEvidence(svc *Service) {
	svc.dynamicAttemptEvidence.Store("s1", &promptAttemptEvidence{
		executionID:      "execution-1",
		promptGeneration: 7,
		evidenceKnown:    true,
	})
}

type blockingTransientRetryMessageService struct {
	*transientRetryMessageServiceStub
	updateStarted chan struct{}
	releaseUpdate chan struct{}
	updateOnce    sync.Once
}

func (s *blockingTransientRetryMessageService) UpdateMessage(ctx context.Context, message *models.Message) error {
	s.updateOnce.Do(func() { close(s.updateStarted) })
	<-s.releaseUpdate
	return s.transientRetryMessageServiceStub.UpdateMessage(ctx, message)
}

func (s *transientRetryMessageServiceStub) ListMessages(context.Context, string) ([]*models.Message, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.messages, nil
}

func (s *transientRetryMessageServiceStub) UpdateMessage(_ context.Context, message *models.Message) error {
	s.updated = append(s.updated, message.ID)
	if s.updateErr != nil {
		return s.updateErr
	}
	for _, existing := range s.messages {
		if existing != nil && existing.ID == message.ID {
			*existing = *message
			return nil
		}
	}
	return errors.New("message not found")
}

func (s *transientRetryMessageServiceStub) DeleteMessage(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	if s.deleteErr != nil {
		return s.deleteErr
	}
	for index, message := range s.messages {
		if message != nil && message.ID == id {
			s.messages = append(s.messages[:index], s.messages[index+1:]...)
			break
		}
	}
	return nil
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
	// Seed a second row through the task service to preserve the legacy state
	// that terminal cleanup must still repair after the writer becomes
	// single-row.
	_, err := taskSvc.CreateMessage(context.Background(), &taskservice.CreateMessageRequest{
		TaskSessionID: "s1",
		TaskID:        "t1",
		Content:       "legacy retry notice",
		AuthorType:    "agent",
		Type:          string(models.MessageTypeStatus),
		Metadata:      map[string]interface{}{"retrying": true},
	})
	require.NoError(t, err)
	svc.scheduleTransientRetry("t1", "s1", "", 2, time.Hour)
	_, err = taskSvc.CreateMessage(context.Background(), &taskservice.CreateMessageRequest{
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

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
func TestTransientRetryStatusMessage_UpdatesSameNotice(t *testing.T) {
	svc, taskSvc, eventBus := newPersistentTransientRetryTestService(t)
	createPersistedTransientRetryNoticeAttempt(t, svc, 1)

	first, err := taskSvc.ListMessages(context.Background(), "s1")
	require.NoError(t, err)
	require.Len(t, first, 1)
	firstNotice := first[0]

	svc.createTransientRetryStatusMessage(
		context.Background(),
		watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentID: "codex-acp"},
		nil,
		2,
		10*time.Second,
		time.Now().UTC().Add(10*time.Second),
	)

	updated, err := taskSvc.ListMessages(context.Background(), "s1")
	require.NoError(t, err)
	require.Len(t, updated, 1)
	updatedNotice := updated[0]
	require.Equal(t, firstNotice.ID, updatedNotice.ID)
	require.Equal(t, firstNotice.CreatedAt, updatedNotice.CreatedAt)
	require.Equal(t, firstNotice.TurnID, updatedNotice.TurnID)
	require.Equal(t, float64(2), updatedNotice.Metadata["attempt"])
	require.Equal(t, float64(10), updatedNotice.Metadata["retry_in_seconds"])
	require.Contains(t, updatedNotice.Content, "attempt 2/5")

	var added, changed int
	for _, recorded := range eventBus.events {
		switch recorded.subject {
		case events.MessageAdded:
			added++
		case events.MessageUpdated:
			changed++
		}
	}
	require.Equal(t, 1, added)
	require.Equal(t, 1, changed)
}

func transientRetryTestMessage(id, taskID, sessionID string, createdAt time.Time, metadata map[string]interface{}) *models.Message {
	return &models.Message{
		ID:            id,
		TaskID:        taskID,
		TaskSessionID: sessionID,
		TurnID:        "turn-legacy",
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeStatus,
		Content:       "legacy retry notice",
		Metadata:      metadata,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
func TestTransientRetryStatusMessage_ConsolidatesDuplicatesAndIgnoresUnrelatedRows(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store := &transientRetryMessageServiceStub{messages: []*models.Message{
		transientRetryTestMessage("retry-later", "t1", "s1", base.Add(2*time.Second), map[string]interface{}{
			"retrying":      true,
			"provider_name": "old-provider",
		}),
		transientRetryTestMessage("retry-earlier", "t1", "s1", base.Add(time.Second), map[string]interface{}{
			"retrying": true,
			"model_id": "old-model",
		}),
		transientRetryTestMessage("other-task", "t2", "s1", base, map[string]interface{}{"retrying": true}),
		transientRetryTestMessage("other-session", "t1", "s2", base, map[string]interface{}{"retrying": true}),
		{
			ID:            "wrong-type",
			TaskID:        "t1",
			TaskSessionID: "s1",
			Type:          models.MessageTypeMessage,
			Metadata:      map[string]interface{}{"retrying": true},
		},
		{
			ID:            "not-retrying",
			TaskID:        "t1",
			TaskSessionID: "s1",
			Type:          models.MessageTypeStatus,
			Metadata:      map[string]interface{}{"retrying": "true"},
		},
		{
			ID:            "unrelated-status",
			TaskID:        "t1",
			TaskSessionID: "s1",
			Type:          models.MessageTypeStatus,
			Metadata:      map[string]interface{}{"retrying": false},
		},
		nil,
	}}
	creator := &mockMessageCreator{}
	svc := &Service{
		logger:                 testLogger(),
		messageCreator:         creator,
		transientRetryMessages: store,
	}

	svc.createTransientRetryStatusMessage(
		context.Background(),
		watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentID: "codex-acp"},
		nil,
		3,
		20*time.Second,
		base.Add(20*time.Second),
	)

	require.Equal(t, []string{"retry-later"}, store.updated)
	require.Equal(t, []string{"retry-earlier"}, store.deleted)
	require.Empty(t, creator.sessionMessages)
	later := store.messages[0]
	require.Contains(t, later.Content, "attempt 3/5")
	require.Equal(t, "codex-acp", later.Metadata["provider_name"])
	require.NotContains(t, later.Metadata, "model_id")
	remaining := transientRetryNotices(store.messages, "t1", "s1")
	require.Len(t, remaining, 1)
	require.Equal(t, "retry-later", remaining[0].ID)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
func TestTransientRetryStatusMessage_StoreErrorsDoNotCreateExtraNotice(t *testing.T) {
	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentID: "codex-acp"}

	t.Run("list error", func(t *testing.T) {
		store := &transientRetryMessageServiceStub{listErr: errors.New("list failed")}
		creator := &mockMessageCreator{}
		svc := &Service{logger: testLogger(), messageCreator: creator, transientRetryMessages: store}

		svc.createTransientRetryStatusMessage(context.Background(), data, nil, 1, time.Second, time.Now().UTC())

		require.Empty(t, creator.sessionMessages)
		require.Empty(t, store.updated)
		require.Empty(t, store.deleted)
	})

	t.Run("update error", func(t *testing.T) {
		store := &transientRetryMessageServiceStub{
			messages:  []*models.Message{transientRetryTestMessage("retry-1", "t1", "s1", time.Now().UTC(), map[string]interface{}{"retrying": true})},
			updateErr: errors.New("update failed"),
		}
		creator := &mockMessageCreator{}
		svc := &Service{logger: testLogger(), messageCreator: creator, transientRetryMessages: store}

		svc.createTransientRetryStatusMessage(context.Background(), data, nil, 2, time.Second, time.Now().UTC())

		require.Equal(t, []string{"retry-1"}, store.updated)
		require.Empty(t, creator.sessionMessages)
		require.Empty(t, store.deleted)
	})
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
func TestTransientRetryStatusMessage_RetainsDuplicatesAfterDeleteErrorForNextWrite(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store := &transientRetryMessageServiceStub{
		messages: []*models.Message{
			transientRetryTestMessage("retry-1", "t1", "s1", base, map[string]interface{}{"retrying": true}),
			transientRetryTestMessage("retry-2", "t1", "s1", base.Add(time.Second), map[string]interface{}{"retrying": true}),
		},
		deleteErr: errors.New("delete failed"),
	}
	svc := &Service{
		logger:                 testLogger(),
		messageCreator:         &mockMessageCreator{},
		transientRetryMessages: store,
	}

	write := func(attempt int) {
		svc.createTransientRetryStatusMessage(
			context.Background(),
			watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentID: "codex-acp"},
			nil,
			attempt,
			time.Second,
			base.Add(time.Duration(attempt)*time.Second),
		)
	}
	write(2)
	require.Equal(t, []string{"retry-1"}, store.deleted)
	require.Len(t, store.messages, 2)

	store.deleteErr = nil
	write(3)
	require.Equal(t, []string{"retry-1", "retry-1"}, store.deleted)
	require.Len(t, store.messages, 1)
	require.Equal(t, "retry-2", store.messages[0].ID)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
func TestTransientRetryStatusMessage_SerializesRetirement(t *testing.T) {
	store := &blockingTransientRetryMessageService{
		transientRetryMessageServiceStub: &transientRetryMessageServiceStub{
			messages: []*models.Message{
				transientRetryTestMessage(
					"retry-1",
					"t1",
					"s1",
					time.Now().UTC(),
					map[string]interface{}{"retrying": true},
				),
			},
		},
		updateStarted: make(chan struct{}),
		releaseUpdate: make(chan struct{}),
	}
	svc := &Service{
		logger:                 testLogger(),
		messageCreator:         &mockMessageCreator{},
		transientRetryMessages: store,
	}
	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentID: "codex-acp"}

	writeDone := make(chan struct{})
	go func() {
		svc.createTransientRetryStatusMessage(
			context.Background(),
			data,
			nil,
			2,
			10*time.Second,
			time.Now().UTC().Add(10*time.Second),
		)
		close(writeDone)
	}()

	select {
	case <-store.updateStarted:
	case <-time.After(time.Second):
		t.Fatal("retry notice update did not start")
	}

	retireDone := make(chan struct{})
	go func() {
		svc.resetTransientRetryWithContext(context.Background(), "s1", true)
		close(retireDone)
	}()
	select {
	case <-retireDone:
		t.Fatal("retry notice retirement overtook the in-flight update")
	case <-time.After(100 * time.Millisecond):
	}

	close(store.releaseUpdate)
	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("retry notice update did not finish")
	}
	select {
	case <-retireDone:
	case <-time.After(time.Second):
		t.Fatal("retry notice retirement did not finish")
	}

	require.Empty(t, transientRetryNotices(store.messages, "t1", "s1"))
	require.Equal(t, []string{"retry-1"}, store.updated)
	require.Equal(t, []string{"retry-1"}, store.deleted)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
func TestHandleTransientFailure_RetiredLifecycleIgnoresLateFailure(t *testing.T) {
	t.Run("explicit cancellation", func(t *testing.T) {
		svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
		svc.rememberTurnPrompt("s1", "retry me", "", false, nil)
		svc.scheduleTransientRetry("t1", "s1", "execution-1", 1, time.Hour)

		require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
		armRetiredTransientPromptEvidence(svc)

		require.True(t, svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "execution-1",
			PromptGeneration: 7,
			ErrorMessage:     overloaded529,
		}), "a late eligible failure must be consumed after cancellation")
		require.Empty(t, retryingMessages(t, taskSvc))
		_, retryArmed := svc.transientRetries.Load("s1")
		require.False(t, retryArmed)
	})

	t.Run("terminal session", func(t *testing.T) {
		svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
		repo, ok := svc.repo.(*sqliterepo.Repository)
		require.True(t, ok)
		svc.rememberTurnPrompt("s1", "retry me", "", false, nil)
		svc.scheduleTransientRetry("t1", "s1", "execution-1", 1, time.Hour)
		require.NoError(t, repo.UpdateTaskSessionState(
			context.Background(), "s1", models.TaskSessionStateCancelled, "terminal cleanup",
		))

		// The production agent-failed path performs the terminal retirement
		// before it drops the failure. The later call represents a buffered
		// eligible provider event from that same execution.
		svc.handleAgentFailed(context.Background(), watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "execution-1",
			ErrorMessage:     overloaded529,
		})
		armRetiredTransientPromptEvidence(svc)

		require.True(t, svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "execution-1",
			PromptGeneration: 7,
			ErrorMessage:     overloaded529,
		}), "a late eligible failure must be consumed after terminal retirement")
		require.Empty(t, retryingMessages(t, taskSvc))
		_, retryArmed := svc.transientRetries.Load("s1")
		require.False(t, retryArmed)
	})
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
func TestHandleTransientFailure_NewPromptAfterRetirementStartsAtAttemptOne(t *testing.T) {
	svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)

	svc.resetTransientRetryWithContext(context.Background(), "s1", true)
	svc.rememberTurnPrompt("s1", "new prompt", "", false, nil)
	armTransientPromptEvidence(svc)

	require.True(t, svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     overloaded529,
	}))

	notices := retryingMessages(t, taskSvc)
	require.Len(t, notices, 1)
	require.Equal(t, float64(1), notices[0].Metadata["attempt"])
	entryValue, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	require.Equal(t, 1, entryValue.(*transientRetryEntry).attempt)
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
func TestHandleTransientFailure_RetiredFenceWaitsForNewPromptIdentity(t *testing.T) {
	svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	svc.rememberTurnPrompt("s1", "old prompt", "", false, nil)
	svc.scheduleTransientRetry("t1", "s1", "execution-old", 1, time.Hour)
	require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))

	// A new initial prompt has a generation before its replacement execution is
	// known. It must not open the retired lifecycle during that interval.
	svc.rememberTurnPrompt("s1", "new prompt", "", false, nil)
	svc.beginInitialPromptAttempt("s1", false)
	require.True(t, svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-old",
		PromptGeneration: 1,
		ErrorMessage:     overloaded529,
	}), "a late failure while the replacement execution is unbound must be consumed")
	require.Empty(t, retryingMessages(t, taskSvc))

	// Binding the replacement identity is the admission boundary that opens the
	// fence. The same late event must then fail closed on identity mismatch.
	svc.bindPromptAttempt("s1", "execution-new", 1)
	require.False(t, svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-old",
		PromptGeneration: 1,
		ErrorMessage:     overloaded529,
	}))
	require.Empty(t, retryingMessages(t, taskSvc))

	require.True(t, svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-new",
		PromptGeneration: 1,
		ErrorMessage:     overloaded529,
	}))
	notices := retryingMessages(t, taskSvc)
	require.Len(t, notices, 1)
	require.Equal(t, float64(1), notices[0].Metadata["attempt"])
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
func TestHandleTransientFailure_ConcurrentFailuresShareNoticeAndTimer(t *testing.T) {
	store := &blockingTransientRetryMessageService{
		transientRetryMessageServiceStub: &transientRetryMessageServiceStub{
			messages: []*models.Message{
				transientRetryTestMessage(
					"retry-1",
					"t1",
					"s1",
					time.Now().UTC(),
					map[string]interface{}{"retrying": true},
				),
			},
		},
		updateStarted: make(chan struct{}),
		releaseUpdate: make(chan struct{}),
	}
	svc, _ := newTransientTestService(t)
	svc.transientRetryMessages = store
	t.Cleanup(svc.cancelAllTransientRetries)
	svc.rememberTurnPrompt("s1", "retry me", "", false, nil)
	armTransientPromptEvidence(svc)
	data := watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     overloaded529,
	}

	firstDone := make(chan bool, 1)
	go func() { firstDone <- svc.handleTransientFailure(context.Background(), data) }()
	select {
	case <-store.updateStarted:
	case <-time.After(time.Second):
		t.Fatal("first transient failure did not enter the notice update")
	}

	secondStarted := make(chan struct{})
	secondDone := make(chan bool, 1)
	go func() {
		close(secondStarted)
		secondDone <- svc.handleTransientFailure(context.Background(), data)
	}()
	<-secondStarted
	// The first update holds the per-session notice mutex. Releasing its
	// deterministic store barrier lets the second failure take the next
	// attempt under the same mutex.
	close(store.releaseUpdate)

	require.True(t, <-firstDone)
	require.True(t, <-secondDone)
	notices := transientRetryNotices(store.messages, "t1", "s1")
	require.Len(t, notices, 1)
	require.Equal(t, 2, notices[0].Metadata["attempt"])
	entryValue, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	require.Equal(t, 2, entryValue.(*transientRetryEntry).attempt)
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
