package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// The task-plan WS actions and their MCP twins in internal/mcp/handlers share
// one error mapper (internal/task/planws). The expectations below are spelled
// out as literals rather than derived from that mapper on purpose: the twin
// file internal/mcp/handlers/task_plan_handlers_test.go carries the same
// literals, so a change to the shared mapping has to fail on both surfaces
// before it can land.

const (
	planTaskID  = "task-plan-ws"
	planlessID  = "task-plan-ws-empty"
	planTestWS  = "ws-plan-ws"
	planTestWF  = "wf-plan-ws"
	invalidJSON = `{"task_id":`
)

// newPlanTestHandlers builds TaskHandlers with only the plan service wired —
// the plan actions touch nothing else.
func newPlanTestHandlers(t *testing.T) *TaskHandlers {
	h, _ := newPlanTestHandlersWithRepo(t)
	return h
}

func newPlanTestHandlersWithRepo(t *testing.T) (*TaskHandlers, *sqliterepo.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	// Registered before the Provide call so the handle still closes if Provide
	// fails and the Fatalf below fires. Cleanups run LIFO, so this one runs
	// after the repository cleanup registered underneath it.
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("repository.Provide: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup repository: %v", err)
		}
	})

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })

	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: planTestWS, Name: "Plan WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: planTestWF, WorkspaceID: planTestWS, Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	for _, id := range []string{planTaskID, planlessID} {
		now := time.Now().UTC()
		task := &models.Task{
			ID:          id,
			WorkspaceID: planTestWS,
			WorkflowID:  planTestWF,
			Title:       "Plan target",
			State:       v1.TaskStateCreated,
			Priority:    "medium",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}

	return &TaskHandlers{planService: service.NewPlanService(repo, eventBus, log), logger: log}, repo
}

func planMsg(t *testing.T, action string, payload string) *ws.Message {
	t.Helper()
	return &ws.Message{ID: "req-1", Action: action, Payload: json.RawMessage(payload)}
}

// assertPlanError decodes a handler reply and checks the error code and message
// a client would see.
func assertPlanError(t *testing.T, out *ws.Message, err error, wantCode, wantMessage string) {
	t.Helper()
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if out.Type != ws.MessageTypeError {
		t.Fatalf("message type = %q, want %q (payload %s)", out.Type, ws.MessageTypeError, out.Payload)
	}
	var payload ws.ErrorPayload
	if jsonErr := json.Unmarshal(out.Payload, &payload); jsonErr != nil {
		t.Fatalf("unmarshal error payload: %v", jsonErr)
	}
	if payload.Code != wantCode {
		t.Errorf("code = %q, want %q", payload.Code, wantCode)
	}
	if payload.Message != wantMessage {
		t.Errorf("message = %q, want %q", payload.Message, wantMessage)
	}
}

// TestPlanWSActionsRejectMalformedPayloads pins the decode failure reply.
func TestPlanWSActionsRejectMalformedPayloads(t *testing.T) {
	h := newPlanTestHandlers(t)
	ctx := context.Background()

	actions := map[string]struct {
		action string
		handle func(context.Context, *ws.Message) (*ws.Message, error)
	}{
		"create": {ws.ActionTaskPlanCreate, h.wsCreateTaskPlan},
		"get":    {ws.ActionTaskPlanGet, h.wsGetTaskPlan},
		"update": {ws.ActionTaskPlanUpdate, h.wsUpdateTaskPlan},
		"delete": {ws.ActionTaskPlanDelete, h.wsDeleteTaskPlan},
	}
	for name, tc := range actions {
		t.Run(name, func(t *testing.T) {
			out, err := tc.handle(ctx, planMsg(t, tc.action, invalidJSON))
			assertPlanError(t, out, err, ws.ErrorCodeBadRequest,
				"Invalid payload: unexpected end of JSON input")
		})
	}
}

// TestPlanWSActionsRejectMissingTaskID pins the validation reply every plan
// action returns for an absent task_id.
func TestPlanWSActionsRejectMissingTaskID(t *testing.T) {
	h := newPlanTestHandlers(t)
	ctx := context.Background()

	actions := map[string]struct {
		action  string
		payload string
		handle  func(context.Context, *ws.Message) (*ws.Message, error)
	}{
		"create": {ws.ActionTaskPlanCreate, `{"content":"body"}`, h.wsCreateTaskPlan},
		"get":    {ws.ActionTaskPlanGet, `{}`, h.wsGetTaskPlan},
		"update": {ws.ActionTaskPlanUpdate, `{"content":"body"}`, h.wsUpdateTaskPlan},
		"delete": {ws.ActionTaskPlanDelete, `{}`, h.wsDeleteTaskPlan},
	}
	for name, tc := range actions {
		t.Run(name, func(t *testing.T) {
			out, err := tc.handle(ctx, planMsg(t, tc.action, tc.payload))
			assertPlanError(t, out, err, ws.ErrorCodeValidation, "task_id is required")
		})
	}
}

// TestPlanWSActionsReportMissingPlan pins the 404 reply for the two actions
// that require an existing plan.
func TestPlanWSActionsReportMissingPlan(t *testing.T) {
	h := newPlanTestHandlers(t)
	ctx := context.Background()

	t.Run("update", func(t *testing.T) {
		out, err := h.wsUpdateTaskPlan(ctx, planMsg(t, ws.ActionTaskPlanUpdate,
			`{"task_id":"`+planlessID+`","content":"body"}`))
		assertPlanError(t, out, err, ws.ErrorCodeNotFound, "Task plan not found")
	})
	t.Run("delete", func(t *testing.T) {
		out, err := h.wsDeleteTaskPlan(ctx, planMsg(t, ws.ActionTaskPlanDelete,
			`{"task_id":"`+planlessID+`"}`))
		assertPlanError(t, out, err, ws.ErrorCodeNotFound, "Task plan not found")
	})
}

func TestPlanWSCreateReportsMissingTask(t *testing.T) {
	h := newPlanTestHandlers(t)
	out, err := h.wsCreateTaskPlan(context.Background(), planMsg(t, ws.ActionTaskPlanCreate,
		`{"task_id":"task-plan-ws-missing","content":"body"}`))
	assertPlanError(t, out, err, ws.ErrorCodeNotFound, "Task not found")
	if strings.Contains(strings.ToLower(string(out.Payload)), "constraint") {
		t.Fatalf("error payload leaks storage details: %s", out.Payload)
	}
}

func TestPlanWSUpdateReportsMissingTask(t *testing.T) {
	h, repo := newPlanTestHandlersWithRepo(t)
	ctx := context.Background()
	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: planTaskID, Content: "initial", CreatedBy: "user",
	}); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	h.planService = service.NewPlanService(&missingTaskOnPlanWriteRepo{Repository: repo}, nil, h.logger)

	out, err := h.wsUpdateTaskPlan(ctx, planMsg(t, ws.ActionTaskPlanUpdate,
		`{"task_id":"`+planTaskID+`","content":"updated"}`))
	assertPlanError(t, out, err, ws.ErrorCodeNotFound, "Task not found")
	if strings.Contains(strings.ToLower(string(out.Payload)), "constraint") {
		t.Fatalf("error payload leaks storage details: %s", out.Payload)
	}
}

type missingTaskOnPlanWriteRepo struct {
	*sqliterepo.Repository
}

func (r *missingTaskOnPlanWriteRepo) WritePlanRevision(
	context.Context,
	*models.TaskPlan,
	*models.TaskPlanRevision,
	*string,
) error {
	return repository.ErrTaskNotFound
}

// TestPlanWSActionsSucceed pins the success payloads across a full CRUD round
// trip, including the browser surface's null reply for a task with no plan and
// its {"success":true} delete reply.
func TestPlanWSActionsSucceed(t *testing.T) {
	h := newPlanTestHandlers(t)
	ctx := context.Background()

	t.Run("get with no plan returns null", func(t *testing.T) {
		out, err := h.wsGetTaskPlan(ctx, planMsg(t, ws.ActionTaskPlanGet, `{"task_id":"`+planlessID+`"}`))
		if err != nil {
			t.Fatalf("wsGetTaskPlan: %v", err)
		}
		if out.Type != ws.MessageTypeResponse {
			t.Fatalf("type = %q, want %q", out.Type, ws.MessageTypeResponse)
		}
		if string(out.Payload) != "null" {
			t.Errorf("payload = %s, want null", out.Payload)
		}
	})

	t.Run("create", func(t *testing.T) {
		out, err := h.wsCreateTaskPlan(ctx, planMsg(t, ws.ActionTaskPlanCreate,
			`{"task_id":"`+planTaskID+`","title":"Ship it","content":"step one"}`))
		if err != nil {
			t.Fatalf("wsCreateTaskPlan: %v", err)
		}
		plan := decodePlanPayload(t, out)
		if plan["task_id"] != planTaskID || plan["content"] != "step one" {
			t.Errorf("plan = %v", plan)
		}
		// The browser surface defaults an unattributed write to the user.
		if plan["created_by"] != "user" {
			t.Errorf("created_by = %v, want user", plan["created_by"])
		}
	})

	t.Run("get", func(t *testing.T) {
		out, err := h.wsGetTaskPlan(ctx, planMsg(t, ws.ActionTaskPlanGet, `{"task_id":"`+planTaskID+`"}`))
		if err != nil {
			t.Fatalf("wsGetTaskPlan: %v", err)
		}
		if plan := decodePlanPayload(t, out); plan["content"] != "step one" {
			t.Errorf("content = %v, want step one", plan["content"])
		}
	})

	t.Run("update", func(t *testing.T) {
		out, err := h.wsUpdateTaskPlan(ctx, planMsg(t, ws.ActionTaskPlanUpdate,
			`{"task_id":"`+planTaskID+`","content":"step two"}`))
		if err != nil {
			t.Fatalf("wsUpdateTaskPlan: %v", err)
		}
		if plan := decodePlanPayload(t, out); plan["content"] != "step two" {
			t.Errorf("content = %v, want step two", plan["content"])
		}
	})

	t.Run("delete", func(t *testing.T) {
		out, err := h.wsDeleteTaskPlan(ctx, planMsg(t, ws.ActionTaskPlanDelete, `{"task_id":"`+planTaskID+`"}`))
		if err != nil {
			t.Fatalf("wsDeleteTaskPlan: %v", err)
		}
		if out.Type != ws.MessageTypeResponse {
			t.Fatalf("type = %q, want %q", out.Type, ws.MessageTypeResponse)
		}
		if string(out.Payload) != `{"success":true}` {
			t.Errorf("payload = %s, want {\"success\":true}", out.Payload)
		}
	})
}

func decodePlanPayload(t *testing.T, out *ws.Message) map[string]any {
	t.Helper()
	if out.Type != ws.MessageTypeResponse {
		t.Fatalf("type = %q, want %q (payload %s)", out.Type, ws.MessageTypeResponse, out.Payload)
	}
	var plan map[string]any
	if err := json.Unmarshal(out.Payload, &plan); err != nil {
		t.Fatalf("unmarshal plan payload: %v", err)
	}
	return plan
}
