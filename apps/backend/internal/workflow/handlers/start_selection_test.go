package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/service"
	"github.com/stretchr/testify/require"
)

type selectionHTTPProvider struct {
	service.WorkflowProvider
	db     *sqlx.DB
	before func()
	source string
}

type selectionWorkspaceProvider struct{}

func (selectionWorkspaceProvider) GetWorkspace(context.Context, string) (*taskmodels.Workspace, error) {
	return &taskmodels.Workspace{Name: "Improve Kandev"}, nil
}

func (p *selectionHTTPProvider) GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error) {
	if p.before != nil {
		f := p.before
		p.before = nil
		f()
	}
	var wf taskmodels.Workflow
	err := p.db.QueryRowContext(ctx, `SELECT id,workspace_id,name FROM workflows WHERE id=?`, id).Scan(&wf.ID, &wf.WorkspaceID, &wf.Name)
	wf.Source = p.source
	return &wf, err
}
func setupHTTPSelection(t *testing.T, initial string) (*workflowHarness, *selectionHTTPProvider, *service.Service) {
	t.Helper()
	h := setupStepRouter(t)
	_, err := h.db.Exec(`INSERT INTO workflows (id,workspace_id,name,created_at,updated_at) VALUES ('wf-test','ws-test','Selection',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	p := &selectionHTTPProvider{db: h.db}
	h.service.SetWorkflowProvider(p)
	writer := service.NewService(h.repo, logger.Default())
	t.Cleanup(func() { _ = writer.Close() })
	for i, id := range []string{"a", "b"} {
		require.NoError(t, writer.CreateStep(context.Background(), &models.WorkflowStep{ID: id, WorkflowID: "wf-test", Name: id, Position: i, IsStartStep: id == initial}))
	}
	return h, p, writer
}
func promoteHTTPSelection(t *testing.T, h *workflowHarness, writer *service.Service, id string) {
	t.Helper()
	step, err := h.repo.GetStep(context.Background(), id)
	require.NoError(t, err)
	step.IsStartStep = true
	require.NoError(t, writer.UpdateStep(context.Background(), step))
}

// @covers AC-TASKS-WORKFLOW-START-SELECTION-001.1, AC-TASKS-WORKFLOW-START-SELECTION-001.3, AC-TASKS-WORKFLOW-START-SELECTION-001.4
func TestWorkflowStartSelectionRegisteredREST(t *testing.T) {
	for _, initial := range []string{"a", "b"} {
		t.Run(initial, func(t *testing.T) {
			h, p, writer := setupHTTPSelection(t, initial)
			selected := "a"
			if initial == "a" {
				selected = "b"
			}
			p.before = func() { promoteHTTPSelection(t, h, writer, selected) }
			rec := recordStepEvents(t, h.eventBus)
			response := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/a", map[string]any{"name": "Renamed"})
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var step models.WorkflowStep
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &step))
			require.Equal(t, selected == "a", step.IsStartStep)
			event := rec.only(t, events.WorkflowStepUpdated)
			require.Equal(t, "a", event.step["id"])
			require.Equal(t, "Renamed", event.step["name"])
			require.Equal(t, selected == "a", event.step["is_start_step"])
			require.Equal(t, step.UpdatedAt, event.step["updated_at"])
			rec.events = nil
			nullResponse := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/a", map[string]any{"name": "Renamed", "is_start_step": nil})
			require.Equal(t, http.StatusOK, nullResponse.Code)
			var nullStep models.WorkflowStep
			require.NoError(t, json.Unmarshal(nullResponse.Body.Bytes(), &nullStep))
			require.Equal(t, selected == "a", nullStep.IsStartStep)
			require.Equal(t, selected == "a", rec.only(t, events.WorkflowStepUpdated).step["is_start_step"])
			for _, id := range []string{"a", "b"} {
				stored, err := h.repo.GetStep(context.Background(), id)
				require.NoError(t, err)
				require.Equal(t, id == selected, stored.IsStartStep)
			}
		})
	}
	t.Run("explicit and failed write", testHTTPSelectionControls)
}
func testHTTPSelectionControls(t *testing.T) {
	h, _, _ := setupHTTPSelection(t, "a")
	rec := recordStepEvents(t, h.eventBus)
	response := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/b", map[string]any{"is_start_step": true})
	require.Equal(t, http.StatusOK, response.Code)
	require.Len(t, rec.events, 2)
	require.Equal(t, "a", rec.events[0].step["id"])
	require.Equal(t, false, rec.events[0].step["is_start_step"])
	require.Equal(t, "b", rec.events[1].step["id"])
	require.Equal(t, true, rec.events[1].step["is_start_step"])
	rec.events = nil
	response = doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/b", map[string]any{"is_start_step": false})
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, false, rec.only(t, events.WorkflowStepUpdated).step["is_start_step"])
	rec.events = nil
	response = doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/a", map[string]any{"is_start_step": true})
	require.Equal(t, http.StatusOK, response.Code)
	rec.events = nil
	before, err := h.repo.ListStepsByWorkflow(context.Background(), "wf-test")
	require.NoError(t, err)
	_, err = h.db.Exec(`CREATE TRIGGER reject_selection BEFORE UPDATE OF name ON workflow_steps WHEN NEW.id='b' BEGIN SELECT RAISE(ABORT,'injected target failure'); END`)
	require.NoError(t, err)
	response = doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/b", map[string]any{"name": "Rejected", "is_start_step": true})
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Empty(t, rec.events)
	after, err := h.repo.ListStepsByWorkflow(context.Background(), "wf-test")
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// @covers AC-TASKS-WORKFLOW-START-SELECTION-001.4
func TestWorkflowStartSelectionAccessFailures(t *testing.T) {
	t.Run("foreign and missing", func(t *testing.T) {
		h := setupScopedRouter(t)
		rec := recordStepEvents(t, h.eventBus)
		before, err := h.repo.GetStep(context.Background(), h.stepA)
		require.NoError(t, err)
		foreign := doAs(t, h, asUser(userB), http.MethodPut, "/api/v1/workflow/steps/"+h.stepA, map[string]any{"name": "Rejected", "is_start_step": true})
		missing := doAs(t, h, asUser(userB), http.MethodPut, "/api/v1/workflow/steps/missing", map[string]any{"name": "Rejected"})
		require.Equal(t, http.StatusNotFound, foreign.Code)
		require.Equal(t, foreign.Code, missing.Code)
		require.Equal(t, foreign.Body.String(), missing.Body.String())
		require.Empty(t, rec.events)
		after, err := h.repo.GetStep(context.Background(), h.stepA)
		require.NoError(t, err)
		require.Equal(t, before, after)
	})
	for _, mode := range []string{"read-only", "workspace-read-only", "invalid", "null"} {
		t.Run(mode, func(t *testing.T) {
			h, p, _ := setupHTTPSelection(t, "a")
			rec := recordStepEvents(t, h.eventBus)
			body := map[string]any{"name": "Rejected", "is_start_step": true}
			expected := http.StatusBadRequest
			switch mode {
			case "read-only":
				p.source = taskmodels.WorkflowSourceGitHub
				expected = http.StatusConflict
			case "workspace-read-only":
				h.service.SetWorkspaceProvider(selectionWorkspaceProvider{})
				expected = http.StatusConflict
			case "invalid":
				body["wip_limit"] = -1
			case "null":
				body["complete_task_on_enter"] = nil
			}
			before, err := h.repo.ListStepsByWorkflow(context.Background(), "wf-test")
			require.NoError(t, err)
			response := doJSON(t, h.router, http.MethodPut, "/api/v1/workflow/steps/b", body)
			require.Equal(t, expected, response.Code, response.Body.String())
			require.Empty(t, rec.events)
			after, err := h.repo.ListStepsByWorkflow(context.Background(), "wf-test")
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
