package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// recordingEventBus records published events for assertions.
type recordingEventBus struct {
	events []recordedEvent
}

type recordedEvent struct {
	subject string
	event   *bus.Event
}

// taskServiceStateRepository exercises runtime task-state reconciliation
// through the same task-service publication path used in production.
type taskServiceStateRepository struct {
	repo    *sqliterepo.Repository
	service *taskservice.Service
}

func newTaskServiceStateRepository(
	repo *sqliterepo.Repository,
	eventBus bus.EventBus,
) *taskServiceStateRepository {
	service := taskservice.NewService(taskservice.Repos{
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
	return &taskServiceStateRepository{repo: repo, service: service}
}

func (r *taskServiceStateRepository) GetTask(ctx context.Context, taskID string) (*v1.Task, error) {
	task, err := r.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return task.ToAPI(), nil
}

func (r *taskServiceStateRepository) UpdateTaskState(
	ctx context.Context,
	taskID string,
	state v1.TaskState,
) error {
	_, err := r.service.UpdateTaskState(ctx, taskID, state)
	return err
}

func (r *taskServiceStateRepository) UpdateTaskStateIfCurrentIn(
	ctx context.Context,
	taskID string,
	state v1.TaskState,
	allowed []v1.TaskState,
) (bool, error) {
	return r.service.UpdateTaskStateIfCurrentIn(ctx, taskID, state, allowed)
}

func (r *taskServiceStateRepository) UpdateTaskStateIfNotArchived(
	ctx context.Context,
	taskID string,
	state v1.TaskState,
) (bool, error) {
	return r.service.UpdateTaskStateIfNotArchived(ctx, taskID, state)
}

func (r *taskServiceStateRepository) UpdateTaskStateIfSessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
) (bool, error) {
	return r.service.UpdateTaskStateIfSessionState(
		ctx,
		taskID,
		sessionID,
		expectedSessionState,
		state,
	)
}

type recordingClarificationCanceller struct {
	sessions              []string
	expiredSessions       []string
	expireContextDeadline []bool
	expireErr             error
}

func (c *recordingClarificationCanceller) DetachSessionAndNotify(_ context.Context, sessionID string) (int, error) {
	c.sessions = append(c.sessions, sessionID)
	return 1, nil
}

func (c *recordingClarificationCanceller) ExpireSessionAndNotify(ctx context.Context, sessionID string) (int, error) {
	c.expiredSessions = append(c.expiredSessions, sessionID)
	_, hasDeadline := ctx.Deadline()
	c.expireContextDeadline = append(c.expireContextDeadline, hasDeadline)
	return 1, c.expireErr
}

type listTaskSessionsErrorRepo struct {
	sessionExecutorStore
}

func (r listTaskSessionsErrorRepo) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return nil, errors.New("list task sessions failed")
}

type failSetSessionMetadataRepo struct {
	repoStore
}

type failSessionStateUpdateRepo struct {
	repoStore
	err error
}

func (r failSessionStateUpdateRepo) UpdateTaskSessionState(
	context.Context,
	string,
	models.TaskSessionState,
	string,
) error {
	return r.err
}

// UpdateTaskSessionStateIfCurrent must also fail: repoStore now declares this
// method, so the embedded repoStore field promotes it and
// persistStrictTaskSessionState's conditionalTaskSessionStateUpdater
// assertion succeeds, routing state transitions through this narrow CAS
// instead of the plain UpdateTaskSessionState this double otherwise
// overrides. Without this override, the transition would silently succeed
// against the embedded real repo instead of surfacing the injected failure.
func (r failSessionStateUpdateRepo) UpdateTaskSessionStateIfCurrent(
	context.Context,
	string,
	models.TaskSessionState,
	models.TaskSessionState,
	string,
) (bool, time.Time, error) {
	return false, time.Time{}, r.err
}

type failSetBaselineRepo struct {
	repoStore
}

func (r failSetBaselineRepo) SetSessionMetadataKeyIfAbsent(
	context.Context,
	string,
	string,
	interface{},
) (bool, error) {
	return false, errors.New("baseline write failed")
}

type concurrentBaselineRepo struct {
	repoStore
	mu         sync.Mutex
	loads      int
	bothLoaded chan struct{}
}

func (r *concurrentBaselineRepo) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	session, err := r.repoStore.GetTaskSession(ctx, id)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.loads++
	if r.loads == 2 {
		close(r.bothLoaded)
	}
	r.mu.Unlock()
	return session, nil
}

func (r *concurrentBaselineRepo) SetSessionMetadataKey(
	ctx context.Context,
	sessionID string,
	key string,
	value interface{},
) error {
	if key == models.SessionMetaKeyACPConfigBaseline {
		<-r.bothLoaded
	}
	return r.repoStore.SetSessionMetadataKey(ctx, sessionID, key, value)
}

func (r *concurrentBaselineRepo) SetSessionMetadataKeyIfAbsent(
	ctx context.Context,
	sessionID string,
	key string,
	value interface{},
) (bool, error) {
	<-r.bothLoaded
	r.mu.Lock()
	defer r.mu.Unlock()
	session, err := r.repoStore.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if _, ok := models.LoadSessionACPConfigBaseline(session.Metadata); ok {
		return false, nil
	}
	if err := r.repoStore.SetSessionMetadataKey(ctx, sessionID, key, value); err != nil {
		return false, err
	}
	return true, nil
}

type mockAutomationRunService struct {
	succeededTaskIDs  []string
	failedTaskIDs     []string
	failedErrorByTask map[string]string
}

type serviceBackedMessageCreator struct {
	mockMessageCreator
	svc *taskservice.Service
}

func (m *serviceBackedMessageCreator) CreateSessionMessage(
	ctx context.Context,
	taskID, content, sessionID, messageType, turnID string,
	metadata map[string]interface{},
	requestsInput bool,
) error {
	_, err := m.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "agent",
		Type:          messageType,
		Metadata:      metadata,
		RequestsInput: requestsInput,
	})
	return err
}

func (m *serviceBackedMessageCreator) UpdateToolCallMessage(
	ctx context.Context,
	taskID, toolCallID, parentToolCallID, status, result, agentSessionID, title, turnID, msgType string,
	normalized *streams.NormalizedPayload,
) error {
	return m.svc.UpdateToolCallMessageWithCreate(
		ctx,
		agentSessionID,
		toolCallID,
		parentToolCallID,
		status,
		result,
		title,
		normalized,
		taskID,
		turnID,
		msgType,
	)
}

func newServiceBackedMessageCreator(repo *sqliterepo.Repository) *serviceBackedMessageCreator {
	services := taskservice.NewService(taskservice.Repos{
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
	}, bus.NewMemoryEventBus(testLogger()), testLogger(), taskservice.RepositoryDiscoveryConfig{})
	return &serviceBackedMessageCreator{svc: services}
}

func (m *mockAutomationRunService) GetAutomation(context.Context, string) (*automation.Automation, error) {
	return nil, nil
}

func (m *mockAutomationRunService) RecordRun(context.Context, *automation.AutomationRun) error {
	return nil
}

func (m *mockAutomationRunService) MarkRunFailedByTaskID(_ context.Context, taskID, errMsg string) error {
	m.failedTaskIDs = append(m.failedTaskIDs, taskID)
	if m.failedErrorByTask == nil {
		m.failedErrorByTask = make(map[string]string)
	}
	m.failedErrorByTask[taskID] = errMsg
	return nil
}

func (m *mockAutomationRunService) MarkRunSucceededByTaskID(_ context.Context, taskID string) error {
	m.succeededTaskIDs = append(m.succeededTaskIDs, taskID)
	return nil
}

func (r failSetSessionMetadataRepo) SetSessionMetadataKey(
	context.Context,
	string,
	string,
	interface{},
) error {
	return errors.New("set session metadata failed")
}

func (b *recordingEventBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	b.events = append(b.events, recordedEvent{subject: subject, event: event})
	return nil
}
func (b *recordingEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (b *recordingEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (b *recordingEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (b *recordingEventBus) Close()            {}
func (b *recordingEventBus) IsConnected() bool { return true }

func TestUpdateTaskSessionStatePublishesPersistedUpdatedAt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", false)

	require.Len(t, eb.events, 1)
	require.Equal(t, events.TaskSessionStateChanged, eb.events[0].subject)
	data, ok := eb.events[0].event.Data.(map[string]interface{})
	require.True(t, ok)
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, session.UpdatedAt.UTC().Format(time.RFC3339Nano), data["updated_at"])
}

func TestUpdateTaskSessionStateExpiresClarificationsOnTerminalTransition(t *testing.T) {
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
			canceller := &recordingClarificationCanceller{}
			svc.clarificationCanceller = canceller

			svc.updateTaskSessionState(ctx, "t1", "s1", terminalState, "", false)

			require.Equal(t, []string{"s1"}, canceller.expiredSessions)
			require.Equal(t, []bool{true}, canceller.expireContextDeadline)
		})
	}
}

func TestExpireClarificationWaitersReturnsPersistenceError(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	wantErr := errors.New("expiry write failed")
	svc.clarificationCanceller = &recordingClarificationCanceller{expireErr: wantErr}

	err := svc.expireClarificationWaiters(context.Background(), "s1")

	require.ErrorIs(t, err, wantErr)
}

func TestUpdateTaskSessionState_EnabledClaudePublishesSettledBackground(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-state-claude", "session-state-claude", "step1")
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	enableClaudeBackgroundPromptHandoffForTest(t, svc)
	setSessionAgentNameForTest(t, svc, "session-state-claude", "claude-acp")
	advertisePromptQueueingForTest(t, svc, "session-state-claude")
	svc.eventBus = eb
	svc.registerBackgroundTask("session-state-claude", "background-1")
	svc.markForegroundIdle("session-state-claude")

	svc.updateTaskSessionState(
		ctx,
		"task-state-claude",
		"session-state-claude",
		models.TaskSessionStateWaitingForInput,
		"",
		false,
	)

	require.Len(t, eb.events, 1)
	require.Equal(t, events.TaskSessionStateChanged, eb.events[0].subject)
	data, ok := eb.events[0].event.Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, string(v1.ForegroundActivityBackground), data["foreground_activity"])
}

func TestTransitionTaskSessionStateReportsAcceptedWrite(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb
	canceller := &recordingClarificationCanceller{}
	svc.clarificationCanceller = canceller

	changed, finalState, err := svc.transitionTaskSessionState(
		ctx,
		"t1",
		"s1",
		models.TaskSessionStateCancelled,
		"coordinator stop",
		nil,
	)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, models.TaskSessionStateCancelled, finalState)
	require.Len(t, eb.events, 1)
	require.Equal(t, events.TaskSessionStateChanged, eb.events[0].subject)
	require.Equal(t, []string{"s1"}, canceller.expiredSessions)
	require.Equal(t, []bool{true}, canceller.expireContextDeadline)
}

func TestTransitionTaskSessionStatePublishesMetadataWrittenByHook(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb
	errorValue := models.LastAgentError{
		Message:    "The selected base branch is not available.",
		Code:       "base_branch_missing",
		OccurredAt: time.Date(2026, 8, 19, 23, 10, 41, 0, time.UTC),
		StampValue: "launch-error-stamp",
	}

	changed, _, err := svc.transitionTaskSessionState(
		ctx,
		"t1",
		"s1",
		models.TaskSessionStateFailed,
		errorValue.Message,
		func() {
			require.NoError(t, repo.SetSessionMetadataKey(
				ctx, "s1", models.SessionMetaKeyLastAgentError, errorValue,
			))
		},
	)

	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, eb.events, 1)
	data, ok := eb.events[0].event.Data.(map[string]interface{})
	require.True(t, ok)
	metadata, ok := data["session_metadata"].(map[string]interface{})
	require.True(t, ok)
	lastError, ok := metadata[models.SessionMetaKeyLastAgentError].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "base_branch_missing", lastError["code"])
}

func TestTransitionTaskSessionStateReportsPersistenceFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	writeFailure := errors.New("database is read-only")
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.repo = failSessionStateUpdateRepo{repoStore: repo, err: writeFailure}
	svc.eventBus = eb

	changed, finalState, err := svc.transitionTaskSessionState(
		ctx,
		"t1",
		"s1",
		models.TaskSessionStateCancelled,
		"coordinator stop",
		nil,
	)

	require.ErrorIs(t, err, writeFailure)
	require.False(t, changed)
	require.Equal(t, models.TaskSessionStateRunning, finalState)
	require.Empty(t, eb.events)
}

func TestHandleSessionModeEvent(t *testing.T) {
	t.Run("publishes plan mode", func(t *testing.T) {
		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb}

		svc.handleSessionModeEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data:      &lifecycle.AgentStreamEventData{CurrentModeID: "plan"},
		})

		require.Len(t, eb.events, 1)
	})

	t.Run("publishes default mode without available modes (mode exit)", func(t *testing.T) {
		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb}

		svc.handleSessionModeEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data:      &lifecycle.AgentStreamEventData{CurrentModeID: "default"},
		})

		require.Len(t, eb.events, 1)
	})

	t.Run("publishes default mode with available modes (initial state)", func(t *testing.T) {
		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb}

		svc.handleSessionModeEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data: &lifecycle.AgentStreamEventData{
				CurrentModeID: "default",
				AvailableModes: []streams.SessionModeInfo{
					{ID: "default", Name: "Default"},
					{ID: "plan", Name: "Plan"},
				},
			},
		})

		require.Len(t, eb.events, 1)
	})

	t.Run("publishes empty mode (mode exit)", func(t *testing.T) {
		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb}

		svc.handleSessionModeEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data:      &lifecycle.AgentStreamEventData{CurrentModeID: ""},
		})

		require.Len(t, eb.events, 1)
	})

	t.Run("skips when session ID is empty", func(t *testing.T) {
		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb}

		svc.handleSessionModeEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "",
			Data:      &lifecycle.AgentStreamEventData{CurrentModeID: "plan"},
		})

		require.Empty(t, eb.events)
	})

	// Regression for issue #1183: a non-empty mode is persisted to session
	// metadata (so it survives backend restart / SSR) without clobbering other
	// keys such as plan_mode.
	t.Run("persists non-empty mode without clobbering plan_mode", func(t *testing.T) {
		ctx := context.Background()
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		require.NoError(t, repo.UpdateSessionMetadata(ctx, "s1", map[string]interface{}{"plan_mode": true}))

		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb, repo: repo}

		svc.handleSessionModeEvent(ctx, &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data:      &lifecycle.AgentStreamEventData{CurrentModeID: "acceptEdits"},
		})

		updated, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		require.Equal(t, "acceptEdits", updated.Metadata[models.SessionMetaKeySessionMode],
			"session mode must be persisted to metadata")
		pm, _ := updated.Metadata["plan_mode"].(bool)
		require.True(t, pm, "plan_mode and other metadata keys must be preserved")
	})

	// An empty CurrentModeID (agent left a special mode) must not overwrite a
	// previously-stored sticky mode.
	t.Run("empty mode does not overwrite stored mode", func(t *testing.T) {
		ctx := context.Background()
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		require.NoError(t, repo.UpdateSessionMetadata(ctx, "s1",
			map[string]interface{}{models.SessionMetaKeySessionMode: "acceptEdits"}))

		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb, repo: repo}

		svc.handleSessionModeEvent(ctx, &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data:      &lifecycle.AgentStreamEventData{CurrentModeID: ""},
		})

		updated, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		require.Equal(t, "acceptEdits", updated.Metadata[models.SessionMetaKeySessionMode])
	})
}

func TestHandleSessionInfoEvent_PersistsACPDebugInfo(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	svc.handleSessionInfoEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			ACPSessionID:     "acp-1",
			SessionTitle:     "List files",
			SessionUpdatedAt: "2026-06-13T19:37:46Z",
			SessionMeta: map[string]any{
				"cursor": map[string]any{"requestId": "req-1"},
			},
		},
	})

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	acp, ok := updated.Metadata["acp"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "acp-1", acp["session_id"])
	require.Equal(t, "List files", acp["title"])
	require.Equal(t, "2026-06-13T19:37:46Z", acp["updated_at"])
	meta, ok := acp["meta"].(map[string]interface{})
	require.True(t, ok)
	cursor, ok := meta["cursor"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "req-1", cursor["requestId"])

	require.Len(t, eb.events, 1)
	require.Equal(t, events.BuildSessionInfoSubject("s1"), eb.events[0].subject)
	eventPayload, ok := eb.events[0].event.Data.(lifecycle.SessionInfoEventPayload)
	require.True(t, ok)
	require.Equal(t, "t1", eventPayload.TaskID)
	require.Equal(t, "s1", eventPayload.SessionID)
	require.Equal(t, "acp-1", eventPayload.ACPSessionID)
	require.Equal(t, "List files", eventPayload.SessionTitle)
	require.Equal(t, "2026-06-13T19:37:46Z", eventPayload.SessionUpdatedAt)
	require.Equal(t, meta, eventPayload.SessionMeta)
}

func TestHandleSessionInfoEvent_PreservesACPDebugInfoOnSparseUpdate(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", "acp", map[string]any{
		"session_id": "acp-1",
		"title":      "List files",
		"updated_at": "2026-06-13T19:37:46Z",
		"meta":       map[string]any{"cursor": map[string]any{"requestId": "req-1"}},
	}))
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	svc.handleSessionInfoEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			SessionUpdatedAt: "2026-06-13T19:40:00Z",
		},
	})

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	acp, ok := updated.Metadata["acp"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "acp-1", acp["session_id"])
	require.Equal(t, "List files", acp["title"])
	require.Equal(t, "2026-06-13T19:40:00Z", acp["updated_at"])
	meta, ok := acp["meta"].(map[string]interface{})
	require.True(t, ok)
	cursor, ok := meta["cursor"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "req-1", cursor["requestId"])

	require.Len(t, eb.events, 1)
	eventPayload, ok := eb.events[0].event.Data.(lifecycle.SessionInfoEventPayload)
	require.True(t, ok)
	require.Equal(t, "acp-1", eventPayload.ACPSessionID)
	require.Equal(t, "List files", eventPayload.SessionTitle)
	require.Equal(t, "2026-06-13T19:40:00Z", eventPayload.SessionUpdatedAt)
}

func TestHandleSessionInfoEvent_SkipsWhenSessionReadFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	svc.handleSessionInfoEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "missing-session",
		Data: &lifecycle.AgentStreamEventData{
			ACPSessionID:     "acp-1",
			SessionUpdatedAt: "2026-06-13T19:40:00Z",
		},
	})

	require.Empty(t, eb.events)
}

func TestHandleSessionInfoEvent_SkipsPublishWhenSessionWriteFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.repo = failSetSessionMetadataRepo{repoStore: repo}
	svc.eventBus = eb

	svc.handleSessionInfoEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			ACPSessionID:     "acp-1",
			SessionUpdatedAt: "2026-06-13T19:40:00Z",
		},
	})

	require.Empty(t, eb.events)
}

func TestPersistTurnPromptMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	turn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)
	configSnapshot := map[string]interface{}{
		"model": "gpt-5.4",
		"config_options": []interface{}{
			map[string]interface{}{
				"id": "reasoning_effort", "name": "Reasoning effort",
				"value": "high", "value_name": "High",
			},
		},
		"config_baseline": map[string]interface{}{"reasoning_effort": "medium"},
	}
	turn.Metadata = map[string]interface{}{
		models.TurnMetaKeyRuntimeConfigSnapshot: configSnapshot,
	}
	require.NoError(t, svc.turnService.UpdateTurn(ctx, turn))

	svc.persistTurnPromptMetadata(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		AgentID:   "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Usage: &streams.PromptUsage{
				InputTokens:                  10,
				OutputTokens:                 20,
				OutputTokensPresent:          true,
				CachedReadTokens:             3,
				CachedWriteTokens:            4,
				ThoughtTokens:                5,
				TotalTokens:                  42,
				ProviderReportedCostSubcents: 123,
				Estimated:                    true,
			},
		},
	}, &models.TaskSession{
		AgentProfileSnapshot: map[string]interface{}{
			"model":      "gpt-5.5",
			"agent_name": "codex-acp",
		},
	})

	updated, err := repo.GetTurn(ctx, turn.ID)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5", updated.Metadata["model"])
	require.Equal(t, configSnapshot, updated.Metadata[models.TurnMetaKeyRuntimeConfigSnapshot])
	require.Equal(t, "codex-acp", updated.Metadata["agent_type"])
	require.Equal(t, "exec-1", updated.Metadata["agent_id"])
	usage, ok := updated.Metadata["prompt_usage"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, float64(42), usage["total_tokens"])
	require.Equal(t, float64(123), usage["provider_reported_cost_subcents"])
	require.Equal(t, true, usage["output_tokens_present"])
	require.Equal(t, true, usage["estimated"])
}

// TestToolEventsWakeSessionAndTaskTogether locks in the fix for the
// REVIEW + RUNNING split: when an out-of-turn tool event (e.g. a Monitor
// watcher firing after on_turn_complete moved the task to REVIEW) wakes
// the session from WAITING_FOR_INPUT, the task must flip to IN_PROGRESS
// in lockstep instead of being left at REVIEW.
func TestToolEventsWakeSessionAndTaskTogether(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name       string
		activeTurn bool
		fire       func(*Service)
	}{
		{
			name: "tool_call event",
			fire: func(svc *Service) {
				svc.handleToolCallEvent(ctx, &lifecycle.AgentStreamEventPayload{
					TaskID:    "t1",
					SessionID: "s1",
					Data: &lifecycle.AgentStreamEventData{
						ToolCallID: "tc1",
						ToolStatus: "running",
					},
				})
			},
		},
		{
			name:       "tool_update completion event for active async turn",
			activeTurn: true,
			fire: func(svc *Service) {
				svc.handleToolUpdateEvent(ctx, &lifecycle.AgentStreamEventPayload{
					TaskID:    "t1",
					SessionID: "s1",
					Data: &lifecycle.AgentStreamEventData{
						ToolCallID: "tc1",
						ToolStatus: agentEventComplete,
					},
				})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")

			// Simulate post-on_turn_complete state: session WAITING, task REVIEW.
			session, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			session.State = models.TaskSessionStateWaitingForInput
			require.NoError(t, repo.UpdateTaskSession(ctx, session))

			taskRepo := newMockTaskRepo()
			svc := createTestService(repo, newMockStepGetter(), taskRepo)
			svc.messageCreator = &mockMessageCreator{}
			if tc.activeTurn {
				svc.turnService = &repoTurnService{repo: repo}
				_, err := svc.turnService.StartTurn(ctx, "s1")
				require.NoError(t, err)
			}

			tc.fire(svc)

			updatedSession, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateRunning, updatedSession.State,
				"session should be woken to RUNNING")
			require.Equal(t, v1.TaskStateInProgress, taskRepo.updatedStates["t1"],
				"task must move to IN_PROGRESS in lockstep — leaving it at REVIEW is the bug")
		})

		t.Run(tc.name+" does not clobber terminal session", func(t *testing.T) {
			// Inverse edge case: a buffered tool event arriving after the
			// session is already terminal must NOT silently flip tasks.state
			// to IN_PROGRESS while the session itself stays terminal.
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")

			session, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			session.State = models.TaskSessionStateCancelled
			require.NoError(t, repo.UpdateTaskSession(ctx, session))

			taskRepo := newMockTaskRepo()
			svc := createTestService(repo, newMockStepGetter(), taskRepo)
			svc.messageCreator = &mockMessageCreator{}
			if tc.activeTurn {
				svc.turnService = &repoTurnService{repo: repo}
				_, err := svc.turnService.StartTurn(ctx, "s1")
				require.NoError(t, err)
			}

			tc.fire(svc)

			updatedSession, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateCancelled, updatedSession.State,
				"terminal session must not be revived by a stale tool event")
			_, taskWritten := taskRepo.updatedStates["t1"]
			require.False(t, taskWritten,
				"task state must not be clobbered when session is terminal")
		})
	}
}

func TestToolUpdateFromCompletedExecutionDoesNotWakeWaitingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.messageCreator = &mockMessageCreator{}

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
	})

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, updated.State)
	require.Equal(t, v1.TaskStateReview, taskRepo.updatedStates["t1"])

	svc.handleToolUpdateEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			ToolCallID: "tc1",
			ToolStatus: agentEventComplete,
		},
	})

	updated, err = repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, updated.State,
		"late tool events from the completed execution must not revive the session")
	require.Equal(t, 1, taskRepo.stateWrites["t1"],
		"late completed-execution tool event must not move the task back to IN_PROGRESS")
}

func TestLateTerminalToolUpdateDoesNotCreateTurnOrWakeWaitingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = newServiceBackedMessageCreator(repo)

	svc.handleToolUpdateEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "task1",
		SessionID:   "session1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "tool-1",
			ToolStatus: agentEventComplete,
		},
	})

	messages, err := repo.ListMessages(ctx, "session1")
	require.NoError(t, err)
	require.Empty(t, messages,
		"late terminal status must not create a fallback message when the original is absent")
	require.Zero(t, openTurnCount(t, repo, "session1"),
		"late terminal status must not create a phantom turn")
	updated, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, updated.State,
		"late terminal status must not wake the settled session")
	require.Zero(t, taskRepo.stateWrites["task1"],
		"late terminal status must not move the task back to IN_PROGRESS")
}

func TestStatuslessToolUpdateDoesNotCreateTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	svc.handleToolUpdateEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "task1",
		SessionID: "session1",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "tool-1",
		},
	})

	require.Zero(t, messages.toolUpdateWrites)
	require.Zero(t, openTurnCount(t, repo, "session1"))
}

func TestResumedSessionStatusDoesNotCreateTurnForWaitingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = newServiceBackedMessageCreator(repo)

	svc.handleSessionStatusEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "task1",
		SessionID: "session1",
		Data: &lifecycle.AgentStreamEventData{
			Type:          "session_status",
			SessionStatus: streams.SessionStatusResumed,
		},
	})

	require.Zero(t, openTurnCount(t, repo, "session1"),
		"resume status metadata must not create a phantom active turn")
	messages, err := repo.ListMessages(ctx, "session1")
	require.NoError(t, err)
	require.Empty(t, messages, "resume status without an active turn must not create a message")
}

func TestResumedSessionStatusUsesExistingActiveTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")

	now := time.Now().UTC()
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn1",
		TaskSessionID: "session1",
		TaskID:        "task1",
		StartedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = newServiceBackedMessageCreator(repo)

	svc.handleSessionStatusEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "task1",
		SessionID: "session1",
		Data: &lifecycle.AgentStreamEventData{
			Type:          "session_status",
			SessionStatus: streams.SessionStatusResumed,
		},
	})

	require.Equal(t, 1, openTurnCount(t, repo, "session1"))
	messages, err := repo.ListMessages(ctx, "session1")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "turn1", messages[0].TurnID)
	require.Equal(t, "Session resumed", messages[0].Content)
}

func TestToolUpdateFromCompletedExecutionDoesNotCreateMessage(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	svc.markExecutionCompleted("s1", "exec-1")

	svc.handleToolUpdateEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			ToolCallID: "tc1",
			ToolStatus: agentEventComplete,
		},
	})

	require.Zero(t, messages.toolUpdateWrites,
		"stale completed-execution tool updates must be dropped before message side effects")
}

func TestToolCallFromCompletedExecutionDoesNotCreateMessage(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	svc.markExecutionCompleted("s1", "exec-1")

	svc.handleToolCallEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			ToolCallID: "tc1",
			ToolStatus: "running",
		},
	})

	require.Zero(t, messages.toolCallWrites,
		"stale completed-execution tool calls must be dropped before message side effects")
}

type blockingToolCallMessageCreator struct {
	mockMessageCreator
	entered chan struct{}
	release chan struct{}
}

func (m *blockingToolCallMessageCreator) CreateToolCallMessage(
	context.Context,
	string, string, string, string, string, string, string, *streams.NormalizedPayload,
) error {
	close(m.entered)
	<-m.release
	m.toolCallWrites++
	return nil
}

func TestAgentStreamEventWaitsForCancellationGuard(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	creator := &blockingToolCallMessageCreator{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = creator

	lock, releaseGuard := svc.acquireCancelInFlightGuard("s1")
	lock.Lock()

	eventDone := make(chan struct{})
	go func() {
		svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
			TaskID:      "t1",
			SessionID:   "s1",
			ExecutionID: "exec-1",
			Data: &lifecycle.AgentStreamEventData{
				Type:       agentEventToolCall,
				ToolCallID: "tool-1",
				ToolStatus: "running",
			},
		})
		close(eventDone)
	}()

	select {
	case <-creator.entered:
		t.Fatal("stream side effect bypassed the cancellation guard")
	case <-time.After(100 * time.Millisecond):
	}

	lock.Unlock()
	releaseGuard()

	select {
	case <-creator.entered:
	case <-time.After(time.Second):
		t.Fatal("stream handler did not proceed after cancellation guard release")
	}
	close(creator.release)
	select {
	case <-eventDone:
	case <-time.After(time.Second):
		t.Fatal("stream handler did not finish")
	}
	require.Equal(t, 1, creator.toolCallWrites)
}

func TestToolCallStreamFromCompletedExecutionDoesNotSaveAgentText(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", State: v1.TaskStateInProgress}
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	svc.markExecutionCompleted("s1", "exec-1")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			Text:       "stale text from completed execution",
			ToolCallID: "tc1",
			ToolStatus: "running",
		},
	})

	require.Zero(t, messages.agentMessageWrites,
		"top-level tool_call guard must run before saveAgentTextIfPresent")
	require.Zero(t, messages.toolCallWrites,
		"top-level tool_call guard must not fall through to handleToolCallEvent")
}

func TestCompleteStreamFromCompletedExecutionFlushesAgentText(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.turnService = &repoTurnService{repo: repo}
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	firstTurn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)
	svc.markExecutionCompleted("s1", "exec-1")
	// StopExecution can emit agent.stopped after agent.completed but before this
	// buffered terminal stream. The stopped marker must not downgrade permission
	// to flush the successful execution's final text and metadata.
	svc.markExecutionFailed("s1", "exec-1")
	svc.completeTurnForSession(ctx, "s1")
	nextTurn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete,
			Text: "final flushed text",
		},
	})

	require.Equal(t, 1, messages.agentMessageWrites,
		"final complete streams must flush text even if agent.completed arrived first")
	require.Equal(t, firstTurn.ID, messages.agentMessages[0].turnID,
		"late terminal complete must write to the turn that belonged to the terminal execution")
	activeTurn, err := svc.turnService.GetActiveTurn(ctx, "s1")
	require.NoError(t, err)
	require.NotNil(t, activeTurn)
	require.Equal(t, nextTurn.ID, activeTurn.ID,
		"late terminal complete must not close a newer active turn")
	require.Zero(t, taskRepo.stateWrites["t1"],
		"final complete streams from terminal executions must not re-run task state reconciliation")
}

func TestCompleteStreamFromCompletedExecutionPersistsTerminalTurnMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.turnService = &repoTurnService{repo: repo}
	firstTurn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)
	svc.markExecutionCompleted("s1", "exec-1")
	svc.completeTurnForSession(ctx, "s1")
	nextTurn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		AgentID:     "agent-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:           agentEventComplete,
			CurrentModelID: "gpt-5.5",
			Usage: &streams.PromptUsage{
				InputTokens:                  10,
				OutputTokens:                 20,
				TotalTokens:                  42,
				ProviderReportedCostSubcents: 123,
			},
		},
	})

	terminalTurn, err := repo.GetTurn(ctx, firstTurn.ID)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5", terminalTurn.Metadata["model"])
	require.Equal(t, "agent-1", terminalTurn.Metadata["agent_id"])
	usage, ok := terminalTurn.Metadata["prompt_usage"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, float64(42), usage["total_tokens"])
	require.Equal(t, float64(123), usage["provider_reported_cost_subcents"])

	activeTurn, err := repo.GetTurn(ctx, nextTurn.ID)
	require.NoError(t, err)
	require.NotContains(t, activeTurn.Metadata, "prompt_usage",
		"late terminal complete metadata must not be written to a newer active turn")
}

func TestCompleteStreamFromCompletedExecutionPublishesTerminalTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb
	svc.turnService = &repoTurnService{repo: repo}
	firstTurn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)
	svc.markExecutionCompleted("s1", "exec-1")
	svc.completeTurnForSession(ctx, "s1")
	_, err = svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete,
		},
	})

	require.Len(t, eb.events, 1)
	require.Equal(t, events.AgentTurnMessageSaved, eb.events[0].subject)
	data, ok := eb.events[0].event.Data.(map[string]string)
	require.True(t, ok)
	require.Equal(t, firstTurn.ID, data["turn_id"],
		"late terminal complete publish must identify the completed turn")
}

func findPromptUsageEvent(t *testing.T, eb *recordingEventBus) *lifecycle.SessionPromptUsageEventPayload {
	t.Helper()
	for _, rec := range eb.events {
		if rec.subject != events.BuildSessionPromptUsageSubject("s1") {
			continue
		}
		payload, ok := rec.event.Data.(lifecycle.SessionPromptUsageEventPayload)
		require.True(t, ok, "session_prompt_usage.updated event carried an unexpected payload type")
		return &payload
	}
	return nil
}

// TestPublishPromptUsage_NonTerminalCompletionUsesReadyTurnSnapshot is the
// regression test for the ordinary (non-terminal) completion path: the agent
// stays running, so agent.ready — not agent.completed — closes the turn.
// handleAgentReady publishes agent.ready synchronously and closes the turn via
// completeTurnForSession before the complete-stream frame for the same
// completion is ever processed (see markReadyTurn's and
// handleCompleteStreamEvent's doc comments), so a live active-turn lookup at
// that point finds nothing and would store turn_id NULL.
func TestPublishPromptUsage_NonTerminalCompletionUsesReadyTurnSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.AgentProfileID = "session-agent"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(7)
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	eb := &recordingEventBus{}
	svc.eventBus = eb

	turn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)

	// The ready event closes the turn synchronously — before this test (or
	// production) ever gets to publish the complete-stream frame below.
	svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", PromptGeneration: 7,
	})

	active, err := svc.turnService.GetActiveTurn(ctx, "s1")
	require.NoError(t, err)
	require.Nil(t, active, "handleAgentReady must have already closed the turn")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:             agentEventComplete,
			PromptGeneration: 7,
			Usage:            &streams.PromptUsage{InputTokens: 10, OutputTokens: 5},
		},
	})

	usageEvent := findPromptUsageEvent(t, eb)
	require.NotNil(t, usageEvent, "expected a session_prompt_usage.updated event to be published")
	require.Equal(t, turn.ID, usageEvent.TurnID,
		"non-terminal completion must carry the turn id the ready event just closed, not NULL")
	require.Equal(t, "session-agent", usageEvent.AgentProfileID,
		"prompt usage must carry the stable profile recorded on the task session")
}

// TestPublishPromptUsage_CompletionPayloadTurnIDOwnsTurn verifies that a
// completion's durable turn identity wins over the currently active turn.
// This is the ordering that occurs when a queued successor starts before the
// completion frame crosses a NATS subject or instance boundary.
func TestPublishPromptUsage_CompletionPayloadTurnIDOwnsTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	svc := createTestService(repo, stepGetter, taskRepo)
	svc.turnService = &repoTurnService{repo: repo}
	eb := &recordingEventBus{}
	svc.eventBus = eb

	completedTurn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)
	svc.completeTurnForSession(ctx, "s1")
	successorTurn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:   agentEventComplete,
			TurnID: completedTurn.ID,
			Usage:  &streams.PromptUsage{InputTokens: 10, OutputTokens: 5},
		},
	})

	usageEvent := findPromptUsageEvent(t, eb)
	require.NotNil(t, usageEvent, "expected a session_prompt_usage.updated event to be published")
	require.Equal(t, completedTurn.ID, usageEvent.TurnID,
		"completion payload must retain its captured turn id")

	active, err := svc.turnService.GetActiveTurn(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, successorTurn.ID, active.ID,
		"a late completion must not close a successor turn")
}

// TestMarkReadyTurn_ZeroGenerationQueuesFIFO is the unit-level regression
// test for R2-F3: markReadyTurn's old promptGeneration==0 early return
// rested on the false premise that generation-less completions never
// publish agent.ready synchronously (see markReadyTurn's doc comment for the
// actual mechanism — handleCompleteEventMarkState calls MarkReady
// unconditionally). Two pending marks on the same (session, execution) with
// no generation to disambiguate them must not collide — they queue FIFO and
// come back out in the order they were recorded.
func TestMarkReadyTurn_ZeroGenerationQueuesFIFO(t *testing.T) {
	svc := &Service{}

	svc.markReadyTurn("s1", "exec-1", 0, "turn-A")
	svc.markReadyTurn("s1", "exec-1", 0, "turn-B")

	first, ok := svc.takeReadyTurnMark("s1", "exec-1", 0)
	require.True(t, ok, "expected a mark for the first pending completion")
	require.Equal(t, "turn-A", first, "FIFO: the first mark recorded must be the first consumed")

	second, ok := svc.takeReadyTurnMark("s1", "exec-1", 0)
	require.True(t, ok, "expected a mark for the second pending completion")
	require.Equal(t, "turn-B", second)

	_, ok = svc.takeReadyTurnMark("s1", "exec-1", 0)
	require.False(t, ok, "queue must be drained after both marks are consumed")

	// A different execution on the same session must not share the queue.
	svc.markReadyTurn("s1", "exec-2", 0, "turn-C")
	_, ok = svc.takeReadyTurnMark("s1", "exec-1", 0)
	require.False(t, ok, "exec-1's (already-drained) queue must not see exec-2's mark")
	turnC, ok := svc.takeReadyTurnMark("s1", "exec-2", 0)
	require.True(t, ok)
	require.Equal(t, "turn-C", turnC)
}

// TestPublishPromptUsage_NonTerminalCompletionZeroGenerationUsesReadyTurnSnapshot
// is the integration-level regression test for R2-F3: the generation-0
// (generation-less transport) sibling of
// TestPublishPromptUsage_NonTerminalCompletionUsesReadyTurnSnapshot. Before
// the fix, markReadyTurn no-op'd for promptGeneration==0, so this scenario
// always stored turn_id NULL even though agent.ready closes the turn here
// exactly as it does for a generation-tracked completion.
func TestPublishPromptUsage_NonTerminalCompletionZeroGenerationUsesReadyTurnSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	eb := &recordingEventBus{}
	svc.eventBus = eb

	turn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)

	svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", PromptGeneration: 0,
	})

	active, err := svc.turnService.GetActiveTurn(ctx, "s1")
	require.NoError(t, err)
	require.Nil(t, active, "handleAgentReady must have already closed the turn")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:  agentEventComplete,
			Usage: &streams.PromptUsage{InputTokens: 10, OutputTokens: 5},
		},
	})

	usageEvent := findPromptUsageEvent(t, eb)
	require.NotNil(t, usageEvent, "expected a session_prompt_usage.updated event to be published")
	require.Equal(t, turn.ID, usageEvent.TurnID,
		"generation-less non-terminal completion must carry the turn id the ready event just closed, not NULL")
}

// TestPublishPromptUsage_TerminalCompletionWithoutReadyMarkFallsBackToTerminalMarker
// covers the terminal (agent.completed) fallback path for a completion that
// never went through handleAgentReady's synchronous ready path at all — e.g.
// a crash mid-prompt, where finishPromptCompletion never runs and no ready
// mark is ever recorded. There, markTerminalExecution's live-turn snapshot
// (captured before completeTurnForSession closes it) is the only source of
// truth, so this test deliberately keeps that ordering.
func TestPublishPromptUsage_TerminalCompletionWithoutReadyMarkFallsBackToTerminalMarker(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	eb := &recordingEventBus{}
	svc.eventBus = eb

	turn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)
	svc.markExecutionCompleted("s1", "exec-1")
	svc.completeTurnForSession(ctx, "s1")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:  agentEventComplete,
			Usage: &streams.PromptUsage{InputTokens: 10, OutputTokens: 5},
		},
	})

	usageEvent := findPromptUsageEvent(t, eb)
	require.NotNil(t, usageEvent, "expected a session_prompt_usage.updated event to be published")
	require.Equal(t, turn.ID, usageEvent.TurnID,
		"terminal completion with no ready mark must fall back to the terminal marker's snapshotted turn id")
}

// TestPublishPromptUsage_TerminalCompletionAfterReadyUsesReadyTurnSnapshot is
// the R2-F1 regression test: the REALISTIC terminal-completion ordering,
// where agent.ready fires (and closes the turn) before the process later
// exits and agent.completed marks the execution terminal.
// finishPromptCompletion (lifecycle package) publishes agent.ready
// synchronously on EVERY successful prompt completion, independent of
// whether the process subsequently exits — so by the time
// markExecutionCompleted runs, the turn markTerminalExecution would snapshot
// via a live lookup is already closed (returns ""), and the correct turn id
// is only available from the ready mark handleAgentReady recorded. Before
// the R2-F1 fix, the terminal branch of handleCompleteStreamEvent read
// terminalMarker.turnID directly and never consulted that mark, so this
// scenario stored turn_id NULL.
func TestPublishPromptUsage_TerminalCompletionAfterReadyUsesReadyTurnSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(7)
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	eb := &recordingEventBus{}
	svc.eventBus = eb

	turn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)

	// The ready event closes the turn synchronously, exactly as it does for
	// the non-terminal case — this fires unconditionally on every successful
	// completion, terminal or not.
	svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", PromptGeneration: 7,
	})

	// The process exits sometime after: agent.completed marks the execution
	// terminal. Its live-turn snapshot finds nothing, since ready already
	// closed the turn above.
	svc.markExecutionCompleted("s1", "exec-1")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:             agentEventComplete,
			PromptGeneration: 7,
			Usage:            &streams.PromptUsage{InputTokens: 10, OutputTokens: 5},
		},
	})

	usageEvent := findPromptUsageEvent(t, eb)
	require.NotNil(t, usageEvent, "expected a session_prompt_usage.updated event to be published")
	require.Equal(t, turn.ID, usageEvent.TurnID,
		"terminal completion after a prior ready event must carry the turn id from the ready mark, not NULL")
}

func findAllPromptUsageEvents(t *testing.T, eb *recordingEventBus, sessionID string) []lifecycle.SessionPromptUsageEventPayload {
	t.Helper()
	var out []lifecycle.SessionPromptUsageEventPayload
	for _, rec := range eb.events {
		if rec.subject != events.BuildSessionPromptUsageSubject(sessionID) {
			continue
		}
		payload, ok := rec.event.Data.(lifecycle.SessionPromptUsageEventPayload)
		require.True(t, ok, "session_prompt_usage.updated event carried an unexpected payload type")
		out = append(out, payload)
	}
	return out
}

// TestUsageEventIDFor is the unit-level regression test for F2: the id must
// be a deterministic function of (session, execution, prompt generation),
// not a value minted fresh per call.
func TestUsageEventIDFor(t *testing.T) {
	a := usageEventIDFor("s1", "exec-1", 3)
	b := usageEventIDFor("s1", "exec-1", 3)
	require.Equal(t, a, b, "same (session, execution, prompt generation) must derive the same id")
	require.NotEmpty(t, a)

	require.NotEqual(t, a, usageEventIDFor("s1", "exec-1", 4),
		"a different prompt generation must derive a different id")
	require.NotEqual(t, a, usageEventIDFor("s1", "exec-2", 3),
		"a different execution must derive a different id")
	require.NotEqual(t, a, usageEventIDFor("s2", "exec-1", 3),
		"a different session must derive a different id")

	// promptGeneration==0 means the completion carries no generation
	// tracking at all (see claimPromptCompletion's early return in the
	// lifecycle package). Deriving a fixed key there would collide across
	// genuinely distinct turns on a generation-less transport and silently
	// under-count cost, so it must keep falling back to a random id.
	require.NotEqual(t, usageEventIDFor("s1", "exec-1", 0), usageEventIDFor("s1", "exec-1", 0),
		"promptGeneration==0 must not derive a stable id")
}

// TestPublishPromptUsage_RepublishedCompletionReusesUsageEventID is the
// integration-level regression test for F2: a random UsageEventID minted on
// every publish can only dedup literal redelivery of the identical
// *bus.Event, which neither event bus provides (see
// internal/events/bus/{memory,nats}.go). The real duplicate source is the
// SAME completion frame reaching publishPromptUsage twice — e.g. a
// reconnecting WS client replaying a buffered stream event, mirroring the
// "late terminal complete" scenario already covered elsewhere in this file
// (TestCompleteStreamFromCompletedExecutionSkipsDuplicateOfficeTeardown and
// neighbors). Both publishes must carry the SAME usage_event_id so the
// office cost subscriber's unique index — proven separately by
// TestPromptUsage_DuplicateUsageEventIDIsIdempotent in internal/office/service
// — rejects the second row as a duplicate rather than double-counting cost.
func TestPublishPromptUsage_RepublishedCompletionReusesUsageEventID(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	eb := &recordingEventBus{}
	svc.eventBus = eb

	_, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)
	svc.markExecutionCompleted("s1", "exec-1")
	svc.completeTurnForSession(ctx, "s1")

	frame := func() *lifecycle.AgentStreamEventPayload {
		return &lifecycle.AgentStreamEventPayload{
			TaskID:      "t1",
			SessionID:   "s1",
			ExecutionID: "exec-1",
			Data: &lifecycle.AgentStreamEventData{
				Type:             agentEventComplete,
				PromptGeneration: 3,
				Usage:            &streams.PromptUsage{InputTokens: 10, OutputTokens: 5},
			},
		}
	}

	// Simulates the same buffered stream frame being delivered twice — same
	// session, execution, and prompt generation both times.
	svc.handleAgentStreamEvent(ctx, frame())
	svc.handleAgentStreamEvent(ctx, frame())

	published := findAllPromptUsageEvents(t, eb, "s1")
	require.Len(t, published, 2, "expected both republished frames to publish a prompt-usage event")
	require.NotEmpty(t, published[0].UsageEventID)
	require.Equal(t, published[0].UsageEventID, published[1].UsageEventID,
		"republishing the same completion must reuse the same usage_event_id so the DB unique index catches the duplicate")
}

// TestPublishPromptUsage_RepublishedCompletionWithoutPromptGenerationMintsDistinctUsageEventIDs
// is the integration-level counterpart to TestUsageEventIDFor's unit-level
// promptGeneration==0 assertion, and the random-identifier-class counterpart
// to TestPublishPromptUsage_RepublishedCompletionReusesUsageEventID's
// deterministic-class coverage (docs/specs/task-cost-ledger/spec.md AC-22).
// A generation-less transport's "same" redelivered completion frame mints a
// fresh random usage_event_id on every publish, so the two publishes carry
// DIFFERENT ids: office/costs already documents that those rows are
// intentionally not deduplicated, and this proves that end to end rather
// than just at usageEventIDFor's own return value.
func TestPublishPromptUsage_RepublishedCompletionWithoutPromptGenerationMintsDistinctUsageEventIDs(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	eb := &recordingEventBus{}
	svc.eventBus = eb

	_, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)
	svc.markExecutionCompleted("s1", "exec-1")
	svc.completeTurnForSession(ctx, "s1")

	frame := func() *lifecycle.AgentStreamEventPayload {
		return &lifecycle.AgentStreamEventPayload{
			TaskID:      "t1",
			SessionID:   "s1",
			ExecutionID: "exec-1",
			Data: &lifecycle.AgentStreamEventData{
				Type:             agentEventComplete,
				PromptGeneration: 0,
				Usage:            &streams.PromptUsage{InputTokens: 10, OutputTokens: 5},
			},
		}
	}

	// Simulates the same buffered stream frame being delivered twice on a
	// generation-less transport - same session and execution, but no
	// prompt-generation tracking either time.
	svc.handleAgentStreamEvent(ctx, frame())
	svc.handleAgentStreamEvent(ctx, frame())

	published := findAllPromptUsageEvents(t, eb, "s1")
	require.Len(t, published, 2, "expected both republished frames to publish a prompt-usage event")
	require.NotEmpty(t, published[0].UsageEventID)
	require.NotEmpty(t, published[1].UsageEventID)
	require.NotEqual(t, published[0].UsageEventID, published[1].UsageEventID,
		"a generation-less transport must mint a fresh id per publish, so this redelivery is recorded as a new row and its rollup applied - not deduplicated")
}

func TestCompleteStreamFromCompletedExecutionSkipsDuplicateOfficeTeardown(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-office-terminal", "s-office-terminal", "exec-office-terminal")

	taskRepo := newMockTaskRepo()
	mgr := &mockAgentManager{}
	mgr.stopAgentArgs = append(mgr.stopAgentArgs, stopAgentCall{ExecutionID: "exec-office-terminal"})
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, mgr)
	svc.markExecutionCompleted("s-office-terminal", "exec-office-terminal")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t-office-terminal",
		SessionID:   "s-office-terminal",
		ExecutionID: "exec-office-terminal",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete,
			Data: map[string]interface{}{
				"stop_reason": "end_turn",
			},
		},
	})

	mgr.mu.Lock()
	stopCalls := append([]stopAgentCall(nil), mgr.stopAgentArgs...)
	mgr.mu.Unlock()
	require.Equal(t, []stopAgentCall{{ExecutionID: "exec-office-terminal"}}, stopCalls,
		"terminal complete streams must not re-run office StopAgent teardown")
	require.Zero(t, taskRepo.stateWrites["t-office-terminal"],
		"terminal complete streams must not re-run task state reconciliation")
}

func TestCompleteStreamFromCompletedExecutionSkipsDuplicateAutomationFinalize(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedAutomationRunSession(t, repo, "t-auto-terminal", "s-auto-terminal", "exec-auto-terminal")

	taskRepo := newMockTaskRepo()
	mgr := &mockAgentManager{}
	automationSvc := &mockAutomationRunService{succeededTaskIDs: []string{"t-auto-terminal"}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, mgr)
	svc.SetAutomationService(automationSvc)
	svc.markExecutionCompleted("s-auto-terminal", "exec-auto-terminal")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t-auto-terminal",
		SessionID:   "s-auto-terminal",
		ExecutionID: "exec-auto-terminal",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete,
			Data: map[string]interface{}{
				"stop_reason": "end_turn",
			},
		},
	})

	require.Equal(t, []string{"t-auto-terminal"}, automationSvc.succeededTaskIDs,
		"terminal complete streams must not re-run automation success finalization")
	require.Empty(t, automationSvc.failedTaskIDs)
	mgr.mu.Lock()
	stopCalls := append([]stopAgentCall(nil), mgr.stopAgentArgs...)
	mgr.mu.Unlock()
	require.Empty(t, stopCalls,
		"terminal complete streams must not re-run automation StopAgent teardown")
}

func TestMessageStreamFromCompletedExecutionDoesNotCreateTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.turnService = &repoTurnService{repo: repo}
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	svc.markExecutionCompleted("s1", "exec-1")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:      "message_streaming",
			MessageID: "msg-1",
			Text:      "stale text from completed execution",
		},
	})

	require.Zero(t, messages.agentStreamWrites,
		"stale completed-execution message streams must be dropped before message side effects")
	turn, err := svc.turnService.GetActiveTurn(ctx, "s1")
	require.NoError(t, err)
	require.Nil(t, turn, "stale stream events must not lazily create a new turn")
}

func TestCompletedExecutionStreamDoesNotCancelClarificationWatchdog(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	watchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.clarificationWatchdogs.Store(
		svc.clarificationWatchdogKey("s1", "pending-1"),
		&clarificationWatchdogEntry{cancel: cancel},
	)
	svc.markExecutionCompleted("s1", "exec-1")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "tc1",
			ToolStatus: agentEventComplete,
		},
	})

	require.Equal(t, 1, countClarificationWatchdogs(svc),
		"stale completed-execution streams must be dropped before watchdog cancellation")
	select {
	case <-watchCtx.Done():
		t.Fatal("stale completed-execution stream cancelled clarification watchdog")
	default:
	}
}

func TestTerminalCompleteStreamDoesNotCancelClarificationWatchdog(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	watchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.clarificationWatchdogs.Store(
		svc.clarificationWatchdogKey("s1", "pending-1"),
		&clarificationWatchdogEntry{cancel: cancel},
	)
	svc.markExecutionCompleted("s1", "exec-1")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete,
			Text: "late complete",
		},
	})

	require.Equal(t, 1, countClarificationWatchdogs(svc),
		"late terminal complete streams must not cancel clarification watchdogs")
	select {
	case <-watchCtx.Done():
		t.Fatal("late terminal complete cancelled clarification watchdog")
	default:
	}
}

func TestTerminalCompleteStreamDetachesClarificationWaitersWithoutCancellingWatchdog(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	canceller := &recordingClarificationCanceller{}
	svc.clarificationCanceller = canceller
	watchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.clarificationWatchdogs.Store(
		svc.clarificationWatchdogKey("s1", "pending-1"),
		&clarificationWatchdogEntry{cancel: cancel},
	)
	svc.markExecutionCompleted("s1", "exec-1")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete,
		},
	})

	require.Equal(t, []string{"s1"}, canceller.sessions)
	require.Equal(t, 1, countClarificationWatchdogs(svc),
		"late terminal complete should detach waiters without cancelling watchdog fallback entries")
	select {
	case <-watchCtx.Done():
		t.Fatal("late terminal complete cancelled clarification watchdog")
	default:
	}
}

func TestWriteTaskReviewStateSkipsWhenSessionListFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.repo = listTaskSessionsErrorRepo{sessionExecutorStore: svc.repo}

	svc.writeTaskReviewState(ctx, "t1", "s1")

	require.Zero(t, taskRepo.stateWrites["t1"],
		"REVIEW writes must fail closed when sibling session reconciliation fails")
}

func TestWriteTaskReviewStateSkipsTerminalTaskState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	task, err := repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.State = v1.TaskStateCompleted
	require.NoError(t, repo.UpdateTask(ctx, task))

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", State: v1.TaskStateCompleted}
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.writeTaskReviewState(ctx, "t1", "s1")

	require.Zero(t, taskRepo.stateWrites["t1"],
		"REVIEW reconcile must not rewind terminal task states")
}

func TestWriteTaskInProgressForRuntimeSkipsArchivedTask(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	require.NoError(t, repo.ArchiveTask(ctx, "t1"))

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.writeTaskInProgressForRuntime(ctx, "t1", "s1")

	require.Zero(t, taskRepo.stateWrites["t1"],
		"runtime reconciliation must not promote archived tasks to IN_PROGRESS")
}

func TestWriteTaskInProgressForRuntimeSkipsCancelledSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx,
		"s1",
		models.TaskSessionStateCancelled,
		"stopped by parent task via MCP",
	))
	task, err := repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	require.NoError(t, repo.UpdateTask(ctx, task))

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.writeTaskInProgressForRuntime(ctx, "t1", "s1")

	require.Zero(t, taskRepo.stateWrites["t1"],
		"cancelled session must not promote task from REVIEW to IN_PROGRESS")
	require.Equal(t, v1.TaskStateReview, taskRepo.tasks["t1"].State)
}

func TestHandleAgentCompleted_CancelledSessionDoesNotForceCleanup(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx,
		"s1",
		models.TaskSessionStateCancelled,
		"stopped by parent task via MCP",
	))

	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-completed", nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	svc.RegisterExecutionStopOwner("s1", "execution-completed", false)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-completed",
	})
	coordinatorStopWaitForGuardReleased(t, svc, "s1")

	agentManager.mu.Lock()
	calls := append([]stopAgentCall(nil), agentManager.stopAgentWithReasonArgs...)
	agentManager.mu.Unlock()
	require.Empty(t, calls, "cancelled completion must not replace graceful teardown with force cleanup")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
}

func TestHandleAgentCompleted_CoordinatorCancellationWinsWhileEventWaits(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-completion-stop-race", "session-completion-stop-race", "step-1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-1"] = &wfmodels.WorkflowStep{
		ID: "step-1", WorkflowID: "wf1", Name: "Work", Position: 0,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
			Type: wfmodels.OnTurnCompleteMoveToNext,
		}}},
	}
	stepGetter.steps["step-2"] = &wfmodels.WorkflowStep{
		ID: "step-2", WorkflowID: "wf1", Name: "Review", Position: 1,
	}
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-completion-stop-race", nil
		},
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, newMockTaskRepo(), manager)

	guard, release := svc.acquireCancelInFlightGuard("session-completion-stop-race")
	guard.Lock()
	eventDone := make(chan struct{})
	go func() {
		svc.handleAgentCompleted(ctx, watcher.AgentEventData{
			TaskID:           "task-completion-stop-race",
			SessionID:        "session-completion-stop-race",
			AgentExecutionID: "execution-completion-stop-race",
		})
		close(eventDone)
	}()
	coordinatorStopWaitForGuardRefs(t, svc, "session-completion-stop-race", 2)
	changed, _, err := repo.CancelActiveTaskSession(
		ctx,
		"session-completion-stop-race",
		coordinatorMCPStopReason,
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, svc.claimExecutionTeardown(
		"session-completion-stop-race",
		"execution-completion-stop-race",
		executionTeardownIntentGraceful,
	))
	guard.Unlock()
	release()
	coordinatorStopAwaitSignal(t, eventDone, "guarded completion event")

	session, err := repo.GetTaskSession(ctx, "session-completion-stop-race")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
	task, err := repo.GetTask(ctx, "task-completion-stop-race")
	require.NoError(t, err)
	require.Equal(t, "step-1", task.WorkflowStepID, "stale completion advanced workflow after cancellation")
	manager.mu.Lock()
	stopCalls := append([]stopAgentCall(nil), manager.stopAgentWithReasonArgs...)
	manager.mu.Unlock()
	require.Empty(t, stopCalls, "stale completion replaced graceful teardown with force cleanup")
}

// TestWriteTaskInProgressForRuntimeUsesArchiveAwareCAS is the TOCTOU
// companion to TestWriteTaskInProgressForRuntimeSkipsArchivedTask
// (carlosflorencio review on PR #1706): the earlier taskArchived() guard is
// a plain, non-transactional read, so an ArchiveTask commit landing in the
// window between that read and the write could still resurrect the task's
// state if the write itself were the unconditional UpdateTaskState. Asserts
// the write goes through UpdateTaskStateIfNotArchived (tracked separately
// from the unconditional path via unconditionalWrites) instead.
func TestWriteTaskInProgressForRuntimeUsesArchiveAwareCAS(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.writeTaskInProgressForRuntime(ctx, "t1", "s1")

	require.Equal(t, 1, taskRepo.stateWrites["t1"],
		"runtime reconciliation must still promote a non-archived task to IN_PROGRESS")
	require.Equal(t, v1.TaskStateInProgress, taskRepo.updatedStates["t1"])
	require.Zero(t, taskRepo.unconditionalWrites["t1"],
		"IN_PROGRESS write must use UpdateTaskStateIfNotArchived, not the unconditional UpdateTaskState")
}

func TestWriteTaskReviewStateSkipsArchivedTask(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.ArchiveTask(ctx, "t1"))

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", State: v1.TaskStateInProgress}
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.writeTaskReviewState(ctx, "t1", "s1")

	require.Zero(t, taskRepo.stateWrites["t1"],
		"REVIEW reconcile must not resurrect an archived task's state")
}

func TestWriteTaskReviewStateOnCancelSkipsArchivedTask(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.ArchiveTask(ctx, "t1"))

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", State: v1.TaskStateInProgress}
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.writeTaskReviewStateOnCancel(ctx, "t1", "s1")

	require.Zero(t, taskRepo.stateWrites["t1"],
		"cancel reconcile must not resurrect an archived task's state")
}

func TestWriteTaskReviewStateOnCancelSkipsWhenSessionActiveAgain(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateRunning
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", State: v1.TaskStateInProgress}
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.writeTaskReviewStateOnCancel(ctx, "t1", "s1")

	require.Zero(t, taskRepo.stateWrites["t1"],
		"cancel reconcile must not move a restarted same session back to REVIEW")
}

func TestWriteTaskReviewStateOnCancelSkipsWhenSessionListFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", State: v1.TaskStateInProgress}
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.repo = listTaskSessionsErrorRepo{sessionExecutorStore: svc.repo}

	svc.writeTaskReviewStateOnCancel(ctx, "t1", "s1")

	require.Zero(t, taskRepo.stateWrites["t1"],
		"cancel REVIEW writes must fail closed when sibling session reconciliation fails")
}

func TestRuntimeStateOwnershipUsesCanonicalOfficeProjection(t *testing.T) {
	tests := []struct {
		name           string
		isFromOffice   bool
		assignee       string
		wantReview     bool
		wantInProgress bool
	}{
		{name: "unassigned Office task", isFromOffice: true},
		{name: "assigned Kanban task", assignee: "assigned-agent", wantReview: true, wantInProgress: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "")
			session, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			session.State = models.TaskSessionStateWaitingForInput
			require.NoError(t, repo.UpdateTaskSession(ctx, session))

			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
			svc := createTestService(repo, newMockStepGetter(), taskRepo)
			svc.repo = ownershipOverrideRepo{
				sessionExecutorStore: repo,
				tasks: map[string]*models.Task{"t1": {
					ID: "t1", IsFromOffice: tt.isFromOffice, AssigneeAgentProfileID: tt.assignee,
				}},
			}

			svc.writeTaskReviewState(ctx, "t1", "s1")
			reviewWritten := taskRepo.updatedStates["t1"] == v1.TaskStateReview
			require.Equal(t, tt.wantReview, reviewWritten)

			taskRepo.updatedStates = make(map[string]v1.TaskState)
			require.NoError(t, repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateStarting, ""))
			require.NoError(t, svc.reconcileTaskStateForRuntime(ctx, "t1", "s1", v1.TaskStateInProgress))
			inProgressWritten := taskRepo.updatedStates["t1"] == v1.TaskStateInProgress
			require.Equal(t, tt.wantInProgress, inProgressWritten)
		})
	}
}

func TestCancelStateOwnershipUsesCanonicalOfficeProjection(t *testing.T) {
	tests := []struct {
		name         string
		isFromOffice bool
		assignee     string
		wantReview   bool
	}{
		{name: "unassigned Office task", isFromOffice: true},
		{name: "assigned Kanban task", assignee: "assigned-agent", wantReview: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "")
			session, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			session.State = models.TaskSessionStateWaitingForInput
			require.NoError(t, repo.UpdateTaskSession(ctx, session))

			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
			svc := createTestService(repo, newMockStepGetter(), taskRepo)
			svc.repo = ownershipOverrideRepo{
				sessionExecutorStore: repo,
				tasks: map[string]*models.Task{"t1": {
					ID: "t1", IsFromOffice: tt.isFromOffice, AssigneeAgentProfileID: tt.assignee,
				}},
			}

			svc.writeTaskReviewStateOnCancel(ctx, "t1", "s1")
			reviewWritten := taskRepo.updatedStates["t1"] == v1.TaskStateReview
			require.Equal(t, tt.wantReview, reviewWritten)
		})
	}
}

func TestToolUpdateFromFailedExecutionDoesNotCreateMessage(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
		ErrorMessage:     "agent failed",
	})

	svc.handleToolUpdateEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			ToolCallID: "tc1",
			ToolStatus: agentEventComplete,
		},
	})

	require.Zero(t, messages.toolUpdateWrites,
		"late tool events from a failed execution must be dropped before message side effects")
}

func TestCompletedExecutionMarkerExpiresAndDeletes(t *testing.T) {
	svc := &Service{}

	svc.markExecutionCompleted("s1", "exec-1")
	require.True(t, svc.isExecutionCompleted("s1", "exec-1"))

	currentKey := terminalExecutionKey("s1", "exec-1")
	expiredAt := time.Now().Add(-time.Minute)
	svc.completedExecutions.Store(currentKey, terminalExecutionMarker{
		expiresAt:           expiredAt,
		allowCompleteStream: true,
	})
	require.False(t, svc.isExecutionCompleted("s1", "exec-1"))
	_, ok := svc.completedExecutions.Load(currentKey)
	require.False(t, ok, "expired marker should be deleted on lookup")

	laterExpiry := time.Now().Add(time.Hour)
	svc.completedExecutions.Store(currentKey, terminalExecutionMarker{
		expiresAt:           laterExpiry,
		allowCompleteStream: true,
	})
	svc.deleteCompletedExecutionIfExpired(currentKey, time.Now())
	_, ok = svc.completedExecutions.Load(currentKey)
	require.True(t, ok, "old expiry callback must not delete a refreshed marker")

	svc.deleteCompletedExecutionIfExpired(currentKey, laterExpiry)
	_, ok = svc.completedExecutions.Load(currentKey)
	require.False(t, ok, "matching expiry callback should delete the marker")
}

func TestTerminalExecutionMarker_CompletionPermissionIsMonotonic(t *testing.T) {
	svc := &Service{}
	const sessionID, executionID = "session-terminal-race", "execution-terminal-race"
	key := terminalExecutionKey(sessionID, executionID)

	for range 25 {
		svc.completedExecutions.Delete(key)
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			svc.markExecutionCompleted(sessionID, executionID)
		}()
		go func() {
			defer wait.Done()
			<-start
			svc.markExecutionFailed(sessionID, executionID)
		}()
		close(start)
		wait.Wait()

		marker, ok := svc.terminalCompleteStreamMarker(sessionID, executionID)
		require.True(t, ok, "concurrent stopped marker downgraded successful completion")
		require.True(t, marker.allowCompleteStream)
	}
}

// TestSetSessionRunning_NoRedundantTaskWrites locks in the dedup: when the
// session is already RUNNING, setSessionRunning must not re-write tasks.state.
// Without the guard, every tool_call / tool_update fired UpdateTaskState
// (2,000+ redundant writes observed on long-running turns).
func TestSetSessionRunning_NoRedundantTaskWrites(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateRunning
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	for i := 0; i < 5; i++ {
		svc.setSessionRunning(ctx, "t1", "s1")
	}

	require.Equal(t, 0, taskRepo.stateWrites["t1"],
		"setSessionRunning must not write tasks.state when session is already RUNNING")
}

// TestSetSessionWaitingForInput_NoRedundantTaskWrites locks in the dedup for
// the WAITING_FOR_INPUT path. Without the guard, both the workflow
// on_turn_complete transition and handleCompleteStreamEvent were writing
// tasks.state=REVIEW back-to-back on every turn.
func TestSetSessionWaitingForInput_NoRedundantTaskWrites(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	for i := 0; i < 5; i++ {
		svc.setSessionWaitingForInput(ctx, "t1", "s1")
	}

	require.Equal(t, 0, taskRepo.stateWrites["t1"],
		"setSessionWaitingForInput must not write tasks.state when session is already WAITING_FOR_INPUT")
}

// TestSetSessionRunning_WritesOnTransition guards against an over-eager dedup
// regression: when the session was NOT already RUNNING, the task state write
// MUST still happen.
func TestSetSessionRunning_PublishesTaskStateBeforeRunningSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.UpdateTaskState(ctx, "t1", v1.TaskStateReview))
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx,
		"s1",
		models.TaskSessionStateWaitingForInput,
		"",
	))

	eventBus := &recordingEventBus{}
	taskRepo := newTaskServiceStateRepository(repo, eventBus)
	svc := &Service{
		logger:   testLogger(),
		eventBus: eventBus,
		repo:     repo,
		taskRepo: taskRepo,
	}

	svc.setSessionRunning(ctx, "t1", "s1")

	task, err := repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateInProgress, task.State)
	require.Len(t, eventBus.events, 2)
	require.Equal(t, events.TaskStateChanged, eventBus.events[0].subject)
	require.Equal(t, events.TaskSessionStateChanged, eventBus.events[1].subject)
}

func TestSetSessionRunning_WritesOnTransition(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.setSessionRunning(ctx, "t1", "s1")

	require.Equal(t, 1, taskRepo.stateWrites["t1"],
		"setSessionRunning must write tasks.state on actual transition")
	require.Equal(t, v1.TaskStateInProgress, taskRepo.updatedStates["t1"])
}

func TestSetSessionStartingWritesTaskInProgress(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateStarting
	session.ErrorMessage = ""
	session.UpdatedAt = time.Now().UTC()

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	require.NoError(t, svc.setSessionStarting(
		ctx, "t1", session, models.TaskSessionStateRunning, true,
	))

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, updated.State)
	require.Equal(t, 1, taskRepo.stateWrites["t1"])
	require.Equal(t, v1.TaskStateInProgress, taskRepo.updatedStates["t1"])
}

func TestSetSessionStartingCanDeferTaskInProgress(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateStarting
	session.ErrorMessage = ""
	session.UpdatedAt = time.Now().UTC()

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	require.NoError(t, svc.setSessionStarting(
		ctx, "t1", session, models.TaskSessionStateRunning, false,
	))

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, updated.State)
	require.Empty(t, taskRepo.stateWrites)
}

// recordingTaskUpdatedPublisher captures task.updated publishes so tests can
// assert the interrupted-marker clear republishes the task.
type recordingTaskUpdatedPublisher struct {
	updatedTaskIDs []string
}

func (r *recordingTaskUpdatedPublisher) PublishTaskUpdated(_ context.Context, task *models.Task, _ ...string) {
	if task != nil {
		r.updatedTaskIDs = append(r.updatedTaskIDs, task.ID)
	}
}
func (r *recordingTaskUpdatedPublisher) PublishTaskStateChanged(context.Context, *models.Task, v1.TaskState) {
}
func (r *recordingTaskUpdatedPublisher) PublishTaskActivityIfChanged(context.Context, string) {}

func seedInterruptedMarker(t *testing.T, repo *sqliterepo.Repository, taskID string) {
	t.Helper()
	if err := repo.SetTaskMetadataKey(context.Background(), taskID, models.MetaKeyInterruptedAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("seed interrupted marker: %v", err)
	}
}

func TestSessionStartClearsInterruptedMarker(t *testing.T) {
	ctx := context.Background()

	t.Run("state hook transition to STARTING clears and republishes", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		seedInterruptedMarker(t, repo, "t1")

		publisher := &recordingTaskUpdatedPublisher{}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.SetTaskEventPublisher(publisher)

		updated, changed := svc.updateTaskSessionStateWithHook(
			ctx, "t1", "s1", models.TaskSessionStateStarting, "", false, nil,
		)
		require.NotNil(t, updated)
		require.True(t, changed)

		task, err := repo.GetTask(ctx, "t1")
		require.NoError(t, err)
		_, marked := task.Metadata[models.MetaKeyInterruptedAt]
		require.False(t, marked, "marker must be cleared when the session enters STARTING")
		require.Equal(t, []string{"t1"}, publisher.updatedTaskIDs,
			"task.updated must be republished after clearing the marker")
	})

	t.Run("launch path via setSessionStarting clears and republishes", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		seedInterruptedMarker(t, repo, "t1")

		session, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		session.State = models.TaskSessionStateStarting
		session.ErrorMessage = ""
		session.UpdatedAt = time.Now().UTC()

		publisher := &recordingTaskUpdatedPublisher{}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.SetTaskEventPublisher(publisher)

		require.NoError(t, svc.setSessionStarting(ctx, "t1", session, models.TaskSessionStateRunning, true))

		task, err := repo.GetTask(ctx, "t1")
		require.NoError(t, err)
		_, marked := task.Metadata[models.MetaKeyInterruptedAt]
		require.False(t, marked, "marker must be cleared on the launch path")
		require.Equal(t, []string{"t1"}, publisher.updatedTaskIDs,
			"task.updated must be republished after clearing the marker")
	})

	t.Run("no marker means no republish", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		publisher := &recordingTaskUpdatedPublisher{}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.SetTaskEventPublisher(publisher)

		updated, changed := svc.updateTaskSessionStateWithHook(
			ctx, "t1", "s1", models.TaskSessionStateStarting, "", false, nil,
		)
		require.NotNil(t, updated)
		require.True(t, changed)
		require.Empty(t, publisher.updatedTaskIDs,
			"no task.updated may be published when no marker was removed")
	})
}

func TestSessionStartClearsAutoStartFailedMarker(t *testing.T) {
	ctx := context.Background()

	t.Run("state hook transition to STARTING", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		require.NoError(t, repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyAutoStartFailed, true))

		publisher := &recordingTaskUpdatedPublisher{}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.SetTaskEventPublisher(publisher)

		_, changed := svc.updateTaskSessionStateWithHook(
			ctx, "t1", "s1", models.TaskSessionStateStarting, "", false, nil,
		)
		require.True(t, changed)

		task, err := repo.GetTask(ctx, "t1")
		require.NoError(t, err)
		_, marked := task.Metadata[models.MetaKeyAutoStartFailed]
		require.False(t, marked)
		require.Equal(t, []string{"t1"}, publisher.updatedTaskIDs)
	})

	t.Run("launch path via setSessionStarting", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		require.NoError(t, repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyAutoStartFailed, true))

		session, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		session.State = models.TaskSessionStateStarting
		session.ErrorMessage = ""
		session.UpdatedAt = time.Now().UTC()

		publisher := &recordingTaskUpdatedPublisher{}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.SetTaskEventPublisher(publisher)

		require.NoError(t, svc.setSessionStarting(ctx, "t1", session, models.TaskSessionStateRunning, true))

		task, err := repo.GetTask(ctx, "t1")
		require.NoError(t, err)
		_, marked := task.Metadata[models.MetaKeyAutoStartFailed]
		require.False(t, marked)
		require.Equal(t, []string{"t1"}, publisher.updatedTaskIDs)
	})
}

func TestAutoStartFailureDoesNotMarkTaskWithActiveSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx, "s1", models.TaskSessionStateStarting, "",
	))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.setTaskAutoStartFailedMarker(ctx, "t1", "task.moved")

	task, err := repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	_, marked := task.Metadata[models.MetaKeyAutoStartFailed]
	require.False(t, marked, "a late failure must not mark a task with active work")
}

// Nothing else ever retires a stored agent failure, so without this clear every
// path that re-derives task status from session metadata resurrects it and the
// error icon never goes away.
func TestClearRecoveredAgentErrorOnTurnCompletion(t *testing.T) {
	ctx := context.Background()

	seedError := func(t *testing.T, repo *sqliterepo.Repository) {
		t.Helper()
		require.NoError(t, repo.SetSessionMetadataKey(
			ctx, "s1", models.SessionMetaKeyLastAgentError,
			models.LastAgentError{Message: "agent crashed", OccurredAt: time.Now().UTC().Add(-time.Hour)},
		))
	}

	errorEvents := func(eb *recordingEventBus) []recordedEvent {
		var out []recordedEvent
		for _, recorded := range eb.events {
			if recorded.event != nil && recorded.event.Type == events.TaskSessionErrorChanged {
				out = append(out, recorded)
			}
		}
		return out
	}

	newService := func(repo *sqliterepo.Repository) (*Service, *recordingEventBus) {
		eb := &recordingEventBus{}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.eventBus = eb
		return svc, eb
	}

	t.Run("clears the record and publishes it inactive", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		seedError(t, repo)
		svc, eb := newService(repo)

		session, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		svc.clearRecoveredAgentError(ctx, "t1", session)

		stored, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		_, ok := models.LoadLastAgentError(stored.Metadata)
		require.False(t, ok, "a recovered failure must not keep driving the error icon")

		// The session-state publish that follows reads session_metadata straight
		// off this object, so a stale copy would re-arm the icon immediately.
		_, ok = models.LoadLastAgentError(session.Metadata)
		require.False(t, ok, "the in-memory copy must be cleared too")

		published := errorEvents(eb)
		require.Len(t, published, 1, "clients need one event to drop the icon")
		data, ok := published[0].event.Data.(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, false, data["active"])
		require.Equal(t, "s1", data["session_id"])
	})

	t.Run("stays silent when the session has no stored error", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		svc, eb := newService(repo)

		session, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		svc.clearRecoveredAgentError(ctx, "t1", session)

		require.Empty(t, errorEvents(eb),
			"every later completion must stay silent so turns do not churn the row")
	})
}

func TestSetSessionStartingRejectsTerminalSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	current, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	current.State = models.TaskSessionStateCancelled
	require.NoError(t, repo.UpdateTaskSession(ctx, current))

	next := *current
	next.State = models.TaskSessionStateStarting
	next.ErrorMessage = ""
	next.UpdatedAt = time.Now().UTC()

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	require.Error(t, svc.setSessionStarting(
		ctx, "t1", &next, models.TaskSessionStateCancelled, true,
	))

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, updated.State)
	require.Empty(t, taskRepo.stateWrites)
}

func TestSetSessionStartingAllowsExplicitlyCancelledResumeWithoutPromotion(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	current, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	current.State = models.TaskSessionStateCancelled
	current.ErrorMessage = "stopped via API"
	require.NoError(t, repo.UpdateTaskSession(ctx, current))

	next := *current
	next.State = models.TaskSessionStateStarting
	next.ErrorMessage = ""
	next.UpdatedAt = time.Now().UTC()

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	require.NoError(t, svc.setSessionStarting(
		ctx, "t1", &next, models.TaskSessionStateCancelled, false,
	))

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, updated.State)
	require.Empty(t, taskRepo.stateWrites)
}

func TestSetSessionStartingRejectsCancellationAfterResumeSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stale, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	stale.State = models.TaskSessionStateStarting

	current, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	current.State = models.TaskSessionStateCancelled
	current.ErrorMessage = "stopped via API"
	require.NoError(t, repo.UpdateTaskSession(ctx, current))

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	require.Error(t, svc.setSessionStarting(
		ctx, "t1", stale, models.TaskSessionStateRunning, false,
	))

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, updated.State)
	require.Empty(t, taskRepo.stateWrites)
}

func TestSetSessionStartingAllowsTerminalResumeWithoutTaskPromotion(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	current, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	current.State = models.TaskSessionStateFailed
	current.ErrorMessage = "previous failure"
	completedAt := time.Now().UTC()
	current.CompletedAt = &completedAt
	require.NoError(t, repo.UpdateTaskSession(ctx, current))

	next := *current
	next.State = models.TaskSessionStateStarting
	next.ErrorMessage = ""
	next.CompletedAt = nil
	next.UpdatedAt = time.Now().UTC()

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	require.NoError(t, svc.setSessionStarting(
		ctx, "t1", &next, models.TaskSessionStateFailed, false,
	))

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, updated.State)
	require.Nil(t, updated.CompletedAt)
	require.Empty(t, taskRepo.stateWrites)
}

// TestSetSessionStartingAllowsArchiveCancelledResumeWithoutPromotion covers
// sessions cancelled by an archive (Service.ArchiveTask or the cascade
// archive path), which carry a distinct ErrorMessage; unlike an explicit
// user/coordinator stop, these must recover into STARTING like a Failed
// session so Resume works normally once the task is unarchived (bug:
// "Can't resume this un-archived task").
func TestSetSessionStartingAllowsArchiveCancelledResumeWithoutPromotion(t *testing.T) {
	for _, reason := range []string{models.SessionArchiveCancelReason, models.SessionArchiveTreeCancelReason} {
		t.Run(reason, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")

			current, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			current.State = models.TaskSessionStateCancelled
			current.ErrorMessage = reason
			require.NoError(t, repo.UpdateTaskSession(ctx, current))

			next := *current
			next.State = models.TaskSessionStateStarting
			next.ErrorMessage = ""
			next.UpdatedAt = time.Now().UTC()

			taskRepo := newMockTaskRepo()
			svc := createTestService(repo, newMockStepGetter(), taskRepo)

			require.NoError(t, svc.setSessionStarting(
				ctx, "t1", &next, models.TaskSessionStateCancelled, false,
			))

			updated, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateStarting, updated.State)
			require.Empty(t, taskRepo.stateWrites)
		})
	}
}

// Pins the call-site wiring: cancelled office turn must NOT leave the session at IDLE.
func TestHandleCompleteStreamEvent_CancelledOfficeSessionLandsWaitingForInput(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-cancel-flow", "s-cancel-flow", "exec-cancel-flow")
	mgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), mgr)

	// Mirror Service.CancelAgent's pre-emptive WAITING_FOR_INPUT write.
	session, err := repo.GetTaskSession(ctx, "s-cancel-flow")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:    "t-cancel-flow",
		SessionID: "s-cancel-flow",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete,
			Data: map[string]interface{}{
				"stop_reason": "cancelled",
			},
		},
	}

	svc.handleCompleteStreamEvent(ctx, payload)

	got, err := repo.GetTaskSession(ctx, "s-cancel-flow")
	require.NoError(t, err)
	require.NotEqual(t, models.TaskSessionStateIdle, got.State,
		"cancelled office turn must not leave the session IDLE — PromptTask would reject the user's next message")
	require.Equal(t, models.TaskSessionStateWaitingForInput, got.State,
		"cancelled office turn must fall through to setSessionWaitingForInput")
	mgr.mu.Lock()
	stopCalls := len(mgr.stopAgentArgs)
	mgr.mu.Unlock()
	require.Zero(t, stopCalls,
		"cancelled office turn must not tear down the agent process — Service.CancelAgent owns lifecycle for user cancels")
}

// Inverse guard: a natural end_turn completion on an office session still parks IDLE + StopAgent.
func TestHandleCompleteStreamEvent_NaturalOfficeCompleteStillIdle(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-natural", "s-natural", "exec-natural")
	mgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), mgr)

	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:    "t-natural",
		SessionID: "s-natural",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete,
			Data: map[string]interface{}{
				"stop_reason": "end_turn",
			},
		},
	}

	svc.handleCompleteStreamEvent(ctx, payload)

	got, err := repo.GetTaskSession(ctx, "s-natural")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateIdle, got.State,
		"natural office turn completion must still park the session in IDLE")
	mgr.mu.Lock()
	stopCalls := len(mgr.stopAgentArgs)
	mgr.mu.Unlock()
	require.Equal(t, 1, stopCalls,
		"natural office turn completion must still call StopAgent to tear down the executor")
}

// Automation tasks are ordinary, non-ephemeral tasks tagged by origin — the
// task/run execution mode is withdrawn, so nothing here sets IsEphemeral.
func seedAutomationRunSession(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID, sessionID, executionID string,
) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID:        "ws-" + taskID,
		Name:      "Automation",
		CreatedAt: now,
		UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          taskID,
		WorkspaceID: "ws-" + taskID,
		Title:       "Automation run",
		Description: "run this",
		State:       v1.TaskStateInProgress,
		Origin:      models.TaskOriginAutomationRun,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        sessionID,
		TaskID:    taskID,
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}))
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
}

func TestHandleCompleteStreamEvent_AutomationRunStopsAndFinalizes(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedAutomationRunSession(t, repo, "t-auto-run", "s-auto-run", "exec-auto-run")

	mgr := &mockAgentManager{}
	automationSvc := &mockAutomationRunService{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), mgr)
	svc.SetAutomationService(automationSvc)

	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:      "t-auto-run",
		SessionID:   "s-auto-run",
		ExecutionID: "exec-auto-run",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete,
			Data: map[string]interface{}{
				"stop_reason": "end_turn",
			},
		},
	}

	svc.handleCompleteStreamEvent(ctx, payload)

	require.Equal(t, []string{"t-auto-run"}, automationSvc.succeededTaskIDs)
	require.Empty(t, automationSvc.failedTaskIDs)
	gotSession, err := repo.GetTaskSession(ctx, "s-auto-run")
	require.NoError(t, err)
	// Not COMPLETED: that state refuses resume, which would make a finished
	// report unanswerable. Success lives on the AutomationRun row instead.
	require.Equal(t, models.TaskSessionStateWaitingForInput, gotSession.State)

	mgr.mu.Lock()
	stopCalls := append([]stopAgentCall(nil), mgr.stopAgentArgs...)
	mgr.mu.Unlock()
	require.Equal(t, []stopAgentCall{{ExecutionID: "exec-auto-run"}}, stopCalls)
}

func TestHandleCompleteStreamEvent_AutomationRunFailureStopsAndFinalizes(t *testing.T) {
	cases := []struct {
		name        string
		data        map[string]interface{}
		wantErrMsg  string
		wantSession models.TaskSessionState
	}{
		{
			name: "stop_reason_error",
			data: map[string]interface{}{
				"stop_reason": "error",
				"error":       "agent failed",
			},
			wantErrMsg:  "agent failed",
			wantSession: models.TaskSessionStateFailed,
		},
		{
			name: "is_error_true",
			data: map[string]interface{}{
				"is_error": true,
				"message":  "adapter failed",
			},
			wantErrMsg:  "adapter failed",
			wantSession: models.TaskSessionStateFailed,
		},
		{
			name: "cancelled",
			data: map[string]interface{}{
				"stop_reason": "cancelled",
			},
			wantErrMsg:  "cancelled",
			wantSession: models.TaskSessionStateCancelled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			taskID := "t-auto-" + tc.name
			sessionID := "s-auto-" + tc.name
			executionID := "exec-auto-" + tc.name
			seedAutomationRunSession(t, repo, taskID, sessionID, executionID)

			mgr := &mockAgentManager{}
			automationSvc := &mockAutomationRunService{}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), mgr)
			svc.SetAutomationService(automationSvc)

			payload := &lifecycle.AgentStreamEventPayload{
				TaskID:      taskID,
				SessionID:   sessionID,
				ExecutionID: executionID,
				Data: &lifecycle.AgentStreamEventData{
					Type: agentEventComplete,
					Data: tc.data,
				},
			}

			svc.handleCompleteStreamEvent(ctx, payload)

			require.Empty(t, automationSvc.succeededTaskIDs)
			require.Equal(t, []string{taskID}, automationSvc.failedTaskIDs)
			require.Equal(t, tc.wantErrMsg, automationSvc.failedErrorByTask[taskID])
			gotSession, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			require.Equal(t, tc.wantSession, gotSession.State)

			mgr.mu.Lock()
			stopCalls := append([]stopAgentCall(nil), mgr.stopAgentArgs...)
			mgr.mu.Unlock()
			require.Equal(t, []stopAgentCall{{ExecutionID: executionID}}, stopCalls)
		})
	}
}

func TestExtractCompleteErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		data *lifecycle.AgentStreamEventData
		want string
	}{
		{name: "nil_data", data: nil, want: ""},
		{
			name: "data_error",
			data: &lifecycle.AgentStreamEventData{Error: "top-level error"},
			want: "top-level error",
		},
		{
			name: "structured_error",
			data: &lifecycle.AgentStreamEventData{Data: map[string]interface{}{"error": "structured error"}},
			want: "structured error",
		},
		{
			name: "structured_message",
			data: &lifecycle.AgentStreamEventData{Data: map[string]interface{}{"message": "structured message"}},
			want: "structured message",
		},
		{
			name: "agent_text_ignored",
			data: &lifecycle.AgentStreamEventData{Text: "agent output"},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractCompleteErrorMessage(&lifecycle.AgentStreamEventPayload{Data: tc.data})
			require.Equal(t, tc.want, got)
		})
	}
}

// TestSetSessionWaitingForInput_WritesOnTransition is the symmetric counterpart
// to TestSetSessionRunning_WritesOnTransition: when the session is NOT already
// WAITING_FOR_INPUT, setSessionWaitingForInput MUST still fire the task write.
// Without this guard an accidental inversion of wasAlreadyWaiting would silently
// stop tasks from ever reaching REVIEW.
func TestSetSessionWaitingForInput_WritesOnTransition(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	// Seed session in RUNNING state (the normal pre-condition for a turn completing).
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateRunning
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.setSessionWaitingForInput(ctx, "t1", "s1")

	require.Equal(t, 1, taskRepo.stateWrites["t1"],
		"setSessionWaitingForInput must write tasks.state on actual transition")
	require.Equal(t, v1.TaskStateReview, taskRepo.updatedStates["t1"])
}

func TestSetSessionWaitingForInput_DoesNotMoveTaskToReviewWhileSiblingRuns(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s-finishing", "step1")

	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "s-running",
		TaskID:    "t1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Second),
		UpdatedAt: now.Add(time.Second),
	}))

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.setSessionWaitingForInput(ctx, "t1", "s-finishing")

	updatedFinishing, err := repo.GetTaskSession(ctx, "s-finishing")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, updatedFinishing.State)
	require.Equal(t, 0, taskRepo.stateWrites["t1"],
		"finishing one session must not move the task to REVIEW while another session is running")
}

func TestWriteTaskReviewState_DoesNotMoveTaskToReviewWhenSameSessionRestarted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateRunning
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	taskRepo := newMockTaskRepo()
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.writeTaskReviewState(ctx, "t1", "s1")

	require.Empty(t, taskRepo.updatedStates,
		"the just-completed session may have restarted before REVIEW reconciliation")
}

func TestSessionStateString(t *testing.T) {
	require.Equal(t, "", sessionStateString(nil),
		"nil session must render as empty so trace logs stay clean")
	require.Equal(t, string(models.TaskSessionStateRunning),
		sessionStateString(&models.TaskSession{State: models.TaskSessionStateRunning}))
}

// TestPersistSessionModel pins the SSR-side behaviour of the session_models
// event handler: a non-empty agent-reported model is written to
// AgentProfileSnapshot["model"] so the model selector trigger doesn't flash
// the profile default on a page reload before WS state catches up.
func TestPersistSessionModel(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := &Service{logger: testLogger(), repo: repo}

	svc.persistSessionModel(ctx, "s1", "gpt-5.4")

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", updated.AgentProfileSnapshot["model"])

	// A no-op write must not touch the DB row, but the visible behaviour is
	// the same: the snapshot still carries the previously-set value.
	svc.persistSessionModel(ctx, "s1", "gpt-5.4")
	again, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", again.AgentProfileSnapshot["model"])

	// An empty model is a no-op (some agents emit session_models without a
	// CurrentModelID before the first ConfigOptionUpdate). The previously
	// persisted value must not be cleared.
	svc.persistSessionModel(ctx, "s1", "")
	preserved, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", preserved.AgentProfileSnapshot["model"])
}

func TestPersistSessionModelDoesNotRestoreStaleActiveStateAfterCancellation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	stale, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, stale.State)
	changed, _, err := repo.CancelActiveTaskSession(ctx, "s1", "coordinator stop")
	require.NoError(t, err)
	require.True(t, changed)
	svc := &Service{logger: testLogger(), repo: repo}

	svc.persistSessionModelOnSession(ctx, "s1", stale, "gpt-after-stop")

	stored, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, stored.State)
	require.Equal(t, "gpt-after-stop", stored.AgentProfileSnapshot["model"])
}

func TestPersistSessionRuntimeConfigFromSessionModels(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := &Service{logger: testLogger(), repo: repo}

	svc.persistSessionRuntimeConfig(ctx, "s1", "gpt-5.3-codex-spark", "", []streams.ConfigOption{
		{ID: "model", Category: "model", CurrentValue: "gpt-5.3-codex-spark"},
		{ID: "reasoning_effort", Category: "thought_level", CurrentValue: "low"},
	})

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	cfg, ok := models.LoadSessionRuntimeConfig(updated.Metadata)
	require.True(t, ok)
	require.Equal(t, "gpt-5.3-codex-spark", cfg.Model)
	require.Equal(t, map[string]string{
		"model":            "gpt-5.3-codex-spark",
		"reasoning_effort": "low",
	}, cfg.ConfigOptions)
}

func TestPersistSessionRuntimeConfigUpdatesProviderState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyRuntimeConfig, models.SessionRuntimeConfig{
		Model: "mock-smart",
		ConfigOptions: map[string]string{
			"model":  "mock-smart",
			"effort": "low",
		},
	}))
	svc := &Service{logger: testLogger(), repo: repo}

	svc.persistSessionRuntimeConfig(ctx, "s1", "mock-fast", "", []streams.ConfigOption{
		{ID: "model", Category: "model", CurrentValue: "mock-fast"},
		{ID: "effort", Category: "thought_level", CurrentValue: "medium"},
	})

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	cfg, ok := models.LoadSessionRuntimeConfig(updated.Metadata)
	require.True(t, ok)
	require.Equal(t, "mock-fast", cfg.Model)
	require.Equal(t, map[string]string{"model": "mock-fast", "effort": "medium"}, cfg.ConfigOptions)
}

func TestHandleSessionModelsEventDefersUnsettledStartupState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateStarting, ""))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyRuntimeConfig, models.SessionRuntimeConfig{
		Model: "gpt-5.6-sol",
		ConfigOptions: map[string]string{
			"model":            "gpt-5.6-sol",
			"reasoning_effort": "high",
		},
	}))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyACPConfigBaseline, map[string]string{
		"model":            "gpt-5.6-sol",
		"reasoning_effort": "medium",
	}))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyACPModelState, lifecycle.SessionModelsSnapshot{
		CurrentModelID: "gpt-5.6-sol",
		Models:         []streams.SessionModelInfo{{ModelID: "gpt-5.6-sol", Name: "GPT-5.6 Sol"}},
		ConfigOptions: []streams.ConfigOption{{
			ID: "model", Category: "model", CurrentValue: "gpt-5.6-sol",
		}},
		ConfigOptionsSettled: true,
	}))
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.AgentProfileSnapshot = map[string]interface{}{"model": "gpt-5.6-sol"}
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	eb := &recordingEventBus{}
	svc := &Service{logger: testLogger(), repo: repo, eventBus: eb}
	unsettled := func(model string) {
		svc.handleSessionModelsEvent(ctx, &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data: &lifecycle.AgentStreamEventData{
				CurrentModelID: model,
				SessionModels:  []streams.SessionModelInfo{{ModelID: model, Name: model}},
				OriginalConfigCandidate: []streams.ConfigOption{{
					Type: "select", ID: "reasoning_effort", CurrentValue: "high",
				}},
				ConfigOptions: []streams.ConfigOption{
					{ID: "model", Category: "model", CurrentValue: model},
					{ID: "reasoning_effort", CurrentValue: "low"},
				},
				Data: map[string]any{"original_config_settled": true},
			},
		})
	}

	unsettled("gpt-5.6-luna")

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	runtimeConfig, ok := models.LoadSessionRuntimeConfig(updated.Metadata)
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", runtimeConfig.Model)
	require.Equal(t, "gpt-5.6-sol", updated.AgentProfileSnapshot["model"])
	modelState, ok := lifecycle.LoadSessionModelsSnapshot(updated.Metadata[models.SessionMetaKeyACPModelState])
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", modelState.CurrentModelID)
	require.Empty(t, eb.events)

	svc.handleSessionModelsEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		AgentID:   "a1",
		Data: &lifecycle.AgentStreamEventData{
			CurrentModelID: "gpt-5.6-sol",
			SessionModels:  []streams.SessionModelInfo{{ModelID: "gpt-5.6-sol", Name: "GPT-5.6 Sol"}},
			ConfigOptions: []streams.ConfigOption{
				{ID: "model", Category: "model", CurrentValue: "gpt-5.6-sol"},
				{ID: "reasoning_effort", CurrentValue: "high"},
			},
			Data: map[string]any{
				"config_options_settled":  true,
				"original_config_settled": true,
			},
		},
	})
	require.Len(t, eb.events, 1)

	require.NoError(t, repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateRunning, ""))
	unsettled("gpt-5.6-luna")
	require.Len(t, eb.events, 2)
	updated, err = repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	runtimeConfig, ok = models.LoadSessionRuntimeConfig(updated.Metadata)
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-luna", runtimeConfig.Model)
}

func TestHandleSessionModelsEventPublishesPersistedConfigBaselineAfterRestart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", "acp_config_baseline", map[string]string{
		"model":            "gpt-5.6-sol",
		"reasoning_effort": "high",
	}))
	eb := &recordingEventBus{}
	svc := &Service{logger: testLogger(), repo: repo, eventBus: eb}

	svc.handleSessionModelsEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		AgentID:   "a1",
		Data: &lifecycle.AgentStreamEventData{
			CurrentModelID: "gpt-5.6-sol",
			ConfigOptions: []streams.ConfigOption{{
				ID: "reasoning_effort", CurrentValue: "low",
			}},
		},
	})

	require.Len(t, eb.events, 1)
	raw, err := json.Marshal(eb.events[0].event.Data)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, map[string]any{
		"model":            "gpt-5.6-sol",
		"reasoning_effort": "high",
	}, payload["config_baseline"])
}

func TestHandleSessionModelsEventCapturesSettledConfigBaselineOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := &Service{logger: testLogger(), repo: repo, eventBus: eb}

	settled := func(reasoning string) {
		svc.handleSessionModelsEvent(ctx, &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data: &lifecycle.AgentStreamEventData{
				CurrentModelID: "gpt-5.6-sol",
				Data:           map[string]any{"config_options_settled": true},
				ConfigOptions: []streams.ConfigOption{
					{ID: "model", Category: "model", CurrentValue: "gpt-5.6-sol"},
					{ID: "reasoning_effort", CurrentValue: reasoning},
				},
			},
		})
	}

	settled("high")
	settled("low")

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	baseline, ok := models.LoadSessionACPConfigBaseline(updated.Metadata)
	require.True(t, ok)
	require.Equal(t, map[string]string{
		"model":            "gpt-5.6-sol",
		"reasoning_effort": "high",
	}, baseline)
	require.Len(t, eb.events, 2)
	lastPayload := eb.events[1].event.Data.(lifecycle.SessionModelsEventPayload)
	require.Equal(t, baseline, lastPayload.ConfigBaseline)
}

func TestHandleSessionModelsEventCapturesOriginalEffectiveConfigurationOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial))
	svc := &Service{logger: testLogger(), repo: repo, eventBus: &recordingEventBus{}}

	svc.handleSessionModelsEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			CurrentModelID: "gpt-5.6-sol",
			OriginalConfigCandidate: []streams.ConfigOption{
				{Type: "select", ID: "reasoning_effort", CurrentValue: "high"},
				{Type: "toggle", ID: "fast_mode", CurrentValue: "on"},
			},
			Data: map[string]any{"original_config_settled": true},
		},
	})

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	original, ok := models.LoadOriginalSessionEffectiveConfiguration(updated.Metadata)
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", original.Model)
	require.Equal(t, map[string]string{"reasoning_effort": "high"}, original.ConfigOptions)

	svc.handleSessionModelsEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			CurrentModelID: "gpt-5.6-luna",
			OriginalConfigCandidate: []streams.ConfigOption{
				{ID: "reasoning_effort", CurrentValue: "low"},
			},
			Data: map[string]any{"original_config_settled": true},
		},
	})

	updated, err = repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	original, ok = models.LoadOriginalSessionEffectiveConfiguration(updated.Metadata)
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", original.Model)
	require.Equal(t, map[string]string{"reasoning_effort": "high"}, original.ConfigOptions)
}

// TestHandleSessionModelFallbackEventPublishesToWS verifies the fallback event publishes the WS notification.

func TestHandleSessionModelFallbackEventPublishesToWS(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := &Service{logger: testLogger(), repo: repo, eventBus: eb}

	svc.handleSessionModelFallbackEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		AgentID:   "a1",
		Data: &lifecycle.AgentStreamEventData{
			FallbackModel: "deepseek/deepseek-v4-flash",
		},
	})

	require.Len(t, eb.events, 1)
	require.Equal(t, events.BuildSessionModelFallbackSubject("s1"), eb.events[0].subject)
	payload := eb.events[0].event.Data.(lifecycle.SessionModelFallbackEventPayload)
	require.Equal(t, "s1", payload.SessionID)
	require.Equal(t, "deepseek/deepseek-v4-flash", payload.FallbackModel)
}

// TestHandleAgentStreamEventRoutesSessionModelFallback pins the dispatch
// mapping: a "session_model_fallback" stream event must reach
// handleSessionModelFallbackEvent and publish the WS notification.
func TestHandleAgentStreamEventRoutesSessionModelFallback(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := &Service{logger: testLogger(), repo: repo, eventBus: eb}

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "exec-1",
		AgentID:     "a1",
		Data: &lifecycle.AgentStreamEventData{
			Type:          "session_model_fallback",
			FallbackModel: "gpt-5",
		},
	})

	require.Len(t, eb.events, 1)
	require.Equal(t, events.BuildSessionModelFallbackSubject("s1"), eb.events[0].subject)
	payload := eb.events[0].event.Data.(lifecycle.SessionModelFallbackEventPayload)
	require.Equal(t, "gpt-5", payload.FallbackModel)
}

// TestHandleSessionModelFallbackEventResolvesSessionFromTask covers the
// delayed-session path: the fallback fires during session init, before the
// execution's task-session id is linked, so the handler must resolve the
// session from the task id instead of dropping the note.
func TestHandleSessionModelFallbackEventResolvesSessionFromTask(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := &Service{logger: testLogger(), repo: repo, eventBus: eb}

	svc.handleSessionModelFallbackEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:  "t1",
		AgentID: "a1",
		Data: &lifecycle.AgentStreamEventData{
			FallbackModel: "gpt-5",
		},
	})

	require.Len(t, eb.events, 1)
	require.Equal(t, events.BuildSessionModelFallbackSubject("s1"), eb.events[0].subject)
	payload := eb.events[0].event.Data.(lifecycle.SessionModelFallbackEventPayload)
	require.Equal(t, "s1", payload.SessionID)
	require.Equal(t, "gpt-5", payload.FallbackModel)
}

func TestHandleSessionModelSelectionWarningPersistsIdempotently(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	messages := &mockMessageCreator{}
	eb := &recordingEventBus{}
	svc := &Service{logger: testLogger(), repo: repo, eventBus: eb, messageCreator: messages}
	warning := &streams.ModelSelectionWarning{
		Kind:              "model_selection_warning",
		DecisionID:        "decision-1",
		Reason:            "requested_not_advertised",
		RequestedModel:    "host-only-model",
		EffectiveModel:    "executor-default",
		AgentID:           "codex-acp",
		ExecutorType:      "ssh",
		ExecutorProfileID: "executor-profile-1",
	}
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		AgentID:   "codex-acp",
		Data: &lifecycle.AgentStreamEventData{
			Type:                  streams.EventTypeSessionModelSelectionWarning,
			ModelSelectionWarning: warning,
		},
	}

	svc.handleAgentStreamEvent(ctx, payload)
	svc.handleAgentStreamEvent(ctx, payload)

	require.Len(t, messages.sessionMessages, 1)
	require.Equal(t, "s1", messages.sessionMessages[0].sessionID)
	require.Equal(t, string(v1.MessageTypeStatus), messages.sessionMessages[0].messageType)
	require.Equal(t, "model_selection_warning", messages.sessionMessages[0].metadata["kind"])
	require.Equal(t, "host-only-model", messages.sessionMessages[0].metadata["requested_model"])
	require.Equal(t, "executor-default", messages.sessionMessages[0].metadata["effective_model"])
	require.Equal(t, []string{"executor_credentials", "copied_agent_configuration", "agent_version"}, messages.sessionMessages[0].metadata["remediation"])
	require.Len(t, eb.events, 1)
	require.Equal(t, events.BuildSessionModelSelectionWarningSubject("s1"), eb.events[0].subject)

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, true, updated.Metadata["model_selection_warning:decision-1"])
}

func TestHandleSessionModelSelectionWarningReleasesClaimAfterPersistenceFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	messages := &mockMessageCreator{sessionMessageErr: errors.New("transient message failure")}
	svc := &Service{logger: testLogger(), repo: repo, messageCreator: messages}
	warning := &streams.ModelSelectionWarning{
		Kind:           "model_selection_warning",
		DecisionID:     "decision-retry",
		Reason:         "requested_not_advertised",
		RequestedModel: "host-only-model",
		EffectiveModel: "executor-default",
	}
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			Type:                  streams.EventTypeSessionModelSelectionWarning,
			ModelSelectionWarning: warning,
		},
	}

	svc.handleAgentStreamEvent(ctx, payload)
	failed, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.NotContains(t, failed.Metadata, "model_selection_warning:decision-retry")
	require.Empty(t, messages.sessionMessages)

	messages.sessionMessageErr = nil
	svc.handleAgentStreamEvent(ctx, payload)
	require.Len(t, messages.sessionMessages, 1)
	completed, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, true, completed.Metadata["model_selection_warning:decision-retry"])
}

func TestHandleSessionModelsEventStoresBaselineCandidateAndPublishesLiveState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := &Service{logger: testLogger(), repo: repo, eventBus: eb}

	svc.handleSessionModelsEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		AgentID:   "a1",
		Data: &lifecycle.AgentStreamEventData{
			CurrentModelID: "gpt-5.6-sol",
			SessionModels: []streams.SessionModelInfo{{
				ModelID: "gpt-5.6-sol", Name: "GPT-5.6-Sol", Description: "Provider model help",
				Meta: map[string]any{"provider_internal": "not needed by task selector"},
			}},
			Data: map[string]any{"config_options_settled": true},
			ConfigOptions: []streams.ConfigOption{
				{ID: "model", Category: "model", CurrentValue: "gpt-5.6-sol"},
				{
					Type: "select", ID: "reasoning_effort", Name: "Reasoning effort",
					Description: "Provider option help", CurrentValue: "low",
					Options: []streams.ConfigOptionValue{{Value: "low", Name: "Low", Description: "Provider value help"}},
				},
				{ID: "fast_mode", CurrentValue: "on"},
			},
			ConfigBaselineCandidate: []streams.ConfigOption{
				{ID: "model", Category: "model", CurrentValue: "gpt-5.6-sol"},
				{ID: "reasoning_effort", CurrentValue: "high"},
				{ID: "fast_mode", CurrentValue: "off"},
			},
		},
	})

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	baseline, ok := models.LoadSessionACPConfigBaseline(updated.Metadata)
	require.True(t, ok)
	require.Equal(t, map[string]string{
		"model":            "gpt-5.6-sol",
		"reasoning_effort": "high",
		"fast_mode":        "off",
	}, baseline)
	runtimeConfig, ok := models.LoadSessionRuntimeConfig(updated.Metadata)
	require.True(t, ok)
	require.Equal(t, "low", runtimeConfig.ConfigOptions["reasoning_effort"])
	require.Equal(t, "on", runtimeConfig.ConfigOptions["fast_mode"])
	modelState, ok := lifecycle.LoadSessionModelsSnapshot(updated.Metadata[models.SessionMetaKeyACPModelState])
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", modelState.CurrentModelID)
	require.Equal(t, "Provider model help", modelState.Models[0].Description)
	require.Nil(t, modelState.Models[0].Meta)
	require.Equal(t, "Provider option help", modelState.ConfigOptions[1].Description)
	require.Equal(t, "Provider value help", modelState.ConfigOptions[1].Options[0].Description)

	require.Len(t, eb.events, 1)
	published := eb.events[0].event.Data.(lifecycle.SessionModelsEventPayload)
	require.Equal(t, "low", configOptionValues(published.ConfigOptions)["reasoning_effort"])
	require.Equal(t, "on", configOptionValues(published.ConfigOptions)["fast_mode"])
	require.Equal(t, baseline, published.ConfigBaseline)
}

func TestSettledConfigBaselineConcurrentCaptureReturnsOneWinner(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "t1", "s1", "step1")
	repo := &concurrentBaselineRepo{repoStore: baseRepo, bothLoaded: make(chan struct{})}
	svc := &Service{logger: testLogger(), repo: repo}

	type baselineResult struct {
		values map[string]string
		err    error
	}
	results := make(chan baselineResult, 2)
	for _, reasoning := range []string{"high", "low"} {
		reasoning := reasoning
		go func() {
			values, err := svc.sessionACPConfigBaselineForEvent(ctx, "s1", &lifecycle.AgentStreamEventData{
				Data: map[string]any{"config_options_settled": true},
				ConfigOptions: []streams.ConfigOption{{
					ID: "reasoning_effort", CurrentValue: reasoning,
				}},
			})
			results <- baselineResult{values: values, err: err}
		}()
	}

	captured := make([]baselineResult, 0, 2)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for len(captured) < 2 {
		select {
		case result := <-results:
			captured = append(captured, result)
		case <-timer.C:
			t.Fatal("timed out waiting for concurrent baseline captures")
		}
	}
	require.NoError(t, captured[0].err)
	require.NoError(t, captured[1].err)
	require.Equal(t, captured[0].values, captured[1].values)
}

func TestSessionModelsEventSkipsMutableSnapshotWhenBaselineWriteFails(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "t1", "s1", "step1")
	eb := &recordingEventBus{}
	svc := &Service{logger: testLogger(), repo: failSetBaselineRepo{repoStore: baseRepo}, eventBus: eb}

	svc.handleSessionModelsEvent(ctx, &lifecycle.AgentStreamEventPayload{
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			Data:           map[string]any{"config_options_settled": true},
			CurrentModelID: "gpt-5.6-sol",
			ConfigOptions: []streams.ConfigOption{{
				ID: "reasoning_effort", CurrentValue: "high",
			}},
		},
	})

	updated, err := baseRepo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	_, snapshotStored := updated.Metadata[models.SessionMetaKeyACPModelState]
	require.False(t, snapshotStored)
	require.Empty(t, eb.events)
}

func TestPersistSessionModelAndRuntimeConfigPersistsSnapshotRuntimeConfigAndCache(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", "context_window", map[string]interface{}{"size": int64(256000)}))
	svc := &Service{logger: testLogger(), repo: repo}

	svc.persistSessionModelAndRuntimeConfig(ctx, "s1", "gpt-5.3-codex-spark", "", nil, []streams.ConfigOption{
		{ID: "reasoning_effort", Category: "thought_level", CurrentValue: "low"},
	})

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.3-codex-spark", updated.AgentProfileSnapshot["model"])
	cfg, ok := models.LoadSessionRuntimeConfig(updated.Metadata)
	require.True(t, ok)
	require.Equal(t, "gpt-5.3-codex-spark", cfg.Model)
	require.Equal(t, "low", cfg.ConfigOptions["reasoning_effort"])
	require.Nil(t, updated.Metadata["context_window"])
	model, _ := svc.runtimeModelBySession.Load("s1")
	require.Equal(t, "gpt-5.3-codex-spark", model)
}
