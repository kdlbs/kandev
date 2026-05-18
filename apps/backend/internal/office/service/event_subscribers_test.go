package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// queueRunDispatcher is a test-only WorkflowEngineDispatcher that turns
// task-scoped engine triggers into the legacy QueueRun call. It exists
// so behavioural tests written before Phase 4 (which assert "publish
// event X produces a queued run") keep passing without each test
// needing to know about engine wiring. Tests that need to assert
// engine routing should use fakeDispatcher (in event_subscribers_engine_test.go).
type queueRunDispatcher struct {
	svc *service.Service
}

func (d *queueRunDispatcher) HandleTrigger(
	ctx context.Context, taskID string, trigger engine.Trigger, payload any, opID string,
) error {
	assignee, err := d.svc.GetTaskAssigneeForTest(ctx, taskID)
	if err != nil || assignee == "" {
		return service.ErrEngineNoSession
	}
	switch trigger {
	case engine.TriggerOnComment:
		p, _ := payload.(engine.OnCommentPayload)
		body, _ := json.Marshal(map[string]string{"task_id": taskID, "comment_id": p.CommentID})
		return d.svc.QueueRun(ctx, assignee, service.RunReasonTaskComment, string(body), opID)
	case engine.TriggerOnBlockerResolved:
		p, _ := payload.(engine.OnBlockerResolvedPayload)
		var resolved string
		if len(p.ResolvedBlockerIDs) > 0 {
			resolved = p.ResolvedBlockerIDs[0]
		}
		body, _ := json.Marshal(map[string]string{"task_id": taskID, "resolved_blocker_id": resolved})
		return d.svc.QueueRun(ctx, assignee, service.RunReasonTaskBlockersResolved, string(body), opID)
	case engine.TriggerOnChildrenCompleted:
		body, _ := json.Marshal(map[string]string{"task_id": taskID})
		return d.svc.QueueRun(ctx, assignee, service.RunReasonTaskChildrenCompleted, string(body), opID)
	case engine.TriggerOnApprovalResolved:
		p, _ := payload.(engine.OnApprovalResolvedPayload)
		body, _ := json.Marshal(map[string]string{
			"approval_id":   p.ApprovalID,
			"status":        p.Status,
			"decision_note": p.Note,
		})
		return d.svc.QueueRun(ctx, assignee, service.RunReasonApprovalResolved, string(body), opID)
	}
	return nil
}

// newTestServiceWithBus creates a service wired to an in-memory event bus.
// Handlers run synchronously so tests can assert effects immediately after Publish.
//
// A queueRunDispatcher is wired by default: it translates the four
// task-scoped engine triggers into the legacy QueueRun call so
// behavioural tests that assert "publishing event X produces a queued
// run" don't need to know about the engine plumbing. Tests that need to
// assert engine routing directly should overwrite the dispatcher with
// fakeDispatcher / nil via svc.SetWorkflowEngineDispatcher.
func newTestServiceWithBus(t *testing.T) (*service.Service, bus.EventBus) {
	t.Helper()
	svc := newTestService(t)
	svc.SetSyncHandlers(true)
	log := logger.Default()
	eb := bus.NewMemoryEventBus(log)
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}
	svc.SetWorkflowEngineDispatcher(&queueRunDispatcher{svc: svc})
	return svc, eb
}

func TestCommentCreated_RelaysAgentComment(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	var relayed atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relayed.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Set up a relay with a test HTTP client.
	relay := service.NewChannelRelayWithClient(svc, ts.Client())
	svc.SetRelay(relay)

	// Create agent + channel.
	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "relay-test",
		Role:        models.AgentRoleAssistant,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	config := `{"webhook_url":"` + ts.URL + `"}`
	channel := &models.Channel{
		WorkspaceID:    "ws-1",
		AgentProfileID: agent.ID,
		Platform:       "webhook",
		Config:         config,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}

	// Create an agent comment on the channel task.
	comment := &models.TaskComment{
		TaskID:         channel.TaskID,
		AuthorType:     "agent",
		AuthorID:       agent.ID,
		Body:           "Status update from agent",
		ReplyChannelID: channel.ID,
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	if !relayed.Load() {
		t.Error("expected agent comment to be relayed to webhook")
	}
}

func TestCommentCreated_UserComment_NotRelayed(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	var relayed atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relayed.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	relay := service.NewChannelRelayWithClient(svc, ts.Client())
	svc.SetRelay(relay)

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "relay-test-user",
		Role:        models.AgentRoleAssistant,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	config := `{"webhook_url":"` + ts.URL + `"}`
	channel := &models.Channel{
		WorkspaceID:    "ws-1",
		AgentProfileID: agent.ID,
		Platform:       "webhook",
		Config:         config,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}

	comment := &models.TaskComment{
		TaskID:         channel.TaskID,
		AuthorType:     "user",
		AuthorID:       "user-1",
		Body:           "User message",
		ReplyChannelID: channel.ID,
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	if relayed.Load() {
		t.Error("user comments should not be relayed")
	}
}

func TestCommentCreated_WakesAssignee(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	event := bus.NewEvent(events.OfficeCommentCreated, "test", map[string]string{
		"task_id":     taskID,
		"comment_id":  "comment-1",
		"author_type": "user",
		"author_id":   "user-1",
	})
	if err := eb.Publish(ctx, events.OfficeCommentCreated, event); err != nil {
		t.Fatalf("publish comment event: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.AgentProfileID == "worker-1" && run.Reason == service.RunReasonTaskComment {
			var payload map[string]string
			if err := json.Unmarshal([]byte(run.Payload), &payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if payload["task_id"] != taskID || payload["comment_id"] != "comment-1" {
				t.Fatalf("payload = %#v, want task/comment IDs", payload)
			}
			return
		}
	}
	t.Fatal("expected task_comment run for assigned worker")
}

func TestCommentCreated_SkipsSelfComment(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-self")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-self")

	event := bus.NewEvent(events.OfficeCommentCreated, "test", map[string]string{
		"task_id":     taskID,
		"comment_id":  "comment-self",
		"author_type": "agent",
		"author_id":   "worker-self",
	})
	if err := eb.Publish(ctx, events.OfficeCommentCreated, event); err != nil {
		t.Fatalf("publish comment event: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.AgentProfileID == "worker-self" && run.Reason == service.RunReasonTaskComment {
			t.Fatal("self comment should not queue task_comment run")
		}
	}
}

func TestCommentCreated_NoAssignee(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	insertTestTask(t, svc, "task-unassigned", "ws-1")

	event := bus.NewEvent(events.OfficeCommentCreated, "test", map[string]string{
		"task_id":     "task-unassigned",
		"comment_id":  "comment-2",
		"author_type": "user",
		"author_id":   "user-1",
	})
	if err := eb.Publish(ctx, events.OfficeCommentCreated, event); err != nil {
		t.Fatalf("publish comment event: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.Reason == service.RunReasonTaskComment {
			t.Fatal("unassigned task should not queue task_comment run")
		}
	}
}

func TestChannelInbound_WakesAssignee(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:          "assistant-1",
		WorkspaceID: "ws-1",
		Name:        "assistant",
		Role:        models.AgentRoleAssistant,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	channel := &models.Channel{
		WorkspaceID:    "ws-1",
		AgentProfileID: agent.ID,
		Platform:       "webhook",
		Config:         `{}`,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}

	if err := svc.HandleChannelInbound(ctx, channel.ID, "external", "hello"); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.AgentProfileID == agent.ID && run.Reason == service.RunReasonTaskComment {
			return
		}
	}
	t.Fatal("expected inbound channel comment to wake assistant")
}

func TestPromptUsage_RecordsCostEvent(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-cost")
	svc.ExecSQL(t, `INSERT INTO tasks (
			id, workspace_id, project_id, title, created_at, updated_at
		) VALUES (
			'task-cost-1', 'ws-1', 'project-1', 'Cost task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)`)
	setTestTaskAssignee(t, svc, "task-cost-1", "worker-cost")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-cost-1",
		"session_id": "session-cost-1",
		"agent_id":   "claude-acp",
		"model":      "gpt-4o-mini",
		"usage": map[string]interface{}{
			"input_tokens":       1_000_000,
			"cached_read_tokens": 1_000_000,
			"output_tokens":      1_000_000,
			"total_tokens":       3_000_000,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-cost-1"), event); err != nil {
		t.Fatalf("publish prompt usage: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost count = %d, want 1", len(costs))
	}

	if costs[0].SessionID != "session-cost-1" || costs[0].AgentProfileID != "worker-cost" {
		t.Fatalf("cost event = %#v, want session and assignee fields", costs[0])
	}
	// claude-acp's agent_id maps to anthropic provider (not the legacy
	// "agent_id-as-provider" behaviour). The model is openai's gpt-4o-mini
	// but the provider is derived from the CLI id, not the model prefix —
	// callers can override with the legacy `provider` JSON field.
	if costs[0].Provider != "anthropic" {
		t.Fatalf("cost event provider = %q, want anthropic (from claude-acp CLI id)", costs[0].Provider)
	}
	if costs[0].Model != "gpt-4o-mini" {
		t.Fatalf("cost event model = %q, want %q", costs[0].Model, "gpt-4o-mini")
	}
	// With no pricing lookup wired and no provider-reported cost, the row
	// records 0 with estimated=true (Layer A miss + no Layer B).
	if costs[0].CostSubcents != 0 {
		t.Fatalf("cost_subcents = %d, want 0 (no pricing lookup wired)", costs[0].CostSubcents)
	}
	if !costs[0].Estimated {
		t.Fatal("expected estimated=true when both lookup layers miss")
	}
}

// TestPromptUsage_AgentTypeDerivesProvider confirms the production path:
// orchestrator's publishPromptUsage populates AgentType from the session
// snapshot (claude-acp / codex-acp / ...) while AgentID carries the
// execution UUID. resolveProvider prefers AgentType.
func TestPromptUsage_AgentTypeDerivesProvider(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-agent-type")
	svc.ExecSQL(t, `INSERT INTO tasks (
			id, workspace_id, project_id, title, created_at, updated_at
		) VALUES (
			'task-agent-type', 'ws-1', 'project-1', 'AgentType test', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)`)
	setTestTaskAssignee(t, svc, "task-agent-type", "worker-agent-type")

	// Production shape: AgentID is the execution UUID; AgentType is the CLI slug.
	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-agent-type",
		"session_id": "session-agent-type",
		"agent_id":   "6f1cb0d2-5f8c-459f-802f-aaaf1959462b",
		"agent_type": "claude-acp",
		"model":      "default", // claude-acp logical alias
		"usage": map[string]interface{}{
			"input_tokens":  6,
			"output_tokens": 7,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-agent-type"), event); err != nil {
		t.Fatalf("publish prompt usage: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost count = %d, want 1", len(costs))
	}
	if costs[0].Provider != "anthropic" {
		t.Fatalf("provider = %q, want %q (from agent_type=claude-acp)", costs[0].Provider, "anthropic")
	}
	if costs[0].Model != "default" {
		t.Fatalf("model = %q, want %q (logical alias preserved verbatim)", costs[0].Model, "default")
	}
}

// TestPromptUsage_FallsBackToProviderField confirms backward compat:
// when a publisher emits the legacy `provider` JSON field (no `agent_id`),
// the subscriber still records it on CostEvent.Provider. This preserves
// any test fixture or future caller that sets provider directly.
func TestPromptUsage_FallsBackToProviderField(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-cost")
	svc.ExecSQL(t, `INSERT INTO tasks (
			id, workspace_id, project_id, title, created_at, updated_at
		) VALUES (
			'task-cost-2', 'ws-1', 'project-1', 'Cost task 2', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)`)
	setTestTaskAssignee(t, svc, "task-cost-2", "worker-cost")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-cost-2",
		"session_id": "session-cost-2",
		"provider":   "legacy-provider",
		"model":      "butler_a", // unknown model — Layer A miss, model prefix unknown

		"usage": map[string]interface{}{
			"input_tokens":  1_000_000,
			"output_tokens": 1_000_000,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-cost-2"), event); err != nil {
		t.Fatalf("publish prompt usage: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost count = %d, want 1", len(costs))
	}
	if costs[0].Provider != "legacy-provider" {
		t.Fatalf("cost event provider = %q, want %q (legacy provider fallback)", costs[0].Provider, "legacy-provider")
	}
}

// TestPromptUsage_PrefersProviderReportedCost confirms Layer A wins:
// when usage_update.cost.amount is forwarded (claude-acp), the row
// stores that cost verbatim and the pricing-lookup is skipped.
func TestPromptUsage_PrefersProviderReportedCost(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-layer-a")
	svc.ExecSQL(t, `INSERT INTO tasks (
			id, workspace_id, project_id, title, created_at, updated_at
		) VALUES (
			'task-layer-a', 'ws-1', 'project-1', 'Layer A', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)`)
	setTestTaskAssignee(t, svc, "task-layer-a", "worker-layer-a")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-layer-a",
		"session_id": "session-layer-a",
		"agent_id":   "claude-acp",
		"model":      "sonnet",
		"usage": map[string]interface{}{
			"input_tokens":                    100,
			"output_tokens":                   200,
			"provider_reported_cost_subcents": 616,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-layer-a"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost count = %d, want 1", len(costs))
	}
	if costs[0].CostSubcents != 616 {
		t.Errorf("cost_subcents = %d, want 616 (Layer A verbatim)", costs[0].CostSubcents)
	}
	if costs[0].Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic (claude-acp -> anthropic)", costs[0].Provider)
	}
	if costs[0].Model != "sonnet" {
		t.Errorf("model = %q, want sonnet (verbatim alias)", costs[0].Model)
	}
}

// TestPromptUsage_CodexEstimatedFlag confirms the synthesised cumulative
// delta path: when Usage.Estimated=true on the wire (codex-acp), the
// cost row carries estimated=true regardless of cost-resolution outcome.
func TestPromptUsage_CodexEstimatedFlag(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-codex")
	svc.ExecSQL(t, `INSERT INTO tasks (
			id, workspace_id, project_id, title, created_at, updated_at
		) VALUES (
			'task-codex', 'ws-1', 'project-1', 'Codex', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)`)
	setTestTaskAssignee(t, svc, "task-codex", "worker-codex")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-codex",
		"session_id": "session-codex",
		"agent_id":   "codex-acp",
		"model":      "gpt-5.4-mini",
		"usage": map[string]interface{}{
			"input_tokens":  350,
			"output_tokens": 0,
			"estimated":     true,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-codex"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost count = %d, want 1", len(costs))
	}
	if !costs[0].Estimated {
		t.Error("expected estimated=true for codex-acp synthesised delta")
	}
	if costs[0].Provider != "openai" {
		t.Errorf("provider = %q, want openai (codex-acp -> openai)", costs[0].Provider)
	}
}

// The legacy ExecutionPolicy stage progression tests (MovedToDone /
// MovedToInReview with-or-without policy) were removed when the
// execution_policy.go module was retired in Phase 4 of
// task-model-unification. Stage progression is now owned by the
// workflow engine; the engine's office-default smoke tests
// (internal/workflow/engine/office_default_smoke_test.go) cover the
// review/approval/done cycle end-to-end.

func TestMovedToDone_WithoutExecutionPolicy_NormalCompletion(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-2")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-2")

	// No execution policy -- normal done path.
	moveEvent := bus.NewEvent("task.moved", "test", map[string]string{
		"task_id":                   taskID,
		"workspace_id":              "ws-1",
		"from_step_id":              "step-1",
		"to_step_id":                "step-done",
		"to_step_name":              "Done",
		"from_step_name":            "In Progress",
		"assignee_agent_profile_id": "worker-2",
		"parent_id":                 "",
		"execution_policy":          "",
	})
	if err := eb.Publish(ctx, "task.moved", moveEvent); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	// No review runs should be created (only the setup channel run from createOfficeTask).
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, w := range runs {
		if w.Reason == "task_assigned" {
			// Check it's not a review_request run by examining the payload.
			var payload map[string]string
			if json.Unmarshal([]byte(w.Payload), &payload) == nil {
				if payload["stage_type"] == "review" {
					t.Error("no review runs should exist for tasks without execution policy")
				}
			}
		}
	}

	// handleTaskMoved must write an activity log entry with action
	// "task_status_changed" when the step name changes.
	activity, err := svc.ListActivity(ctx, "ws-1", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	found := false
	for _, a := range activity {
		if a.Action == "task_status_changed" && a.TargetID == taskID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected activity log entry with action 'task_status_changed' for task %s, got %+v", taskID, activity)
	}
}

func TestHandleTaskStatusChanged_LogsActivity(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-status")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-status")

	statusEvt := bus.NewEvent(events.OfficeTaskStatusChanged, "test", map[string]interface{}{
		"task_id":      taskID,
		"new_status":   "in_progress",
		"workspace_id": "ws-1",
	})
	if err := eb.Publish(ctx, events.OfficeTaskStatusChanged, statusEvt); err != nil {
		t.Fatalf("publish office.task.status_changed: %v", err)
	}

	activity, err := svc.ListActivity(ctx, "ws-1", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	found := false
	for _, a := range activity {
		if a.Action == "task_status_changed" && a.TargetID == taskID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected activity entry with action 'task_status_changed' for task %s, got %+v", taskID, activity)
	}
}

func TestAutoPostAgentComment_CreatesSessionComment(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-bridge")
	taskID := createOfficeTask(t, svc, "ws-1", "agent-bridge")

	event := bus.NewEvent(events.AgentTurnMessageSaved, "orchestrator", map[string]string{
		"task_id":    taskID,
		"session_id": "sess-1",
		"agent_text": "Here is my analysis.",
		"agent_id":   "agent-bridge",
	})
	if err := eb.Publish(ctx, events.AgentTurnMessageSaved, event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	comments, err := svc.ListComments(ctx, taskID)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}

	for _, c := range comments {
		if c.Source == "session" && c.AuthorType == "agent" && c.Body == "Here is my analysis." {
			return // found expected comment
		}
	}
	t.Fatalf("expected session comment from agent, got %+v", comments)
}

func TestAutoPostAgentComment_SkipsNonOfficeTasks(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	// Insert a task without an assignee (not an office task).
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-noassignee', 'ws-1', 'bare task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	event := bus.NewEvent(events.AgentTurnMessageSaved, "orchestrator", map[string]string{
		"task_id":    "task-noassignee",
		"session_id": "sess-bare",
		"agent_text": "This should not appear.",
		"agent_id":   "some-agent",
	})
	if err := eb.Publish(ctx, events.AgentTurnMessageSaved, event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	comments, err := svc.ListComments(ctx, "task-noassignee")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected no comments for task without assignee, got %d", len(comments))
	}
}

// Pins the per-turn dedup behavior. Office sessions are reused across
// turns (same DB session_id, same ACP session id), so each new agent
// turn must produce its own session comment. Only the SAME body firing
// twice (event re-delivery for one turn) should be deduped.
func TestAutoPostAgentComment_DedupBehavior(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-dedup")
	taskID := createOfficeTask(t, svc, "ws-1", "agent-dedup")

	publishTurn := func(text string) {
		t.Helper()
		event := bus.NewEvent(events.AgentTurnMessageSaved, "orchestrator", map[string]string{
			"task_id":    taskID,
			"session_id": "sess-dedup",
			"agent_text": text,
			"agent_id":   "agent-dedup",
		})
		if err := eb.Publish(ctx, events.AgentTurnMessageSaved, event); err != nil {
			t.Fatalf("publish event: %v", err)
		}
	}

	// Turn 1.
	publishTurn("First response.")
	// Turn 1 re-delivery (same body) → should be deduped.
	publishTurn("First response.")
	// Turn 2 (same session, new body) → must NOT be deduped.
	publishTurn("Second response after a user comment.")

	comments, err := svc.ListComments(ctx, taskID)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}

	sessionBodies := map[string]int{}
	for _, c := range comments {
		if c.Source == "session" {
			sessionBodies[c.Body]++
		}
	}
	if sessionBodies["First response."] != 1 {
		t.Errorf("expected exactly 1 session comment for turn 1 body, got %d",
			sessionBodies["First response."])
	}
	if sessionBodies["Second response after a user comment."] != 1 {
		t.Errorf("expected exactly 1 session comment for turn 2 body, got %d",
			sessionBodies["Second response after a user comment."])
	}
}
