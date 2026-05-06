package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

type handlerHarness struct {
	router     *gin.Engine
	token      string
	comments   *handlerCommentWriter
	status     *recordingTaskStatusUpdater
	runEvents  *recordingRunEvents
	agentSvc   *agents.AgentService
	repository *sqlite.Repository
}

type handlerCommentWriter struct {
	comments []*models.TaskComment
}

func (r *handlerCommentWriter) CreateComment(_ context.Context, comment *models.TaskComment) error {
	r.comments = append(r.comments, comment)
	return nil
}

type recordingTaskStatusUpdater struct {
	updates []TaskStatusUpdate
}

func (r *recordingTaskStatusUpdater) UpdateTaskStatusAsAgent(_ context.Context, update TaskStatusUpdate) error {
	r.updates = append(r.updates, update)
	return nil
}

type handlerTaskCreator struct{}

func (r *handlerTaskCreator) CreateOfficeSubtaskAsAgent(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
	_ string,
) (string, error) {
	return "task-created", nil
}

type recordingRunEvent struct {
	runID     string
	eventType string
	level     string
	payload   map[string]interface{}
}

type recordingRunEvents struct {
	events []recordingRunEvent
}

func (r *recordingRunEvents) AppendRunEvent(
	_ context.Context,
	runID string,
	eventType string,
	level string,
	payload map[string]interface{},
) {
	r.events = append(r.events, recordingRunEvent{
		runID:     runID,
		eventType: eventType,
		level:     level,
		payload:   payload,
	})
}

func TestRuntimeHandler_PostCommentUsesRuntimeToken(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{
		CanPostComments: true,
	}.WithTaskScope("task-1"))

	resp := h.request(t, http.MethodPost, "/runtime/comments", map[string]string{
		"body": "runtime comment",
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	if len(h.comments.comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(h.comments.comments))
	}
	comment := h.comments.comments[0]
	if comment.TaskID != "task-1" || comment.AuthorID != "agent-1" || comment.AuthorType != "agent" {
		t.Fatalf("comment identity = %#v", comment)
	}
	assertActionRunEvent(t, h.runEvents, "post_comment", "task", "task-1")
}

func TestRuntimeHandler_WriteAndReadMemoryUsesRuntimeNamespace(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{
		CanReadMemory:  true,
		CanWriteMemory: true,
	}.WithTaskScope("task-1"))

	path := "/runtime/memory/workspaces/ws-1/memory/agents/agent-1/knowledge/runtime-note"
	put := h.request(t, http.MethodPut, path, map[string]string{"content": "remember this"})
	if put.Code != http.StatusOK {
		t.Fatalf("put status = %d, want %d; body=%s", put.Code, http.StatusOK, put.Body.String())
	}

	get := h.request(t, http.MethodGet, path, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d; body=%s", get.Code, http.StatusOK, get.Body.String())
	}
	var body struct {
		Memory models.AgentMemory `json:"memory"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode memory: %v", err)
	}
	if body.Memory.Content != "remember this" || body.Memory.Layer != "knowledge" {
		t.Fatalf("unexpected memory: %+v", body.Memory)
	}
	assertActionRunEvent(t, h.runEvents, "write_memory", "memory",
		"/workspaces/ws-1/memory/agents/agent-1/knowledge/runtime-note")
}

func TestRuntimeHandler_CreateAgentBindsSnakeCasePayload(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{
		CanCreateAgents: true,
	}.WithTaskScope("task-1"))

	resp := h.request(t, http.MethodPost, "/runtime/agents", map[string]interface{}{
		"name":                    "Runtime Worker",
		"role":                    "worker",
		"desired_skills":          `["runtime-skill"]`,
		"executor_preference":     `{"type":"local_pc"}`,
		"max_concurrent_sessions": 2,
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	var body struct {
		Agent models.AgentInstance `json:"agent"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode agent: %v", err)
	}
	if body.Agent.DesiredSkills != `["runtime-skill"]` ||
		body.Agent.ExecutorPreference != `{"type":"local_pc"}` ||
		body.Agent.MaxConcurrentSessions != 2 {
		t.Fatalf("snake-case fields did not bind: %+v", body.Agent)
	}
	assertActionRunEvent(t, h.runEvents, "create_agent", "agent", body.Agent.ID)
}

func TestRuntimeHandler_DeniesOutOfScopeStatusUpdateAndLogsRunEvent(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{
		CanUpdateTaskStatus: true,
	}.WithTaskScope("task-1"))

	resp := h.request(t, http.MethodPost, "/runtime/tasks/task-2/status", map[string]string{
		"status": "in_review",
	})

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if len(h.status.updates) != 0 {
		t.Fatalf("status updates = %d, want 0", len(h.status.updates))
	}
	assertDeniedRunEvent(t, h.runEvents, "update_task_status", "task", "task-2")
}

func TestRuntimeHandler_DeniesMissingCapabilityAndLogsRunEvent(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{}.WithTaskScope("task-1"))

	resp := h.request(t, http.MethodPost, "/runtime/tasks/task-1/status", map[string]string{
		"status": "in_review",
	})

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if len(h.status.updates) != 0 {
		t.Fatalf("status updates = %d, want 0", len(h.status.updates))
	}
	assertDeniedRunEvent(t, h.runEvents, "update_task_status", "task", "task-1")
}

func newRuntimeHandlerHarness(t *testing.T, caps Capabilities) *handlerHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	agentSvc := agents.NewAgentService(repo, logger.Default(), nil)
	agentSvc.SetAuth(agents.NewAgentAuth("runtime-handler-test-key"))
	agent := &models.AgentInstance{
		ID:          "agent-1",
		WorkspaceID: "ws-1",
		Name:        "CEO",
		Role:        models.AgentRoleCEO,
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	capabilityJSON, err := MarshalCapabilities(caps)
	if err != nil {
		t.Fatalf("marshal capabilities: %v", err)
	}
	token, err := agentSvc.MintRuntimeJWT("agent-1", "task-1", "ws-1", "run-1", "sess-1", capabilityJSON)
	if err != nil {
		t.Fatalf("mint runtime token: %v", err)
	}
	comments := &handlerCommentWriter{}
	status := &recordingTaskStatusUpdater{}
	runEvents := &recordingRunEvents{}
	router := gin.New()
	RegisterRoutes(router.Group(""), NewHandler(
		agentSvc,
		NewActions(ActionDependencies{
			Comments:   comments,
			Tasks:      &handlerTaskCreator{},
			TaskStatus: status,
			Agents:     agentSvc,
		}),
		nil,
		runEvents,
	))
	return &handlerHarness{
		router:     router,
		token:      token,
		comments:   comments,
		status:     status,
		runEvents:  runEvents,
		agentSvc:   agentSvc,
		repository: repo,
	}
}

func (h *handlerHarness) request(
	t *testing.T,
	method string,
	path string,
	payload interface{},
) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, req)
	return resp
}

func assertActionRunEvent(
	t *testing.T,
	runEvents *recordingRunEvents,
	action string,
	targetType string,
	targetID string,
) {
	t.Helper()
	if len(runEvents.events) != 1 {
		t.Fatalf("run events = %d, want 1", len(runEvents.events))
	}
	event := runEvents.events[0]
	if event.runID != "run-1" || event.eventType != "runtime.action" || event.level != "info" {
		t.Fatalf("event identity = %#v", event)
	}
	if event.payload["action"] != action ||
		event.payload["target_type"] != targetType ||
		event.payload["target_id"] != targetID ||
		event.payload["agent_id"] != "agent-1" ||
		event.payload["session_id"] != "sess-1" {
		t.Fatalf("event payload = %#v", event.payload)
	}
}

func assertDeniedRunEvent(
	t *testing.T,
	runEvents *recordingRunEvents,
	action string,
	targetType string,
	targetID string,
) {
	t.Helper()
	if len(runEvents.events) != 1 {
		t.Fatalf("run events = %d, want 1", len(runEvents.events))
	}
	event := runEvents.events[0]
	if event.runID != "run-1" || event.eventType != "runtime.denied" || event.level != "warn" {
		t.Fatalf("event identity = %#v", event)
	}
	if event.payload["action"] != action ||
		event.payload["target_type"] != targetType ||
		event.payload["target_id"] != targetID ||
		event.payload["agent_id"] != "agent-1" ||
		event.payload["session_id"] != "sess-1" {
		t.Fatalf("event payload = %#v", event.payload)
	}
}
