package automation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	store := setupTestStore(t)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	eb := bus.NewMemoryEventBus(log)
	return NewService(store, eb, log)
}

func TestCreateAutomationResponse_IncludesWebhookSecret(t *testing.T) {
	// Mirrors the WS create flow: the server should return the plaintext
	// webhook secret exactly once, so the UI can show it to the user.
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, err := ws.NewRequest("req-1", ws.ActionAutomationCreate, &CreateAutomationRequest{
		WorkspaceID:       "ws-1",
		Name:              "with secret",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := wsCreate(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("expected response, got %v: %s", resp.Type, string(resp.Payload))
	}

	var got CreateAutomationResponse
	if err := json.Unmarshal(resp.Payload, &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Automation == nil {
		t.Fatalf("expected automation in response, got %+v", got)
	}
	if got.ID == "" {
		t.Fatalf("expected non-empty automation id, got %+v", got)
	}
	if got.WebhookSecret == "" {
		t.Fatal("expected non-empty webhook secret in create response")
	}
}

func TestWsRevealWebhookSecret_Roundtrip(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID:       "ws-1",
		Name:              "reveal me",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationWebhookRevealSecret, map[string]any{"id": a.ID, "workspace_id": "ws-1"})
	resp, err := wsRevealWebhookSecret(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("expected response, got %v: %s", resp.Type, string(resp.Payload))
	}

	var got RevealWebhookSecretResponse
	if err := json.Unmarshal(resp.Payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.WebhookSecret == "" {
		t.Fatal("expected non-empty webhook secret")
	}
	// The reveal must return the same secret that the store generated —
	// otherwise the user's copy from the create response would stop working.
	if got.WebhookSecret != a.WebhookSecret {
		t.Errorf("reveal returned a different secret than create: reveal=%q create=%q", got.WebhookSecret, a.WebhookSecret)
	}
}

func TestWsRevealWebhookSecret_NotFound(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationWebhookRevealSecret, map[string]any{"id": "does-not-exist", "workspace_id": "ws-1"})
	resp, err := wsRevealWebhookSecret(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("expected error response, got %v: %s", resp.Type, string(resp.Payload))
	}

	var ep ws.ErrorPayload
	if err := json.Unmarshal(resp.Payload, &ep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ep.Code != ws.ErrorCodeNotFound {
		t.Errorf("expected NOT_FOUND, got %q", ep.Code)
	}
}

func TestWsRevealWebhookSecret_RequiresID(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationWebhookRevealSecret, map[string]any{})
	resp, err := wsRevealWebhookSecret(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("expected error, got %v: %s", resp.Type, string(resp.Payload))
	}
	var ep ws.ErrorPayload
	if err := json.Unmarshal(resp.Payload, &ep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ep.Code != ws.ErrorCodeBadRequest {
		t.Errorf("expected BAD_REQUEST, got %q", ep.Code)
	}
}

func TestWsRevealWebhookSecret_RejectsCrossWorkspace(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	// Create automation in workspace A.
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID:       "ws-A",
		Name:              "workspace A automation",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Attempt to reveal using workspace B's id — must return NOT_FOUND, not the secret.
	req, _ := ws.NewRequest("req-1", ws.ActionAutomationWebhookRevealSecret, map[string]any{"id": a.ID, "workspace_id": "ws-B"})
	resp, err := wsRevealWebhookSecret(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("expected error response for cross-workspace reveal, got %v: %s", resp.Type, string(resp.Payload))
	}

	var ep ws.ErrorPayload
	if err := json.Unmarshal(resp.Payload, &ep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ep.Code != ws.ErrorCodeNotFound {
		t.Errorf("expected NOT_FOUND to avoid disclosing existence, got %q", ep.Code)
	}
}

func TestWsDeleteRun_DeletesRun(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	// Create an automation and a run.
	a := &Automation{WorkspaceID: "ws-1", Name: "X", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSkipped,
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := svc.store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	req, err := ws.NewRequest("req-del", ws.ActionAutomationRunDelete, map[string]string{"run_id": run.ID})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := wsDeleteRun(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("expected response, got %v: %s", resp.Type, string(resp.Payload))
	}

	// Run should be gone.
	got, _ := svc.store.GetRun(ctx, run.ID)
	if got != nil {
		t.Error("expected run to be deleted")
	}
}

func TestWsDeleteRun_RequiresRunID(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, err := ws.NewRequest("req-1", ws.ActionAutomationRunDelete, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := wsDeleteRun(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	var ep struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(resp.Payload, &ep)
	if ep.Code != ws.ErrorCodeBadRequest {
		t.Errorf("expected BAD_REQUEST, got %q", ep.Code)
	}
}

func TestWsDeleteAllRuns_ClearsAllRuns(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "Y", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := svc.store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusSkipped,
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	req, err := ws.NewRequest("req-all", ws.ActionAutomationRunsDeleteAll, map[string]string{"automation_id": a.ID})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := wsDeleteAllRuns(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("expected response, got %v: %s", resp.Type, string(resp.Payload))
	}

	runs, _ := svc.store.ListRuns(ctx, a.ID, 50)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs after delete-all, got %d", len(runs))
	}
}

// fakeTaskDeleter records deletions and can inject errors per task ID.
type fakeTaskDeleter struct {
	deleted []string
	errors  map[string]error
}

func (f *fakeTaskDeleter) DeleteTask(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	if f.errors != nil {
		if err, ok := f.errors[id]; ok {
			return err
		}
	}
	return nil
}

func TestService_DeleteRun_CallsTaskDeleter(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{}
	svc.SetTaskDeleter(deleter)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		TaskID:       "task-xyz",
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := svc.store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteRun(ctx, run.ID); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}

	// Task deleter must have been called.
	if len(deleter.deleted) != 1 || deleter.deleted[0] != "task-xyz" {
		t.Errorf("expected DeleteTask(task-xyz), got %v", deleter.deleted)
	}
	// Run row must be gone.
	got, _ := svc.store.GetRun(ctx, run.ID)
	if got != nil {
		t.Error("expected run row to be removed")
	}
}

func TestService_DeleteRun_TaskNotFound_StillDeletesRun(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{
		errors: map[string]error{"task-gone": taskrepo.ErrTaskNotFound},
	}
	svc.SetTaskDeleter(deleter)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "B", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSkipped,
		TaskID:       "task-gone",
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := svc.store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	// Must succeed even though the task is not found.
	if err := svc.DeleteRun(ctx, run.ID); err != nil {
		t.Fatalf("DeleteRun with not-found task: %v", err)
	}

	// Run row must still be gone.
	got, _ := svc.store.GetRun(ctx, run.ID)
	if got != nil {
		t.Error("expected run row to be removed despite task-not-found")
	}
}

func TestService_DeleteAllRuns_CallsTaskDeleterForEach(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{}
	svc.SetTaskDeleter(deleter)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "C", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	taskIDs := []string{"task-1", "task-2", "task-3"}
	for _, tid := range taskIDs {
		if err := svc.store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusTaskCreated,
			TaskID:       tid,
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Also one run with no task_id (fire-and-forget / skipped).
	if err := svc.store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSkipped,
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteAllRuns(ctx, a.ID); err != nil {
		t.Fatalf("DeleteAllRuns: %v", err)
	}

	// All three task IDs must have been passed to DeleteTask.
	if len(deleter.deleted) != 3 {
		t.Errorf("expected 3 task deletions, got %d: %v", len(deleter.deleted), deleter.deleted)
	}
	// All run rows gone.
	runs, _ := svc.store.ListRuns(ctx, a.ID, 50)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}
}

func TestService_DeleteAllRuns_TaskNotFound_StillClearsRuns(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{
		errors: map[string]error{"task-stale": taskrepo.ErrTaskNotFound},
	}
	svc.SetTaskDeleter(deleter)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "D", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	for _, tid := range []string{"task-stale", "task-ok"} {
		if err := svc.store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusTaskCreated,
			TaskID:       tid,
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.DeleteAllRuns(ctx, a.ID); err != nil {
		t.Fatalf("DeleteAllRuns with not-found task: %v", err)
	}

	runs, _ := svc.store.ListRuns(ctx, a.ID, 50)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs after delete-all, got %d", len(runs))
	}
}

// TestDeleteAllRuns_AutomationSurvives is a regression guard: deleting all run
// rows — including issuing real DELETE SQL against task rows in the shared
// in-memory DB — must never delete the parent automation row. A real DB-level
// deleter catches SQL trigger / ON DELETE CASCADE regressions. Note: event
// handler side-effects are not covered here (no orchestrator runs in this test).
func TestDeleteAllRuns_AutomationSurvives(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create a minimal tasks table in the same in-memory DB so the
	// real-deleter can insert and then DELETE task rows.
	if _, err := store.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS tasks (id TEXT PRIMARY KEY, title TEXT, state TEXT)`); err != nil {
		t.Fatal("create tasks table:", err)
	}

	// sqliteTaskDeleter deletes from the real tasks table in the same DB —
	// any SQL cascade or trigger that touched automations would fire here.
	realDeleter := &sqliteTaskDeleter{db: store.db}

	log, _ := logger.NewFromZap(zap.NewNop())
	eb := bus.NewMemoryEventBus(log)
	svc := NewService(store, eb, log)
	svc.SetTaskDeleter(realDeleter)

	a := &Automation{WorkspaceID: "ws-1", Name: "Survives", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	// Insert real task rows and create runs referencing them.
	taskIDs := []string{"task-a", "task-b", "task-c"}
	for _, tid := range taskIDs {
		if _, err := store.db.ExecContext(ctx,
			`INSERT INTO tasks (id, title, state) VALUES (?, 'Test task', 'running')`, tid); err != nil {
			t.Fatal("insert task:", err)
		}
		if err := store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusTaskCreated,
			TaskID:       tid,
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Skipped runs without task IDs.
	for range 3 {
		if err := store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID, TriggerType: TriggerTypeScheduled,
			Status: RunStatusSkipped, TriggerData: json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.DeleteAllRuns(ctx, a.ID); err != nil {
		t.Fatalf("DeleteAllRuns: %v", err)
	}

	// Automation row must still exist after real task DELETEs fired.
	got, err := store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetAutomation after DeleteAllRuns: %v", err)
	}
	if got == nil {
		t.Error("automation was deleted by DeleteAllRuns — regression")
		return
	}
	if got.Name != "Survives" {
		t.Errorf("unexpected automation name %q", got.Name)
	}

	// Runs must be gone.
	runs, _ := store.ListRuns(ctx, a.ID, 50)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}
	// Task rows should have been deleted.
	if len(realDeleter.deleted) != 3 {
		t.Errorf("expected 3 task deletions, got %d: %v", len(realDeleter.deleted), realDeleter.deleted)
	}
}

// sqliteTaskDeleter deletes from the real tasks table in the same in-memory
// DB, so any SQL trigger or ON DELETE CASCADE that touches automations fires.
type sqliteTaskDeleter struct {
	db      *sqlx.DB
	deleted []string
}

func (d *sqliteTaskDeleter) DeleteTask(ctx context.Context, id string) error {
	d.deleted = append(d.deleted, id)
	_, err := d.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	return err
}
