package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// --- Test table types ---

// testEvent describes one trigger to fire and what to expect afterward.
type testEvent struct {
	Trigger            engine.Trigger
	SetRunning         bool                    // set session to RUNNING before firing
	ExpectStep         string                  // expected step name after event
	ExpectTransitioned bool                    // whether a transition should occur
	ExpectQueued       bool                    // auto-start message queued
	ExpectResets       int                     // cumulative RestartAgentProcess calls
	ExpectState        models.TaskSessionState // expected session state (zero value = skip check)
}

// workflowTestCase is one test table entry.
type workflowTestCase struct {
	Name          string
	WorkflowJSON  string // raw JSON in WorkflowExport format
	StartStep     string // step name to start at
	IsPassthrough bool
	ResetErr      error
	Events        []testEvent
}

// --- Workflow JSON ---

const developmentWorkflowJSON = `{
  "version": 1,
  "type": "kandev_workflow",
  "workflows": [
    {
      "name": "Development",
      "steps": [
        {
          "name": "Backlog",
          "position": 0,
          "color": "bg-neutral-400",
          "events": {
            "on_turn_start": [{"type": "move_to_next"}],
            "on_turn_complete": [{"type": "move_to_step", "config": {"step_position": 1}}]
          },
          "is_start_step": true,
          "allow_manual_move": true
        },
        {
          "name": "In Progress",
          "position": 1,
          "color": "bg-blue-500",
          "events": {
            "on_enter": [{"type": "auto_start_agent"}],
            "on_turn_complete": [{"type": "move_to_step", "config": {"step_position": 2}}]
          },
          "allow_manual_move": true
        },
        {
          "name": "New Context",
          "position": 2,
          "color": "bg-yellow-500",
          "events": {
            "on_enter": [{"type": "auto_start_agent"}, {"type": "reset_agent_context"}],
            "on_turn_start": [{"type": "move_to_previous"}],
            "on_turn_complete": [{"type": "move_to_next"}]
          },
          "allow_manual_move": true
        },
        {
          "name": "New Step",
          "position": 3,
          "color": "bg-slate-500",
          "events": {
            "on_enter": [{"type": "reset_agent_context"}],
            "on_turn_complete": [{"type": "move_to_next"}]
          },
          "allow_manual_move": true
        },
        {
          "name": "Done",
          "position": 4,
          "color": "bg-green-500",
          "events": {
            "on_turn_start": [{"type": "move_to_step", "config": {"step_position": 1}}]
          },
          "allow_manual_move": true
        }
      ]
    }
  ]
}`

// --- Test cases ---

var workflowTestCases = []workflowTestCase{
	{
		Name:         "five_step_full_lifecycle",
		WorkflowJSON: developmentWorkflowJSON,
		StartStep:    "Backlog",
		Events: []testEvent{
			// Agent finishes at Backlog → In Progress (auto_start queues)
			{Trigger: engine.TriggerOnTurnComplete, ExpectStep: "In Progress",
				ExpectTransitioned: true, ExpectQueued: true, ExpectResets: 0},
			// Agent finishes at In Progress → New Context (reset + auto_start)
			{Trigger: engine.TriggerOnTurnComplete, SetRunning: true, ExpectStep: "New Context",
				ExpectTransitioned: true, ExpectQueued: true, ExpectResets: 1},
			// Agent starts at New Context → on_turn_start → back to In Progress
			{Trigger: engine.TriggerOnTurnStart, SetRunning: true, ExpectStep: "In Progress",
				ExpectTransitioned: true, ExpectQueued: false, ExpectResets: 1},
			// Agent finishes at In Progress → New Context again
			{Trigger: engine.TriggerOnTurnComplete, SetRunning: true, ExpectStep: "New Context",
				ExpectTransitioned: true, ExpectQueued: true, ExpectResets: 2},
			// Agent finishes at New Context → New Step (reset, no auto_start)
			{Trigger: engine.TriggerOnTurnComplete, SetRunning: true, ExpectStep: "New Step",
				ExpectTransitioned: true, ExpectQueued: false, ExpectResets: 3},
			// Agent finishes at New Step → Done
			{Trigger: engine.TriggerOnTurnComplete, SetRunning: true, ExpectStep: "Done",
				ExpectTransitioned: true, ExpectQueued: false, ExpectResets: 3},
			// User sends message at Done → on_turn_start → In Progress
			{Trigger: engine.TriggerOnTurnStart, SetRunning: true, ExpectStep: "In Progress",
				ExpectTransitioned: true, ExpectQueued: false, ExpectResets: 3},
		},
	},
	{
		Name:         "no_transition_at_terminal_step",
		WorkflowJSON: developmentWorkflowJSON,
		StartStep:    "Done",
		Events: []testEvent{
			{Trigger: engine.TriggerOnTurnComplete, ExpectStep: "Done",
				ExpectTransitioned: false, ExpectQueued: false, ExpectResets: 0,
				ExpectState: models.TaskSessionStateWaitingForInput},
		},
	},
	{
		Name:         "reset_failure_blocks_auto_start",
		WorkflowJSON: developmentWorkflowJSON,
		StartStep:    "In Progress",
		ResetErr:     errors.New("restart failed"),
		Events: []testEvent{
			// Transition happens, reset is attempted (call count=1) but fails → auto_start not reached
			{Trigger: engine.TriggerOnTurnComplete, ExpectStep: "New Context",
				ExpectTransitioned: true, ExpectQueued: false, ExpectResets: 1,
				ExpectState: models.TaskSessionStateWaitingForInput},
		},
	},
	{
		Name:          "passthrough_skips_auto_start",
		WorkflowJSON:  developmentWorkflowJSON,
		StartStep:     "In Progress",
		IsPassthrough: true,
		Events: []testEvent{
			// Passthrough: reset fires but auto_start is skipped
			{Trigger: engine.TriggerOnTurnComplete, ExpectStep: "New Context",
				ExpectTransitioned: true, ExpectQueued: false, ExpectResets: 1,
				ExpectState: models.TaskSessionStateWaitingForInput},
		},
	},
}

// --- Test runner ---

func TestWorkflowE2E(t *testing.T) {
	for _, tc := range workflowTestCases {
		t.Run(tc.Name, func(t *testing.T) {
			runWorkflowTestCase(t, tc)
		})
	}
}

func runWorkflowTestCase(t *testing.T, tc workflowTestCase) {
	t.Helper()
	ctx := context.Background()

	sg, nameToID := buildWorkflowFromJSON(t, tc.WorkflowJSON)
	repo := setupTestRepo(t)
	startStepID := nameToID[tc.StartStep]
	seedSession(t, repo, "t1", "s1", startStepID)
	setSessionExecID(t, repo, "s1", "exec-1")

	agentMgr := &mockAgentManager{
		isPassthrough:     tc.IsPassthrough,
		restartProcessErr: tc.ResetErr,
	}
	svc := createEngineService(t, repo, sg, agentMgr)

	for i, ev := range tc.Events {
		label := fmt.Sprintf("event[%d]_%s", i, ev.Trigger)
		t.Run(label, func(t *testing.T) {
			runSingleEvent(t, ctx, repo, svc, agentMgr, ev, nameToID)
		})
	}
}

func runSingleEvent(
	t *testing.T,
	ctx context.Context,
	repo sessionExecutorStore,
	svc *Service,
	agentMgr *mockAgentManager,
	ev testEvent,
	nameToID map[string]string,
) {
	t.Helper()

	if ev.SetRunning {
		setSessionState(t, ctx, repo, "s1", models.TaskSessionStateRunning)
	}
	// Drain queue from previous event if needed
	svc.messageQueue.TakeQueued(ctx, "s1")

	var transitioned bool
	switch ev.Trigger {
	case engine.TriggerOnTurnComplete:
		transitioned = svc.processOnTurnCompleteViaEngine(ctx, "t1", "s1")
	case engine.TriggerOnTurnStart:
		transitioned = svc.processOnTurnStartViaEngine(ctx, "t1", "s1")
	default:
		t.Fatalf("unsupported trigger: %s", ev.Trigger)
	}

	if transitioned != ev.ExpectTransitioned {
		t.Errorf("transitioned = %v, want %v", transitioned, ev.ExpectTransitioned)
	}

	assertStepByName(t, ctx, repo, "s1", ev.ExpectStep, nameToID)
	assertResetCalls(t, agentMgr, ev.ExpectResets)
	assertQueueState(t, ctx, svc, "s1", ev.ExpectQueued)

	if ev.ExpectState != "" {
		assertSessionState(t, ctx, repo, "s1", ev.ExpectState)
	}
}

// --- Helpers ---

// buildWorkflowFromJSON parses WorkflowExport JSON and populates a mockStepGetter.
// Returns the step getter and a name→stepID map.
func buildWorkflowFromJSON(t *testing.T, jsonStr string) (*mockStepGetter, map[string]string) {
	t.Helper()
	var export wfmodels.WorkflowExport
	if err := json.Unmarshal([]byte(jsonStr), &export); err != nil {
		t.Fatalf("failed to unmarshal workflow JSON: %v", err)
	}
	if len(export.Workflows) == 0 {
		t.Fatal("workflow JSON contains no workflows")
	}

	steps := export.Workflows[0].Steps
	posToID := make(map[int]string, len(steps))
	for _, sp := range steps {
		posToID[sp.Position] = fmt.Sprintf("step-%d", sp.Position)
	}

	sg := newMockStepGetter()
	nameToID := make(map[string]string, len(steps))
	for _, sp := range steps {
		id := posToID[sp.Position]
		events := wfmodels.ConvertPositionToStepID(sp.Events, posToID)
		sg.steps[id] = &wfmodels.WorkflowStep{
			ID: id, WorkflowID: "wf1", Name: sp.Name, Position: sp.Position,
			Prompt: sp.Prompt, Events: events,
		}
		nameToID[sp.Name] = id
	}
	return sg, nameToID
}

// createEngineService creates a Service with the workflow engine initialized.
func createEngineService(t *testing.T, repo sessionExecutorStore, sg *mockStepGetter, agentMgr *mockAgentManager) *Service {
	t.Helper()
	log := testLogger()
	svc := &Service{
		logger:       log,
		repo:         repo,
		taskRepo:     newMockTaskRepo(),
		agentManager: agentMgr,
		messageQueue: messagequeue.NewService(log),
	}
	svc.SetWorkflowStepGetter(sg)
	return svc
}

// setSessionExecID sets the AgentExecutionID on a session.
func setSessionExecID(t *testing.T, repo sessionExecutorStore, sessionID, execID string) {
	t.Helper()
	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session %s: %v", sessionID, err)
	}
	session.AgentExecutionID = execID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to set exec ID on session %s: %v", sessionID, err)
	}
}

// setSessionState updates the session state in the database.
func setSessionState(t *testing.T, ctx context.Context, repo sessionExecutorStore, sessionID string, state models.TaskSessionState) {
	t.Helper()
	if err := repo.UpdateTaskSessionState(ctx, sessionID, state, ""); err != nil {
		t.Fatalf("failed to set session state to %s: %v", state, err)
	}
}

// assertStepByName verifies the session's current workflow step matches the expected step name.
func assertStepByName(t *testing.T, ctx context.Context, repo sessionExecutorStore, sessionID, expectName string, nameToID map[string]string) {
	t.Helper()
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	expectID := nameToID[expectName]
	gotID := ""
	if session.WorkflowStepID != nil {
		gotID = *session.WorkflowStepID
	}
	if gotID != expectID {
		t.Errorf("session step = %q (want %q for %q)", gotID, expectID, expectName)
	}
}

// assertResetCalls verifies the cumulative count of RestartAgentProcess calls.
func assertResetCalls(t *testing.T, agentMgr *mockAgentManager, expectCount int) {
	t.Helper()
	if got := len(agentMgr.restartProcessCalls); got != expectCount {
		t.Errorf("restart calls = %d, want %d", got, expectCount)
	}
}

// assertQueueState verifies whether a message is queued for the session.
func assertQueueState(t *testing.T, ctx context.Context, svc *Service, sessionID string, expectQueued bool) {
	t.Helper()
	status := svc.messageQueue.GetStatus(ctx, sessionID)
	if status.IsQueued != expectQueued {
		t.Errorf("queue.IsQueued = %v, want %v", status.IsQueued, expectQueued)
	}
}

// assertSessionState verifies the session's current state.
func assertSessionState(t *testing.T, ctx context.Context, repo sessionExecutorStore, sessionID string, expect models.TaskSessionState) {
	t.Helper()
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if session.State != expect {
		t.Errorf("session state = %q, want %q", session.State, expect)
	}
}
