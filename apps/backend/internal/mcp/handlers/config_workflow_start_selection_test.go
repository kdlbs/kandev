package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/workflow/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

type selectionMCPProvider struct {
	service.WorkflowProvider
	db     *sqlx.DB
	before func()
	source string
}

func (p *selectionMCPProvider) GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error) {
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

type mcpSelectionFixture struct {
	h          *Handlers
	provider   *selectionMCPProvider
	writer     *service.Service
	repo       *repository.Repository
	dispatcher *ws.Dispatcher
	db         *sqlx.DB
	events     *[]publishedStepEvent
}

func setupMCPSelection(t *testing.T, initial string) *mcpSelectionFixture {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE workflows (id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL DEFAULT '',workflow_template_id TEXT DEFAULT '',name TEXT NOT NULL,description TEXT DEFAULT '',created_at TIMESTAMP NOT NULL,updated_at TIMESTAMP NOT NULL)`)
	require.NoError(t, err)
	repo, err := repository.NewWithDB(db, db, nil)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO workflows (id,name,created_at,updated_at) VALUES ('wf-test','Selection',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	svc := service.NewService(repo, testLogger(t))
	writer := service.NewService(repo, testLogger(t))
	t.Cleanup(func() { _ = svc.Close(); _ = writer.Close() })
	p := &selectionMCPProvider{db: db}
	svc.SetWorkflowProvider(p)
	eb := bus.NewMemoryEventBus(testLogger(t))
	h := &Handlers{workflowSvc: svc, workflowCtrl: workflowctrl.NewController(svc), logger: testLogger(t), eventBus: eb}
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	for i, id := range []string{"a", "b"} {
		require.NoError(t, writer.CreateStep(context.Background(), &models.WorkflowStep{ID: id, WorkflowID: "wf-test", Name: id, Position: i, IsStartStep: id == initial}))
	}
	_, err = eb.Subscribe(events.WorkflowStepUpdated, func(_ context.Context, ev *bus.Event) error {
		require.Equal(t, "mcp-handlers", ev.Source)
		return nil
	})
	require.NoError(t, err)
	return &mcpSelectionFixture{h: h, provider: p, writer: writer, repo: repo, dispatcher: dispatcher, db: db, events: collectWorkflowStepEvents(t, eb, events.WorkflowStepUpdated, events.WorkflowStepCreated, events.WorkflowStepDeleted)}
}
func (f *mcpSelectionFixture) dispatch(t *testing.T, ctx context.Context, payload map[string]any) *ws.Message {
	t.Helper()
	response, err := f.dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPUpdateWorkflowStep, payload))
	require.NoError(t, err)
	require.NotNil(t, response)
	return response
}

// @covers AC-TASKS-WORKFLOW-START-SELECTION-001.1, AC-TASKS-WORKFLOW-START-SELECTION-001.2, AC-TASKS-WORKFLOW-START-SELECTION-001.3, AC-TASKS-WORKFLOW-START-SELECTION-001.4
func TestWorkflowStartSelectionRegisteredMCP(t *testing.T) {
	for _, initial := range []string{"a", "b"} {
		t.Run(initial, func(t *testing.T) {
			f := setupMCPSelection(t, initial)
			selected := "a"
			if initial == "a" {
				selected = "b"
			}
			f.provider.before = func() {
				step, err := f.repo.GetStep(context.Background(), selected)
				require.NoError(t, err)
				step.IsStartStep = true
				require.NoError(t, f.writer.UpdateStep(context.Background(), step))
			}
			response := f.dispatch(t, context.Background(), map[string]any{"step_id": "a", "name": "Renamed"})
			require.Equal(t, ws.MessageTypeResponse, response.Type)
			var body workflowctrl.GetStepResponse
			require.NoError(t, json.Unmarshal(response.Payload, &body))
			require.Equal(t, selected == "a", body.Step.IsStartStep)
			require.Empty(t, body.DemotedStartSteps)
			require.Len(t, *f.events, 1)
			event := (*f.events)[0]
			require.Equal(t, "a", event.step["id"])
			require.Equal(t, "Renamed", event.step["name"])
			require.Equal(t, selected == "a", event.step["is_start_step"])
			require.Equal(t, body.Step.UpdatedAt, event.step["updated_at"])
			*f.events = nil
			nullResponse := f.dispatch(t, context.Background(), map[string]any{"step_id": "a", "is_start_step": nil})
			require.Equal(t, ws.MessageTypeResponse, nullResponse.Type)
			var nullBody workflowctrl.GetStepResponse
			require.NoError(t, json.Unmarshal(nullResponse.Payload, &nullBody))
			require.Equal(t, selected == "a", nullBody.Step.IsStartStep)
			require.Empty(t, nullBody.DemotedStartSteps)
			require.Len(t, *f.events, 1)
			require.Equal(t, selected == "a", (*f.events)[0].step["is_start_step"])
			for _, id := range []string{"a", "b"} {
				stored, err := f.repo.GetStep(context.Background(), id)
				require.NoError(t, err)
				require.Equal(t, id == selected, stored.IsStartStep)
			}
		})
	}
	t.Run("explicit and rollback", testMCPSelectionControls)
	t.Run("foreign missing and read-only", testMCPSelectionFailures)
}
func testMCPSelectionControls(t *testing.T) {
	f := setupMCPSelection(t, "a")
	response := f.dispatch(t, context.Background(), map[string]any{"step_id": "b", "is_start_step": true})
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Len(t, *f.events, 2)
	require.Equal(t, "a", (*f.events)[0].step["id"])
	require.Equal(t, false, (*f.events)[0].step["is_start_step"])
	require.Equal(t, "b", (*f.events)[1].step["id"])
	require.Equal(t, true, (*f.events)[1].step["is_start_step"])
	*f.events = nil
	response = f.dispatch(t, context.Background(), map[string]any{"step_id": "b", "is_start_step": false})
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Len(t, *f.events, 1)
	require.Equal(t, false, (*f.events)[0].step["is_start_step"])
	response = f.dispatch(t, context.Background(), map[string]any{"step_id": "a", "is_start_step": true})
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	*f.events = nil
	before, err := f.repo.ListStepsByWorkflow(context.Background(), "wf-test")
	require.NoError(t, err)
	_, err = f.db.Exec(`CREATE TRIGGER reject_selection BEFORE UPDATE OF name ON workflow_steps WHEN NEW.id='b' BEGIN SELECT RAISE(ABORT,'injected failure'); END`)
	require.NoError(t, err)
	response = f.dispatch(t, context.Background(), map[string]any{"step_id": "b", "name": "Rejected", "is_start_step": true})
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Empty(t, *f.events)
	after, err := f.repo.ListStepsByWorkflow(context.Background(), "wf-test")
	require.NoError(t, err)
	require.Equal(t, before, after)
}
func testMCPSelectionFailures(t *testing.T) {
	f := setupMCPSelection(t, "a")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "foreign", Role: authn.RoleMember})
	f.h.workflowSvc.SetWorkflowAccessChecker(func(context.Context, string) error { return service.ErrNotVisible })
	before, err := f.repo.ListStepsByWorkflow(context.Background(), "wf-test")
	require.NoError(t, err)
	denied := f.dispatch(t, ctx, map[string]any{"step_id": "b", "name": "Rejected", "is_start_step": true})
	missing := f.dispatch(t, ctx, map[string]any{"step_id": "missing", "name": "Rejected"})
	require.Equal(t, ws.MessageTypeError, denied.Type)
	require.Equal(t, denied.Payload, missing.Payload)
	f.provider.source = taskmodels.WorkflowSourceGitHub
	readonly := f.dispatch(t, context.Background(), map[string]any{"step_id": "b", "is_start_step": true})
	require.Equal(t, ws.MessageTypeError, readonly.Type)
	require.Empty(t, *f.events)
	after, err := f.repo.ListStepsByWorkflow(context.Background(), "wf-test")
	require.NoError(t, err)
	require.Equal(t, before, after)
}
