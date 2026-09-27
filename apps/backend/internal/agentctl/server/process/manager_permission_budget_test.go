package process

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestPermissionParkedUntilBudgetCancel pins
// AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.5 (system design part 2,
// "Permission requests"): a permission request parked while detached stays
// pending until reattach or until the offline budget cancels the turn, and is
// never approved or denied automatically.
func TestPermissionParkedUntilBudgetCancel(t *testing.T) {
	m := &Manager{
		cfg:                &config.InstanceConfig{SessionID: "session-1"},
		logger:             newTestLogger(t),
		updatesCh:          make(chan adapter.AgentEvent, 4),
		pendingPermissions: make(map[string]*PendingPermission),
	}
	// A permission request only exists mid-turn.
	m.activeTurnCount.Store(1)

	if m.IsAttached() {
		t.Fatal("IsAttached() = true on a fresh Manager, want false (starts detached)")
	}

	respCh := make(chan *adapter.PermissionResponse, 1)
	go func() {
		resp, _ := m.handlePermissionRequest(context.Background(), &adapter.PermissionRequest{
			ToolCallID: "tool-1",
			Title:      "Run command",
			Options: []adapter.PermissionOption{
				{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
			},
		})
		respCh <- resp
	}()

	select {
	case event := <-m.updatesCh:
		if event.Type != adapter.EventTypePermissionRequest {
			t.Fatalf("event.Type = %v, want EventTypePermissionRequest", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("permission request notification never reached updatesCh")
	}

	// Nothing resolves the request while detached: no auto-approve, no
	// auto-deny, no timeout.
	select {
	case resp := <-respCh:
		t.Fatalf("handlePermissionRequest returned %+v before budget enforcement, want it still pending", resp)
	case <-time.After(200 * time.Millisecond):
	}

	// Drive the real offline-budget expiry path (episode 1 matches a freshly
	// built attachmentState), the same entry point the budget timer itself
	// uses. With an active turn and no adapter set, the first cancel attempt
	// trivially succeeds (no adapter to cancel), so enforcement reaches
	// "cancel any still-pending permission request" through the real
	// Manager.CancelPendingPermissions hook.
	m.ensureAttach().expire(1)

	select {
	case resp := <-respCh:
		if resp == nil || !resp.Cancelled {
			t.Fatalf("response = %+v, want Cancelled true, never approved or denied", resp)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handlePermissionRequest never returned after budget enforcement cancelled the turn")
	}

	select {
	case event := <-m.updatesCh:
		if event.Type != adapter.EventTypePermissionCancelled {
			t.Fatalf("event.Type = %v, want EventTypePermissionCancelled", event.Type)
		}
	default:
		t.Fatal("no permission-cancelled notification was sent")
	}
}
