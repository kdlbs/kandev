package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

type recordingPermissionResponder struct {
	resolved  []string
	cancelled []string
}

func (r *recordingPermissionResponder) ResolvePermission(
	_ context.Context, requestID, pendingID, optionID string,
) (*agentctl.PermissionResolveResponse, error) {
	r.resolved = append(r.resolved, requestID+"/"+pendingID+"/"+optionID)
	return &agentctl.PermissionResolveResponse{}, nil
}

func (r *recordingPermissionResponder) CancelPermission(
	_ context.Context, requestID, pendingID string,
) (*agentctl.PermissionCancelResponse, error) {
	r.cancelled = append(r.cancelled, requestID+"/"+pendingID)
	return &agentctl.PermissionCancelResponse{}, nil
}

// A taskless run has no session, so the orchestrator never sees its permission
// requests; the lifecycle manager must settle them or the agent hangs.
func TestSettleTasklessPermission(t *testing.T) {
	cases := []struct {
		name          string
		event         agentctl.AgentEvent
		wantResolved  []string
		wantCancelled []string
	}{
		{
			name: "pending automatic approval is confirmed",
			event: agentctl.AgentEvent{
				RequestID: "req", PendingID: "pend",
				AutoApprovedOptionID: "allow_once", AutoApprovalPending: true,
			},
			wantResolved: []string{"req/pend/allow_once"},
		},
		{
			name: "already delivered automatic approval is only a record",
			event: agentctl.AgentEvent{
				RequestID: "req", PendingID: "pend",
				AutoApprovedOptionID: "allow_once",
			},
		},
		{
			name:          "request nobody can answer is rejected",
			event:         agentctl.AgentEvent{RequestID: "req", PendingID: "pend"},
			wantCancelled: []string{"req/pend"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			responder := &recordingPermissionResponder{}
			settleTasklessPermission(context.Background(), newTestLogger(), responder, "exec", tc.event)
			require.Equal(t, tc.wantResolved, responder.resolved)
			require.Equal(t, tc.wantCancelled, responder.cancelled)
		})
	}
}
