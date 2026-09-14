package plugins

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/webapp"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestWebAppProtocolContextDoesNotExposeCredentials(t *testing.T) {
	svc := &Service{}
	binding := webapp.CapabilityBinding{
		UserID: "user-1", InstanceID: "instance-1", PluginID: "plugin-1",
		ReleaseID: "release-1", WebAppKey: "board", Placement: "task-canvas",
		ScopeKind: "task", WorkspaceID: "workspace-1", TaskID: "task-1",
		Permissions: []string{"api_read:tasks", "state"},
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	svc.handleWebAppProtocol(response, request, "ignored", binding, "v1/context")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := body["user_id"]; ok {
		t.Fatal("context exposed user_id")
	}
	if _, ok := body["token"]; ok {
		t.Fatal("context exposed token")
	}
	if body["instance_id"] != "instance-1" || body["protocol_version"] != float64(1) {
		t.Fatalf("context = %#v", body)
	}
}

func TestWebAppProtocolStateUsesRevisionPreconditions(t *testing.T) {
	store := newWebAppProtocolStateStore(t)
	svc := &Service{instanceState: store}
	binding := webapp.CapabilityBinding{
		UserID: "user-1", InstanceID: "instance-1", PluginID: "plugin-1",
		ReleaseID: "release-1", WebAppKey: "board", Placement: "task-canvas",
		ScopeKind: "task", Permissions: []string{"state"},
	}

	missing := httptest.NewRecorder()
	svc.handleWebAppProtocol(missing, httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"enabled":true}`)), "", binding, "v1/state/preferences")
	if missing.Code != http.StatusPreconditionRequired || !containsBody(missing, "plugin_state_precondition_required") {
		t.Fatalf("missing precondition = %d %s", missing.Code, missing.Body.String())
	}

	first := httptest.NewRecorder()
	initial := httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"enabled":true}`))
	initial.Header.Set("If-Match", `"0"`)
	svc.handleWebAppProtocol(first, initial, "", binding, "v1/state/preferences")
	if first.Code != http.StatusOK || !containsBody(first, `"revision":1`) {
		t.Fatalf("initial state write = %d %s", first.Code, first.Body.String())
	}

	stale := httptest.NewRecorder()
	staleRequest := httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"enabled":false}`))
	staleRequest.Header.Set("If-Match", `"0"`)
	svc.handleWebAppProtocol(stale, staleRequest, "", binding, "v1/state/preferences")
	if stale.Code != http.StatusConflict || !containsBody(stale, `"current_revision":1`) || containsBody(stale, "enabled") {
		t.Fatalf("stale state write = %d %s", stale.Code, stale.Body.String())
	}

	read := httptest.NewRecorder()
	svc.handleWebAppProtocol(read, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/state/preferences")
	if read.Code != http.StatusOK || !containsBody(read, `"enabled":true`) {
		t.Fatalf("state read = %d %s", read.Code, read.Body.String())
	}
}

func TestValidateWebAppPermissionsRequiresCurrentScopedGrants(t *testing.T) {
	connection, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "protocol-grants.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	connection.SetMaxOpenConns(1)
	pool := db.NewPool(sqlx.NewDb(connection, "sqlite3"), sqlx.NewDb(connection, "sqlite3"))
	t.Cleanup(func() { _ = pool.Close() })
	store, err := instances.NewStore(pool)
	if err != nil {
		t.Fatalf("new instance store: %v", err)
	}
	if err := store.Create(context.Background(), instances.Instance{
		ID: "instance-1", PluginID: "plugin-1", SourceKind: instances.SourceLocalCanvas,
		ScopeKind: instances.ScopeTask, WorkspaceID: "workspace-1", TaskID: "task-1",
		Status: instances.StatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	svc := &Service{}
	svc.SetWebAppStorage(store, nil)
	binding := webapp.CapabilityBinding{
		InstanceID: "instance-1", ScopeKind: instances.ScopeTask,
		Permissions: []string{"api_read:tasks", "state"},
	}
	raw := json.RawMessage(`{"capabilities":{"api_read":["tasks"],"state":true}}`)
	if err := svc.validateWebAppPermissions(context.Background(), binding, raw); err == nil {
		t.Fatal("permission validation succeeded without grants")
	}
	for _, grant := range []instances.Grant{
		{InstanceID: "instance-1", PermissionKind: "api_read", Resource: "tasks", ScopeCeiling: instances.ScopeTask, ApprovedBy: "user-1"},
		{InstanceID: "instance-1", PermissionKind: "state", ScopeCeiling: instances.ScopeTask, ApprovedBy: "user-1"},
	} {
		if err := store.AddGrant(context.Background(), grant); err != nil {
			t.Fatalf("add grant %s: %v", grant.PermissionKind, err)
		}
	}
	if err := svc.validateWebAppPermissions(context.Background(), binding, raw); err != nil {
		t.Fatalf("permission validation with grants: %v", err)
	}
}

func TestWebAppProtocolTaskWorkflowAndMessageRoutesUseHostAdapters(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{
		APIRead:  []string{"tasks", "workflows"},
		APIWrite: []string{"tasks", "messages"},
	})
	task := &taskmodels.Task{
		ID: "task-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		WorkflowStepID: "step-1", Title: "Canvas task", State: v1.TaskStateTODO,
	}
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "workspace-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"workspace-1": {task}}
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": task}
	d.workflows.workflows = map[string][]*taskmodels.Workflow{"workspace-1": {{ID: "workflow-1", WorkspaceID: "workspace-1", Name: "Main"}}}
	d.steps.steps = map[string][]*wfmodels.WorkflowStep{"workflow-1": {{ID: "step-1", WorkflowID: "workflow-1", Name: "Doing", Position: 1}}}
	d.taskWriter.moveResult = &TaskMoveResult{
		Task:         &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1", WorkflowStepID: "step-2", Title: "Moved"},
		Transitioned: true,
		FromStepID:   "step-1",
	}
	d.messenger.result = PluginMessageResult{SessionID: "session-1", Status: "queued"}

	svc := &Service{
		taskData: d.tasks, workflows: d.workflows, workflowSteps: d.steps,
		taskWriter:  d.taskWriter,
		messenger:   d.messenger,
		taskStarter: d.starter,
	}
	binding := webapp.CapabilityBinding{
		UserID: "user-1", InstanceID: "instance-1", PluginID: "plugin-1",
		ReleaseID: "release-1", WebAppKey: "board", Placement: "workspace-canvas",
		ScopeKind: instances.ScopeWorkspace, WorkspaceID: "workspace-1",
		Permissions: []string{"api_read:tasks", "api_read:workflows", "api_write:tasks", "api_write:messages"},
	}

	list := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/?limit=10", nil)
	svc.handleWebAppProtocol(list, request, "", binding, "v1/data/tasks")
	if list.Code != http.StatusOK || !containsBody(list, `"workflow_step_id":"step-1"`) {
		t.Fatalf("task list = %d %s", list.Code, list.Body.String())
	}

	steps := httptest.NewRecorder()
	svc.handleWebAppProtocol(steps, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/workflows/workflow-1/steps")
	if steps.Code != http.StatusOK || !containsBody(steps, `"id":"step-1"`) {
		t.Fatalf("workflow steps = %d %s", steps.Code, steps.Body.String())
	}

	patch := httptest.NewRecorder()
	patchRequest := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"workflow_step_id":"step-2"}`))
	svc.handleWebAppProtocol(patch, patchRequest, "", binding, "v1/data/tasks/task-1")
	if patch.Code != http.StatusOK || d.taskWriter.moveCalls != 1 || d.taskWriter.lastMove.TaskID != "task-1" || d.taskWriter.lastMove.WorkflowStepID != "step-2" {
		t.Fatalf("task patch = %d %s, move input = %+v", patch.Code, patch.Body.String(), d.taskWriter.lastMove)
	}
	if d.taskWriter.updateCalls != 0 {
		t.Fatalf("workflow-step patch used UpdateTask %d times", d.taskWriter.updateCalls)
	}

	message := httptest.NewRecorder()
	messageRequest := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"text":"continue","session_id":"session-1"}`))
	svc.handleWebAppProtocol(message, messageRequest, "", binding, "v1/data/tasks/task-1/messages")
	if message.Code != http.StatusAccepted || d.messenger.lastText != "continue" || d.messenger.lastSource != "plugin:plugin-1" {
		t.Fatalf("message send = %d %s, messenger = %+v", message.Code, message.Body.String(), d.messenger)
	}
}

func newWebAppProtocolStateStore(t *testing.T) *state.InstanceStore {
	t.Helper()
	connection, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "protocol-state.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	connection.SetMaxOpenConns(1)
	pool := db.NewPool(sqlx.NewDb(connection, "sqlite3"), sqlx.NewDb(connection, "sqlite3"))
	t.Cleanup(func() { _ = pool.Close() })
	store, err := state.NewInstanceStore(pool)
	if err != nil {
		t.Fatalf("new instance state store: %v", err)
	}
	return store
}

func containsBody(response *httptest.ResponseRecorder, value string) bool {
	return bytes.Contains(response.Body.Bytes(), []byte(value))
}
