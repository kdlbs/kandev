package handlers

// TestHandleStepComplete_AdvancesFieldReflectsAutoAdvanceRequiresSignal is
// Slice B of the ISSUE-5 fix: accepted:true only means the signal was
// durably recorded, not that it will move the task — a step whose
// AutoAdvanceRequiresSignal is false never reads the bag at turn end, so an
// agent can get accepted:true and still see nothing advance. The response
// additively carries `advances` (and, when false, a `note` explaining why)
// so an agent doesn't have to reverse-engineer that gap from a stalled
// board. A lookup failure must omit `advances` entirely rather than guess
// (see TestHandleStepComplete_AdvancesFieldOmittedOnLookupFailure and the
// pre-existing TestHandleStepComplete_FirstCallAccepted, whose handler has
// no workflowCtrl wired at all).

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	workflowsvc "github.com/kandev/kandev/internal/workflow/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// newTestWorkflowController builds a real, in-memory-SQLite-backed
// workflow controller so handleStepComplete's h.workflowCtrl.GetStep call
// exercises the same authorization (AuthorizeStep, trivially satisfied by
// context.Background()'s unscoped caller) and lookup path production uses,
// mirroring setupParitySurfaces in workflow_step_parity_test.go.
func newTestWorkflowController(t *testing.T) (*workflowctrl.Controller, *workflowrepo.Repository) {
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

	repo, err := workflowrepo.NewWithDB(db, db, nil)
	require.NoError(t, err)
	svc := workflowsvc.NewService(repo, testLogger(t))
	t.Cleanup(func() { _ = svc.Close() })
	return workflowctrl.NewController(svc), repo
}

func TestHandleStepComplete_AdvancesFieldReflectsAutoAdvanceRequiresSignal(t *testing.T) {
	moveToNext := []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}}
	cases := []struct {
		name                      string
		autoAdvanceRequiresSignal bool
		onTurnComplete            []wfmodels.OnTurnCompleteAction
		wantAdvances              bool
		wantNote                  bool
	}{
		{name: "signal-gated step with a move action advances", autoAdvanceRequiresSignal: true, onTurnComplete: moveToNext, wantAdvances: true, wantNote: false},
		{name: "non-signal-gated step does not advance", autoAdvanceRequiresSignal: false, onTurnComplete: moveToNext, wantAdvances: false, wantNote: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			taskSvc, taskRepo := newTestTaskService(t)
			ctrl, wfRepo := newTestWorkflowController(t)
			ctx := context.Background()

			seedStepCompleteTarget(t, taskRepo, "task-advances", "session-advances", "step-advances", models.TaskSessionStateRunning)
			seedAgentProfileSnapshot(t, taskRepo, "session-advances", "claude-advances")
			require.NoError(t, wfRepo.CreateStep(ctx, &wfmodels.WorkflowStep{
				ID:                        "step-advances",
				WorkflowID:                "wf-advances",
				Name:                      "Step Advances",
				Position:                  0,
				AutoAdvanceRequiresSignal: tc.autoAdvanceRequiresSignal,
				Events:                    wfmodels.StepEvents{OnTurnComplete: tc.onTurnComplete},
			}))

			h := newStepCompleteHandler(t, taskSvc, taskRepo, &mcpRecordingEventBus{})
			h.workflowCtrl = ctrl

			msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
				"task_id":    "task-advances",
				"session_id": "session-advances",
				"summary":    "implementation finished",
			})
			resp, err := h.handleStepComplete(ctx, msg)
			require.NoError(t, err)
			require.NotNil(t, resp)

			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(resp.Payload, &payload))
			assert.Equal(t, true, payload["accepted"], "accepted must stay true regardless of advances")
			require.Contains(t, payload, "advances")
			assert.Equal(t, tc.wantAdvances, payload["advances"])
			_, hasNote := payload["note"]
			assert.Equal(t, tc.wantNote, hasNote)
		})
	}
}

// TestHandleStepComplete_SignalGatedStepWithoutMoveActionDoesNotAdvance
// covers a signal-gated step whose on_turn_complete has no move action the
// workflow engine would run. The signal is still accepted and recorded, but
// the response must say it will not move the task instead of reporting
// advances:true for a transition that does not exist.
func TestHandleStepComplete_SignalGatedStepWithoutMoveActionDoesNotAdvance(t *testing.T) {
	cases := []struct {
		name           string
		onTurnComplete []wfmodels.OnTurnCompleteAction
	}{
		{name: "no on_turn_complete actions"},
		{
			name:           "disable_plan_mode only",
			onTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteDisablePlanMode}},
		},
		{
			name:           "move_to_step without a target step",
			onTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToStep}},
		},
		{
			name: "move_to_step targeting current step",
			onTurnComplete: []wfmodels.OnTurnCompleteAction{{
				Type:   wfmodels.OnTurnCompleteMoveToStep,
				Config: map[string]interface{}{"step_id": "step-no-move"},
			}},
		},
		{
			name: "self-targeting move blocks later move",
			onTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step-no-move"}},
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step-later"}},
			},
		},
		{
			name: "move_to_next that requires approval",
			onTurnComplete: []wfmodels.OnTurnCompleteAction{{
				Type:   wfmodels.OnTurnCompleteMoveToNext,
				Config: map[string]interface{}{"requires_approval": true},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			taskSvc, taskRepo := newTestTaskService(t)
			ctrl, wfRepo := newTestWorkflowController(t)
			ctx := context.Background()

			seedStepCompleteTarget(t, taskRepo, "task-no-move", "session-no-move", "step-no-move", models.TaskSessionStateRunning)
			seedAgentProfileSnapshot(t, taskRepo, "session-no-move", "claude-no-move")
			require.NoError(t, wfRepo.CreateStep(ctx, &wfmodels.WorkflowStep{
				ID:                        "step-no-move",
				WorkflowID:                "wf-no-move",
				Name:                      "Waiting",
				Position:                  0,
				AutoAdvanceRequiresSignal: true,
				Events:                    wfmodels.StepEvents{OnTurnComplete: tc.onTurnComplete},
			}))

			recorder := &mcpRecordingEventBus{}
			h := newStepCompleteHandler(t, taskSvc, taskRepo, recorder)
			h.workflowCtrl = ctrl

			msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
				"task_id":    "task-no-move",
				"session_id": "session-no-move",
				"summary":    "implementation finished",
			})
			resp, err := h.handleStepComplete(ctx, msg)
			require.NoError(t, err)
			require.NotNil(t, resp)

			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(resp.Payload, &payload))
			assert.Equal(t, true, payload["accepted"], "accepted must stay true: the signal was recorded")
			assert.Equal(t, false, payload["advances"], "a step without a move action cannot advance on the signal")
			note, _ := payload["note"].(string)
			assert.Contains(t, note, "on_turn_complete", "the note must name the missing on_turn_complete move action")

			session, err := taskRepo.GetTaskSession(ctx, "session-no-move")
			require.NoError(t, err)
			signal, has := models.LoadPendingStepSignal(session.Metadata)
			require.True(t, has, "an accepted signal is durably recorded")
			assert.Equal(t, "step-no-move", signal.StepID)
			assert.Len(t, recorder.events, 1, "an accepted signal is published once")
		})
	}
}

// TestHandleStepComplete_AdvancesFieldOmittedOnLookupFailure pins the
// never-guess rule: when the step lookup itself fails (here, the step row
// simply does not exist in the workflow store), the response must not
// carry an `advances` key at all, rather than defaulting to true or false.
func TestHandleStepComplete_AdvancesFieldOmittedOnLookupFailure(t *testing.T) {
	taskSvc, taskRepo := newTestTaskService(t)
	ctrl, _ := newTestWorkflowController(t)
	ctx := context.Background()

	seedStepCompleteTarget(t, taskRepo, "task-no-step", "session-no-step", "step-missing", models.TaskSessionStateRunning)
	seedAgentProfileSnapshot(t, taskRepo, "session-no-step", "claude-no-step")
	// Deliberately no wfRepo.CreateStep call: "step-missing" does not exist
	// in the workflow store the controller reads from.

	h := newStepCompleteHandler(t, taskSvc, taskRepo, &mcpRecordingEventBus{})
	h.workflowCtrl = ctrl

	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-no-step",
		"session_id": "session-no-step",
		"summary":    "implementation finished",
	})
	resp, err := h.handleStepComplete(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, true, payload["accepted"])
	assert.NotContains(t, payload, "advances", "a failed step lookup must never guess an advances value")
	assert.NotContains(t, payload, "note")
}
