package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/controller"
	"github.com/kandev/kandev/internal/workflow/models"
)

// fakeWorkflowProvider is the task-domain seam the workflow service reads
// workflows through. It is an ordered in-memory map so export ordering and
// import dedup-by-name are both observable.
type fakeWorkflowProvider struct {
	workflows []*taskmodels.Workflow
	listErr   error
	created   []string
	updated   []string
}

func (p *fakeWorkflowProvider) ListWorkflows(
	_ context.Context, workspaceID string, includeHidden bool,
) ([]*taskmodels.Workflow, error) {
	if p.listErr != nil {
		return nil, p.listErr
	}
	out := make([]*taskmodels.Workflow, 0, len(p.workflows))
	for _, wf := range p.workflows {
		if wf.WorkspaceID != workspaceID {
			continue
		}
		if wf.Hidden && !includeHidden {
			continue
		}
		out = append(out, wf)
	}
	return out, nil
}

func (p *fakeWorkflowProvider) GetWorkflow(_ context.Context, id string) (*taskmodels.Workflow, error) {
	for _, wf := range p.workflows {
		if wf.ID == id {
			return wf, nil
		}
	}
	return nil, errors.New("workflow not found")
}

func (p *fakeWorkflowProvider) CreateWorkflow(
	_ context.Context, workspaceID, name, description string,
) (*taskmodels.Workflow, error) {
	wf := &taskmodels.Workflow{
		ID:          "created-" + name,
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
	}
	p.workflows = append(p.workflows, wf)
	p.created = append(p.created, name)
	return wf, nil
}

func (p *fakeWorkflowProvider) UpdateWorkflow(_ context.Context, workflow *taskmodels.Workflow) error {
	p.updated = append(p.updated, workflow.ID)
	return nil
}

// fakeWorkspaceProvider resolves workspaces for the read-only guard.
type fakeWorkspaceProvider struct {
	workspaces map[string]*taskmodels.Workspace
}

func (p *fakeWorkspaceProvider) GetWorkspace(_ context.Context, id string) (*taskmodels.Workspace, error) {
	if ws, ok := p.workspaces[id]; ok {
		return ws, nil
	}
	return nil, errors.New("workspace not found")
}

// doRaw issues a request with a raw (possibly non-JSON) body.
func doRaw(t *testing.T, h *workflowHarness, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

// requireJSONError asserts the response is exactly {"error": <want>}.
func requireJSONError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantError string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, wantStatus, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %s: %v", rec.Body.String(), err)
	}
	if body["error"] != wantError {
		t.Fatalf("error = %q, want %q", body["error"], wantError)
	}
}

// TestListStepsEndpoints pins the two list surfaces. The workspace variant
// resolves through a join, so a step created under a workflow in another
// workspace must not leak into it.
func TestListStepsEndpoints(t *testing.T) {
	h := setupStepRouter(t)
	if _, err := h.db.Exec(
		`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		 VALUES ('workflow-1','ws-1','Main',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
		        ('workflow-2','ws-2','Other',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed workflows: %v", err)
	}
	mine := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
	})
	createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-2", "name": "Elsewhere", "position": 0,
	})

	t.Run("by workflow", func(t *testing.T) {
		rec := doJSON(t, h.router, http.MethodGet, "/api/v1/workflows/workflow-1/workflow/steps", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got controller.ListStepsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got.Steps) != 1 || got.Steps[0].ID != mine.ID || got.Steps[0].Name != "Backlog" {
			t.Fatalf("steps = %#v, want only the workflow-1 step", got.Steps)
		}
	})

	t.Run("by workspace", func(t *testing.T) {
		rec := doJSON(t, h.router, http.MethodGet, "/api/v1/workspaces/ws-1/workflow-steps", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got controller.ListStepsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got.Steps) != 1 || got.Steps[0].ID != mine.ID {
			t.Fatalf("steps = %#v, want only the ws-1 step", got.Steps)
		}
	})

	t.Run("empty workspace returns an empty list, not an error", func(t *testing.T) {
		rec := doJSON(t, h.router, http.MethodGet, "/api/v1/workspaces/ws-none/workflow-steps", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})
}

// TestGetStepEndpoint pins the single-step body and the not-found mapping.
func TestGetStepEndpoint(t *testing.T) {
	h := setupStepRouter(t)
	step := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Backlog", "position": 0, "color": "#123456",
	})

	rec := doJSON(t, h.router, http.MethodGet, "/api/v1/workflow/steps/"+step.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got controller.GetStepResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Step == nil || got.Step.ID != step.ID || got.Step.Name != "Backlog" || got.Step.Color != "#123456" {
		t.Fatalf("step = %#v", got.Step)
	}

	missing := doJSON(t, h.router, http.MethodGet, "/api/v1/workflow/steps/nope", nil)
	requireJSONError(t, missing, http.StatusNotFound, "Step not found")
}

// TestStepMutationValidationRejections pins each rejection code, because the
// handler collapses several service errors into three distinct statuses and
// the kanban UI branches on them.
func TestStepMutationValidationRejections(t *testing.T) {
	t.Run("create with malformed json", func(t *testing.T) {
		h := setupStepRouter(t)
		rec := recordStepEvents(t, h.eventBus)
		resp := doRaw(t, h, http.MethodPost, "/api/v1/workflow/steps", "{")
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", resp.Code, resp.Body.String())
		}
		if len(rec.events) != 0 {
			t.Fatalf("rejected create published %v", rec.subjects())
		}
	})

	t.Run("create without workflow_id or name", func(t *testing.T) {
		h := setupStepRouter(t)
		rec := recordStepEvents(t, h.eventBus)
		for _, body := range []map[string]interface{}{
			{"name": "Backlog"},
			{"workflow_id": "workflow-1"},
		} {
			resp := doJSON(t, h.router, http.MethodPost, "/api/v1/workflow/steps", body)
			requireJSONError(t, resp, http.StatusBadRequest, "workflow_id and name are required")
		}
		if len(rec.events) != 0 {
			t.Fatalf("rejected create published %v", rec.subjects())
		}
	})

	t.Run("update with a self-referential pull source", func(t *testing.T) {
		h := setupStepRouter(t)
		step := createStepViaHTTP(t, h.router, map[string]interface{}{
			"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
		})
		rec := recordStepEvents(t, h.eventBus)

		resp := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/"+step.ID, map[string]interface{}{
			"pull_from_step_id": step.ID,
		})
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", resp.Code, resp.Body.String())
		}
		if len(rec.events) != 0 {
			t.Fatalf("rejected update published %v", rec.subjects())
		}
	})

	t.Run("update of a missing step is 404", func(t *testing.T) {
		h := setupStepRouter(t)
		rec := recordStepEvents(t, h.eventBus)
		resp := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/nope", map[string]interface{}{
			"name": "Ghost",
		})
		requireJSONError(t, resp, http.StatusNotFound, "Step not found")
		if len(rec.events) != 0 {
			t.Fatalf("404 update published %v", rec.subjects())
		}
	})

	t.Run("delete of a missing step is 404 and publishes nothing", func(t *testing.T) {
		h := setupStepRouter(t)
		rec := recordStepEvents(t, h.eventBus)
		resp := doJSON(t, h.router, http.MethodDelete, "/api/v1/workflow/steps/nope", nil)
		requireJSONError(t, resp, http.StatusNotFound, "Step not found")
		if len(rec.events) != 0 {
			t.Fatalf("404 delete published %v", rec.subjects())
		}
	})
}

// TestStepMutationsBlockedOnReadOnlyWorkflows covers the two read-only causes
// with their own message: a GitHub-synced workflow and a workflow in the
// Improve Kandev workspace. Both are 409 and neither may publish an event or
// touch the database.
func TestStepMutationsBlockedOnReadOnlyWorkflows(t *testing.T) {
	cases := []struct {
		name      string
		workflow  *taskmodels.Workflow
		workspace *taskmodels.Workspace
		wantError string
	}{
		{
			name:      "github-synced workflow",
			workflow:  &taskmodels.Workflow{ID: "workflow-1", WorkspaceID: "ws-1", Source: taskmodels.WorkflowSourceGitHub},
			workspace: &taskmodels.Workspace{ID: "ws-1", Name: "Normal"},
			wantError: "workflow is managed by GitHub sync and is read-only; " +
				"edit its definition in the synced repository",
		},
		{
			name:      "improve kandev workspace",
			workflow:  &taskmodels.Workflow{ID: "workflow-1", WorkspaceID: "ws-1"},
			workspace: &taskmodels.Workspace{ID: "ws-1", Name: taskmodels.WorkspaceNameImproveKandev},
			wantError: "this workspace is managed by Improve Kandev and is read-only",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := setupStepRouter(t)
			// Create the step before the guard is wired, so the delete/update
			// cases exercise a real row rather than a missing one.
			step := createStepViaHTTP(t, h.router, map[string]interface{}{
				"workflow_id": "workflow-1", "name": "Backlog", "position": 0,
			})
			h.service.SetWorkflowProvider(&fakeWorkflowProvider{workflows: []*taskmodels.Workflow{tc.workflow}})
			h.service.SetWorkspaceProvider(&fakeWorkspaceProvider{
				workspaces: map[string]*taskmodels.Workspace{tc.workspace.ID: tc.workspace},
			})
			rec := recordStepEvents(t, h.eventBus)

			create := doJSON(t, h.router, http.MethodPost, "/api/v1/workflow/steps", map[string]interface{}{
				"workflow_id": "workflow-1", "name": "Nope", "position": 1,
			})
			requireJSONError(t, create, http.StatusConflict, tc.wantError)

			update := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/"+step.ID,
				map[string]interface{}{"name": "Nope"})
			requireJSONError(t, update, http.StatusConflict, tc.wantError)

			del := doJSON(t, h.router, http.MethodDelete, "/api/v1/workflow/steps/"+step.ID, nil)
			requireJSONError(t, del, http.StatusConflict, tc.wantError)

			reorder := doJSON(t, h.router, http.MethodPut,
				"/api/v1/workflows/workflow-1/workflow/steps/reorder",
				map[string]interface{}{"step_ids": []string{step.ID}})
			requireJSONError(t, reorder, http.StatusConflict, tc.wantError)

			template := doJSON(t, h.router, http.MethodPost, "/api/v1/workflows/workflow-1/workflow/steps",
				map[string]interface{}{"template_id": "whatever"})
			requireJSONError(t, template, http.StatusConflict, tc.wantError)

			if len(rec.events) != 0 {
				t.Fatalf("read-only rejections published %v", rec.subjects())
			}
			steps, err := h.repo.ListStepsByWorkflow(context.Background(), "workflow-1")
			if err != nil {
				t.Fatalf("list steps: %v", err)
			}
			if len(steps) != 1 || steps[0].Name != "Backlog" {
				t.Fatalf("read-only rejections mutated the database: %#v", steps)
			}
		})
	}
}

// TestReorderStepsEndpoint pins the persisted positions, not just the 200:
// the handler's own body is a bare success flag, so only the stored order can
// tell a working reorder from a no-op.
func TestReorderStepsEndpoint(t *testing.T) {
	h := setupStepRouter(t)
	first := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "First", "position": 0,
	})
	second := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "workflow-1", "name": "Second", "position": 1,
	})

	rec := doJSON(t, h.router, http.MethodPut, "/api/v1/workflows/workflow-1/workflow/steps/reorder",
		map[string]interface{}{"step_ids": []string{second.ID, first.ID}})
	if rec.Code != http.StatusOK || rec.Body.String() != `{"success":true}` {
		t.Fatalf("reorder = %d %s", rec.Code, rec.Body.String())
	}

	steps, err := h.repo.ListStepsByWorkflow(context.Background(), "workflow-1")
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(steps) != 2 || steps[0].ID != second.ID || steps[1].ID != first.ID {
		t.Fatalf("stored order = %#v, want the reversed order", steps)
	}

	t.Run("malformed json is 400", func(t *testing.T) {
		resp := doRaw(t, h, http.MethodPut, "/api/v1/workflows/workflow-1/workflow/steps/reorder", "{")
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", resp.Code, resp.Body.String())
		}
	})

	t.Run("an unknown step id in the list is 404 and leaves the order alone", func(t *testing.T) {
		resp := doJSON(t, h.router, http.MethodPut, "/api/v1/workflows/workflow-1/workflow/steps/reorder",
			map[string]interface{}{"step_ids": []string{"nope"}})
		requireJSONError(t, resp, http.StatusNotFound, "Step not found")
		steps, err := h.repo.ListStepsByWorkflow(context.Background(), "workflow-1")
		if err != nil {
			t.Fatalf("list steps: %v", err)
		}
		if len(steps) != 2 || steps[0].ID != second.ID || steps[1].ID != first.ID {
			t.Fatalf("failed reorder changed the order: %#v", steps)
		}
	})
}

// TestListHistoryEndpointSeparatesDeniedFromBroken guards the comment on
// httpListHistoryBySession: an unauthorized/missing session must read as 404
// so session existence is not leaked, while a genuine read failure must stay a
// 500 rather than being masked as not-found.
func TestListHistoryEndpointSeparatesDeniedFromBroken(t *testing.T) {
	t.Run("empty history for an unknown session", func(t *testing.T) {
		h := setupStepRouter(t)
		rec := doJSON(t, h.router, http.MethodGet, "/api/v1/sessions/session-1/workflow/history", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got controller.ListHistoryResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got.History) != 0 {
			t.Fatalf("history = %#v, want empty", got.History)
		}
	})

	t.Run("denied session is 404", func(t *testing.T) {
		for _, denial := range []error{taskmodels.ErrTaskSessionNotFound, repoerrors.ErrTaskNotFound} {
			h := setupStepRouter(t)
			h.service.SetSessionAccessChecker(func(context.Context, string) error { return denial })
			rec := doJSON(t, h.router, http.MethodGet, "/api/v1/sessions/session-1/workflow/history", nil)
			requireJSONError(t, rec, http.StatusNotFound, "session not found")
		}
	})

	t.Run("read failure stays a 500", func(t *testing.T) {
		h := setupStepRouter(t)
		h.service.SetSessionAccessChecker(func(context.Context, string) error {
			return errors.New("database is locked")
		})
		rec := doJSON(t, h.router, http.MethodGet, "/api/v1/sessions/session-1/workflow/history", nil)
		requireJSONError(t, rec, http.StatusInternalServerError, "failed to list history")
	})
}

// TestExportWorkflowEndpoints pins the YAML content type and the exported
// document, plus the `ids` filter semantics: absent means "everything
// non-hidden", present restricts to that set and may name a hidden workflow.
func TestExportWorkflowEndpoints(t *testing.T) {
	newExportHarness := func(t *testing.T) *workflowHarness {
		t.Helper()
		h := setupStepRouter(t)
		h.service.SetWorkflowProvider(&fakeWorkflowProvider{workflows: []*taskmodels.Workflow{
			{ID: "workflow-1", WorkspaceID: "ws-1", Name: "Main", Description: "Primary"},
			{ID: "workflow-2", WorkspaceID: "ws-1", Name: "Hidden", Hidden: true},
		}})
		createStepViaHTTP(t, h.router, map[string]interface{}{
			"workflow_id": "workflow-1", "name": "Backlog", "position": 0, "color": "#123456",
		})
		return h
	}

	decodeExport := func(t *testing.T, rec *httptest.ResponseRecorder) models.WorkflowExport {
		t.Helper()
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/x-yaml" {
			t.Fatalf("Content-Type = %q, want application/x-yaml", ct)
		}
		var export models.WorkflowExport
		if err := yaml.Unmarshal(rec.Body.Bytes(), &export); err != nil {
			t.Fatalf("decode yaml %s: %v", rec.Body.String(), err)
		}
		return export
	}

	t.Run("single workflow export carries its steps", func(t *testing.T) {
		h := newExportHarness(t)
		export := decodeExport(t, doJSON(t, h.router, http.MethodGet, "/api/v1/workflows/workflow-1/export", nil))
		if export.Version != models.ExportVersion || export.Type != models.ExportType {
			t.Fatalf("export envelope = %d/%q", export.Version, export.Type)
		}
		if len(export.Workflows) != 1 || export.Workflows[0].Name != "Main" {
			t.Fatalf("workflows = %#v", export.Workflows)
		}
		steps := export.Workflows[0].Steps
		if len(steps) != 1 || steps[0].Name != "Backlog" || steps[0].Color != "#123456" {
			t.Fatalf("steps = %#v", steps)
		}
	})

	t.Run("workspace export without ids omits hidden workflows", func(t *testing.T) {
		h := newExportHarness(t)
		export := decodeExport(t, doJSON(t, h.router, http.MethodGet, "/api/v1/workspaces/ws-1/workflows/export", nil))
		if len(export.Workflows) != 1 || export.Workflows[0].Name != "Main" {
			t.Fatalf("workflows = %#v, want only the visible one", export.Workflows)
		}
	})

	t.Run("explicit ids restrict the export and may include a hidden workflow", func(t *testing.T) {
		h := newExportHarness(t)
		export := decodeExport(t, doJSON(t, h.router,
			http.MethodGet, "/api/v1/workspaces/ws-1/workflows/export?ids=workflow-2", nil))
		if len(export.Workflows) != 1 || export.Workflows[0].Name != "Hidden" {
			t.Fatalf("workflows = %#v, want only the named hidden one", export.Workflows)
		}
	})

	t.Run("an ids list that matches nothing exports nothing", func(t *testing.T) {
		h := newExportHarness(t)
		export := decodeExport(t, doJSON(t, h.router,
			http.MethodGet, "/api/v1/workspaces/ws-1/workflows/export?ids=%20,%20", nil))
		if len(export.Workflows) != 0 {
			t.Fatalf("workflows = %#v, want none", export.Workflows)
		}
	})

	t.Run("provider failure is 500", func(t *testing.T) {
		h := setupStepRouter(t)
		h.service.SetWorkflowProvider(&fakeWorkflowProvider{listErr: errors.New("database is locked")})
		rec := doJSON(t, h.router, http.MethodGet, "/api/v1/workspaces/ws-1/workflows/export", nil)
		requireJSONError(t, rec, http.StatusInternalServerError, "Failed to export workflows")
	})
}

// TestImportWorkflowsEndpoint covers a successful import (created workflow and
// its persisted steps), dedup-by-name, and the two rejection shapes: unparsable
// YAML and a structurally invalid document.
func TestImportWorkflowsEndpoint(t *testing.T) {
	const validYAML = `version: 1
type: kandev_workflow
workflows:
  - name: Imported
    description: From YAML
    steps:
      - name: Backlog
        position: 0
        color: "#111111"
      - name: Doing
        position: 1
        color: "#222222"
`

	t.Run("creates the workflow and persists its steps", func(t *testing.T) {
		h := setupStepRouter(t)
		provider := &fakeWorkflowProvider{}
		h.service.SetWorkflowProvider(provider)

		rec := doRaw(t, h, http.MethodPost, "/api/v1/workspaces/ws-1/workflows/import", validYAML)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got struct {
			Created []string `json:"created"`
			Skipped []string `json:"skipped"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got.Created) != 1 || got.Created[0] != "Imported" || len(got.Skipped) != 0 {
			t.Fatalf("result = %#v", got)
		}
		if len(provider.created) != 1 || provider.created[0] != "Imported" {
			t.Fatalf("provider created = %v", provider.created)
		}
		steps, err := h.repo.ListStepsByWorkflow(context.Background(), "created-Imported")
		if err != nil {
			t.Fatalf("list steps: %v", err)
		}
		if len(steps) != 2 || steps[0].Name != "Backlog" || steps[1].Name != "Doing" {
			t.Fatalf("persisted steps = %#v", steps)
		}
	})

	t.Run("a workflow whose name already exists is skipped, not duplicated", func(t *testing.T) {
		h := setupStepRouter(t)
		provider := &fakeWorkflowProvider{workflows: []*taskmodels.Workflow{
			{ID: "existing", WorkspaceID: "ws-1", Name: "Imported"},
		}}
		h.service.SetWorkflowProvider(provider)

		rec := doRaw(t, h, http.MethodPost, "/api/v1/workspaces/ws-1/workflows/import", validYAML)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got struct {
			Created []string `json:"created"`
			Skipped []string `json:"skipped"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got.Created) != 0 || len(got.Skipped) != 1 || got.Skipped[0] != "Imported" {
			t.Fatalf("result = %#v, want the name skipped", got)
		}
		if len(provider.created) != 0 {
			t.Fatalf("skipped import still created %v", provider.created)
		}
	})

	t.Run("unparsable yaml is 400", func(t *testing.T) {
		h := setupStepRouter(t)
		h.service.SetWorkflowProvider(&fakeWorkflowProvider{})
		rec := doRaw(t, h, http.MethodPost, "/api/v1/workspaces/ws-1/workflows/import", "version: [1")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Invalid YAML") {
			t.Fatalf("body = %s, want an Invalid YAML message", rec.Body.String())
		}
	})

	t.Run("structurally invalid export is 400 with the validation reason", func(t *testing.T) {
		h := setupStepRouter(t)
		provider := &fakeWorkflowProvider{}
		h.service.SetWorkflowProvider(provider)
		rec := doRaw(t, h, http.MethodPost, "/api/v1/workspaces/ws-1/workflows/import",
			"version: 99\ntype: kandev_workflow\nworkflows: []\n")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "unsupported export version") {
			t.Fatalf("body = %s, want the version reason", rec.Body.String())
		}
		if len(provider.created) != 0 {
			t.Fatalf("invalid import created %v", provider.created)
		}
	})
}
