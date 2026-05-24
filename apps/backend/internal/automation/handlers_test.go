package automation

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	store := setupTestStore(t)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	eb := bus.NewMemoryEventBus(log)
	return NewService(store, eb, log)
}

func TestCreateAutomationResponse_IncludesWebhookSecret(t *testing.T) {
	// Mirrors the WS create flow: the server should return the plaintext
	// webhook secret exactly once, so the UI can show it to the user.
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, err := ws.NewRequest("req-1", ws.ActionAutomationCreate, &CreateAutomationRequest{
		WorkspaceID:       "ws-1",
		Name:              "with secret",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := wsCreate(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("expected response, got %v: %s", resp.Type, string(resp.Payload))
	}

	var got CreateAutomationResponse
	if err := json.Unmarshal(resp.Payload, &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Automation == nil {
		t.Fatalf("expected automation in response, got %+v", got)
	}
	if got.ID == "" {
		t.Fatalf("expected non-empty automation id, got %+v", got)
	}
	if got.WebhookSecret == "" {
		t.Fatal("expected non-empty webhook secret in create response")
	}
}

func TestWsRevealWebhookSecret_Roundtrip(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID:       "ws-1",
		Name:              "reveal me",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationWebhookRevealSecret, map[string]any{"id": a.ID, "workspace_id": "ws-1"})
	resp, err := wsRevealWebhookSecret(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("expected response, got %v: %s", resp.Type, string(resp.Payload))
	}

	var got RevealWebhookSecretResponse
	if err := json.Unmarshal(resp.Payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.WebhookSecret == "" {
		t.Fatal("expected non-empty webhook secret")
	}
	// The reveal must return the same secret that the store generated —
	// otherwise the user's copy from the create response would stop working.
	if got.WebhookSecret != a.WebhookSecret {
		t.Errorf("reveal returned a different secret than create: reveal=%q create=%q", got.WebhookSecret, a.WebhookSecret)
	}
}

func TestWsRevealWebhookSecret_NotFound(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationWebhookRevealSecret, map[string]any{"id": "does-not-exist", "workspace_id": "ws-1"})
	resp, err := wsRevealWebhookSecret(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("expected error response, got %v: %s", resp.Type, string(resp.Payload))
	}

	var ep ws.ErrorPayload
	if err := json.Unmarshal(resp.Payload, &ep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ep.Code != ws.ErrorCodeNotFound {
		t.Errorf("expected NOT_FOUND, got %q", ep.Code)
	}
}

func TestWsRevealWebhookSecret_RequiresID(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationWebhookRevealSecret, map[string]any{})
	resp, err := wsRevealWebhookSecret(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("expected error, got %v: %s", resp.Type, string(resp.Payload))
	}
	var ep ws.ErrorPayload
	if err := json.Unmarshal(resp.Payload, &ep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ep.Code != ws.ErrorCodeBadRequest {
		t.Errorf("expected BAD_REQUEST, got %q", ep.Code)
	}
}

func TestWsRevealWebhookSecret_RejectsCrossWorkspace(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	// Create automation in workspace A.
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID:       "ws-A",
		Name:              "workspace A automation",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Attempt to reveal using workspace B's id — must return NOT_FOUND, not the secret.
	req, _ := ws.NewRequest("req-1", ws.ActionAutomationWebhookRevealSecret, map[string]any{"id": a.ID, "workspace_id": "ws-B"})
	resp, err := wsRevealWebhookSecret(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("expected error response for cross-workspace reveal, got %v: %s", resp.Type, string(resp.Payload))
	}

	var ep ws.ErrorPayload
	if err := json.Unmarshal(resp.Payload, &ep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ep.Code != ws.ErrorCodeNotFound {
		t.Errorf("expected NOT_FOUND to avoid disclosing existence, got %q", ep.Code)
	}
}
