package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

type managedCompletionFailingTurnService struct {
	*repoTurnService
	failComplete bool
}

func (s *managedCompletionFailingTurnService) CompleteTurn(ctx context.Context, turnID string) error {
	if s.failComplete {
		return errors.New("injected turn completion failure")
	}
	return s.repoTurnService.CompleteTurn(ctx, turnID)
}

type persistedManagedStreamMessagePublisher struct {
	*mockMessageCreator
	service *taskservice.Service
}

func (p *persistedManagedStreamMessagePublisher) PublishManagedAgentStreamMessage(
	ctx context.Context,
	messageID string,
	updated bool,
) error {
	message, err := p.service.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}
	eventType := events.MessageAdded
	if updated {
		eventType = events.MessageUpdated
	}
	return p.service.PublishMessageEvent(ctx, eventType, message)
}

type managedCompletionReceiptAgentManager struct {
	*mockAgentManager
	pending  bool
	ackCalls int
}

func (m *managedCompletionReceiptAgentManager) ManagedAgentCompletionPending(context.Context, string) (bool, error) {
	return m.pending, nil
}

func (m *managedCompletionReceiptAgentManager) AcknowledgeManagedAgentCompletion(context.Context, string) error {
	m.ackCalls++
	m.pending = false
	return nil
}

func TestManagedCompletionReplayIsAcknowledgedOnlyOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-managed-completion", "s-managed-completion", "step1")
	manager := &managedCompletionReceiptAgentManager{mockAgentManager: &mockAgentManager{}, pending: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.turnService = &repoTurnService{repo: repo}
	eventBus := &recordingEventBus{}
	svc.eventBus = eventBus
	turn, err := svc.turnService.StartTurn(ctx, "s-managed-completion")
	require.NoError(t, err)
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID: "t-managed-completion", SessionID: "s-managed-completion", ExecutionID: "exec-managed-completion",
		ManagedAgentOperationID: "operation-managed-completion",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete, TurnID: turn.ID,
			Data: map[string]interface{}{"remote_terminal": true, "stop_reason": "end_turn"},
		},
	}

	svc.handleAgentStreamEvent(ctx, payload)
	effectsAfterFirstDelivery := len(eventBus.events)
	svc.handleAgentStreamEvent(ctx, payload)

	require.Equal(t, 1, manager.ackCalls, "the completion receipt should be acknowledged once")
	var turnMessageEvents int
	for _, event := range eventBus.events {
		if event.subject == events.AgentTurnMessageSaved {
			turnMessageEvents++
		}
	}
	require.Equal(t, 1, turnMessageEvents, "replayed completion must not republish turn completion")
	require.Len(t, eventBus.events, effectsAfterFirstDelivery, "replayed completion must not repeat workflow or queue event effects")
	active, err := svc.turnService.GetActiveTurn(ctx, "s-managed-completion")
	require.NoError(t, err)
	require.Nil(t, active, "the captured turn should be closed by the first delivery")
}

func TestManagedCompletionIsReplayableWhenCapturedTurnCannotClose(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-managed-completion-retry", "s-managed-completion-retry", "step1")
	manager := &managedCompletionReceiptAgentManager{mockAgentManager: &mockAgentManager{}, pending: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	turns := &managedCompletionFailingTurnService{
		repoTurnService: &repoTurnService{repo: repo}, failComplete: true,
	}
	svc.turnService = turns
	eventBus := &recordingEventBus{}
	svc.eventBus = eventBus
	turn, err := turns.StartTurn(ctx, "s-managed-completion-retry")
	require.NoError(t, err)
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID: "t-managed-completion-retry", SessionID: "s-managed-completion-retry", ExecutionID: "exec-retry",
		ManagedAgentOperationID: "operation-retry",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete, TurnID: turn.ID,
			Data: map[string]interface{}{"remote_terminal": true, "stop_reason": "end_turn"},
		},
	}

	svc.handleAgentStreamEvent(ctx, payload)
	require.Zero(t, manager.ackCalls, "failed captured-turn close must leave durable completion pending")
	require.True(t, manager.pending)
	require.Empty(t, eventsForSubject(eventBus.events, events.AgentTurnMessageSaved), "failed turn close must stop downstream completion effects")

	turns.failComplete = false
	svc.handleAgentStreamEvent(ctx, payload)
	require.Equal(t, 1, manager.ackCalls, "successful replay should acknowledge once")
	require.Len(t, eventsForSubject(eventBus.events, events.AgentTurnMessageSaved), 1)
	svc.handleAgentStreamEvent(ctx, payload)
	require.Equal(t, 1, manager.ackCalls)
	require.Len(t, eventsForSubject(eventBus.events, events.AgentTurnMessageSaved), 1,
		"replay after acknowledgment must not repeat workflow or queue events")
}

func eventsForSubject(recorded []recordedEvent, subject string) []*bus.Event {
	var events []*bus.Event
	for _, event := range recorded {
		if event.subject == subject {
			events = append(events, event.event)
		}
	}
	return events
}

func commitManagedStreamFrame(
	t *testing.T,
	ctx context.Context,
	repo *sqliterepo.Repository,
	service *Service,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	turnID, eventID, frameType, messageID, content, status string,
	appendMessage bool,
) {
	t.Helper()
	messageType := models.MessageTypeMessage
	if frameType == "tool_call" || frameType == "tool_update" {
		messageType = models.MessageTypeToolCall
	}
	message := &models.Message{
		ID: messageID, TaskSessionID: binding.SessionID, TaskID: binding.TaskID,
		TurnID: turnID, AuthorType: models.MessageAuthorAgent, AuthorID: binding.ExecutionID,
		Type: messageType, Content: content,
	}
	if _, err := repo.CommitManagedAgentStreamEvent(ctx, models.ManagedAgentStreamEvent{
		BindingID: binding.ID, OperationID: operation.ID, RemoteRunID: operation.RemoteRunID,
		EventID: eventID, EventType: frameType, DispatchGeneration: operation.DispatchGeneration,
		Message: message, AppendMessage: appendMessage, AssistantMessageStarted: true,
	}); err != nil {
		t.Fatalf("commit managed stream frame %s: %v", eventID, err)
	}
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID: binding.TaskID, SessionID: binding.SessionID, ExecutionID: binding.ExecutionID,
		ManagedAgentOperationID: operation.ID,
		Data: &lifecycle.AgentStreamEventData{
			Type: frameType, TurnID: turnID, Text: content, MessageID: messageID,
			MessageType: string(messageType), IsAppend: appendMessage,
			ToolCallID: "tool-call-1", ToolName: content, ToolTitle: content, ToolStatus: status,
		},
	}
	if frameType == "assistant" {
		payload.Data.Type = "message_streaming"
	}
	service.handleAgentStreamEvent(ctx, payload)
}

func TestManagedStreamFramesUsePersistedMessagesWithoutAppendingTwice(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-managed-stream", "s-managed-stream", "step1")
	turns := &repoTurnService{repo: repo}
	turn, err := turns.StartTurn(ctx, "s-managed-stream")
	require.NoError(t, err)
	binding, operation := seedAcceptedManagedStream(t, ctx, repo, turn.ID)
	eventBus := &recordingEventBus{}
	messageService := newTaskServiceStateRepository(repo, eventBus).service
	publisher := &persistedManagedStreamMessagePublisher{
		mockMessageCreator: &mockMessageCreator{}, service: messageService,
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = publisher
	svc.turnService = turns
	svc.eventBus = eventBus
	assistantID := "cursor-cloud-assistant-" + operation.ID
	commitManagedStreamFrame(t, ctx, repo, svc, binding, operation, turn.ID, "assistant-1",
		"assistant", assistantID, "Hello ", "", false)
	commitManagedStreamFrame(t, ctx, repo, svc, binding, operation, turn.ID, "assistant-2",
		"assistant", assistantID, "world", "", true)
	toolID := "cursor-cloud-tool-" + operation.ID + "-tool-call-1"
	commitManagedStreamFrame(t, ctx, repo, svc, binding, operation, turn.ID, "tool-call",
		"tool_call", toolID, "Search", "running", false)
	commitManagedStreamFrame(t, ctx, repo, svc, binding, operation, turn.ID, "tool-update",
		"tool_update", toolID, "Search", "completed", false)

	messages, err := repo.ListMessages(ctx, binding.SessionID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	stored := map[string]*models.Message{}
	for _, message := range messages {
		stored[message.ID] = message
	}
	require.Equal(t, "Hello world", stored[assistantID].Content)
	require.Equal(t, "Search", stored[toolID].Content)
	require.Zero(t, publisher.agentStreamWrites, "the runtime projection already owns assistant transcript persistence")
	require.Zero(t, publisher.toolCallWrites, "the runtime projection already owns tool-call persistence")
	require.Zero(t, publisher.toolUpdateWrites, "the runtime projection already owns tool-update persistence")
}

func seedAcceptedManagedStream(
	t *testing.T,
	ctx context.Context,
	repo *sqliterepo.Repository,
	turnID string,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation) {
	t.Helper()
	now := time.Now().UTC()
	binding := &models.ManagedAgentBinding{
		ID: "managed-stream-binding", SessionID: "s-managed-stream", TaskID: "t-managed-stream",
		WorkspaceID: "ws1", UserID: "user-1", ExecutionID: "cloud-execution",
		ProviderKind: "cursor_cloud", ExecutorID: "cursor-cloud", ExecutorProfileID: "profile-1",
		CredentialRef: "secret-ref", RemoteAgentID: "bc-11111111-1111-4111-8111-111111111111",
		Lifecycle: models.ManagedAgentBindingCreating,
		Launch: models.ManagedAgentLaunchSnapshot{
			RepositoryID: "repo-1", RepositoryURL: "https://github.com/acme/repo",
			StartingRef: "main", Model: "model-1", CallbackURL: "https://kandev.example",
		},
	}
	operation := &models.ManagedAgentOperation{
		ID: "managed-stream-operation", BindingID: binding.ID, PromptTurnID: "managed-stream-turn",
		Kind: models.ManagedAgentOperationCreate, RequestDigest: "managed-stream-digest",
		RequestSnapshot: models.ManagedAgentRequestSnapshot{
			Prompt: "stream response", TurnID: turnID, RepositoryURL: binding.Launch.RepositoryURL,
			StartingRef: "main", Model: "model-1", CallbackURL: binding.Launch.CallbackURL,
		},
	}
	reservedBinding, reservedOperation, _, err := repo.ReserveManagedAgentStart(
		ctx, binding, operation, "managed-stream-worker", now.Add(time.Minute),
	)
	require.NoError(t, err)
	_, err = repo.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: reservedOperation.ID, ExpectedRevision: reservedOperation.Revision,
		ExpectedBindingRevision: reservedBinding.Revision, LeaseOwner: reservedBinding.DispatchOwner,
		State: models.ManagedAgentSubmissionAccepted, RemoteRunID: "managed-stream-run",
	})
	require.NoError(t, err)
	acceptedBinding, err := repo.GetManagedAgentBindingBySession(ctx, binding.SessionID)
	require.NoError(t, err)
	acceptedOperation, err := repo.GetManagedAgentLatestOperation(ctx, acceptedBinding.ID)
	require.NoError(t, err)
	return acceptedBinding, acceptedOperation
}

func TestDeferCompleteEventStateTransitionForRemoteTerminal(t *testing.T) {
	svc := &Service{logger: testLogger()}
	session := &models.TaskSession{State: models.TaskSessionStateRunning}
	remoteComplete := &lifecycle.AgentStreamEventPayload{
		Data: &lifecycle.AgentStreamEventData{Data: map[string]interface{}{"remote_terminal": true}},
	}
	ordinaryComplete := &lifecycle.AgentStreamEventPayload{Data: &lifecycle.AgentStreamEventData{}}

	require.False(t, svc.deferCompleteEventStateTransition(remoteComplete, session))
	require.True(t, svc.deferCompleteEventStateTransition(ordinaryComplete, session))
}
