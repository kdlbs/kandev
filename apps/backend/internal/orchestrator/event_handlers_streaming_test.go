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

type recordingClarificationCanceller struct {
	sessions []string
}

func (c *recordingClarificationCanceller) DetachSessionAndNotify(_ context.Context, sessionID string) int {
	c.sessions = append(c.sessions, sessionID)
	return 1
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
	seedRunModeAutomationSession(t, repo, "t-auto-terminal", "s-auto-terminal", "exec-auto-terminal")

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

	require.NoError(t, svc.setSessionStarting(ctx, "t1", session, true))

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

	require.NoError(t, svc.setSessionStarting(ctx, "t1", session, false))

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, updated.State)
	require.Empty(t, taskRepo.stateWrites)
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

	require.Error(t, svc.setSessionStarting(ctx, "t1", &next, true))

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, updated.State)
	require.Empty(t, taskRepo.stateWrites)
}

func TestSetSessionStartingRejectsCancelledTerminalResumeWithoutPromotion(t *testing.T) {
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

	require.Error(t, svc.setSessionStarting(ctx, "t1", &next, false))

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

	require.NoError(t, svc.setSessionStarting(ctx, "t1", &next, false))

	updated, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, updated.State)
	require.Nil(t, updated.CompletedAt)
	require.Empty(t, taskRepo.stateWrites)
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

func seedRunModeAutomationSession(
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
		IsEphemeral: true,
		Origin:      models.TaskOriginAutomationRun,
		Metadata: map[string]interface{}{
			"execution_mode": string(automation.ExecutionModeRun),
		},
		CreatedAt: now,
		UpdatedAt: now,
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

func TestHandleCompleteStreamEvent_RunModeAutomationStopsAndFinalizes(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedRunModeAutomationSession(t, repo, "t-auto-run", "s-auto-run", "exec-auto-run")

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
	require.Equal(t, models.TaskSessionStateCompleted, gotSession.State)

	mgr.mu.Lock()
	stopCalls := append([]stopAgentCall(nil), mgr.stopAgentArgs...)
	mgr.mu.Unlock()
	require.Equal(t, []stopAgentCall{{ExecutionID: "exec-auto-run"}}, stopCalls)
}

func TestHandleCompleteStreamEvent_RunModeAutomationFailureStopsAndFinalizes(t *testing.T) {
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
			seedRunModeAutomationSession(t, repo, taskID, sessionID, executionID)

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
