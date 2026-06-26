package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventtypes "github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowsvc "github.com/kandev/kandev/internal/workflow/service"

	"github.com/kandev/kandev/internal/workflow/repository"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// memWorkflowProvider is a minimal in-memory WorkflowProvider for import tests.
// The canonical mock lives in the workflow/service test package, which can't be
// imported here, so we keep a tiny local copy covering only the methods the
// import path uses.
type memWorkflowProvider struct {
	workflows []*taskmodels.Workflow
}

func (m *memWorkflowProvider) ListWorkflows(_ context.Context, workspaceID string, _ bool) ([]*taskmodels.Workflow, error) {
	var result []*taskmodels.Workflow
	for _, wf := range m.workflows {
		if wf.WorkspaceID == workspaceID {
			result = append(result, wf)
		}
	}
	return result, nil
}

func (m *memWorkflowProvider) GetWorkflow(_ context.Context, id string) (*taskmodels.Workflow, error) {
	for _, wf := range m.workflows {
		if wf.ID == id {
			return wf, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *memWorkflowProvider) CreateWorkflow(_ context.Context, workspaceID, name, description string) (*taskmodels.Workflow, error) {
	now := time.Now().UTC()
	wf := &taskmodels.Workflow{
		ID:          "wf-" + name,
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.workflows = append(m.workflows, wf)
	return wf, nil
}

func (m *memWorkflowProvider) UpdateWorkflow(_ context.Context, workflow *taskmodels.Workflow) error {
	for i, wf := range m.workflows {
		if wf.ID == workflow.ID {
			m.workflows[i] = workflow
			return nil
		}
	}
	return sql.ErrNoRows
}

// setupImportHandlers wires a Handlers value backed by an in-memory workflow
// service so handleImportWorkflow can persist for real.
func setupImportHandlers(t *testing.T) (*Handlers, *memWorkflowProvider, *repository.Repository) {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	// Pin the pool to a single connection: each new connection to an in-memory
	// SQLite DB gets its own isolated database, so a second pooled connection
	// would not see the schema created on the first, causing flaky failures.
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	// workflows table is normally owned by the task repo; create it for the test.
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
		description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
	)`)
	require.NoError(t, err)

	repo, err := repository.NewWithDB(db, db, nil)
	require.NoError(t, err)

	svc := workflowsvc.NewService(repo, testLogger(t))
	provider := &memWorkflowProvider{}
	svc.SetWorkflowProvider(provider)

	h := &Handlers{workflowSvc: svc, logger: testLogger(t).WithFields()}
	return h, provider, repo
}

func TestHandleCreateWorkflowStep_PublishesDemotedStartStep(t *testing.T) {
	h, _, repo := setupImportHandlers(t)
	ctx := context.Background()
	h.workflowCtrl = workflowctrl.NewController(h.workflowSvc)
	eb := eventbus.NewMemoryEventBus(testLogger(t))
	h.eventBus = eb

	type publishedStepEvent struct {
		subject string
		step    map[string]interface{}
	}
	var published []publishedStepEvent
	capture := func(subject string) func(context.Context, *eventbus.Event) error {
		return func(_ context.Context, ev *eventbus.Event) error {
			data, ok := ev.Data.(map[string]interface{})
			require.True(t, ok)
			step, ok := data["step"].(map[string]interface{})
			require.True(t, ok)
			published = append(published, publishedStepEvent{subject: subject, step: step})
			return nil
		}
	}
	_, err := eb.Subscribe(eventtypes.WorkflowStepUpdated, capture(eventtypes.WorkflowStepUpdated))
	require.NoError(t, err)
	_, err = eb.Subscribe(eventtypes.WorkflowStepCreated, capture(eventtypes.WorkflowStepCreated))
	require.NoError(t, err)

	require.NoError(t, repo.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID:                 "old-start",
		WorkflowID:         "wf-test",
		Name:               "Old Start",
		Position:           0,
		IsStartStep:        true,
		ShowInCommandPanel: true,
	}))
	isStart := true
	msg := makeWSMessage(t, ws.ActionMCPCreateWorkflowStep, map[string]interface{}{
		"workflow_id":   "wf-test",
		"name":          "New Start",
		"position":      1,
		"is_start_step": isStart,
	})

	resp, err := h.handleCreateWorkflowStep(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Len(t, published, 2)

	assert.Equal(t, eventtypes.WorkflowStepUpdated, published[0].subject)
	assert.Equal(t, "old-start", published[0].step["id"])
	assert.False(t, published[0].step["is_start_step"].(bool))
	assert.Equal(t, eventtypes.WorkflowStepCreated, published[1].subject)
	assert.True(t, published[1].step["is_start_step"].(bool))
}

func TestHandleImportWorkflow_PersistsWorkflow(t *testing.T) {
	h, provider, repo := setupImportHandlers(t)

	doc := `version: 1
type: kandev_workflow
workflows:
  - name: Sprint Board
    description: A sprint workflow
    steps:
      - name: Todo
        position: 0
        color: "#3b82f6"
        is_start_step: true
      - name: Done
        position: 1
        color: "#22c55e"
`
	msg := makeWSMessage(t, ws.ActionMCPImportWorkflow, map[string]interface{}{
		"workspace_id": "ws-1",
		"document":     doc,
	})

	resp, err := h.handleImportWorkflow(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var result workflowsvc.ImportResult
	require.NoError(t, json.Unmarshal(resp.Payload, &result))
	assert.Equal(t, []string{"Sprint Board"}, result.Created)
	assert.Empty(t, result.Skipped)

	// The workflow row was created via the provider.
	require.Len(t, provider.workflows, 1)
	created := provider.workflows[0]
	assert.Equal(t, "Sprint Board", created.Name)
	assert.Equal(t, "ws-1", created.WorkspaceID)

	// Its steps were persisted to the repository.
	steps, err := repo.ListStepsByWorkflow(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	assert.Equal(t, "Todo", steps[0].Name)
	assert.True(t, steps[0].IsStartStep)
	assert.Equal(t, "Done", steps[1].Name)
}

func TestHandleImportWorkflow_SkipsDuplicateName(t *testing.T) {
	h, provider, _ := setupImportHandlers(t)
	provider.workflows = append(provider.workflows, &taskmodels.Workflow{
		ID: "wf-existing", WorkspaceID: "ws-1", Name: "Sprint Board",
	})

	doc := "version: 1\ntype: kandev_workflow\nworkflows:\n  - name: Sprint Board\n    steps: []\n"
	msg := makeWSMessage(t, ws.ActionMCPImportWorkflow, map[string]interface{}{
		"workspace_id": "ws-1",
		"document":     doc,
	})

	resp, err := h.handleImportWorkflow(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var result workflowsvc.ImportResult
	require.NoError(t, json.Unmarshal(resp.Payload, &result))
	assert.Empty(t, result.Created)
	assert.Equal(t, []string{"Sprint Board"}, result.Skipped)
}

func TestHandleImportWorkflow_MissingWorkspaceID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPImportWorkflow, map[string]interface{}{
		"document": "version: 1",
	})

	resp, err := h.handleImportWorkflow(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleImportWorkflow_MissingDocument(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPImportWorkflow, map[string]interface{}{
		"workspace_id": "ws-1",
	})

	resp, err := h.handleImportWorkflow(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleImportWorkflow_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPImportWorkflow,
		Payload: json.RawMessage(`not json`),
	}

	resp, err := h.handleImportWorkflow(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleImportWorkflow_DocumentTooLarge(t *testing.T) {
	h := &Handlers{}
	big := make([]byte, (1<<20)+1)
	for i := range big {
		big[i] = 'a'
	}
	msg := makeWSMessage(t, ws.ActionMCPImportWorkflow, map[string]interface{}{
		"workspace_id": "ws-1",
		"document":     string(big),
	})

	resp, err := h.handleImportWorkflow(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleImportWorkflow_InvalidDocument(t *testing.T) {
	h, _, _ := setupImportHandlers(t)
	msg := makeWSMessage(t, ws.ActionMCPImportWorkflow, map[string]interface{}{
		"workspace_id": "ws-1",
		"document":     "version: 1\ntype: kandev_workflow\nworkflows: [oops",
	})

	resp, err := h.handleImportWorkflow(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleImportWorkflow_ValidationError(t *testing.T) {
	h, _, _ := setupImportHandlers(t)
	// Wrong export type fails WorkflowExport.Validate — a client-side error
	// surfaced as a validation error so the agent can correct its document.
	msg := makeWSMessage(t, ws.ActionMCPImportWorkflow, map[string]interface{}{
		"workspace_id": "ws-1",
		"document":     "version: 1\ntype: not_kandev\nworkflows:\n  - name: X\n    steps: []\n",
	})

	resp, err := h.handleImportWorkflow(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}
