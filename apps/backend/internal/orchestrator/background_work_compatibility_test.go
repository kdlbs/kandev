package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// legacyDetachedShellWire is the previous agentctl wire shape: a recognized
// detached launch serialized without the later negative verification marker.
const legacyDetachedShellWire = `{
	"kind": "shell_exec",
	"shell_exec": {"command": "sleep 30", "background": true},
	"background_work": {"kind": "shell", "work_id": "shell-legacy", "detached": true}
}`

// TestLegacyBackgroundWorkWireStillAttestsParked pins AC-001.3: an older
// agentctl or retained journal frame omits the new marker, and that omission
// must keep the existing parked-projection behavior.
func TestLegacyBackgroundWorkWireStillAttestsParked(t *testing.T) {
	const sessionID = "sess-legacy-wire"
	const taskID = "task-legacy-wire"
	repo := newParkedTestRepo(&models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}

	var payload streams.NormalizedPayload
	require.NoError(t, json.Unmarshal([]byte(legacyDetachedShellWire), &payload))
	require.True(t, payload.IsVerifiedBackgroundWork())

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: "exec-legacy",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "tool-legacy",
			ToolStatus: "completed",
			Normalized: &payload,
		},
	})
	require.True(t, svc.ObservedDetachedLaunch(sessionID),
		"a legacy frame without the marker must still attest the detached launch")

	svc.settleParkedProjectionSync(context.Background(), taskID, sessionID)
	parked, revision := svc.ParkedSnapshot(sessionID)
	require.True(t, parked, "a live probe plus legacy attestation must park")
	require.Equal(t, uint64(1), revision)
}

// TestExplicitlyUnverifiedWireCannotAttestParked pins the other direction:
// OpenCode's explicit negative marker must not set the parked attestation even
// when the payload is otherwise a recognized detached shell.
func TestExplicitlyUnverifiedWireCannotAttestParked(t *testing.T) {
	const sessionID = "sess-unverified-wire"
	const taskID = "task-unverified-wire"
	repo := newParkedTestRepo(&models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	})
	svc, _, probe := newParkedTestService(t, repo)
	probe.results = []executor.ProbeResult{executor.ProbeResultLive}

	var payload streams.NormalizedPayload
	require.NoError(t, json.Unmarshal([]byte(`{
		"kind": "shell_exec",
		"shell_exec": {"command": "sleep 30", "background": true},
		"background_work": {
			"kind": "shell", "work_id": "shell-oc", "detached": true,
			"run_id": "shell-oc", "unverified": true,
			"capabilities": {"discovery": "events_only", "output": "none"}
		}
	}`), &payload))
	require.False(t, payload.IsVerifiedBackgroundWork())

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: "exec-unverified",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "tool-oc",
			ToolStatus: "completed",
			Normalized: &payload,
		},
	})
	require.False(t, svc.ObservedDetachedLaunch(sessionID))
}

// TestBuildObservationFromNormalizedUsesProviderIdentityOnly pins that the
// normalized projection never invents a provider run key and carries the
// adapter's capability declaration.
func TestBuildObservationFromNormalizedUsesProviderIdentityOnly(t *testing.T) {
	caps := streams.WorkloadCapabilities{
		Discovery: "events_only",
		Output:    "none",
	}
	payload := streams.NewShellExec("sleep 30", "", "", 0, true)
	payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, "item-1", true, false)
	payload.SetBackgroundWorkCapabilities(caps)

	obs := buildObservationFromNormalized(&lifecycle.AgentStreamEventPayload{
		SessionID: "sess-build",
		Data: &lifecycle.AgentStreamEventData{
			ToolCallID: "tool-1",
			ToolTitle:  "sleep 30",
			Normalized: payload,
		},
	}, "turn-1")

	require.NotNil(t, obs)
	require.Equal(t, "item-1", obs.WorkID)
	require.Empty(t, obs.RunID, "run identity must come from the provider, never from WorkID")
	require.Equal(t, "none", obs.Capabilities.Output)
	require.Equal(t, "turn-1", obs.OriginTurnID)
}
