package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newMessageTestService seeds one workspace/workflow/task/session so message
// writes have a real session and task to hang off.
func newMessageTestService(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-msg", Name: "Messages"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-msg", WorkspaceID: "ws-msg", Name: "flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-msg", WorkspaceID: "ws-msg", WorkflowID: "wf-msg", WorkflowStepID: "step-1",
		Title: "Messages", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-msg", TaskID: "task-msg", Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/workspace/messages",
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-msg", TaskID: "task-msg", TaskEnvironmentID: "env-msg", State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	seedTurn(t, repo, "turn-msg", "sess-msg", "task-msg")
	bus.ClearEvents()
	return svc, bus, repo
}

// seedTurn inserts the turn row that every message must reference.
func seedTurn(t *testing.T, repo *sqliterepo.Repository, turnID, sessionID, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.CreateTurn(context.Background(), &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: taskID, StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed turn %s: %v", turnID, err)
	}
}

func seedMessage(t *testing.T, repo *sqliterepo.Repository, message *models.Message) *models.Message {
	t.Helper()
	if message.TurnID == "" {
		message.TurnID = "turn-msg"
	}
	if message.TaskSessionID == "" {
		message.TaskSessionID = "sess-msg"
	}
	if message.TaskID == "" {
		message.TaskID = "task-msg"
	}
	if message.Type == "" {
		message.Type = models.MessageTypeMessage
	}
	if message.AuthorType == "" {
		message.AuthorType = models.MessageAuthorAgent
	}
	if err := repo.CreateMessage(context.Background(), message); err != nil {
		t.Fatalf("seed message %s: %v", message.ID, err)
	}
	return message
}

func TestCreateMessageAppliesDefaultsStartsTurnAndPublishes(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()

	message, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: "sess-msg",
		Content:       "hello",
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if message.AuthorType != models.MessageAuthorUser {
		t.Fatalf("author type = %q, want the user default", message.AuthorType)
	}
	if message.Type != models.MessageTypeMessage {
		t.Fatalf("type = %q, want the message default", message.Type)
	}
	if message.TaskID != "task-msg" {
		t.Fatalf("task id = %q, want it resolved from the session", message.TaskID)
	}
	if message.TurnID == "" {
		t.Fatal("turn id must be filled by starting a turn")
	}

	stored, err := repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.Content != "hello" || stored.TurnID != message.TurnID {
		t.Fatalf("persisted message = %+v", stored)
	}
	turn, err := repo.GetTurn(ctx, message.TurnID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.CompletedAt != nil {
		t.Fatal("a normal message must start an open turn")
	}

	types := eventTypes(bus.GetPublishedEvents())
	if len(types) == 0 || types[len(types)-1] != events.MessageAdded {
		t.Fatalf("published %v, want a trailing %s", types, events.MessageAdded)
	}
}

func TestCreateMessageHonorsExplicitFields(t *testing.T) {
	svc, _, _ := newMessageTestService(t)
	ctx := context.Background()

	message, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: "sess-msg",
		TaskID:        "task-msg",
		TurnID:        "turn-msg",
		AuthorType:    "agent",
		AuthorID:      "agent-1",
		Type:          string(models.MessageTypeToolExecute),
		Content:       "run",
		RequestsInput: true,
		Metadata:      map[string]interface{}{"tool_call_id": "tc-1"},
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if message.AuthorType != models.MessageAuthorAgent || message.AuthorID != "agent-1" {
		t.Fatalf("author = %q/%q", message.AuthorType, message.AuthorID)
	}
	if message.Type != models.MessageTypeToolExecute {
		t.Fatalf("type = %q", message.Type)
	}
	if message.TurnID != "turn-msg" {
		t.Fatalf("turn id = %q, want the supplied turn preserved", message.TurnID)
	}
	if !message.RequestsInput {
		t.Fatal("requests_input must be carried through")
	}
	if message.Metadata["tool_call_id"] != "tc-1" {
		t.Fatalf("metadata = %v", message.Metadata)
	}
}

func TestCreateMessageWithCompletedTurnRecordsClosedTurn(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()

	message, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: "sess-msg",
		Content:       "already done",
		CompletedTurn: true,
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	turn, err := repo.GetTurn(ctx, message.TurnID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.CompletedAt == nil {
		t.Fatal("CompletedTurn must create an already-completed turn")
	}
	if turn.TaskSessionID != "sess-msg" || turn.TaskID != "task-msg" {
		t.Fatalf("turn = %+v, want it bound to the session's task", turn)
	}
}

func TestCreateMessageFailsWhenSessionIsUnknown(t *testing.T) {
	svc, bus, _ := newMessageTestService(t)

	if _, err := svc.CreateMessage(context.Background(), &CreateMessageRequest{
		TaskSessionID: "sess-missing", Content: "x",
	}); err == nil {
		t.Fatal("CreateMessage must fail for an unknown session")
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("failed create published %d events, want none", len(published))
	}
}

func TestCreateMessageIdempotentReplaysExistingRow(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateMessageIdempotent(ctx, "", &CreateMessageRequest{TaskSessionID: "sess-msg"}); err == nil {
		t.Fatal("an empty id must be rejected")
	}

	first, err := svc.CreateMessageIdempotent(ctx, "msg-idem", &CreateMessageRequest{
		TaskSessionID: "sess-msg", Content: "first",
	})
	if err != nil {
		t.Fatalf("first CreateMessageIdempotent: %v", err)
	}
	if first.ID != "msg-idem" {
		t.Fatalf("id = %q, want the caller-owned id", first.ID)
	}
	afterFirst := len(bus.GetPublishedEvents())

	replay, err := svc.CreateMessageIdempotent(ctx, "msg-idem", &CreateMessageRequest{
		TaskSessionID: "sess-msg", Content: "second",
	})
	if err != nil {
		t.Fatalf("replayed CreateMessageIdempotent: %v", err)
	}
	if replay.Content != "first" {
		t.Fatalf("replay content = %q, want the committed first row", replay.Content)
	}
	if got := len(bus.GetPublishedEvents()); got != afterFirst {
		t.Fatalf("replay published %d extra events, want none", got-afterFirst)
	}

	all, err := repo.ListMessages(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("stored %d messages, want the replay to be a no-op", len(all))
	}
}

func TestCreateMessageWithIDPersistsCallerID(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()

	message, err := svc.CreateMessageWithID(ctx, "msg-stream", &CreateMessageRequest{
		TaskSessionID: "sess-msg", AuthorType: "agent", Content: "chunk",
	})
	if err != nil {
		t.Fatalf("CreateMessageWithID: %v", err)
	}
	if message.ID != "msg-stream" || message.AuthorType != models.MessageAuthorAgent {
		t.Fatalf("message = %+v", message)
	}
	if message.TurnID == "" {
		t.Fatal("streaming create must resolve a turn")
	}
	if _, err := repo.GetMessage(ctx, "msg-stream"); err != nil {
		t.Fatalf("persisted lookup: %v", err)
	}
	types := eventTypes(bus.GetPublishedEvents())
	if len(types) == 0 || types[len(types)-1] != events.MessageAdded {
		t.Fatalf("published %v, want a trailing %s", types, events.MessageAdded)
	}
}

func TestGetMessageScopesToSessionOwner(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	seedTurn(t, repo, "turn-b", "sess-b", "task-b")
	seedMessage(t, repo, &models.Message{
		ID: "msg-b", TaskSessionID: "sess-b", TaskID: "task-b", TurnID: "turn-b", Content: "secret output",
	})

	if _, err := svc.GetMessage(ctxAs("user-a"), "msg-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign GetMessage = %v, want ErrTaskNotFound", err)
	}
	got, err := svc.GetMessage(ctxAs("user-b"), "msg-b")
	if err != nil {
		t.Fatalf("owner GetMessage: %v", err)
	}
	if got.Content != "secret output" {
		t.Fatalf("content = %q", got.Content)
	}
	if _, err := svc.GetMessage(context.Background(), "msg-b"); err != nil {
		t.Fatalf("internal GetMessage: %v", err)
	}
}

func TestListMessagesPaginatedClampsLimit(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	for _, id := range []string{"m1", "m2", "m3"} {
		seedMessage(t, repo, &models.Message{ID: id, Content: id})
	}

	all, _, err := svc.ListMessagesPaginated(ctx, ListMessagesRequest{TaskSessionID: "sess-msg"})
	if err != nil {
		t.Fatalf("ListMessagesPaginated: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("unpaginated list = %d, want 3", len(all))
	}

	page, hasMore, err := svc.ListMessagesPaginated(ctx, ListMessagesRequest{
		TaskSessionID: "sess-msg", Limit: 2,
	})
	if err != nil {
		t.Fatalf("paginated: %v", err)
	}
	if len(page) != 2 || !hasMore {
		t.Fatalf("page = %d messages, hasMore = %v; want 2 and true", len(page), hasMore)
	}

	clamped, _, err := svc.ListMessagesPaginated(ctx, ListMessagesRequest{
		TaskSessionID: "sess-msg", Limit: MaxMessagesPageSize + 500,
	})
	if err != nil {
		t.Fatalf("clamped: %v", err)
	}
	if len(clamped) != 3 {
		t.Fatalf("clamped list = %d, want all 3", len(clamped))
	}
}

func TestListMessagesForPluginFiltersBySession(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	seedMessage(t, repo, &models.Message{ID: "m-plugin", Content: "visible"})

	got, err := svc.ListMessagesForPlugin(context.Background(), models.PluginMessageFilter{
		SessionIDs: []string{"sess-msg"}, Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListMessagesForPlugin: %v", err)
	}
	if len(got) != 1 || got[0].ID != "m-plugin" {
		t.Fatalf("plugin messages = %+v, want just m-plugin", got)
	}
}

func TestDeleteMessagePublishesDeletedEvent(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{ID: "msg-del", Content: "bye"})

	if err := svc.DeleteMessage(ctx, "msg-del"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.MessageDeleted {
		t.Fatalf("published %v, want exactly one %s", types, events.MessageDeleted)
	}
	if _, err := repo.GetMessage(ctx, "msg-del"); err == nil {
		t.Fatal("message row must be gone")
	}
}

func TestUpdateMessagePublishesUpdatedEvent(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	message := seedMessage(t, repo, &models.Message{ID: "msg-upd", Content: "before"})

	message.Content = "after"
	if err := svc.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("UpdateMessage: %v", err)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.MessageUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.MessageUpdated)
	}
	stored, err := repo.GetMessage(ctx, "msg-upd")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.Content != "after" {
		t.Fatalf("content = %q, want after", stored.Content)
	}
}

func TestAppendMessageContentAccumulatesAndPublishes(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{ID: "msg-append", Content: "Hel"})

	if err := svc.AppendMessageContent(ctx, "msg-append", "lo "); err != nil {
		t.Fatalf("AppendMessageContent: %v", err)
	}
	if err := svc.AppendMessageContent(ctx, "msg-append", "world"); err != nil {
		t.Fatalf("second AppendMessageContent: %v", err)
	}
	stored, err := repo.GetMessage(ctx, "msg-append")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.Content != "Hello world" {
		t.Fatalf("content = %q, want Hello world", stored.Content)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 2 {
		t.Fatalf("published %v, want one update per append", types)
	}

	if err := svc.AppendMessageContent(ctx, "msg-missing", "x"); err == nil {
		t.Fatal("appending to an unknown message must fail")
	}
}

func TestAppendThinkingContentInitializesAndAccumulates(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{ID: "msg-think", Content: ""})

	if err := svc.AppendThinkingContent(ctx, "msg-think", "step 1. "); err != nil {
		t.Fatalf("AppendThinkingContent: %v", err)
	}
	stored, err := repo.GetMessage(ctx, "msg-think")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.Metadata["thinking"] != "step 1. " {
		t.Fatalf("thinking = %v, want the first chunk on a fresh metadata map", stored.Metadata["thinking"])
	}

	if err := svc.AppendThinkingContent(ctx, "msg-think", "step 2."); err != nil {
		t.Fatalf("second AppendThinkingContent: %v", err)
	}
	stored, err = repo.GetMessage(ctx, "msg-think")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.Metadata["thinking"] != "step 1. step 2." {
		t.Fatalf("thinking = %v, want both chunks", stored.Metadata["thinking"])
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 2 {
		t.Fatalf("published %v, want one update per append", types)
	}

	if err := svc.AppendThinkingContent(ctx, "msg-missing", "x"); err == nil {
		t.Fatal("appending thinking to an unknown message must fail")
	}
}

func TestUpdateToolCallMessageAppliesStatusResultAndTitle(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{
		ID: "msg-tool", Type: models.MessageTypeToolExecute, Content: "old title",
		Metadata: map[string]interface{}{"tool_call_id": "tc-1", "status": "running"},
	})

	if err := svc.UpdateToolCallMessage(ctx, "sess-msg", "tc-1", "complete", "exit 0", "new title", nil); err != nil {
		t.Fatalf("UpdateToolCallMessage: %v", err)
	}
	stored, err := repo.GetMessage(ctx, "msg-tool")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.Metadata["status"] != "complete" {
		t.Fatalf("status = %v, want complete", stored.Metadata["status"])
	}
	if stored.Metadata["result"] != "exit 0" {
		t.Fatalf("result = %v, want exit 0", stored.Metadata["result"])
	}
	if stored.Content != "new title" || stored.Metadata["title"] != "new title" {
		t.Fatalf("title not applied: content=%q metadata=%v", stored.Content, stored.Metadata["title"])
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.MessageUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.MessageUpdated)
	}
}

func TestUpdateToolCallMessageRetypesFromNormalizedKind(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{
		ID: "msg-normalized", Type: models.MessageTypeToolExecute, Content: "cmd",
		Metadata: map[string]interface{}{"tool_call_id": "tc-norm"},
	})

	normalized := streams.NewShellExec("make test", "/workspace", "", 0, false)
	if err := svc.UpdateToolCallMessage(ctx, "sess-msg", "tc-norm", "complete", "", "", normalized); err != nil {
		t.Fatalf("UpdateToolCallMessage: %v", err)
	}
	stored, err := repo.GetMessage(ctx, "msg-normalized")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	wantType := models.MessageType(normalized.Kind().ToMessageType())
	if stored.Type != wantType {
		t.Fatalf("type = %q, want %q from the normalized kind", stored.Type, wantType)
	}
	if stored.Metadata["normalized"] == nil {
		t.Fatal("normalized payload must be stored in metadata")
	}
}

func TestApplyToolCallMessageUpdateRefusesPermissionRequests(t *testing.T) {
	svc, _, _ := newMessageTestService(t)
	message := &models.Message{
		ID: "msg-perm", Type: models.MessageTypePermissionRequest, Content: "Allow?",
		Metadata: map[string]interface{}{"status": "approved", "tool_call_id": "tc-1"},
	}

	svc.applyToolCallMessageUpdate(message, "error", "result", "new title", nil)

	if message.Metadata["status"] != "approved" {
		t.Fatalf("status = %v, want the user's decision preserved", message.Metadata["status"])
	}
	if message.Type != models.MessageTypePermissionRequest {
		t.Fatalf("type = %q, want permission_request preserved", message.Type)
	}
	if message.Content != "Allow?" {
		t.Fatalf("content = %q, want it untouched", message.Content)
	}
	if _, ok := message.Metadata["result"]; ok {
		t.Fatal("result must not be written onto a permission request")
	}
}

func TestUpdateToolCallMessageWithCreateFallsBackToCreation(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()

	normalized := streams.NewShellExec("ls", "/workspace", "", 0, false)
	err := svc.UpdateToolCallMessageWithCreate(ctx, "sess-msg", "tc-new", "tc-parent", "running", "",
		"List files", normalized, "task-msg", "", string(models.MessageTypeToolExecute))
	if err != nil {
		t.Fatalf("UpdateToolCallMessageWithCreate: %v", err)
	}

	created, err := repo.GetMessageByToolCallID(ctx, "sess-msg", "tc-new")
	if err != nil {
		t.Fatalf("GetMessageByToolCallID: %v", err)
	}
	if created.Type != models.MessageTypeToolExecute || created.Content != "List files" {
		t.Fatalf("created message = %+v", created)
	}
	if created.Metadata["status"] != "running" || created.Metadata["title"] != "List files" {
		t.Fatalf("metadata = %v", created.Metadata)
	}
	if created.Metadata["parent_tool_call_id"] != "tc-parent" {
		t.Fatalf("parent tool call = %v, want tc-parent for subagent nesting", created.Metadata["parent_tool_call_id"])
	}
	if created.Metadata["normalized"] == nil {
		t.Fatal("normalized payload must be carried into the fallback message")
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) == 0 || types[len(types)-1] != events.MessageAdded {
		t.Fatalf("published %v, want the fallback create to publish %s", types, events.MessageAdded)
	}
}

func TestUpdateToolCallMessageWithoutFallbackInfoReturnsError(t *testing.T) {
	svc, bus, _ := newMessageTestService(t)

	err := svc.UpdateToolCallMessage(context.Background(), "sess-msg", "tc-unknown", "complete", "", "", nil)
	if err == nil {
		t.Fatal("updating an unknown tool call without create info must fail")
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("failed update published %d events, want none", len(published))
	}
}

func TestUpdatePermissionMessageSetsStatus(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{
		ID: "msg-permission", Type: models.MessageTypePermissionRequest, Content: "Allow?",
		Metadata: map[string]interface{}{"request_id": "req-1", "pending_id": "pend-1"},
	})

	if err := svc.UpdatePermissionMessage(ctx, "task-msg", "sess-msg", "req-1", "pend-1", models.PermissionStatusApproved); err != nil {
		t.Fatalf("UpdatePermissionMessage: %v", err)
	}
	stored, err := repo.GetMessage(ctx, "msg-permission")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.Metadata["status"] != string(models.PermissionStatusApproved) {
		t.Fatalf("status = %v, want approved", stored.Metadata["status"])
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.MessageUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.MessageUpdated)
	}

	if err := svc.UpdatePermissionMessage(ctx, "task-msg", "sess-msg", "req-1", "pend-missing", models.PermissionStatusApproved); err == nil {
		t.Fatal("an unknown pending id must fail")
	}

	if err := svc.UpdatePermissionMessage(ctx, "task-msg", "sess-msg", "req-stale", "pend-1", models.PermissionStatusApproved); err == nil {
		t.Fatal("a stale request id reusing the same pending id must not match")
	}
}

func TestUpdatePermissionMessageExpiryCancelsRelatedToolCall(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{
		ID: "msg-perm-expire", Type: models.MessageTypePermissionRequest, Content: "Allow?",
		Metadata: map[string]interface{}{"request_id": "req-exp", "pending_id": "pend-exp", "tool_call_id": "tc-exp"},
	})
	seedMessage(t, repo, &models.Message{
		ID: "msg-tool-expire", Type: models.MessageTypeToolExecute, Content: "run",
		Metadata: map[string]interface{}{"tool_call_id": "tc-exp", "status": "running"},
	})

	if err := svc.UpdatePermissionMessage(ctx, "task-msg", "sess-msg", "req-exp", "pend-exp", models.PermissionStatusExpired); err != nil {
		t.Fatalf("UpdatePermissionMessage: %v", err)
	}

	permission, err := repo.GetMessage(ctx, "msg-perm-expire")
	if err != nil {
		t.Fatalf("GetMessage permission: %v", err)
	}
	if permission.Metadata["status"] != string(models.PermissionStatusExpired) {
		t.Fatalf("permission status = %v, want expired", permission.Metadata["status"])
	}
	tool, err := repo.GetMessage(ctx, "msg-tool-expire")
	if err != nil {
		t.Fatalf("GetMessage tool: %v", err)
	}
	if tool.Metadata["status"] != "error" {
		t.Fatalf("tool status = %v, want the related tool call cancelled", tool.Metadata["status"])
	}
}

func TestUpdateClarificationMessageForQuestionStoresAnswer(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{
		ID: "msg-clarify", Content: "Which one?",
		Metadata: map[string]interface{}{"pending_id": "pend-c", "question_id": "q-1"},
	})

	if err := svc.UpdateClarificationMessageForQuestion(ctx, "sess-msg", "pend-c", "q-1", "answered", "option-b"); err != nil {
		t.Fatalf("UpdateClarificationMessageForQuestion: %v", err)
	}
	stored, err := repo.GetMessage(ctx, "msg-clarify")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.Metadata["status"] != "answered" {
		t.Fatalf("status = %v, want answered", stored.Metadata["status"])
	}
	if stored.Metadata["response"] != "option-b" {
		t.Fatalf("response = %v, want option-b", stored.Metadata["response"])
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.MessageUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.MessageUpdated)
	}

	if err := svc.UpdateClarificationMessageForQuestion(ctx, "sess-msg", "pend-c", "q-missing", "answered", nil); err == nil {
		t.Fatal("an unknown question id must fail")
	}
}

func TestUpdateClarificationMessageForQuestionKeepsAnswerWhenNil(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{
		ID: "msg-clarify-nil", Content: "Which one?",
		Metadata: map[string]interface{}{"pending_id": "pend-n", "question_id": "q-n", "response": "prior"},
	})

	if err := svc.UpdateClarificationMessageForQuestion(ctx, "sess-msg", "pend-n", "q-n", "rejected", nil); err != nil {
		t.Fatalf("UpdateClarificationMessageForQuestion: %v", err)
	}
	stored, err := repo.GetMessage(ctx, "msg-clarify-nil")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if stored.Metadata["status"] != "rejected" {
		t.Fatalf("status = %v, want rejected", stored.Metadata["status"])
	}
	if stored.Metadata["response"] != "prior" {
		t.Fatalf("response = %v, want the prior answer left alone", stored.Metadata["response"])
	}
}
