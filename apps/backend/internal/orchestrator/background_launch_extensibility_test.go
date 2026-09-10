package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp/backgroundlaunch"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeVendorLaunchRecognizer struct{ agentID string }

func (f fakeVendorLaunchRecognizer) AgentID() string { return f.agentID }

func (f fakeVendorLaunchRecognizer) RecognizesDetachedLaunch(payload *streams.NormalizedPayload) bool {
	return payload != nil && payload.ShellExec() != nil && payload.ShellExec().Background
}

// AC-69(a): a second agent's recogniser, registered through the public
// backgroundlaunch.Register API with zero change to orchestrator production
// code, drives a detached-shell launch all the way through the parked
// projection — attestation, the settle-time probe, and both the session- and
// task-level snapshots. acp's own background_launch_extensibility_test.go
// proves the registry accepts a second recogniser and stamps its output;
// this proves that stamp is sufficient for the rest of the pipeline to park.
func TestSecondRegisteredRecognizer_DetachedLaunchParksThroughSettle(t *testing.T) {
	const fakeAgentID = "orchestrator-extensibility-test-agent"
	backgroundlaunch.Register(fakeVendorLaunchRecognizer{agentID: fakeAgentID})
	t.Cleanup(func() { backgroundlaunch.Unregister(fakeAgentID) })

	const sessionID = "sess-second-agent"
	const taskID = "task-second-agent"
	repo := newParkedTestRepo(&models.TaskSession{ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}

	// Mirrors stampBackgroundShellWork (acp/normalize.go) exactly: the
	// orchestrator never talks to the registry itself, only to whatever the
	// adapter layer already stamped onto the normalized payload.
	payload := streams.NewShellExec("sleep 300", "", "", 0, true)
	if backgroundlaunch.RecognizesDetachedLaunch(fakeAgentID, payload) {
		payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, "", true, false)
	}

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "tool-1",
			ToolStatus: "completed",
			Normalized: payload,
		},
	})
	if !svc.ObservedDetachedLaunch(sessionID) {
		t.Fatalf("expected the second agent's detached shell launch to set the attestation")
	}

	svc.settleParkedProjectionSync(context.Background(), taskID, sessionID)

	if parked, revision := svc.ParkedSnapshot(sessionID); !parked || revision != 1 {
		t.Fatalf("ParkedSnapshot = (%v, %d), want (true, 1)", parked, revision)
	}
	if parked, _ := svc.TaskParkedSnapshot(taskID); !parked {
		t.Fatalf("TaskParkedSnapshot(%q).parked = false, want true", taskID)
	}
}
