package acp

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

// openCodeBackgroundAdapter builds an adapter for the real OpenCode agent
// identity with the shared background-work feature at enabled.
func openCodeBackgroundAdapter(t *testing.T, enabled bool) *Adapter {
	t.Helper()
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "json",
		OutputPath: "stderr",
	})
	return NewAdapter(&shared.Config{
		AgentID:                    openCodeAgentID,
		WorkDir:                    "/tmp/test",
		AgentBackgroundWorkEnabled: enabled,
	}, log)
}

// openCodeLaunchFrames returns the captured OpenCode shape: an execute
// tool_call whose input carries background=true, followed by a tool_call_update
// whose result carries metadata.shellID plus metadata.status=running.
func openCodeLaunchFrames(toolCallID, shellID, command string) (*acp.SessionUpdateToolCall, *acp.SessionToolCallUpdate) {
	call := &acp.SessionUpdateToolCall{
		ToolCallId:    acp.ToolCallId(toolCallID),
		Kind:          acp.ToolKindExecute,
		Status:        acp.ToolCallStatusInProgress,
		Title:         command,
		SessionUpdate: "tool_call",
		RawInput: map[string]any{
			"command":    command,
			"background": true,
		},
	}
	completed := acp.ToolCallStatusCompleted
	update := &acp.SessionToolCallUpdate{
		ToolCallId:    acp.ToolCallId(toolCallID),
		Status:        &completed,
		SessionUpdate: "tool_call_update",
		RawOutput: map[string]any{
			"title":  command,
			"output": "Background process started",
			"metadata": map[string]any{
				"shellID": shellID,
				"status":  "running",
			},
		},
	}
	return call, update
}

// TestOpenCodeBackgroundLaunchIdentity pins AC-007.1: three confirmed launches
// produce three stable shared workloads with their own native shell identity,
// command, and launch association. The launch tool completing does not end any
// of them (AC-007.2).
func TestOpenCodeBackgroundLaunchIdentity(t *testing.T) {
	a := openCodeBackgroundAdapter(t, true)

	type launch struct{ toolCallID, shellID, command string }
	launches := []launch{
		{"call-ci", "shell-ci", "gh pr checks 4821 --watch"},
		{"call-await", "shell-await", "./scripts/pr-await 4821"},
		{"call-tests", "shell-tests", "go test ./..."},
	}

	for _, l := range launches {
		call, update := openCodeLaunchFrames(l.toolCallID, l.shellID, l.command)
		require.NotNil(t, a.convertToolCallUpdate("ses-oc", call))
		event := a.convertToolCallResultUpdate("ses-oc", update)
		require.NotNil(t, event)
		require.Equal(t, streams.EventTypeToolUpdate, event.Type)
		require.Equal(t, toolStatusComplete, event.ToolStatus,
			"the launch tool itself is complete once the agent returns its result")

		shell := event.NormalizedPayload.ShellExec()
		require.NotNil(t, shell)
		require.Equal(t, l.command, shell.Command)
		require.True(t, shell.Background, "OpenCode reports the launch as background presentation")

		work := event.NormalizedPayload.BackgroundWork()
		require.NotNil(t, work, "a confirmed launch must carry shared background-work identity")
		require.Equal(t, streams.BackgroundWorkKindShell, work.Kind)
		require.Equal(t, l.shellID, work.WorkID)
		require.True(t, work.Detached)
		require.False(t, work.Ended, "a running shell is not a completed workload")
		require.Equal(t, l.shellID, work.RunID,
			"the native shell identity is the provider run key for OpenCode")
		require.NotNil(t, work.Capabilities,
			"the frame must declare its observation-only capability")
		require.Equal(t, "none", work.Capabilities.Output)
		require.Empty(t, work.Capabilities.Actions)

		a.mu.RLock()
		retained, tracked := a.activeToolCalls[l.toolCallID]
		a.mu.RUnlock()
		require.True(t, tracked, "the launch binding must outlive the terminal launch tool")
		require.Equal(t, l.shellID, retained.BackgroundWork().WorkID)
	}

	// A later job outcome for one shell addresses that binding by native
	// identity rather than reviving the launch call as foreground work.
	completed := acp.ToolCallStatusCompleted
	outcome := a.convertToolCallResultUpdate("ses-oc", &acp.SessionToolCallUpdate{
		ToolCallId:    "call-await",
		Status:        &completed,
		SessionUpdate: "tool_call_update",
		RawOutput:     map[string]any{"output": "1 check passed", "exit_code": 0},
	})
	require.NotNil(t, outcome)
	require.Equal(t, "shell-await", outcome.NormalizedPayload.BackgroundWork().WorkID)
	require.True(t, outcome.NormalizedPayload.BackgroundWork().Ended,
		"an observed exit ends the workload, not the launch tool")
	for _, l := range []string{"call-ci", "call-tests"} {
		a.mu.RLock()
		_, tracked := a.activeToolCalls[l]
		a.mu.RUnlock()
		require.True(t, tracked, "an unrelated job outcome must not disturb sibling jobs")
	}
}

// TestOpenCodeLaunchCompleteIsNotJobComplete pins AC-007.2 at the observation
// boundary: the launch tool's terminal status is not terminal evidence for the
// launched shell.
func TestOpenCodeLaunchCompleteIsNotJobComplete(t *testing.T) {
	a := openCodeBackgroundAdapter(t, true)
	call, update := openCodeLaunchFrames("call-ci", "shell-ci", "gh pr checks --watch")
	require.NotNil(t, a.convertToolCallUpdate("ses-oc", call))
	event := a.convertToolCallResultUpdate("ses-oc", update)
	require.NotNil(t, event)

	require.True(t, event.NormalizedPayload.IsActiveBackgroundWork())
	require.True(t, event.NormalizedPayload.IsDetachedBackgroundLaunch())
	require.False(t, event.NormalizedPayload.IsVerifiedBackgroundWork(),
		"OpenCode 2.2.0 ends delivery with the prompt, so the launch is inspect-only")
	require.Nil(t, event.NormalizedPayload.ShellExec().Output.ExitCode,
		"a launch result reports no job exit code")
}

// TestOpenCodeLegacyBackgroundUnknown pins AC-007.4/AC-008.5: once the legacy
// foreground prompt settles, retained unverified work becomes explicitly
// unknown instead of a permanent running row. It keeps identity and command,
// fabricates no exit code or output, and stops fencing later prompts.
func TestOpenCodeLegacyBackgroundUnknown(t *testing.T) {
	a := openCodeBackgroundAdapter(t, true)
	call, update := openCodeLaunchFrames("call-ci", "shell-ci", "gh pr checks --watch")
	require.NotNil(t, a.convertToolCallUpdate("ses-oc", call))
	require.NotNil(t, a.convertToolCallResultUpdate("ses-oc", update))
	drainEvents(a)

	a.settleUnverifiedOpenCodeBackgroundWork("ses-oc")

	events := drainEvents(a)
	require.Len(t, events, 1)
	event := events[0]
	require.Equal(t, streams.EventTypeBackgroundWorkUpdated, event.Type)
	require.NotNil(t, event.BackgroundWork)
	require.Equal(t, "shell-ci", event.BackgroundWork.WorkID)
	require.Equal(t, streams.RunStateUnknown, event.BackgroundWork.State)
	require.Equal(t, streams.WorkloadKindShell, event.BackgroundWork.Kind)
	require.Equal(t, "gh pr checks --watch", event.BackgroundWork.Title)
	require.Nil(t, event.BackgroundWork.ExitCode, "unknown activity never invents an exit status")
	require.Empty(t, event.BackgroundWork.Output)
	require.Empty(t, event.BackgroundWork.Capabilities.Actions,
		"observation-only jobs advertise no control")
	require.Equal(t, "call-ci", event.BackgroundWork.SourceCallID,
		"settlement must retain the original launch-call association")
	require.Equal(t, "shell-ci", event.BackgroundWork.RunID,
		"the provider's shell identity is the only execution identity")

	for _, ev := range events {
		require.NotEqual(t, toolStatusCancelled, ev.ToolStatus,
			"a settled launch must not be reported as a cancelled tool")
	}

	a.mu.RLock()
	_, tracked := a.activeToolCalls["call-ci"]
	a.mu.RUnlock()
	require.False(t, tracked, "unknown work must not keep fencing later prompts")
}

// TestOpenCodeLegacySettlementLeavesVerifiedWorkAlone guards the negotiated
// path this package must not break: verified background work survives prompt
// settlement and stays actionable.
func TestOpenCodeLegacySettlementLeavesVerifiedWorkAlone(t *testing.T) {
	a := openCodeBackgroundAdapter(t, true)
	call, update := openCodeLaunchFrames("call-ci", "shell-ci", "gh pr checks --watch")
	require.NotNil(t, a.convertToolCallUpdate("ses-oc", call))
	require.NotNil(t, a.convertToolCallResultUpdate("ses-oc", update))
	a.activeToolCalls["call-ci"].SetBackgroundWorkIdentity(
		streams.BackgroundWorkKindShell, "shell-ci", true, false,
	)
	drainEvents(a)

	a.settleUnverifiedOpenCodeBackgroundWork("ses-oc")

	require.Empty(t, drainEvents(a))
	a.mu.RLock()
	_, tracked := a.activeToolCalls["call-ci"]
	a.mu.RUnlock()
	require.True(t, tracked)
}

// TestOpenCodeBackgroundWorkFeatureOff pins AC-001.3's disabled path: with the
// shared feature off, the same wire frames keep ordinary shell behavior.
func TestOpenCodeBackgroundWorkFeatureOff(t *testing.T) {
	a := openCodeBackgroundAdapter(t, false)
	call, update := openCodeLaunchFrames("call-ci", "shell-ci", "gh pr checks --watch")
	require.NotNil(t, a.convertToolCallUpdate("ses-oc", call))
	event := a.convertToolCallResultUpdate("ses-oc", update)
	require.NotNil(t, event)
	require.Nil(t, event.NormalizedPayload.BackgroundWork(),
		"feature-off launches must not publish background-work identity")
	require.True(t, event.NormalizedPayload.ShellExec().Background,
		"ordinary shell presentation is unchanged")

	drainEvents(a)
	a.settleUnverifiedOpenCodeBackgroundWork("ses-oc")
	require.Empty(t, drainEvents(a))
}

// TestOpenCodeBackgroundLaunchRejectsUnrelatedProvider keeps another agent's
// identical-looking metadata out of the OpenCode recognizer.
func TestOpenCodeBackgroundLaunchRejectsUnrelatedProvider(t *testing.T) {
	a := newTestAdapterForAgent(claudeAgentID)
	a.backgroundWorkEnabled = true
	a.normalizer.backgroundWorkEnabled = true
	call := &acp.SessionUpdateToolCall{
		ToolCallId:    "call-other",
		Kind:          acp.ToolKindExecute,
		Status:        acp.ToolCallStatusInProgress,
		SessionUpdate: "tool_call",
		RawInput:      map[string]any{"command": "sleep 30", "background": true},
	}
	require.NotNil(t, a.convertToolCallUpdate("ses-other", call))
	completed := acp.ToolCallStatusCompleted
	event := a.convertToolCallResultUpdate("ses-other", &acp.SessionToolCallUpdate{
		ToolCallId:    "call-other",
		Status:        &completed,
		SessionUpdate: "tool_call_update",
		RawOutput: map[string]any{"metadata": map[string]any{
			"shellID": "shell-other",
			"status":  "running",
		}},
	})
	require.NotNil(t, event)
	require.Nil(t, event.NormalizedPayload.BackgroundWork())
}

func TestOpenCodeBackgroundLaunchRecognizer(t *testing.T) {
	running := func(shellID string) map[string]any {
		return map[string]any{"output": "Background process started", "metadata": map[string]any{
			"shellID": shellID,
			"status":  "running",
		}}
	}
	cases := []struct {
		name   string
		result any
		wantID string
		wantOK bool
	}{
		{"confirmed launch", running("shell-1"), "shell-1", true},
		{"lowercase identity spelling", map[string]any{"metadata": map[string]any{
			"shellId": "shell-2", "status": "running",
		}}, "shell-2", true},
		{"rawOutput envelope", map[string]any{"rawOutput": running("shell-3")}, "shell-3", true},
		{"completed rather than running", map[string]any{"metadata": map[string]any{
			"shellID": "shell-4", "status": "completed",
		}}, "", false},
		{"missing status", map[string]any{"metadata": map[string]any{
			"shellID": "shell-5",
		}}, "", false},
		{"missing identity", map[string]any{"metadata": map[string]any{
			"status": "running",
		}}, "", false},
		{"empty identity", map[string]any{"metadata": map[string]any{
			"shellID": "  ", "status": "running",
		}}, "", false},
		{"conflicting identities", map[string]any{"metadata": map[string]any{
			"shellID": "shell-6", "shellId": "shell-7", "status": "running",
		}}, "", false},
		{"malformed status", map[string]any{"metadata": map[string]any{
			"shellID": "shell-8", "status": 7,
		}}, "", false},
		{"banner text only", "Background process started shellID=shell-9", "", false},
		{"ordinary completed shell", map[string]any{
			"output":   "ok",
			"metadata": map[string]any{"exit": 0},
		}, "", false},
		{"nil result", nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			launch, ok := recognizeOpenCodeBackgroundLaunch(tc.result)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantID, launch.ShellID)
		})
	}
}

// TestOpenCodeBackgroundLaunchNeedsBothObservations pins the correlation rule:
// a background input without a confirmed result, and a running result without
// the background input, are both non-launches.
func TestOpenCodeBackgroundLaunchNeedsBothObservations(t *testing.T) {
	result := map[string]any{"metadata": map[string]any{"shellID": "shell-1", "status": "running"}}

	foreground := streams.NewShellExec("sleep 30", "", "", 0, false)
	if _, ok := confirmsOpenCodeBackgroundLaunch(foreground, result); ok {
		t.Fatal("a running shell result without the background input is not a confirmed launch")
	}

	background := streams.NewShellExec("sleep 30", "", "", 0, true)
	if _, ok := confirmsOpenCodeBackgroundLaunch(background, map[string]any{"output": "ok"}); ok {
		t.Fatal("the background input without a confirmed running result is not a launch")
	}
	if _, ok := confirmsOpenCodeBackgroundLaunch(nil, result); ok {
		t.Fatal("a nil payload is not a launch")
	}
	if launch, ok := confirmsOpenCodeBackgroundLaunch(background, result); !ok || launch.ShellID != "shell-1" {
		t.Fatalf("expected the correlated launch, got %+v ok=%v", launch, ok)
	}
}

// TestOpenCodePrepareSessionCancelSettlesBeforeCancelledSweep pins the cancel
// path: a confirmed launch becomes unknown instead of a cancelled tool that
// still claims live background work.
func TestOpenCodePrepareSessionCancelSettlesBeforeCancelledSweep(t *testing.T) {
	a := openCodeBackgroundAdapter(t, true)
	call, update := openCodeLaunchFrames("call-ci", "shell-ci", "gh pr checks --watch")
	require.NotNil(t, a.convertToolCallUpdate("ses-oc", call))
	require.NotNil(t, a.convertToolCallResultUpdate("ses-oc", update))
	drainEvents(a)

	a.prepareSessionCancel("ses-oc")
	a.cancelActiveToolCalls("ses-oc")

	events := drainEvents(a)
	require.Len(t, events, 1)
	require.Equal(t, streams.EventTypeBackgroundWorkUpdated, events[0].Type)
	require.Equal(t, streams.RunStateUnknown, events[0].BackgroundWork.State)
	require.Equal(t, "call-ci", events[0].BackgroundWork.SourceCallID)
	for _, ev := range events {
		require.NotEqual(t, toolStatusCancelled, ev.ToolStatus,
			"a settled launch must not be reported as a cancelled tool")
	}
}

// TestACPInspectionOnlyProducersDeclareCapabilities pins that Claude ACP shell
// and subagent producers publish their explicit inspect-only contract instead
// of relying on a read-time fallback.
func TestACPInspectionOnlyProducersDeclareCapabilities(t *testing.T) {
	n := NewNormalizer(claudeAgentID)
	shell := n.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "sleep 30", "run_in_background": true},
	})
	require.NotNil(t, shell)
	work := shell.BackgroundWork()
	require.NotNil(t, work)
	require.NotNil(t, work.Capabilities)
	require.Equal(t, "events_only", work.Capabilities.Discovery)
	require.Equal(t, "none", work.Capabilities.Output)
	require.Empty(t, work.Capabilities.Actions)

	subagent := streams.NewSubagentTask("Investigate", "do it", "general-purpose")
	stampSubagentBackgroundWork(subagent, claudeAgentID)
	require.NotNil(t, subagent.BackgroundWork())
	require.NotNil(t, subagent.BackgroundWork().Capabilities)
	require.Equal(t, "none", subagent.BackgroundWork().Capabilities.Output)
	require.Empty(t, subagent.BackgroundWork().Capabilities.Actions)
}
