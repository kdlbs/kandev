package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedPriorUserMessages creates n user messages through the repository's
// atomic create boundary (live timestamps), returning them in creation order.
func seedPriorUserMessages(t *testing.T, repo interface {
	CreateMessage(ctx context.Context, message *models.Message) error
}, sessionID, turnID, taskID string, n int) []*models.Message {
	t.Helper()
	seeded := make([]*models.Message, 0, n)
	for i := 0; i < n; i++ {
		msg := &models.Message{
			TaskSessionID: sessionID,
			TaskID:        taskID,
			TurnID:        turnID,
			AuthorType:    models.MessageAuthorUser,
			Content:       "prior prompt",
		}
		if err := repo.CreateMessage(context.Background(), msg); err != nil {
			t.Fatalf("seed prior user message %d: %v", i, err)
		}
		seeded = append(seeded, msg)
	}
	return seeded
}

// eventData extracts the structured data payload from a published bus event.
func eventData(t *testing.T, bus *MockEventBus) map[string]interface{} {
	t.Helper()
	for _, eventType := range []string{events.MessageAdded, events.MessageUpdated} {
		if countEvents(bus.GetPublishedEvents(), eventType) == 1 {
			return singlePublishedEventDataOfType(t, bus, eventType)
		}
	}
	t.Fatalf("expected one message event, got %v", eventTypes(bus.GetPublishedEvents()))
	return nil
}

// TestCreateMessageReturnsAndPublishesPromptIndex: after N-1 prior user
// messages, CreateMessage returns PromptIndex == N and the published
// message.added event carries prompt_index: N.
func TestCreateMessageReturnsAndPublishesPromptIndex(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	prior := seedPriorUserMessages(t, repo, "sess-msg", "turn-msg", "task-msg", 2)
	if prior[0].PromptIndex != 1 || prior[1].PromptIndex != 2 {
		t.Fatalf("prior ordinals = (%d, %d), want (1, 2)", prior[0].PromptIndex, prior[1].PromptIndex)
	}
	bus.ClearEvents()

	message, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: "sess-msg",
		Content:       "third prompt",
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if message.PromptIndex != 3 {
		t.Fatalf("returned PromptIndex = %d, want 3", message.PromptIndex)
	}
	data := eventData(t, bus)
	if got, ok := data["prompt_index"]; !ok || got != 3 {
		t.Fatalf("message.added prompt_index = %#v, want 3", got)
	}
}

// TestCreateMessageWithIDUserReturnsPromptIndex: a fresh user
// CreateMessageWithID returns its ordinal and publishes it.
func TestCreateMessageWithIDUserReturnsPromptIndex(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	seedPriorUserMessages(t, repo, "sess-msg", "turn-msg", "task-msg", 1)
	bus.ClearEvents()

	message, err := svc.CreateMessageWithID(ctx, "fresh-stream-id", &CreateMessageRequest{
		TaskSessionID: "sess-msg",
		Content:       "second prompt",
	})
	if err != nil {
		t.Fatalf("CreateMessageWithID: %v", err)
	}
	if message.PromptIndex != 2 {
		t.Fatalf("returned PromptIndex = %d, want 2", message.PromptIndex)
	}
	data := eventData(t, bus)
	if got, ok := data["prompt_index"]; !ok || got != 2 {
		t.Fatalf("message.added prompt_index = %#v, want 2", got)
	}
}

// TestCreateMessageIdempotentReplayReturnsIndexedMessage: replaying a
// caller-owned ID returns the same indexed row without a duplicate add event.
func TestCreateMessageIdempotentReplayReturnsIndexedMessage(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()

	created, err := svc.CreateMessageIdempotent(ctx, "client-id-1", &CreateMessageRequest{
		TaskSessionID: "sess-msg",
		Content:       "first prompt",
	})
	if err != nil {
		t.Fatalf("first CreateMessageIdempotent: %v", err)
	}
	if created.PromptIndex != 1 {
		t.Fatalf("created PromptIndex = %d, want 1", created.PromptIndex)
	}
	bus.ClearEvents()

	replayed, err := svc.CreateMessageIdempotent(ctx, "client-id-1", &CreateMessageRequest{
		TaskSessionID: "sess-msg",
		Content:       "first prompt",
	})
	if err != nil {
		t.Fatalf("replay CreateMessageIdempotent: %v", err)
	}
	if replayed.ID != "client-id-1" || replayed.PromptIndex != 1 {
		t.Fatalf("replayed = %s (index %d), want client-id-1 with index 1", replayed.ID, replayed.PromptIndex)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 0 {
		t.Fatalf("replay published %v, want no duplicate add event", types)
	}

	// The stored row still carries the ordinal.
	stored, err := repo.GetMessageWithPromptIndex(ctx, "client-id-1")
	if err != nil {
		t.Fatalf("GetMessageWithPromptIndex: %v", err)
	}
	if stored.PromptIndex != 1 {
		t.Fatalf("stored PromptIndex = %d, want 1", stored.PromptIndex)
	}
}

// TestUpdateMessageUserPublishesPromptIndex: a user UpdateMessage re-reads
// the affected row through GetMessageWithPromptIndex so message.updated
// carries the stable ordinal.
func TestUpdateMessageUserPublishesPromptIndex(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	prior := seedPriorUserMessages(t, repo, "sess-msg", "turn-msg", "task-msg", 1)
	bus.ClearEvents()

	message := prior[0]
	message.Content = "edited prompt"
	if err := svc.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("UpdateMessage: %v", err)
	}
	data := eventData(t, bus)
	if got, ok := data["prompt_index"]; !ok || got != 1 {
		t.Fatalf("message.updated prompt_index = %#v, want 1", got)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.MessageUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.MessageUpdated)
	}
}

// TestAgentMessagePathsPublishNoPromptIndex: agent creates and streaming
// content/thinking updates stay on the hot 12-column path and never carry
// prompt_index in their events.
func TestAgentMessagePathsPublishNoPromptIndex(t *testing.T) {
	svc, bus, _ := newMessageTestService(t)
	ctx := context.Background()

	agentMsg, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: "sess-msg",
		AuthorType:    "agent",
		Content:       "agent reply",
	})
	if err != nil {
		t.Fatalf("agent CreateMessage: %v", err)
	}
	if agentMsg.PromptIndex != 0 {
		t.Fatalf("agent create PromptIndex = %d, want 0", agentMsg.PromptIndex)
	}
	data := eventData(t, bus)
	if _, ok := data["prompt_index"]; ok {
		t.Fatalf("agent message.added carries prompt_index %v", data["prompt_index"])
	}
	bus.ClearEvents()

	if err := svc.AppendMessageContent(ctx, agentMsg.ID, " more"); err != nil {
		t.Fatalf("AppendMessageContent: %v", err)
	}
	data = eventData(t, bus)
	if _, ok := data["prompt_index"]; ok {
		t.Fatalf("agent append event carries prompt_index %v", data["prompt_index"])
	}
	bus.ClearEvents()

	if err := svc.AppendThinkingContent(ctx, agentMsg.ID, " reasoning"); err != nil {
		t.Fatalf("AppendThinkingContent: %v", err)
	}
	data = eventData(t, bus)
	if _, ok := data["prompt_index"]; ok {
		t.Fatalf("agent thinking event carries prompt_index %v", data["prompt_index"])
	}
}

// TestSequentialLiveCreatesAssignDistinctOrdinals: rapid consecutive live
// user creates (no explicit timestamps, no clock control) always receive
// distinct, consistent ordinals and events — the repository boundary keeps
// the session's prompt ordering monotonic even when creates land in the same
// microsecond.
func TestSequentialLiveCreatesAssignDistinctOrdinals(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	bus.ClearEvents()

	ids := []string{}
	for i := 0; i < 3; i++ {
		message, err := svc.CreateMessage(ctx, &CreateMessageRequest{
			TaskSessionID: "sess-msg",
			Content:       "prompt",
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		ids = append(ids, message.ID)
		data := eventData(t, bus)
		if got, ok := data["prompt_index"]; !ok || got != i+1 {
			t.Fatalf("create %d event prompt_index = %#v, want %d", i, got, i+1)
		}
		bus.ClearEvents()
	}

	// Reread ordinals are distinct and consistent with the events.
	seen := map[int]bool{}
	for _, id := range ids {
		stored, err := repo.GetMessageWithPromptIndex(ctx, id)
		if err != nil {
			t.Fatalf("GetMessageWithPromptIndex(%s): %v", id, err)
		}
		if stored.PromptIndex < 1 || stored.PromptIndex > 3 {
			t.Fatalf("stored ordinal %d out of range", stored.PromptIndex)
		}
		if seen[stored.PromptIndex] {
			t.Fatalf("duplicate ordinal %d for %s", stored.PromptIndex, id)
		}
		seen[stored.PromptIndex] = true
	}
	if len(seen) != 3 {
		t.Fatalf("distinct ordinals = %d, want 3", len(seen))
	}
}

// capturingMessagesRepo wraps the real MessageRepository and records whether
// the service passes a ZERO CreatedAt into CreateMessage — the live/import
// discriminator for the atomic per-session create boundary. A pre-populated
// timestamp would classify the message as an explicit import and reject
// same-microsecond (or backward-clock) live creates instead of advancing.
type capturingMessagesRepo struct {
	repository.MessageRepository
	createdAtZeroAtRepoEntry []bool
}

// CreateMessage persists a message in the fake repository, mirroring the repository contract.
func (m *capturingMessagesRepo) CreateMessage(ctx context.Context, message *models.Message) error {
	m.createdAtZeroAtRepoEntry = append(m.createdAtZeroAtRepoEntry, message.CreatedAt.IsZero())
	return m.MessageRepository.CreateMessage(ctx, message)
}

// TestServiceUserCreateLeavesTimestampToBoundary pins the review fix: the
// service must NOT pre-populate CreatedAt for user messages, so the
// repository boundary sees a zero timestamp and runs the LIVE branch (the
// one-tick advance for colliding keys) instead of the explicit-import branch.
func TestServiceUserCreateLeavesTimestampToBoundary(t *testing.T) {
	_, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-bound", Name: "Boundary"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-bound", WorkspaceID: "ws-bound", Name: "flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-bound", WorkspaceID: "ws-bound", WorkflowID: "wf-bound", WorkflowStepID: "step-1",
		Title: "Boundary", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-bound", TaskID: "task-bound", State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	seedTurn(t, repo, "turn-bound", "sess-bound", "task-bound")

	wrapped := &capturingMessagesRepo{MessageRepository: repo}
	bus := NewMockEventBus()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewService(Repos{
		Workspaces:        repo,
		Tasks:             repo,
		TaskRepos:         repo,
		Workflows:         repo,
		Messages:          wrapped,
		Turns:             repo,
		Sessions:          repo,
		GitSnapshots:      repo,
		RepoEntities:      repo,
		RepositoryCleanup: repo,
		Executors:         repo,
		Environments:      repo,
		TaskEnvironments:  repo,
		Reviews:           repo,
		ResourceCleanups:  repo,
	}, bus, log, RepositoryDiscoveryConfig{})

	message, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: "sess-bound",
		Content:       "live prompt",
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if len(wrapped.createdAtZeroAtRepoEntry) != 1 || !wrapped.createdAtZeroAtRepoEntry[0] {
		t.Fatalf("service passed CreatedAt.IsZero()=false to the repository; live boundary would misclassify the message as an explicit import")
	}
	if message.PromptIndex != 1 {
		t.Fatalf("returned PromptIndex = %d, want 1", message.PromptIndex)
	}
	if message.CreatedAt.IsZero() {
		t.Fatal("the repository boundary must assign CreatedAt for live creates")
	}
}
