package process

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestRespondToPermissionRejectsOptionNotOffered(t *testing.T) {
	responseCh := make(chan *adapter.PermissionResponse, 1)
	m := &Manager{
		logger: newTestLogger(t),
		pendingPermissions: map[string]*PendingPermission{
			"pending-1": {
				ID: "pending-1",
				Request: &adapter.PermissionRequest{Options: []adapter.PermissionOption{
					{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
					{OptionID: "reject-once", Kind: streams.PermissionOptionKindRejectOnce},
				}},
				ResponseCh: responseCh,
			},
		},
	}

	err := m.RespondToPermission("pending-1", "invented-option", false)
	if err == nil {
		t.Fatal("expected an unknown option error")
	}
	if len(responseCh) != 0 {
		t.Fatal("unknown option must not reach the provider response channel")
	}
}

func TestResolvePermissionConsumesExactRequestOnce(t *testing.T) {
	responseCh := make(chan *adapter.PermissionResponse, 1)
	m := permissionTestManager(t, responseCh)

	result, err := m.ResolvePermission("request-1", "pending-1", "allow-once")
	if err != nil {
		t.Fatalf("resolve permission: %v", err)
	}
	if result.OptionKind != streams.PermissionOptionKindAllowOnce || result.Status != "resolved" {
		t.Fatalf("unexpected result: %+v", result)
	}
	response := <-responseCh
	if response.OptionID != "allow-once" || response.Cancelled {
		t.Fatalf("unexpected provider response: %+v", response)
	}

	_, err = m.ResolvePermission("request-1", "pending-1", "allow-once")
	assertPermissionErrorCode(t, err, streams.PermissionErrorAlreadyResolved)
	if len(responseCh) != 0 {
		t.Fatal("replayed resolution reached the provider")
	}
}

func TestResolvePermissionRejectsStaleAndUnknownOptions(t *testing.T) {
	tests := []struct {
		name      string
		requestID string
		optionID  string
		wantCode  string
	}{
		{name: "replaced request", requestID: "older-request", optionID: "allow-once", wantCode: streams.PermissionErrorStale},
		{name: "unknown option", requestID: "request-1", optionID: "allow-forever", wantCode: streams.PermissionErrorOptionNotOffered},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responseCh := make(chan *adapter.PermissionResponse, 1)
			m := permissionTestManager(t, responseCh)
			_, err := m.ResolvePermission(test.requestID, "pending-1", test.optionID)
			assertPermissionErrorCode(t, err, test.wantCode)
			if len(responseCh) != 0 {
				t.Fatal("invalid resolution reached the provider")
			}
		})
	}
}

func TestResolvePermissionConcurrentCallsDeliverOnce(t *testing.T) {
	responseCh := make(chan *adapter.PermissionResponse, 1)
	m := permissionTestManager(t, responseCh)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.ResolvePermission("request-1", "pending-1", "allow-once")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	successes := 0
	failures := 0
	for err := range errs {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 || len(responseCh) != 1 {
		t.Fatalf("successes=%d failures=%d deliveries=%d, want 1/1/1", successes, failures, len(responseCh))
	}
}

func TestCancelPermissionRequiresExactRequestGeneration(t *testing.T) {
	responseCh := make(chan *adapter.PermissionResponse, 1)
	m := permissionTestManager(t, responseCh)
	if _, err := m.CancelPermission("older-request", "pending-1"); err == nil {
		t.Fatal("expected stale generation error")
	}
	if len(responseCh) != 0 {
		t.Fatal("stale cancellation reached the provider")
	}
	result, err := m.CancelPermission("request-1", "pending-1")
	if err != nil || result.Status != "cancelled" {
		t.Fatalf("cancel result=%+v err=%v", result, err)
	}
	if response := <-responseCh; !response.Cancelled {
		t.Fatalf("provider response = %+v, want cancelled", response)
	}
}

func TestListPendingPermissionsIsDeterministicAndReturnsCopies(t *testing.T) {
	createdAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	first := permissionTestPending("request-b", "pending-b", createdAt)
	second := permissionTestPending("request-a", "pending-a", createdAt)
	m := &Manager{pendingPermissions: map[string]*PendingPermission{
		first.ID:  first,
		second.ID: second,
	}}

	permissions := m.ListPendingPermissions()
	if len(permissions) != 2 || permissions[0].RequestID != "request-a" || permissions[1].RequestID != "request-b" {
		t.Fatalf("unexpected permission order: %+v", permissions)
	}
	permissions[0].Options[0].Name = "mutated"
	again := m.ListPendingPermissions()
	if again[0].Options[0].Name == "mutated" {
		t.Fatal("list returned mutable live option storage")
	}
}

func permissionTestManager(t *testing.T, responseCh chan *adapter.PermissionResponse) *Manager {
	t.Helper()
	pending := permissionTestPending("request-1", "pending-1", time.Now().UTC())
	pending.ResponseCh = responseCh
	return &Manager{
		logger:             newTestLogger(t),
		pendingPermissions: map[string]*PendingPermission{pending.ID: pending},
	}
}

func permissionTestPending(requestID, pendingID string, createdAt time.Time) *PendingPermission {
	return &PendingPermission{
		ID:        pendingID,
		RequestID: requestID,
		Request: &adapter.PermissionRequest{Options: []adapter.PermissionOption{
			{OptionID: "allow-once", Name: "Allow once", Kind: streams.PermissionOptionKindAllowOnce},
		}},
		Snapshot: streams.PendingAgentPermission{
			RequestID: requestID,
			PendingID: pendingID,
			Options: []streams.PermissionChoice{
				{OptionID: "allow-once", Name: "Allow once", Kind: streams.PermissionOptionKindAllowOnce},
			},
			CreatedAt: createdAt,
			Status:    streams.PermissionStatusPending,
		},
		ResponseCh: make(chan *adapter.PermissionResponse, 1),
		CreatedAt:  createdAt,
		State:      streams.PermissionStatusPending,
	}
}

func assertPermissionErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	var operationErr *PermissionOperationError
	if !errors.As(err, &operationErr) {
		t.Fatalf("error = %v, want PermissionOperationError", err)
	}
	if operationErr.Code != want {
		t.Fatalf("error code = %q, want %q", operationErr.Code, want)
	}
}

func TestPermissionRequestEventHasKandevGenerationID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &Manager{
		cfg: &config.InstanceConfig{
			TaskID:    "task-1",
			SessionID: "session-1",
		},
		logger:             newTestLogger(t),
		updatesCh:          make(chan adapter.AgentEvent, 2),
		pendingPermissions: make(map[string]*PendingPermission),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = m.handlePermissionRequest(ctx, &adapter.PermissionRequest{
			PendingID:  "provider-pending-1",
			ToolCallID: "tool-1",
			Title:      "Run command",
			Options: []adapter.PermissionOption{
				{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
			},
		})
	}()

	event := <-m.updatesCh
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	requestID, _ := payload["request_id"].(string)
	if requestID == "" || requestID == "provider-pending-1" {
		t.Fatalf("request_id = %q, want a distinct Kandev generation ID; event=%s", requestID, encoded)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("permission handler did not exit after cancellation")
	}
}

func TestPermissionNotificationDoesNotExposeSecretsOrEnvironment(t *testing.T) {
	const secret = "sk-abcdefghijklmnopqrstuvwxyz123456"
	m := &Manager{
		logger:    newTestLogger(t),
		updatesCh: make(chan adapter.AgentEvent, 1),
	}
	pending := &PendingPermission{
		ID: "pending-1",
		Request: &adapter.PermissionRequest{
			SessionID:  "provider-session",
			ToolCallID: "tool-1",
			Title:      "Run command with token=" + secret,
			ActionType: "command",
			ActionDetails: map[string]any{
				"description": "Run command",
				"raw_input": map[string]any{
					"command": "curl -H 'Authorization: Bearer " + secret + "' https://example.com",
					"cwd":     "/workspace/project",
					"env":     map[string]any{"API_TOKEN": secret},
				},
			},
		},
	}
	pending.Snapshot = m.permissionSnapshot(pending)

	m.sendPermissionNotification(pending)
	event := <-m.updatesCh
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("permission notification leaked secret: %s", encoded)
	}
	if strings.Contains(string(encoded), "API_TOKEN") {
		t.Fatalf("permission notification leaked environment key: %s", encoded)
	}
}

func TestPermissionSnapshotMarksPresentationRedaction(t *testing.T) {
	const secret = "sk-abcdefghijklmnopqrstuvwxyz123456"
	m := &Manager{logger: newTestLogger(t)}
	pending := &PendingPermission{
		Request: &adapter.PermissionRequest{
			Title:      "Run command with token=" + secret,
			ActionType: "command",
			ActionDetails: map[string]any{
				"description": "Run command",
				"raw_input":   map[string]any{"command": "echo safe"},
			},
			Options: []adapter.PermissionOption{
				{OptionID: "allow-once", Name: "Allow with token=" + secret, Kind: streams.PermissionOptionKindAllowOnce},
			},
		},
	}

	snapshot := m.permissionSnapshot(pending)
	if !snapshot.Action.Redacted {
		t.Fatal("presentation redaction should set action.redacted")
	}
	if snapshot.Title == pending.Request.Title || snapshot.Options[0].Name == pending.Request.Options[0].Name {
		t.Fatalf("snapshot retained unsanitized presentation text: %+v", snapshot)
	}
}

// TestPermissionRequestResolveVersusContextCancelIsExactlyOnce exercises the
// two terminal paths that used to bypass each other: ResolvePermission and the
// handlePermissionRequest goroutine's own ctx.Done() branch. Before
// consumePermission unified them, a resolve racing a context cancellation
// could both "succeed": the resolve delivered into a buffered channel nobody
// was reading anymore, while the handler had already returned Cancelled=true
// via ctx.Done(). Run many times under -race so the exactly-once invariant
// (never both, never neither) survives real scheduling, not just one order.
func TestPermissionRequestResolveVersusContextCancelIsExactlyOnce(t *testing.T) {
	for range 50 {
		m := &Manager{
			cfg: &config.InstanceConfig{
				TaskID:    "task-1",
				SessionID: "session-1",
			},
			logger:             newTestLogger(t),
			updatesCh:          make(chan adapter.AgentEvent, 2),
			pendingPermissions: make(map[string]*PendingPermission),
		}
		ctx, cancel := context.WithCancel(context.Background())

		type handlerResult struct {
			resp *adapter.PermissionResponse
			err  error
		}
		handlerDone := make(chan handlerResult, 1)
		go func() {
			resp, err := m.handlePermissionRequest(ctx, &adapter.PermissionRequest{
				PendingID:  "pending-race",
				ToolCallID: "tool-1",
				Title:      "Run command",
				Options: []adapter.PermissionOption{
					{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
				},
			})
			handlerDone <- handlerResult{resp: resp, err: err}
		}()

		event := <-m.updatesCh
		requestID := event.RequestID

		start := make(chan struct{})
		var wg sync.WaitGroup
		var resolveErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			cancel()
		}()
		go func() {
			defer wg.Done()
			<-start
			_, resolveErr = m.ResolvePermission(requestID, "pending-race", "allow-once")
		}()
		close(start)
		wg.Wait()

		result := <-handlerDone

		if resolveErr == nil {
			if result.resp == nil || result.resp.Cancelled || result.resp.OptionID != "allow-once" {
				t.Fatalf("resolve won but handler response = %+v, want the resolved option", result.resp)
			}
			continue
		}
		var opErr *PermissionOperationError
		if !errors.As(resolveErr, &opErr) {
			t.Fatalf("resolve error = %v, want PermissionOperationError", resolveErr)
		}
		if opErr.Code != streams.PermissionErrorStale && opErr.Code != streams.PermissionErrorAlreadyResolved {
			t.Fatalf("resolve error code = %q, want stale or already-resolved", opErr.Code)
		}
		if result.resp == nil || !result.resp.Cancelled {
			t.Fatalf("context-cancel won but handler response = %+v, want Cancelled=true", result.resp)
		}
	}
}
