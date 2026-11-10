package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/pause"
	"github.com/kandev/kandev/internal/office/service"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type handoffCommentPublishFailureBus struct {
	bus.EventBus
	fail bool
}

func (b *handoffCommentPublishFailureBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if b.fail && subject == events.OfficeCommentCreated {
		return errors.New("injected comment event failure")
	}
	return b.EventBus.Publish(ctx, subject, event)
}

func seedHandoffCompletionTasks(t *testing.T, svc *service.Service, assigneeID string) {
	t.Helper()
	svc.ExecSQL(t, `ALTER TABLE tasks ADD COLUMN metadata TEXT DEFAULT '{}'`)
	svc.ExecSQL(t, `INSERT INTO workspaces (id, office_workflow_id) VALUES ('source-ws', 'office-flow')`)
	svc.ExecSQL(t, `INSERT INTO workspaces (id, office_workflow_id) VALUES ('delivery-ws', '')`)
	svc.ExecSQL(t, `INSERT INTO tasks
		(id, workspace_id, workflow_id, title, state, metadata, created_at, updated_at)
		VALUES ('source-task', 'source-ws', 'office-flow', 'Source task', 'IN_PROGRESS', '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	metadata, err := json.Marshal(map[string]interface{}{
		taskmodels.MetaKeyHandoffSource: map[string]interface{}{
			"source_task_id": "source-task", "source_workspace_id": "source-ws",
			"source_session_id": "source-session", "source_agent_profile_id": "source-agent",
			"handed_off_at": "2026-11-01T00:00:00Z",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks
		(id, workspace_id, workflow_id, title, state, metadata, created_at, updated_at)
		VALUES ('delivery-task', 'delivery-ws', '', 'Delivery task', 'COMPLETED', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, string(metadata))
	createTestAgent(t, svc, "source-ws", "source-agent")
	if assigneeID != "" {
		setTestTaskAssignee(t, svc, "source-task", assigneeID)
	}
}

func seedLegacyHandoffCompletionComment(t *testing.T, svc *service.Service, comment *models.TaskComment) {
	t.Helper()
	if err := svc.RepoForTest().CreateTaskComment(context.Background(), comment); err != nil {
		t.Fatalf("seed legacy handoff completion comment: %v", err)
	}
}

func publishHandoffCompletion(t *testing.T, eb bus.EventBus) {
	t.Helper()
	event := bus.NewEvent(events.TaskStateChanged, "test", map[string]string{
		"task_id": "delivery-task", "old_state": "IN_PROGRESS", "new_state": "COMPLETED",
	})
	if err := eb.Publish(context.Background(), events.TaskStateChanged, event); err != nil {
		t.Fatalf("publish task state change: %v", err)
	}
}

func TestHandoffCompletion_ReportOnlyGrantStoresCommentWithoutWake(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	seedHandoffCompletionTasks(t, svc, "source-agent")
	completionKey := service.RunReasonHandoffCompleted + ":delivery-task:COMPLETED"
	legacyComment := &models.TaskComment{
		ID:         "11111111-1111-4111-8111-111111111111",
		TaskID:     "source-task",
		AuthorType: "agent",
		AuthorID:   "source-agent",
		Body:       "Handed-off task \"Delivery task\" (delivery-task) reached state COMPLETED.\nLegacy detail retained.\n<!-- handoff-completion:" + completionKey + " -->",
		Source:     "handoff",
		CreatedAt:  time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
	}
	seedLegacyHandoffCompletionComment(t, svc, legacyComment)
	var publishedComments []service.CommentPostedData
	sub, err := eb.Subscribe(events.OfficeCommentCreated, func(_ context.Context, event *bus.Event) error {
		if data, ok := event.Data.(service.CommentPostedData); ok {
			publishedComments = append(publishedComments, data)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe to comment events: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (service.HandoffCompletionAuthorization, error) {
		return service.HandoffCompletionAuthorization{Report: true}, nil
	})

	publishHandoffCompletion(t, eb)
	publishHandoffCompletion(t, eb)

	comments, err := svc.RepoForTest().ListTaskComments(context.Background(), "source-task")
	if err != nil {
		t.Fatalf("list report comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("report comments = %d, want 1", len(comments))
	}
	if comments[0].ID != legacyComment.ID {
		t.Fatalf("report ID = %q, want legacy UUID %q", comments[0].ID, legacyComment.ID)
	}
	if comments[0].Body != legacyComment.Body || comments[0].AuthorID != legacyComment.AuthorID ||
		!comments[0].CreatedAt.Equal(legacyComment.CreatedAt) {
		t.Fatalf("legacy report changed after replay: got %+v, want %+v", comments[0], legacyComment)
	}
	receipt, err := svc.RepoForTest().GetHandoffCompletionOutbox(context.Background(), completionKey)
	if err != nil {
		t.Fatalf("read handoff completion receipt: %v", err)
	}
	if receipt == nil || receipt.CommentID != legacyComment.ID {
		t.Fatalf("receipt comment ID = %+v, want legacy UUID %q", receipt, legacyComment.ID)
	}
	if len(publishedComments) != 1 || publishedComments[0].CommentID != legacyComment.ID ||
		publishedComments[0].TaskID != legacyComment.TaskID || publishedComments[0].AuthorID != legacyComment.AuthorID {
		t.Fatalf("published comment events = %+v, want the single stored legacy comment", publishedComments)
	}
	var wakeCount int
	if err := svc.RepoForTest().ReaderDB().Get(&wakeCount,
		`SELECT COUNT(*) FROM runs WHERE reason = ?`, service.RunReasonHandoffCompleted); err != nil {
		t.Fatalf("count completion wakes: %v", err)
	}
	if wakeCount != 0 {
		t.Fatalf("completion wakes = %d, want none for a report-only grant", wakeCount)
	}
}

func TestHandoffCompletion_UnrelatedAnchorsCreateCleanReport(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	seedHandoffCompletionTasks(t, svc, "")
	completionKey := service.RunReasonHandoffCompleted + ":delivery-task:COMPLETED"
	for _, comment := range []*models.TaskComment{
		{
			ID: "22222222-2222-4222-8222-222222222222", TaskID: "delivery-task", AuthorType: "agent",
			AuthorID: "source-agent", Body: "Wrong task.\n<!-- handoff-completion:" + completionKey + " -->",
			Source: "handoff", CreatedAt: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			ID: "33333333-3333-4333-8333-333333333333", TaskID: "source-task", AuthorType: "agent",
			AuthorID: "source-agent", Body: "Wrong source.\n<!-- handoff-completion:" + completionKey + " -->",
			Source: "user", CreatedAt: time.Date(2026, 11, 1, 12, 1, 0, 0, time.UTC),
		},
		{
			ID: "44444444-4444-4444-8444-444444444444", TaskID: "source-task", AuthorType: "agent",
			AuthorID: "source-agent", Body: "Wrong marker.\n<!-- handoff-completion:" + completionKey + "-other -->",
			Source: "handoff", CreatedAt: time.Date(2026, 11, 1, 12, 2, 0, 0, time.UTC),
		},
	} {
		seedLegacyHandoffCompletionComment(t, svc, comment)
	}
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (service.HandoffCompletionAuthorization, error) {
		return service.HandoffCompletionAuthorization{Report: true}, nil
	})

	publishHandoffCompletion(t, eb)

	comments, err := svc.RepoForTest().ListTaskComments(context.Background(), "source-task")
	if err != nil {
		t.Fatalf("list report comments: %v", err)
	}
	if len(comments) != 3 {
		t.Fatalf("source comments = %d, want two unrelated comments and one new report", len(comments))
	}
	var newReport *models.TaskComment
	for _, comment := range comments {
		if comment.ID == completionKey {
			newReport = comment
			break
		}
	}
	if newReport == nil {
		t.Fatalf("new report with completion key %q was not stored: %+v", completionKey, comments)
	}
	if strings.Contains(newReport.Body, "<!-- handoff-completion:") {
		t.Fatalf("new report exposes the legacy lookup marker: %q", newReport.Body)
	}
	receipt, err := svc.RepoForTest().GetHandoffCompletionOutbox(context.Background(), completionKey)
	if err != nil {
		t.Fatalf("read handoff completion receipt: %v", err)
	}
	if receipt == nil || receipt.CommentID != completionKey {
		t.Fatalf("receipt comment ID = %+v, want completion key %q", receipt, completionKey)
	}
}

func TestHandoffCompletion_UnassignedSourceStillGetsReport(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	seedHandoffCompletionTasks(t, svc, "")
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (service.HandoffCompletionAuthorization, error) {
		return service.HandoffCompletionAuthorization{Report: true}, nil
	})

	publishHandoffCompletion(t, eb)

	comments, err := svc.RepoForTest().ListTaskComments(context.Background(), "source-task")
	if err != nil {
		t.Fatalf("list report comments: %v", err)
	}
	if len(comments) != 1 || !strings.Contains(comments[0].Body, "Delivery task") {
		t.Fatalf("report comments = %+v, want a completion report on the unassigned source", comments)
	}
}

func TestHandoffCompletion_DuplicateTerminalEventKeepsSingleReport(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	seedHandoffCompletionTasks(t, svc, "")
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (service.HandoffCompletionAuthorization, error) {
		return service.HandoffCompletionAuthorization{Report: true}, nil
	})

	publishHandoffCompletion(t, eb)
	publishHandoffCompletion(t, eb)

	comments, err := svc.RepoForTest().ListTaskComments(context.Background(), "source-task")
	if err != nil {
		t.Fatalf("list report comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("report comments = %d, want 1 after duplicate delivery", len(comments))
	}
}

func TestHandoffCompletion_PromptGrantCanWakeWithoutPublishingReport(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	seedHandoffCompletionTasks(t, svc, "source-agent")
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (service.HandoffCompletionAuthorization, error) {
		return service.HandoffCompletionAuthorization{Prompt: true}, nil
	})

	publishHandoffCompletion(t, eb)

	comments, err := svc.RepoForTest().ListTaskComments(context.Background(), "source-task")
	if err != nil {
		t.Fatalf("list report comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("report comments = %d, want none without report permission", len(comments))
	}
	if got := countQueuedRuns(t, svc, "source-agent", service.RunReasonHandoffCompleted); got != 1 {
		t.Fatalf("handoff completion wakes = %d, want one with prompt permission", got)
	}
	runs, err := svc.ListRuns(context.Background(), "source-ws")
	if err != nil {
		t.Fatalf("list source-task runs: %v", err)
	}
	var payload string
	for _, run := range runs {
		if run.Reason == service.RunReasonHandoffCompleted {
			payload = run.Payload
			break
		}
	}
	if payload == "" {
		t.Fatal("source-task run list does not contain the handoff completion wake")
	}
	prompt := service.BuildPrompt(service.BuildPromptContextForTest(
		svc, context.Background(), service.RunReasonHandoffCompleted, payload,
	))
	for _, want := range []string{"Delivery task", "COMPLETED", "completion status"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt-only wake is missing %q: %s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Completion report:") {
		t.Errorf("prompt-only wake contains an unapproved report comment: %s", prompt)
	}
}

func TestHandoffCompletion_PausedWakeRetriesFromDurableOutbox(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	seedHandoffCompletionTasks(t, svc, "source-agent")
	completionKey := service.RunReasonHandoffCompleted + ":delivery-task:COMPLETED"
	seedLegacyHandoffCompletionComment(t, svc, &models.TaskComment{
		ID: "55555555-5555-4555-8555-555555555555", TaskID: "source-task", AuthorType: "agent",
		AuthorID: "source-agent",
		Body: "Handed-off task \"Delivery task\" (delivery-task) reached state COMPLETED.\n" +
			"Persisted detail: keep this report body.\n<!-- handoff-completion:" + completionKey + " -->",
		Source: "handoff", CreatedAt: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
	})
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (service.HandoffCompletionAuthorization, error) {
		return service.HandoffCompletionAuthorization{Report: true, Prompt: true}, nil
	})
	ctx := context.Background()
	pauseService := pause.NewService(
		svc.RepoForTest(), noopTaskCanceller{},
		&fakeWorkspaceChecker{known: map[string]bool{"source-ws": true}}, logger.Default(),
	)
	svc.SetPauseGate(pauseService)
	if _, err := pauseService.Pause(ctx, "source-ws", "test pause", "user-1", "user"); err != nil {
		t.Fatalf("pause source workspace: %v", err)
	}

	publishHandoffCompletion(t, eb)

	if got := countQueuedRuns(t, svc, "source-agent", service.RunReasonHandoffCompleted); got != 0 {
		t.Fatalf("handoff completion wakes while paused = %d, want 0", got)
	}
	var pending bool
	if err := svc.RepoForTest().ReaderDB().Get(&pending,
		`SELECT wake_pending FROM office_handoff_completion_outbox WHERE completion_key = ?`,
		service.RunReasonHandoffCompleted+":delivery-task:COMPLETED"); err != nil {
		t.Fatalf("read pending handoff wake: %v", err)
	}
	if !pending {
		t.Fatal("paused completion wake is not pending in the durable outbox")
	}
	if _, err := pauseService.Resume(ctx, "source-ws", "resume", "user-1", "user"); err != nil {
		t.Fatalf("resume source workspace: %v", err)
	}
	if err := svc.ReplayPendingHandoffCompletionOutbox(ctx); err != nil {
		t.Fatalf("replay handoff completion outbox: %v", err)
	}
	if got := countQueuedRuns(t, svc, "source-agent", service.RunReasonHandoffCompleted); got != 1 {
		t.Fatalf("handoff completion wakes after resume = %d, want 1", got)
	}
	runs, err := svc.ListRuns(ctx, "source-ws")
	if err != nil {
		t.Fatalf("list source-task runs: %v", err)
	}
	var completionPayload string
	for _, run := range runs {
		if run.Reason == service.RunReasonHandoffCompleted {
			completionPayload = run.Payload
			break
		}
	}
	if completionPayload == "" {
		t.Fatal("source-task run list does not contain the handoff completion wake")
	}
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonHandoffCompleted, completionPayload)
	prompt := service.BuildPrompt(pc)
	for _, want := range []string{"Delivery task", "COMPLETED", "Handed-off task", "Persisted detail: keep this report body."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("handoff wake prompt missing %q: %s", want, prompt)
		}
	}
	if err := svc.RepoForTest().ReaderDB().Get(&pending,
		`SELECT wake_pending FROM office_handoff_completion_outbox WHERE completion_key = ?`,
		service.RunReasonHandoffCompleted+":delivery-task:COMPLETED"); err != nil {
		t.Fatalf("read replayed handoff wake: %v", err)
	}
	if pending {
		t.Fatal("replayed completion wake remains pending")
	}
}

func TestHandoffCompletion_DistinctDeliveryWakesDoNotCoalesce(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceWithBus(t)
	seedHandoffCompletionTasks(t, svc, "source-agent")
	svc.SetRunsService(runsservice.New(svc.RepoForTest().RunsRepository(), nil, logger.Default(), nil))
	for _, deliveryID := range []string{"delivery-a", "delivery-b"} {
		payload, err := json.Marshal(map[string]string{
			"task_id": "source-task", "delivery_task_id": deliveryID, "delivery_state": "COMPLETED",
		})
		if err != nil {
			t.Fatal(err)
		}
		key := service.RunReasonHandoffCompleted + ":" + deliveryID + ":COMPLETED"
		if err := svc.QueueRunFromTaskBoundary(ctx, "source-agent", service.RunReasonHandoffCompleted,
			string(payload), key, "source-task"); err != nil {
			t.Fatalf("queue completion for %s: %v", deliveryID, err)
		}
	}
	if got := countQueuedRuns(t, svc, "source-agent", service.RunReasonHandoffCompleted); got != 2 {
		t.Fatalf("handoff completion wakes = %d, want one per delivery", got)
	}
}

func TestHandoffCompletion_InlineQueueDoesNotCoalesce(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceWithBus(t)
	seedHandoffCompletionTasks(t, svc, "source-agent")
	for _, deliveryID := range []string{"delivery-a", "delivery-b"} {
		payload, err := json.Marshal(map[string]string{
			"task_id": "source-task", "delivery_task_id": deliveryID, "delivery_state": "COMPLETED",
		})
		if err != nil {
			t.Fatal(err)
		}
		key := service.RunReasonHandoffCompleted + ":" + deliveryID + ":COMPLETED"
		if err := svc.QueueRunFromTaskBoundary(ctx, "source-agent", service.RunReasonHandoffCompleted,
			string(payload), key, "source-task"); err != nil {
			t.Fatalf("queue completion for %s: %v", deliveryID, err)
		}
	}
	if got := countQueuedRuns(t, svc, "source-agent", service.RunReasonHandoffCompleted); got != 2 {
		t.Fatalf("inline handoff completion wakes = %d, want one per delivery", got)
	}
}

func TestHandoffCompletion_PendingWakeIsSuppressedAfterPromptGrantRevoked(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	seedHandoffCompletionTasks(t, svc, "source-agent")
	promptAllowed := true
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (service.HandoffCompletionAuthorization, error) {
		return service.HandoffCompletionAuthorization{Report: true, Prompt: promptAllowed}, nil
	})
	ctx := context.Background()
	pauseService := pause.NewService(
		svc.RepoForTest(), noopTaskCanceller{},
		&fakeWorkspaceChecker{known: map[string]bool{"source-ws": true}}, logger.Default(),
	)
	svc.SetPauseGate(pauseService)
	if _, err := pauseService.Pause(ctx, "source-ws", "test pause", "user-1", "user"); err != nil {
		t.Fatalf("pause source workspace: %v", err)
	}
	publishHandoffCompletion(t, eb)
	promptAllowed = false
	if _, err := pauseService.Resume(ctx, "source-ws", "resume", "user-1", "user"); err != nil {
		t.Fatalf("resume source workspace: %v", err)
	}
	if err := svc.ReplayPendingHandoffCompletionOutbox(ctx); err != nil {
		t.Fatalf("replay handoff completion outbox: %v", err)
	}
	if got := countQueuedRuns(t, svc, "source-agent", service.RunReasonHandoffCompleted); got != 0 {
		t.Fatalf("handoff completion wakes after prompt grant revocation = %d, want 0", got)
	}
}

func TestHandoffCompletion_PendingWakeIsSuppressedAfterSourceReassignment(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	seedHandoffCompletionTasks(t, svc, "source-agent")
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (service.HandoffCompletionAuthorization, error) {
		return service.HandoffCompletionAuthorization{Report: true, Prompt: true}, nil
	})
	ctx := context.Background()
	pauseService := pause.NewService(
		svc.RepoForTest(), noopTaskCanceller{},
		&fakeWorkspaceChecker{known: map[string]bool{"source-ws": true}}, logger.Default(),
	)
	svc.SetPauseGate(pauseService)
	if _, err := pauseService.Pause(ctx, "source-ws", "test pause", "user-1", "user"); err != nil {
		t.Fatalf("pause source workspace: %v", err)
	}
	publishHandoffCompletion(t, eb)
	createTestAgent(t, svc, "source-ws", "replacement-agent")
	setTestTaskAssignee(t, svc, "source-task", "replacement-agent")
	if _, err := pauseService.Resume(ctx, "source-ws", "resume", "user-1", "user"); err != nil {
		t.Fatalf("resume source workspace: %v", err)
	}
	if err := svc.ReplayPendingHandoffCompletionOutbox(ctx); err != nil {
		t.Fatalf("replay handoff completion outbox: %v", err)
	}
	for _, agentID := range []string{"source-agent", "replacement-agent"} {
		if got := countQueuedRuns(t, svc, agentID, service.RunReasonHandoffCompleted); got != 0 {
			t.Errorf("handoff completion wakes for %s after reassignment = %d, want 0", agentID, got)
		}
	}
}

func TestHandoffCompletion_CommentEventRetriesFromDurableOutbox(t *testing.T) {
	memoryBus := bus.NewMemoryEventBus(logger.Default())
	wrappedBus := &handoffCommentPublishFailureBus{EventBus: memoryBus, fail: true}
	svc := newTestService(t, service.ServiceOptions{Logger: logger.Default(), EventBus: wrappedBus})
	svc.SetSyncHandlers(true)
	if err := svc.RegisterEventSubscribers(wrappedBus); err != nil {
		t.Fatalf("register event subscribers: %v", err)
	}
	svc.SetWorkflowEngineDispatcher(&queueRunDispatcher{svc: svc})
	seedHandoffCompletionTasks(t, svc, "source-agent")
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (service.HandoffCompletionAuthorization, error) {
		return service.HandoffCompletionAuthorization{Report: true}, nil
	})
	event := bus.NewEvent(events.TaskStateChanged, "test", map[string]string{
		"task_id": "delivery-task", "old_state": "IN_PROGRESS", "new_state": "COMPLETED",
	})
	// The completion handler reports the nested publication failure. The durable
	// outbox is the recovery path, so the source event's acknowledgement is not
	// relevant to this assertion.
	_ = wrappedBus.Publish(context.Background(), events.TaskStateChanged, event)
	var pending bool
	completionKey := service.RunReasonHandoffCompleted + ":delivery-task:COMPLETED"
	if err := svc.RepoForTest().ReaderDB().Get(&pending,
		`SELECT comment_event_pending FROM office_handoff_completion_outbox WHERE completion_key = ?`,
		completionKey); err != nil {
		t.Fatalf("read pending comment event: %v", err)
	}
	if !pending {
		t.Fatal("failed report event is not pending in the durable outbox")
	}

	wrappedBus.fail = false
	if err := svc.ReplayPendingHandoffCompletionOutbox(context.Background()); err != nil {
		t.Fatalf("replay handoff completion outbox: %v", err)
	}
	if err := svc.RepoForTest().ReaderDB().Get(&pending,
		`SELECT comment_event_pending FROM office_handoff_completion_outbox WHERE completion_key = ?`,
		completionKey); err != nil {
		t.Fatalf("read replayed comment event: %v", err)
	}
	if pending {
		t.Fatal("published report event remains pending")
	}
}
