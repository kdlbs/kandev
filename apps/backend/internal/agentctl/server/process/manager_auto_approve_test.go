package process

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// autoApproveManager builds a Manager with blanket auto-approval enabled and a
// buffered updates channel, so a fall-through to the pending flow can be
// observed without a live agent.
func autoApproveManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{
		cfg:                &config.InstanceConfig{AutoApprovePermissions: true},
		logger:             newTestLogger(t),
		updatesCh:          make(chan adapter.AgentEvent, 4),
		pendingPermissions: make(map[string]*PendingPermission),
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.7
func TestAutoApprovePermissionSelectsAllowListedAfterReject(t *testing.T) {
	m := autoApproveManager(t)

	decision, ok := m.autoApprovePermission(&adapter.PermissionRequest{
		Options: []adapter.PermissionOption{
			{OptionID: "reject", Kind: streams.PermissionOptionKindRejectOnce},
			{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
		},
	})

	if !ok {
		t.Fatalf("ok = false, want true when an allow option is offered")
	}
	if decision.response == nil || decision.response.Cancelled || decision.response.OptionID != "allow-once" {
		t.Fatalf("response = %+v, want option allow-once", decision.response)
	}
	if decision.option.Kind != streams.PermissionOptionKindAllowOnce {
		t.Fatalf("selected kind = %q, want allow_once", decision.option.Kind)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.1, .2
func TestAutoApprovePermissionFallsThroughWithoutAllowOption(t *testing.T) {
	m := autoApproveManager(t)

	decision, ok := m.autoApprovePermission(&adapter.PermissionRequest{
		Options: []adapter.PermissionOption{
			{OptionID: "reject-once", Kind: streams.PermissionOptionKindRejectOnce},
			{OptionID: "reject-always", Kind: streams.PermissionOptionKindRejectAlways},
		},
	})

	if ok {
		t.Fatalf("ok = true with decision %+v, want fall-through to the interactive prompt", decision)
	}
	if decision.response != nil {
		t.Fatalf("response = %+v, want nil so the caller keeps the pending flow", decision.response)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.3
func TestAutoApprovePermissionFallsThroughWithoutOptions(t *testing.T) {
	m := autoApproveManager(t)

	decision, ok := m.autoApprovePermission(&adapter.PermissionRequest{})

	if ok {
		t.Fatalf("ok = true with decision %+v, want fall-through rather than a cancellation", decision)
	}
	if decision.response != nil {
		t.Fatalf("response = %+v, want nil rather than a cancellation", decision.response)
	}
}

// Providers are not required to spell the kind exactly as Kandev does; an
// unrecognized spelling must not silently degrade into a refusal.
func TestAutoApprovePermissionNormalizesOptionKind(t *testing.T) {
	for _, kind := range []string{"Allow_Once", " allow_once ", "ALLOW_ALWAYS"} {
		t.Run(kind, func(t *testing.T) {
			m := autoApproveManager(t)
			decision, ok := m.autoApprovePermission(&adapter.PermissionRequest{
				Options: []adapter.PermissionOption{
					{OptionID: "allow", Kind: streams.PermissionOptionKind(kind)},
				},
			})
			if !ok || decision.response == nil || decision.response.OptionID != "allow" {
				t.Fatalf("kind %q: ok = %v, response = %+v, want the allow option selected", kind, ok, decision.response)
			}
		})
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.6, .8, .10
// With blanket auto-approval enabled and no approvable option, the request must
// reach the user as a pending permission instead of being answered with a
// refusal the user never sees.
func TestHandlePermissionRequestPromptsWhenAutoApproveCannotApprove(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := autoApproveManager(t)

	type result struct {
		response *adapter.PermissionResponse
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		response, err := m.handlePermissionRequest(ctx, &adapter.PermissionRequest{
			SessionID:  "session-1",
			ToolCallID: "tool-1",
			PendingID:  "pending-1",
			Title:      "Run git commit",
			ActionType: string(streams.ActionTypeCommand),
			Options: []adapter.PermissionOption{
				{OptionID: "reject", Kind: streams.PermissionOptionKindRejectOnce},
			},
		})
		resultCh <- result{response: response, err: err}
	}()

	select {
	case event := <-m.updatesCh:
		if event.Type != adapter.EventTypePermissionRequest {
			t.Fatalf("event type = %q, want %q", event.Type, adapter.EventTypePermissionRequest)
		}
		if event.PendingID != "pending-1" {
			t.Fatalf("pending id = %q, want pending-1", event.PendingID)
		}
	case res := <-resultCh:
		t.Fatalf("handlePermissionRequest answered without prompting: %+v (err=%v)", res.response, res.err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the permission notification")
	}

	if err := m.RespondToPermission("pending-1", "reject", false); err != nil {
		t.Fatalf("RespondToPermission returned error: %v", err)
	}

	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Fatalf("handlePermissionRequest returned error: %v", res.err)
		}
		if res.response == nil || res.response.OptionID != "reject" {
			t.Fatalf("response = %+v, want the option the user selected", res.response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the permission response")
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.4, .9
// An auto-approved call must leave a record. Without one, a session where
// Kandev answered is indistinguishable from one where the agent never asked.
func TestAutoApprovedPermissionIsRecorded(t *testing.T) {
	m := autoApproveManager(t)

	response, err := m.handlePermissionRequest(context.Background(), &adapter.PermissionRequest{
		SessionID:  "session-1",
		ToolCallID: "tool-1",
		PendingID:  "pending-1",
		Title:      "Run git commit",
		ActionType: string(streams.ActionTypeCommand),
		Options: []adapter.PermissionOption{
			{OptionID: "allow-once", Name: "Yes", Kind: streams.PermissionOptionKindAllowOnce},
			{OptionID: "reject", Name: "No", Kind: streams.PermissionOptionKindRejectOnce},
		},
	})
	if err != nil {
		t.Fatalf("handlePermissionRequest returned error: %v", err)
	}
	if response == nil || response.OptionID != "allow-once" {
		t.Fatalf("response = %+v, want the allow option", response)
	}

	select {
	case event := <-m.updatesCh:
		if event.Type != adapter.EventTypePermissionRequest {
			t.Fatalf("event type = %q, want %q", event.Type, adapter.EventTypePermissionRequest)
		}
		if event.AutoApprovedOptionID != "allow-once" {
			t.Fatalf("auto approved option = %q, want allow-once", event.AutoApprovedOptionID)
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal auto-approved permission event: %v", err)
		}
		var eventData map[string]any
		if err := json.Unmarshal(encoded, &eventData); err != nil {
			t.Fatalf("decode auto-approved permission event: %v", err)
		}
		if got := eventData["auto_approved_option_kind"]; got != "allow_once" {
			t.Fatalf("auto approved option kind = %v, want allow_once", got)
		}
		if got := eventData["auto_approval_source"]; got != "auto_approve" {
			t.Fatalf("auto approval source = %v, want auto_approve", got)
		}
		if event.PendingID != "pending-1" {
			t.Fatalf("pending id = %q, want pending-1", event.PendingID)
		}
		if len(event.PermissionOptions) != 2 {
			t.Fatalf("options = %d, want the offered set preserved", len(event.PermissionOptions))
		}
	default:
		t.Fatal("auto-approved permission produced no record on the updates channel")
	}

	// The request must not linger as pending: it is already answered.
	m.permissionMu.Lock()
	pendingCount := len(m.pendingPermissions)
	m.permissionMu.Unlock()
	if pendingCount != 0 {
		t.Fatalf("pending permissions = %d, want 0 for an answered request", pendingCount)
	}
}

func TestAutoApproveDoesNotAnswerWhenDecisionRecordCannotBeQueued(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := autoApproveManager(t)
	for i := 0; i < cap(m.updatesCh); i++ {
		m.updatesCh <- adapter.AgentEvent{Type: adapter.EventTypeError}
	}
	cancel()

	response, err := m.handlePermissionRequest(ctx, &adapter.PermissionRequest{
		SessionID:  "session-1",
		ToolCallID: "tool-1",
		PendingID:  "pending-full-channel",
		Options: []adapter.PermissionOption{
			{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
		},
	})
	if err != nil {
		t.Fatalf("handlePermissionRequest returned error: %v", err)
	}
	if response == nil || !response.Cancelled || response.OptionID != "" {
		t.Fatalf("response = %+v, want cancellation without an approval when the decision record cannot be queued", response)
	}
}
