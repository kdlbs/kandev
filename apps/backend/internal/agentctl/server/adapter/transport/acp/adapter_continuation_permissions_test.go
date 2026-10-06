package acp

import (
	"context"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func continuationPermissionFixture(t *testing.T) (*Adapter, *promptTurnState, *PermissionRequest) {
	t.Helper()
	a, turn := newCursorPromptTurn(t, 7)
	a.cfg.ProviderInterruptionContinuation = true
	a.capabilities.LoadSession = true
	a.sessionID = "session-1"
	a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{
		ToolCallId: "cmd-1", Title: "Execute fixture", Kind: acpsdk.ToolKindExecute, Status: acpsdk.ToolCallStatusInProgress,
	}}), 7)
	return a, turn, &PermissionRequest{SessionID: "session-1", ToolCallID: "cmd-1", Options: []streams.PermissionOption{
		{OptionID: "allow", Kind: streams.PermissionOptionKindAllowOnce},
		{OptionID: "reject", Kind: streams.PermissionOptionKindRejectOnce},
	}}
}

func completeContinuationPermissionTool(a *Adapter) {
	status := acpsdk.ToolCallStatusCompleted
	a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
		ToolCallId: "cmd-1", Status: &status,
	}}), 7)
}

func TestCursorContinuationEvidencePermissionOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *PermissionResponse
		err      error
		safe     bool
	}{
		{"approved completed tool", &PermissionResponse{OptionID: "allow"}, nil, true},
		{"rejected", &PermissionResponse{OptionID: "reject"}, nil, false},
		{"cancelled", &PermissionResponse{Cancelled: true}, nil, false},
		{"unoffered choice", &PermissionResponse{OptionID: "unknown"}, nil, false},
		{"missing response", nil, nil, false},
		{"delivery error", &PermissionResponse{OptionID: "allow"}, errors.New("delivery failed"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, turn, req := continuationPermissionFixture(t)
			a.SetPermissionHandler(func(context.Context, *PermissionRequest) (*PermissionResponse, error) {
				return tc.response, tc.err
			})
			_, _ = a.handlePermissionRequest(t.Context(), req)
			completeContinuationPermissionTool(a)
			require.Equal(t, tc.safe, a.continuationSafetySnapshot(turn).SafeFor(7))
		})
	}
}

func TestCursorContinuationEvidencePendingPermission(t *testing.T) {
	a, turn, req := continuationPermissionFixture(t)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	a.SetPermissionHandler(func(context.Context, *PermissionRequest) (*PermissionResponse, error) {
		close(entered)
		<-release
		return &PermissionResponse{OptionID: "allow"}, nil
	})
	go func() {
		defer close(done)
		_, _ = a.handlePermissionRequest(t.Context(), req)
	}()
	<-entered
	completeContinuationPermissionTool(a)
	snapshot := a.continuationSafetySnapshot(turn)
	close(release)
	<-done
	require.True(t, snapshot.Pending, "a completed tool cannot override an unresolved permission")
	require.False(t, snapshot.SafeFor(7))
	require.True(t, a.continuationSafetySnapshot(turn).SafeFor(7))
}

func TestCursorContinuationEvidencePermissionRequiresTrackedTool(t *testing.T) {
	a, turn := newCursorPromptTurn(t, 7)
	a.cfg.ProviderInterruptionContinuation = true
	a.capabilities.LoadSession = true
	a.sessionID = "session-1"
	a.SetPermissionHandler(func(context.Context, *PermissionRequest) (*PermissionResponse, error) {
		return &PermissionResponse{OptionID: "allow"}, nil
	})
	_, err := a.handlePermissionRequest(t.Context(), &PermissionRequest{SessionID: "session-1", ToolCallID: "missing",
		Options: []PermissionOption{{OptionID: "allow", Kind: streams.PermissionOptionKindAllowOnce}},
	})
	require.NoError(t, err)
	require.False(t, a.continuationSafetySnapshot(turn).SafeFor(7), "approval cannot establish an unobserved tool outcome")
}

func TestCursorContinuationEvidencePermissionBeforeToolNotification(t *testing.T) {
	a, turn := newCursorPromptTurn(t, 7)
	a.cfg.ProviderInterruptionContinuation = true
	a.capabilities.LoadSession = true
	a.sessionID = "session-1"
	a.SetPermissionHandler(func(context.Context, *PermissionRequest) (*PermissionResponse, error) {
		return &PermissionResponse{OptionID: "allow"}, nil
	})
	_, err := a.handlePermissionRequest(t.Context(), &PermissionRequest{SessionID: "session-1", ToolCallID: "cmd-1",
		Options: []PermissionOption{{OptionID: "allow", Kind: streams.PermissionOptionKindAllowOnce}},
	})
	require.NoError(t, err)
	before := a.continuationSafetySnapshot(turn)
	require.False(t, before.SafeFor(7), "an approval alone does not prove a tool outcome")
	a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{
		ToolCallId: "cmd-1", Title: "Execute fixture", Kind: acpsdk.ToolKindExecute, Status: acpsdk.ToolCallStatusCompleted,
	}}), 7)
	require.True(t, a.continuationSafetySnapshot(turn).SafeFor(7), "a later completed call satisfies the approved permission")
	require.False(t, before.SafeFor(7), "published evidence remains immutable")
}

func TestCursorContinuationEvidencePermissionResolutionKeepsTurnOwner(t *testing.T) {
	a, predecessor, req := continuationPermissionFixture(t)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	a.SetPermissionHandler(func(context.Context, *PermissionRequest) (*PermissionResponse, error) {
		close(entered)
		<-release
		return &PermissionResponse{Cancelled: true}, nil
	})
	go func() {
		defer close(done)
		_, _ = a.handlePermissionRequest(t.Context(), req)
	}()
	<-entered
	_, successor := a.registerPromptTurn(t.Context(), 8)
	t.Cleanup(func() { a.clearPromptTurn(successor) })
	close(release)
	<-done
	require.False(t, a.continuationSafetySnapshot(predecessor).SafeFor(7))
	require.True(t, a.continuationSafetySnapshot(successor).SafeFor(8), "a delayed refusal cannot poison the successor")
}

func TestCursorContinuationEvidencePermissionV1AndDisabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		a, turn := newCursorPromptTurn(t, 7)
		a.cfg.ProviderInterruptionContinuation = enabled
		a.dialect.continuationSupport = streams.ContinuationNativeSavedHistoryV1
		a.capabilities.LoadSession = true
		a.sessionID = "session-1"
		a.SetPermissionHandler(func(context.Context, *PermissionRequest) (*PermissionResponse, error) {
			return &PermissionResponse{OptionID: "allow"}, nil
		})
		_, err := a.handlePermissionRequest(t.Context(), &PermissionRequest{SessionID: "session-1", ToolCallID: "read-1"})
		require.NoError(t, err)
		if enabled {
			require.False(t, a.continuationSafetySnapshot(turn).SafeFor(7), "V1 keeps its original permission veto")
		} else {
			require.Nil(t, a.continuationSafetySnapshot(turn))
			require.False(t, turn.continuationUnsafe, "disabled mode derives no permission evidence")
		}
	}
}

func TestCursorContinuationEvidenceHandoffCannotOwnLatePermission(t *testing.T) {
	a, predecessor, req := continuationPermissionFixture(t)
	predecessor.allowHandoff, predecessor.gateOwned, predecessor.handedOff = true, true, true
	_, successor := newPromptTurnState(t.Context(), 8, true)
	_, transferred := a.tryTransferPromptTurn(successor, true)
	require.True(t, transferred)
	finish := a.beginContinuationPermission(req.SessionID, req.ToolCallID, req.Options)
	require.NotNil(t, finish)
	finish(&PermissionResponse{OptionID: "allow"}, nil)
	a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{
		ToolCallId: "cmd-1", Title: "Execute fixture", Kind: acpsdk.ToolKindExecute, Status: acpsdk.ToolCallStatusCompleted,
	}}), 8)
	require.False(t, a.continuationSafetySnapshot(successor).SafeFor(8), "unattributed predecessor work cannot authorize successor continuation")
	a.clearPromptTurn(successor)
	_, serial := a.registerPromptTurn(t.Context(), 9)
	t.Cleanup(func() { a.clearPromptTurn(serial) })
	require.True(t, a.continuationSafetySnapshot(serial).SafeFor(9), "a serialized successor starts with fresh evidence")
}
