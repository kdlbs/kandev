package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/controller"
	"github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// requireWSError asserts the dispatched message came back as an error frame
// with the given code. The code is what the frontend branches on, so a
// validation failure reported as INTERNAL_ERROR is a real regression even
// though both are "not a response".
func requireWSError(t *testing.T, msg *ws.Message, wantCode string) ws.ErrorPayload {
	t.Helper()
	if msg.Type != ws.MessageTypeError {
		t.Fatalf("message type = %q, want error; payload = %s", msg.Type, msg.Payload)
	}
	var payload ws.ErrorPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload.Code != wantCode {
		t.Fatalf("error code = %q, want %q (message %q)", payload.Code, wantCode, payload.Message)
	}
	return payload
}

// requireWSResponse asserts a success frame and decodes its payload.
func requireWSResponse(t *testing.T, msg *ws.Message, into any) {
	t.Helper()
	if msg.Type != ws.MessageTypeResponse {
		t.Fatalf("message type = %q, want response; payload = %s", msg.Type, msg.Payload)
	}
	if msg.ID != "req-1" {
		t.Fatalf("correlation id = %q, want req-1", msg.ID)
	}
	if err := json.Unmarshal(msg.Payload, into); err != nil {
		t.Fatalf("decode response payload: %v", err)
	}
}

// seedTemplate inserts a workflow template directly so the template routes have
// a known row to read, independent of whatever the repo seeds at boot.
func seedTemplate(t *testing.T, h *workflowHarness, id, name string) {
	t.Helper()
	// Distinct template step IDs matter: the apply path builds its
	// template-ID -> new-UUID map keyed by them, so two steps sharing an ID
	// would collapse into one row.
	steps := `[{"id":"tpl-backlog","name":"Backlog","position":0,"color":"#111111"},` +
		`{"id":"tpl-doing","name":"Doing","position":1,"color":"#222222"}]`
	if _, err := h.db.Exec(
		`INSERT INTO workflow_templates (id, name, description, is_system, steps, created_at, updated_at)
		 VALUES (?, ?, 'seeded', 0, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id, name, steps); err != nil {
		t.Fatalf("seed template: %v", err)
	}
}

// TestTemplateEndpoints covers both transports for the template pair: the
// listed template must include the seeded row, and an unknown ID is a 404 over
// HTTP and a NOT_FOUND frame over WS.
func TestTemplateEndpoints(t *testing.T) {
	t.Run("http", func(t *testing.T) {
		h := setupStepRouter(t)
		seedTemplate(t, h, "template-1", "Seeded Flow")

		list := doJSON(t, h.router, http.MethodGet, "/api/v1/workflow/templates", nil)
		if list.Code != http.StatusOK {
			t.Fatalf("list status = %d, body = %s", list.Code, list.Body.String())
		}
		var listed controller.ListTemplatesResponse
		if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if !containsTemplate(listed.Templates, "template-1") {
			t.Fatalf("templates = %#v, want the seeded row", listed.Templates)
		}

		get := doJSON(t, h.router, http.MethodGet, "/api/v1/workflow/templates/template-1", nil)
		if get.Code != http.StatusOK {
			t.Fatalf("get status = %d, body = %s", get.Code, get.Body.String())
		}
		var single controller.GetTemplateResponse
		if err := json.Unmarshal(get.Body.Bytes(), &single); err != nil {
			t.Fatalf("decode get: %v", err)
		}
		if single.Template == nil || single.Template.Name != "Seeded Flow" || len(single.Template.Steps) != 2 {
			t.Fatalf("template = %#v", single.Template)
		}

		missing := doJSON(t, h.router, http.MethodGet, "/api/v1/workflow/templates/nope", nil)
		requireJSONError(t, missing, http.StatusNotFound, "Template not found")
	})

	t.Run("ws", func(t *testing.T) {
		h := setupStepRouter(t)
		seedTemplate(t, h, "template-1", "Seeded Flow")

		var listed controller.ListTemplatesResponse
		requireWSResponse(t, h.dispatch(t, ws.ActionWorkflowTemplateList, map[string]any{}), &listed)
		if !containsTemplate(listed.Templates, "template-1") {
			t.Fatalf("templates = %#v, want the seeded row", listed.Templates)
		}

		var single controller.GetTemplateResponse
		requireWSResponse(t, h.dispatch(t, ws.ActionWorkflowTemplateGet, map[string]any{"id": "template-1"}), &single)
		if single.Template == nil || single.Template.Name != "Seeded Flow" {
			t.Fatalf("template = %#v", single.Template)
		}

		requireWSError(t, h.dispatch(t, ws.ActionWorkflowTemplateGet, map[string]any{"id": ""}), ws.ErrorCodeValidation)
		requireWSError(t, h.dispatch(t, ws.ActionWorkflowTemplateGet, map[string]any{"id": "nope"}), ws.ErrorCodeNotFound)
		requireWSError(t, h.dispatch(t, ws.ActionWorkflowTemplateGet, map[string]any{"id": 7}), ws.ErrorCodeBadRequest)
	})
}

func containsTemplate(templates []*models.WorkflowTemplate, id string) bool {
	for _, tpl := range templates {
		if tpl != nil && tpl.ID == id {
			return true
		}
	}
	return false
}

// TestWSStepReadActions covers the two step read actions on the WS transport,
// including the three-way split between a malformed payload (BAD_REQUEST), a
// missing required field (VALIDATION_ERROR) and a failed lookup.
func TestWSStepReadActions(t *testing.T) {
	h := setupStepRouter(t)
	step := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
	})

	t.Run("list", func(t *testing.T) {
		var listed controller.ListStepsResponse
		requireWSResponse(t,
			h.dispatch(t, ws.ActionWorkflowStepList, map[string]any{"workflow_id": "workflow-1"}), &listed)
		if len(listed.Steps) != 1 || listed.Steps[0].ID != step.ID {
			t.Fatalf("steps = %#v", listed.Steps)
		}

		payload := requireWSError(t,
			h.dispatch(t, ws.ActionWorkflowStepList, map[string]any{}), ws.ErrorCodeValidation)
		if payload.Message != "workflow_id is required" {
			t.Errorf("message = %q", payload.Message)
		}
		requireWSError(t,
			h.dispatch(t, ws.ActionWorkflowStepList, map[string]any{"workflow_id": 7}), ws.ErrorCodeBadRequest)
	})

	t.Run("get", func(t *testing.T) {
		var got controller.GetStepResponse
		requireWSResponse(t, h.dispatch(t, ws.ActionWorkflowStepGet, map[string]any{"id": step.ID}), &got)
		if got.Step == nil || got.Step.Name != "Backlog" {
			t.Fatalf("step = %#v", got.Step)
		}
		requireWSError(t, h.dispatch(t, ws.ActionWorkflowStepGet, map[string]any{"id": ""}), ws.ErrorCodeValidation)
		requireWSError(t, h.dispatch(t, ws.ActionWorkflowStepGet, map[string]any{"id": "nope"}), ws.ErrorCodeNotFound)
	})
}

// TestWSListHistoryAction covers the history action's required-field guard and
// its success shape.
func TestWSListHistoryAction(t *testing.T) {
	h := setupStepRouter(t)

	var got controller.ListHistoryResponse
	requireWSResponse(t,
		h.dispatch(t, ws.ActionWorkflowHistoryList, map[string]any{"session_id": "session-1"}), &got)
	if len(got.History) != 0 {
		t.Fatalf("history = %#v, want empty", got.History)
	}

	payload := requireWSError(t,
		h.dispatch(t, ws.ActionWorkflowHistoryList, map[string]any{}), ws.ErrorCodeValidation)
	if payload.Message != "session_id is required" {
		t.Errorf("message = %q", payload.Message)
	}
	requireWSError(t,
		h.dispatch(t, ws.ActionWorkflowHistoryList, map[string]any{"session_id": 7}), ws.ErrorCodeBadRequest)

	// The session access checker reports missing or foreign sessions with the
	// task-domain sentinel, not workflow service.ErrNotVisible. The WS mapping
	// must preserve the HTTP endpoint's 404 behavior for that error path.
	h.service.SetSessionAccessChecker(func(context.Context, string) error {
		return taskmodels.ErrTaskSessionNotFound
	})
	payload = requireWSError(t,
		h.dispatch(t, ws.ActionWorkflowHistoryList, map[string]any{"session_id": "session-1"}), ws.ErrorCodeNotFound)
	if payload.Message != notFoundMessage {
		t.Errorf("message = %q, want %q", payload.Message, notFoundMessage)
	}
}

// TestCreateStepsFromTemplate covers both transports of the template-apply
// mutation: the steps it writes, and each rejection.
//
// The apply path deliberately does NOT publish workflow_step.created for the
// steps it writes — the kanban board reloads its columns off the template
// response instead. Asserting the absence here means a later publish added to
// this path has to be a deliberate change rather than a silent one.
func TestCreateStepsFromTemplate(t *testing.T) {
	t.Run("http applies the template steps", func(t *testing.T) {
		h := setupStepRouter(t)
		seedTemplate(t, h, "template-1", "Seeded Flow")
		rec := recordStepEvents(t, h.eventBus)

		resp := doJSON(t, h.router, http.MethodPost, "/api/v1/workflows/workflow-1/workflow/steps",
			map[string]interface{}{"template_id": "template-1"})
		if resp.Code != http.StatusCreated || resp.Body.String() != `{"success":true}` {
			t.Fatalf("apply = %d %s", resp.Code, resp.Body.String())
		}
		steps := listStepNames(t, h, "workflow-1")
		if len(steps) != 2 || steps[0] != "Backlog" || steps[1] != "Doing" {
			t.Fatalf("persisted steps = %v, want the template's two steps", steps)
		}
		if len(rec.events) != 0 {
			t.Fatalf("template apply published %v; the board reloads instead", rec.subjects())
		}
	})

	t.Run("http rejections", func(t *testing.T) {
		h := setupStepRouter(t)
		malformed := doRaw(t, h, http.MethodPost, "/api/v1/workflows/workflow-1/workflow/steps", "{")
		requireJSONError(t, malformed, http.StatusBadRequest, "Invalid request")

		unknown := doJSON(t, h.router, http.MethodPost, "/api/v1/workflows/workflow-1/workflow/steps",
			map[string]interface{}{"template_id": "nope"})
		requireJSONError(t, unknown, http.StatusNotFound, "Step not found")
		if names := listStepNames(t, h, "workflow-1"); len(names) != 0 {
			t.Fatalf("rejected apply wrote %v", names)
		}
	})

	t.Run("ws applies the template steps", func(t *testing.T) {
		h := setupStepRouter(t)
		seedTemplate(t, h, "template-1", "Seeded Flow")

		var got map[string]bool
		requireWSResponse(t, h.dispatch(t, ws.ActionWorkflowStepCreate, map[string]any{
			"workflow_id": "workflow-1", "template_id": "template-1",
		}), &got)
		if !got["success"] {
			t.Fatalf("response = %#v", got)
		}
		if names := listStepNames(t, h, "workflow-1"); len(names) != 2 {
			t.Fatalf("persisted steps = %v", names)
		}
	})

	t.Run("ws rejections", func(t *testing.T) {
		h := setupStepRouter(t)
		for _, payload := range []map[string]any{
			{"template_id": "template-1"},
			{"workflow_id": "workflow-1"},
		} {
			frame := requireWSError(t,
				h.dispatch(t, ws.ActionWorkflowStepCreate, payload), ws.ErrorCodeValidation)
			if frame.Message != "workflow_id and template_id are required" {
				t.Errorf("message = %q", frame.Message)
			}
		}
		requireWSError(t, h.dispatch(t, ws.ActionWorkflowStepCreate, map[string]any{
			"workflow_id": 7,
		}), ws.ErrorCodeBadRequest)
		requireWSError(t, h.dispatch(t, ws.ActionWorkflowStepCreate, map[string]any{
			"workflow_id": "workflow-1", "template_id": "nope",
		}), ws.ErrorCodeInternalError)
		if names := listStepNames(t, h, "workflow-1"); len(names) != 0 {
			t.Fatalf("rejected apply wrote %v", names)
		}
	})
}

// TestStepMutationStoreFailureIsNotMisreported guards the default branch of
// writeStepMutationError: a write that fails for a reason which is neither a
// known validation message nor a not-found must stay a 500. Misreporting it as
// 400 or 404 would tell the board the input was bad and stop it retrying.
//
// The trigger is a template whose two steps share a template-level ID: they
// collapse onto one generated UUID and the second insert violates the primary
// key, which no validation rule catches.
func TestStepMutationStoreFailureIsNotMisreported(t *testing.T) {
	h := setupStepRouter(t)
	if _, err := h.db.Exec(
		`INSERT INTO workflow_templates (id, name, description, is_system, steps, created_at, updated_at)
		 VALUES ('collide','Colliding','',0,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`[{"id":"same","name":"One","position":0},{"id":"same","name":"Two","position":1}]`); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	rec := recordStepEvents(t, h.eventBus)

	resp := doJSON(t, h.router, http.MethodPost, "/api/v1/workflows/workflow-1/workflow/steps",
		map[string]interface{}{"template_id": "collide"})
	requireJSONError(t, resp, http.StatusInternalServerError, "Failed to save step")
	if len(rec.events) != 0 {
		t.Fatalf("failed apply published %v", rec.subjects())
	}
}

// TestUpdateStepMalformedJSON pins the 400 for an unparsable update body, with
// the parse reason preserved so the client can tell it apart from a rejected
// but well-formed request.
func TestUpdateStepMalformedJSON(t *testing.T) {
	h := setupStepRouter(t)
	step := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
	})
	rec := recordStepEvents(t, h.eventBus)

	resp := doRaw(t, h, http.MethodPut, "/api/v1/workflow/steps/"+step.ID, "{")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "Invalid request: ") {
		t.Fatalf("body = %s, want the parse reason", resp.Body.String())
	}
	if len(rec.events) != 0 {
		t.Fatalf("rejected update published %v", rec.subjects())
	}
}

// TestExportSingleWorkflowFailureIs500 covers the remaining export branch: an
// unresolvable workflow is a server error, not an empty document, so the UI
// does not offer the user a download of nothing.
func TestExportSingleWorkflowFailureIs500(t *testing.T) {
	h := setupStepRouter(t)
	h.service.SetWorkflowProvider(&fakeWorkflowProvider{})
	rec := doJSON(t, h.router, http.MethodGet, "/api/v1/workflows/nope/export", nil)
	requireJSONError(t, rec, http.StatusInternalServerError, "Failed to export workflow")
}

// listStepNames returns the persisted step names for a workflow, in order.
func listStepNames(t *testing.T, h *workflowHarness, workflowID string) []string {
	t.Helper()
	rec := doJSON(t, h.router, http.MethodGet, "/api/v1/workflows/"+workflowID+"/workflow/steps", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list steps status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var listed controller.ListStepsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode steps: %v", err)
	}
	names := make([]string, 0, len(listed.Steps))
	for _, step := range listed.Steps {
		names = append(names, step.Name)
	}
	return names
}

// TestUnknownWorkflowActionIsRejected pins the dispatcher contract the handlers
// rely on: an action this package never registered is answered, not dropped.
func TestUnknownWorkflowActionIsRejected(t *testing.T) {
	h := setupStepRouter(t)
	requireWSError(t, h.dispatch(t, "workflow.step.delete", map[string]any{"id": "x"}), ws.ErrorCodeUnknownAction)
	if h.dispatcher.HasHandler(events.WorkflowStepDeleted) {
		t.Fatal("the event subject must not be registered as a dispatchable action")
	}
}
