package process

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// The unattended and attended directions of
// REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005, asserted below the browser layer
// so a regression is caught without an E2E run.
func TestPermissionContractBothDirections(t *testing.T) {
	commandRequest := func() *adapter.PermissionRequest {
		return &adapter.PermissionRequest{
			SessionID:  "session-1",
			ToolCallID: "tool-1",
			PendingID:  "pending-1",
			Title:      "Run git commit",
			ActionType: string(streams.ActionTypeCommand),
			Options: []adapter.PermissionOption{
				{OptionID: "allow-once", Name: "Yes", Kind: streams.PermissionOptionKindAllowOnce},
				{OptionID: "reject", Name: "No", Kind: streams.PermissionOptionKindRejectOnce},
			},
		}
	}

	t.Run("unattended profile answers without a person", func(t *testing.T) {
		m := &Manager{
			cfg:                &config.InstanceConfig{AutoApprovePermissions: true},
			logger:             newTestLogger(t),
			updatesCh:          make(chan adapter.AgentEvent, 4),
			pendingPermissions: make(map[string]*PendingPermission),
		}

		response, err := m.handlePermissionRequest(context.Background(), commandRequest())
		if err != nil {
			t.Fatalf("handlePermissionRequest returned error: %v", err)
		}
		if response == nil || response.Cancelled || response.OptionID != "allow-once" {
			t.Fatalf("response = %+v, want the allow option with nobody answering", response)
		}
	})

	t.Run("default profile holds the call until answered", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m := &Manager{
			cfg:                &config.InstanceConfig{},
			logger:             newTestLogger(t),
			updatesCh:          make(chan adapter.AgentEvent, 4),
			pendingPermissions: make(map[string]*PendingPermission),
		}

		type result struct {
			response *adapter.PermissionResponse
			err      error
		}
		resultCh := make(chan result, 1)
		go func() {
			response, err := m.handlePermissionRequest(ctx, commandRequest())
			resultCh <- result{response: response, err: err}
		}()

		select {
		case event := <-m.updatesCh:
			if event.Type != adapter.EventTypePermissionRequest {
				t.Fatalf("event type = %q, want a permission request", event.Type)
			}
			if event.AutoApprovedOptionID != "" {
				t.Fatalf("event = %+v, want a pending request rather than a recorded approval", event)
			}
		case res := <-resultCh:
			t.Fatalf("the default profile answered on its own: %+v (err=%v)", res.response, res.err)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the permission prompt")
		}

		if err := m.RespondToPermission("pending-1", "allow-once", false); err != nil {
			t.Fatalf("RespondToPermission returned error: %v", err)
		}
		select {
		case res := <-resultCh:
			if res.err != nil || res.response == nil || res.response.OptionID != "allow-once" {
				t.Fatalf("response = %+v (err=%v), want the answered option", res.response, res.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the answered response")
		}
	})
}
