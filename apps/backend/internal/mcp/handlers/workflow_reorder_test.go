package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
	workflowsvc "github.com/kandev/kandev/internal/workflow/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func setupReorderHandlers(t *testing.T) *Handlers {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE workflows (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`)
	require.NoError(t, err)
	repo, err := repository.NewWithDB(db, db, nil)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO workflows (id, name, created_at, updated_at) VALUES ('wf', 'Test', ?, ?)`, time.Now().UTC(), time.Now().UTC())
	require.NoError(t, err)
	svc := workflowsvc.NewService(repo, testLogger(t))
	t.Cleanup(func() { _ = svc.Close() })
	return &Handlers{workflowSvc: svc, workflowCtrl: workflowctrl.NewController(svc), logger: testLogger(t).WithFields()}
}

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.4
func TestHandleReorderWorkflowStepsOrderValidation(t *testing.T) {
	for _, name := range []string{"empty workflow", "missing", "null", "malformed", "nonempty workflow", "duplicate", "omitted", "missing workflow"} {
		t.Run(name, func(t *testing.T) {
			h := setupReorderHandlers(t)
			ctx := context.Background()
			payload := map[string]any{"workflow_id": "wf", "step_ids": []string{}}
			want := ""
			switch name {
			case "missing":
				delete(payload, "step_ids")
				want = ws.ErrorCodeValidation
			case "null":
				payload["step_ids"] = nil
				want = ws.ErrorCodeValidation
			case "malformed":
				payload["step_ids"] = []int{1}
				want = ws.ErrorCodeBadRequest
			case "missing workflow":
				payload["workflow_id"] = "missing"
				want = ws.ErrorCodeNotFound
			case "nonempty workflow", "duplicate", "omitted":
				first := &wfmodels.WorkflowStep{WorkflowID: "wf", Name: "First", Color: "#111111"}
				second := &wfmodels.WorkflowStep{WorkflowID: "wf", Name: "Second", Color: "#222222"}
				require.NoError(t, h.workflowSvc.CreateStep(ctx, first))
				require.NoError(t, h.workflowSvc.CreateStep(ctx, second))
				if name == "duplicate" {
					payload["step_ids"] = []string{first.ID, first.ID}
				}
				if name == "omitted" {
					payload["step_ids"] = []string{first.ID}
				}
				want = ws.ErrorCodeValidation
			}
			before, err := h.workflowSvc.ListStepsByWorkflow(ctx, "wf")
			require.NoError(t, err)
			resp, err := h.handleReorderWorkflowSteps(ctx, makeWSMessage(t, ws.ActionMCPReorderWorkflowStep, payload))
			require.NoError(t, err)
			if want == "" {
				require.Equal(t, ws.MessageTypeResponse, resp.Type, "%s", resp.Payload)
			} else {
				requireErrorCode(t, resp, want)
			}
			after, err := h.workflowSvc.ListStepsByWorkflow(ctx, "wf")
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
