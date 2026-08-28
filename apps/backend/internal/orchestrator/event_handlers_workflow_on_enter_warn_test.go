package orchestrator

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestProcessOnEnter_UnrecognizedActionType_WarnsAndContinuesDispatch covers
// AC-A6 — the bug report's own headline acceptance bullet: "An on_enter
// action the handler does not recognise emits a WARNING naming the step and
// action type. The silent discard is what made this take a full afternoon
// to find." Build round 1 (commit d4c3dd42a) added the default-branch
// warning in dispatchOnEnterActions but shipped with zero test coverage for
// it; Testing round 1 verified the behavior with an uncommitted throwaway
// probe and routed the card back for exactly this permanent test.
func TestProcessOnEnter_UnrecognizedActionType_WarnsAndContinuesDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-a6", "session-a6", "step-a6")

	session, err := repo.GetTaskSession(ctx, "session-a6")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)

	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build observed logger: %v", err)
	}
	svc.logger = log

	// A genuinely unrecognized action type followed by a real one: the
	// warning must not abort the loop, so set_session_mode must still run
	// and persist.
	step := &wfmodels.WorkflowStep{
		ID:         "step-a6",
		WorkflowID: "wf-a6",
		Name:       "AC-A6 Step",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterActionType("totally_bogus_action")},
			{Type: wfmodels.OnEnterSetSessionMode, Config: map[string]interface{}{"mode": "acceptEdits"}},
		}},
	}

	svc.processOnEnter(ctx, "task-a6", session, step, "", 0)

	var warnings []observer.LoggedEntry
	for _, e := range logs.All() {
		if e.Message == "processOnEnter: unrecognized on_enter action type" {
			warnings = append(warnings, e)
		}
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d WARNING(s) for the unrecognized action, want exactly 1 (all entries: %+v)", len(warnings), logs.All())
	}
	fields := warnings[0].ContextMap()
	if fields["step_id"] != "step-a6" {
		t.Errorf("warning step_id = %v, want %q", fields["step_id"], "step-a6")
	}
	if fields["action_type"] != "totally_bogus_action" {
		t.Errorf("warning action_type = %v, want %q", fields["action_type"], "totally_bogus_action")
	}

	reloaded, err := repo.GetTaskSession(ctx, "session-a6")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if got := reloaded.Metadata[models.SessionMetaKeySessionMode]; got != "acceptEdits" {
		t.Errorf("set_session_mode after the unrecognized action = %v, want %q — the warning must not abort dispatch of subsequent actions", got, "acceptEdits")
	}
}
