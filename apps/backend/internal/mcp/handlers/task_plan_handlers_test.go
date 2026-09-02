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

// The MCP plan actions and their browser twins in internal/task/handlers share
// one error mapper (internal/task/planws). The expectations below are spelled
// out as literals rather than derived from that mapper on purpose: the twin
// file internal/task/handlers/task_plan_handlers_test.go carries the same
// literals, so a change to the shared mapping has to fail on both surfaces
// before it can land.

const (
	mcpPlanTaskID  = "task-plan-mcp"
	mcpPlanlessID  = "task-plan-mcp-empty"
	mcpPlanWS      = "ws-plan-mcp"
	mcpPlanWF      = "wf-plan-mcp"
	mcpInvalidJSON = `{"task_id":`
)

// newMCPPlanTestHandlers builds Handlers with only the plan service wired —
// the plan actions touch nothing else.
func newMCPPlanTestHandlers(t *testing.T) *Handlers {
	h, _ := newMCPPlanTestHandlersWithRepo(t)
	return h
}

func newMCPPlanTestHandlersWithRepo(t *testing.T) (*Handlers, *sqliterepo.Repository) {
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
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: mcpPlanWS, Name: "Plan WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: mcpPlanWF, WorkspaceID: mcpPlanWS, Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	for _, id := range []string{mcpPlanTaskID, mcpPlanlessID} {
		now := time.Now().UTC()
		task := &models.Task{
			ID:          id,
			WorkspaceID: mcpPlanWS,
			WorkflowID:  mcpPlanWF,
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

	return &Handlers{planService: service.NewPlanService(repo, eventBus, log), logger: log}, repo
}

func mcpPlanMsg(t *testing.T, action string, payload string) *ws.Message {
	t.Helper()
	return &ws.Message{ID: "req-1", Action: action, Payload: json.RawMessage(payload)}
}

func assertMCPPlanError(t *testing.T, out *ws.Message, err error, wantCode, wantMessage string) {
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

// TestMCPPlanActionsRejectMalformedPayloads pins the decode failure reply.
func TestMCPPlanActionsRejectMalformedPayloads(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	actions := map[string]struct {
		action string
		handle func(context.Context, *ws.Message) (*ws.Message, error)
	}{
		"create": {ws.ActionMCPCreateTaskPlan, h.handleCreateTaskPlan},
		"get":    {ws.ActionMCPGetTaskPlan, h.handleGetTaskPlan},
		"update": {ws.ActionMCPUpdateTaskPlan, h.handleUpdateTaskPlan},
		"delete": {ws.ActionMCPDeleteTaskPlan, h.handleDeleteTaskPlan},
	}
	for name, tc := range actions {
		t.Run(name, func(t *testing.T) {
			out, err := tc.handle(ctx, mcpPlanMsg(t, tc.action, mcpInvalidJSON))
			assertMCPPlanError(t, out, err, ws.ErrorCodeBadRequest,
				"Invalid payload: unexpected end of JSON input")
		})
	}
}

// TestMCPPlanActionsRejectMissingTaskID pins the validation reply every plan
// action returns for an absent task_id, matching the browser surface.
func TestMCPPlanActionsRejectMissingTaskID(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	actions := map[string]struct {
		action  string
		payload string
		handle  func(context.Context, *ws.Message) (*ws.Message, error)
	}{
		"create": {ws.ActionMCPCreateTaskPlan, `{"content":"body"}`, h.handleCreateTaskPlan},
		"get":    {ws.ActionMCPGetTaskPlan, `{}`, h.handleGetTaskPlan},
		"update": {ws.ActionMCPUpdateTaskPlan, `{"content":"body"}`, h.handleUpdateTaskPlan},
		"delete": {ws.ActionMCPDeleteTaskPlan, `{}`, h.handleDeleteTaskPlan},
	}
	for name, tc := range actions {
		t.Run(name, func(t *testing.T) {
			out, err := tc.handle(ctx, mcpPlanMsg(t, tc.action, tc.payload))
			assertMCPPlanError(t, out, err, ws.ErrorCodeValidation, "task_id is required")
		})
	}
}

// TestMCPPlanActionsRequireContent pins the agent-only guard the browser
// surface does not have: an agent must send plan content.
func TestMCPPlanActionsRequireContent(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		out, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
			`{"task_id":"`+mcpPlanTaskID+`"}`))
		assertMCPPlanError(t, out, err, ws.ErrorCodeValidation, "content is required")
	})
	t.Run("update", func(t *testing.T) {
		out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
			`{"task_id":"`+mcpPlanTaskID+`"}`))
		assertMCPPlanError(t, out, err, ws.ErrorCodeValidation, "content is required")
	})
}

// TestMCPPlanActionsReportMissingPlan pins the 404 reply for the two actions
// that require an existing plan, matching the browser surface.
func TestMCPPlanActionsReportMissingPlan(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	t.Run("update", func(t *testing.T) {
		out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
			`{"task_id":"`+mcpPlanlessID+`","content":"body"}`))
		assertMCPPlanError(t, out, err, ws.ErrorCodeNotFound, "Task plan not found")
	})
	t.Run("delete", func(t *testing.T) {
		out, err := h.handleDeleteTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPDeleteTaskPlan,
			`{"task_id":"`+mcpPlanlessID+`"}`))
		assertMCPPlanError(t, out, err, ws.ErrorCodeNotFound, "Task plan not found")
	})
}

func TestMCPPlanCreateReportsMissingTask(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	out, err := h.handleCreateTaskPlan(context.Background(), mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		`{"task_id":"task-plan-mcp-missing","content":"body"}`))
	assertMCPPlanError(t, out, err, ws.ErrorCodeNotFound, "Task not found")
	if strings.Contains(strings.ToLower(string(out.Payload)), "constraint") {
		t.Fatalf("error payload leaks storage details: %s", out.Payload)
	}
}

func TestMCPPlanUpdateReportsMissingTask(t *testing.T) {
	h, repo := newMCPPlanTestHandlersWithRepo(t)
	ctx := context.Background()
	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Content: "initial", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	h.planService = service.NewPlanService(&missingTaskOnPlanWriteRepo{Repository: repo}, nil, h.logger)

	out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		`{"task_id":"`+mcpPlanTaskID+`","content":"updated"}`))
	assertMCPPlanError(t, out, err, ws.ErrorCodeNotFound, "Task not found")
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
	bool,
	bool,
) error {
	return repository.ErrTaskNotFound
}

// TestMCPPlanActionsSucceed pins the success payloads across a full CRUD round
// trip. Two replies deliberately differ from the browser surface and must not
// be "unified" away: get returns {} rather than null for a task with no plan,
// and an unattributed write is attributed to the agent, not the user.
func TestMCPPlanActionsSucceed(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	t.Run("get with no plan returns an empty object", func(t *testing.T) {
		out, err := h.handleGetTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPGetTaskPlan,
			`{"task_id":"`+mcpPlanlessID+`"}`))
		if err != nil {
			t.Fatalf("handleGetTaskPlan: %v", err)
		}
		if out.Type != ws.MessageTypeResponse {
			t.Fatalf("type = %q, want %q", out.Type, ws.MessageTypeResponse)
		}
		if string(out.Payload) != "{}" {
			t.Errorf("payload = %s, want {}", out.Payload)
		}
	})

	t.Run("create", func(t *testing.T) {
		out, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
			`{"task_id":"`+mcpPlanTaskID+`","title":"Ship it","content":"step one"}`))
		if err != nil {
			t.Fatalf("handleCreateTaskPlan: %v", err)
		}
		plan := decodeMCPPlanPayload(t, out)
		if plan["task_id"] != mcpPlanTaskID || plan["content"] != "step one" {
			t.Errorf("plan = %v", plan)
		}
		if plan["created_by"] != "agent" {
			t.Errorf("created_by = %v, want agent", plan["created_by"])
		}
	})

	t.Run("get", func(t *testing.T) {
		out, err := h.handleGetTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPGetTaskPlan,
			`{"task_id":"`+mcpPlanTaskID+`"}`))
		if err != nil {
			t.Fatalf("handleGetTaskPlan: %v", err)
		}
		if plan := decodeMCPPlanPayload(t, out); plan["content"] != "step one" {
			t.Errorf("content = %v, want step one", plan["content"])
		}
	})

	t.Run("update", func(t *testing.T) {
		out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
			`{"task_id":"`+mcpPlanTaskID+`","content":"step two"}`))
		if err != nil {
			t.Fatalf("handleUpdateTaskPlan: %v", err)
		}
		if plan := decodeMCPPlanPayload(t, out); plan["content"] != "step two" {
			t.Errorf("content = %v, want step two", plan["content"])
		}
	})

	t.Run("delete", func(t *testing.T) {
		out, err := h.handleDeleteTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPDeleteTaskPlan,
			`{"task_id":"`+mcpPlanTaskID+`"}`))
		if err != nil {
			t.Fatalf("handleDeleteTaskPlan: %v", err)
		}
		if out.Type != ws.MessageTypeResponse {
			t.Fatalf("type = %q, want %q", out.Type, ws.MessageTypeResponse)
		}
		if string(out.Payload) != `{"success":true}` {
			t.Errorf("payload = %s, want {\"success\":true}", out.Payload)
		}
	})
}

func decodeMCPPlanPayload(t *testing.T, out *ws.Message) map[string]any {
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
