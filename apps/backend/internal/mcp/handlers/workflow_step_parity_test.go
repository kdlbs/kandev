package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	eventtypes "github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	workflowhandlers "github.com/kandev/kandev/internal/workflow/handlers"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
	workflowsvc "github.com/kandev/kandev/internal/workflow/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// The MCP surface (this package) and the REST surface
// (internal/workflow/handlers) both mutate workflow steps and both publish
// workflow_step.* events that the WS gateway and the orchestrator queue
// reconciler consume. The tests below pin that the two emit equivalent
// payloads for the same mutation, so the surfaces cannot drift apart again.
// They live here because this is the only package that can construct both.

type paritySurfaceEvent struct {
	subject string
	source  string
	step    map[string]interface{}
}

func recordParityStepEvents(t *testing.T, eventBus *eventbus.MemoryEventBus) *[]paritySurfaceEvent {
	t.Helper()
	var recorded []paritySurfaceEvent
	for _, subject := range []string{
		eventtypes.WorkflowStepCreated,
		eventtypes.WorkflowStepUpdated,
		eventtypes.WorkflowStepDeleted,
	} {
		_, err := eventBus.Subscribe(subject, func(_ context.Context, ev *eventbus.Event) error {
			data, ok := ev.Data.(map[string]interface{})
			require.True(t, ok)
			step, ok := data["step"].(map[string]interface{})
			require.True(t, ok)
			recorded = append(recorded, paritySurfaceEvent{subject: ev.Subject, source: ev.Source, step: step})
			return nil
		})
		require.NoError(t, err)
	}
	return &recorded
}

// setupParitySurfaces wires the MCP handlers and the REST router over one
// workflow store and one event bus.
func setupParitySurfaces(t *testing.T) (*Handlers, *gin.Engine, *eventbus.MemoryEventBus) {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
		description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
	)`)
	require.NoError(t, err)

	repo, err := repository.NewWithDB(db, db, nil)
	require.NoError(t, err)

	log := testLogger(t)
	svc := workflowsvc.NewService(repo, log)
	t.Cleanup(func() { _ = svc.Close() })
	ctrl := workflowctrl.NewController(svc)
	eventBus := eventbus.NewMemoryEventBus(log)

	mcp := &Handlers{workflowSvc: svc, workflowCtrl: ctrl, eventBus: eventBus, logger: log.WithFields()}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	workflowhandlers.RegisterRoutes(router, ws.NewDispatcher(), ctrl, eventBus, log)

	return mcp, router, eventBus
}

func parityHTTPJSON(t *testing.T, router *gin.Engine, method, path string, body interface{}, wantStatus int) []byte {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, wantStatus, rec.Code, "body = %s", rec.Body.String())
	return rec.Body.Bytes()
}

// assertStepPayloadsEquivalent compares two step payloads field by field,
// ignoring keys whose values legitimately differ between two separate
// mutations (identity and timestamps).
func assertStepPayloadsEquivalent(t *testing.T, mcpStep, httpStep map[string]interface{}, ignore ...string) {
	t.Helper()
	skip := map[string]bool{}
	for _, key := range ignore {
		skip[key] = true
	}
	mcpKeys := make([]string, 0, len(mcpStep))
	for key := range mcpStep {
		mcpKeys = append(mcpKeys, key)
	}
	for _, key := range mcpKeys {
		_, ok := httpStep[key]
		require.True(t, ok, "REST payload is missing %q that the MCP payload carries", key)
	}
	require.Len(t, httpStep, len(mcpStep), "payload key counts differ: mcp=%v http=%v", mcpStep, httpStep)
	for key, mcpValue := range mcpStep {
		if skip[key] {
			continue
		}
		require.True(t, reflect.DeepEqual(mcpValue, httpStep[key]),
			"field %q differs: mcp=%#v http=%#v", key, mcpValue, httpStep[key])
	}
}

func TestWorkflowStepCreateEventParityAcrossSurfaces(t *testing.T) {
	mcp, router, eventBus := setupParitySurfaces(t)
	ctx := context.Background()
	recorded := recordParityStepEvents(t, eventBus)

	stepFields := map[string]interface{}{
		"workflow_id":                   "wf-parity",
		"name":                          "Review",
		"color":                         "#abcdef",
		"prompt":                        "review the diff",
		"agent_profile_id":              "profile-parity",
		"profile_session_start_policy":  "reuse",
		"profile_session_end_policy":    "park",
		"allow_manual_move":             true,
		"show_in_command_panel":         true,
		"auto_advance_requires_signal":  true,
		"cancel_triggers_turn_complete": true,
		"wip_limit":                     4,
	}

	mcpPayload := map[string]interface{}{"position": 0}
	httpPayload := map[string]interface{}{"position": 1}
	for key, value := range stepFields {
		mcpPayload[key] = value
		httpPayload[key] = value
	}

	resp, err := mcp.handleCreateWorkflowStep(ctx, makeWSMessage(t, ws.ActionMCPCreateWorkflowStep, mcpPayload))
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, resp.Type, "mcp create failed: %s", string(resp.Payload))

	parityHTTPJSON(t, router, http.MethodPost, "/api/v1/workflow/steps", httpPayload, http.StatusCreated)

	require.Len(t, *recorded, 2, "each surface should publish exactly one created event")
	mcpEvent, httpEvent := (*recorded)[0], (*recorded)[1]
	require.Equal(t, eventtypes.WorkflowStepCreated, mcpEvent.subject)
	require.Equal(t, eventtypes.WorkflowStepCreated, httpEvent.subject)
	require.Equal(t, "mcp-handlers", mcpEvent.source)
	require.Equal(t, "workflow-handlers", httpEvent.source)
	assertStepPayloadsEquivalent(t, mcpEvent.step, httpEvent.step, "id", "position", "created_at", "updated_at")
}

func TestWorkflowStepUpdateEventParityAcrossSurfaces(t *testing.T) {
	mcp, router, eventBus := setupParitySurfaces(t)
	ctx := context.Background()

	created := parityHTTPJSON(t, router, http.MethodPost, "/api/v1/workflow/steps", map[string]interface{}{
		"workflow_id": "wf-parity",
		"name":        "Backlog",
		"position":    0,
	}, http.StatusCreated)
	var step wfmodels.WorkflowStep
	require.NoError(t, json.Unmarshal(created, &step))

	recorded := recordParityStepEvents(t, eventBus)

	update := map[string]interface{}{"name": "Triage", "wip_limit": 7, "prompt": "triage it"}
	mcpUpdate := map[string]interface{}{"step_id": step.ID}
	for key, value := range update {
		mcpUpdate[key] = value
	}

	resp, err := mcp.handleUpdateWorkflowStep(ctx, makeWSMessage(t, ws.ActionMCPUpdateWorkflowStep, mcpUpdate))
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, resp.Type, "mcp update failed: %s", string(resp.Payload))

	parityHTTPJSON(t, router, http.MethodPut, "/api/v1/workflow/steps/"+step.ID, update, http.StatusOK)

	require.Len(t, *recorded, 2, "each surface should publish exactly one updated event")
	mcpEvent, httpEvent := (*recorded)[0], (*recorded)[1]
	require.Equal(t, eventtypes.WorkflowStepUpdated, mcpEvent.subject)
	require.Equal(t, eventtypes.WorkflowStepUpdated, httpEvent.subject)
	require.Equal(t, "mcp-handlers", mcpEvent.source)
	require.Equal(t, "workflow-handlers", httpEvent.source)
	// Same step, same edit through both surfaces: only the write timestamp may differ.
	assertStepPayloadsEquivalent(t, mcpEvent.step, httpEvent.step, "updated_at")
}

func TestWorkflowStepDeleteEventParityAcrossSurfaces(t *testing.T) {
	mcp, router, eventBus := setupParitySurfaces(t)
	ctx := context.Background()

	newStep := func(name string, position int) wfmodels.WorkflowStep {
		body := parityHTTPJSON(t, router, http.MethodPost, "/api/v1/workflow/steps", map[string]interface{}{
			"workflow_id": "wf-parity",
			"name":        name,
			"position":    position,
			"color":       "#abcdef",
		}, http.StatusCreated)
		var step wfmodels.WorkflowStep
		require.NoError(t, json.Unmarshal(body, &step))
		return step
	}
	mcpStep := newStep("Doomed A", 0)
	httpStep := newStep("Doomed A", 1)

	recorded := recordParityStepEvents(t, eventBus)

	resp, err := mcp.handleDeleteWorkflowStep(ctx, makeWSMessage(t, ws.ActionMCPDeleteWorkflowStep, map[string]interface{}{
		"step_id": mcpStep.ID,
	}))
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, resp.Type, "mcp delete failed: %s", string(resp.Payload))

	parityHTTPJSON(t, router, http.MethodDelete, "/api/v1/workflow/steps/"+httpStep.ID, nil, http.StatusOK)

	require.Len(t, *recorded, 2, "each surface should publish exactly one deleted event")
	mcpEvent, httpEvent := (*recorded)[0], (*recorded)[1]
	require.Equal(t, eventtypes.WorkflowStepDeleted, mcpEvent.subject)
	require.Equal(t, eventtypes.WorkflowStepDeleted, httpEvent.subject)
	assertStepPayloadsEquivalent(t, mcpEvent.step, httpEvent.step, "id", "position", "created_at", "updated_at")
}
