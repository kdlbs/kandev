package utility

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"go.uber.org/zap"
)

// probeCaptureAgent is the agent side of the probe handshake. It records the
// InitializeRequest and then answers session/new with an empty session so the
// probe can finish.
type probeCaptureAgent struct {
	mu  sync.Mutex
	req acp.InitializeRequest
}

func (a *probeCaptureAgent) Initialize(_ context.Context, req acp.InitializeRequest) (acp.InitializeResponse, error) {
	a.mu.Lock()
	a.req = req
	a.mu.Unlock()
	return acp.InitializeResponse{ProtocolVersion: req.ProtocolVersion}, nil
}

func (a *probeCaptureAgent) captured() acp.InitializeRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.req
}

func (*probeCaptureAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}
func (*probeCaptureAgent) Cancel(context.Context, acp.CancelNotification) error { return nil }
func (*probeCaptureAgent) CloseSession(context.Context, acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}
func (*probeCaptureAgent) DeleteSession(context.Context, acp.DeleteSessionRequest) (acp.DeleteSessionResponse, error) {
	return acp.DeleteSessionResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionDelete)
}
func (*probeCaptureAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}
func (*probeCaptureAgent) Logout(context.Context, acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, nil
}
func (*probeCaptureAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{SessionId: acp.SessionId("probe-session")}, nil
}
func (*probeCaptureAgent) Prompt(context.Context, acp.PromptRequest) (acp.PromptResponse, error) {
	return acp.PromptResponse{}, nil
}
func (*probeCaptureAgent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}
func (*probeCaptureAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}
func (*probeCaptureAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

func probeHandshakeMeta(t *testing.T, agentID string) map[string]any {
	t.Helper()

	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeCaptureAgent{}
	acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := e.probeACPSession(ctx, c2aW, a2cR, t.TempDir(), agentID); err != nil {
		t.Fatalf("probeACPSession(%q): %v", agentID, err)
	}
	return fake.captured().ClientCapabilities.Meta
}

// The probe and the live session adapter are SEPARATE handshakes. When only the
// adapter opted in, the agent-models surface advertised the exploded fast=true
// rows while sessions ran on the bare ids — so the UI offered a model list no
// session used, and could not select the id a stored profile referenced.
func TestProbeACPSession_AdvertisesParameterizedModelPickerToCursor(t *testing.T) {
	meta := probeHandshakeMeta(t, acpcompat.CursorAgentID)

	if meta[acpcompat.ParameterizedModelPickerMetaKey] != true {
		t.Fatalf("probe meta[%q] = %v, want true (meta: %v)",
			acpcompat.ParameterizedModelPickerMetaKey,
			meta[acpcompat.ParameterizedModelPickerMetaKey], meta)
	}
}

// The one-shot inference path is the THIRD handshake, and it applies a
// caller-supplied model — so opting out of the picker here would reject the
// very model ids the session path and the probe hand out.
func TestExecuteACPSession_AdvertisesParameterizedModelPickerToCursor(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeCaptureAgent{}
	acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := e.executeACPSession(ctx, c2aW, a2cR, t.TempDir(),
		acpcompat.CursorAgentID, "hello", "", nil, "", nil); err != nil {
		t.Fatalf("executeACPSession: %v", err)
	}

	meta := fake.captured().ClientCapabilities.Meta
	if meta[acpcompat.ParameterizedModelPickerMetaKey] != true {
		t.Fatalf("inference meta[%q] = %v, want true (meta: %v)",
			acpcompat.ParameterizedModelPickerMetaKey,
			meta[acpcompat.ParameterizedModelPickerMetaKey], meta)
	}
}

func TestProbeACPSession_LeavesOtherAgentsHandshakeUnchanged(t *testing.T) {
	for _, agentID := range []string{"claude-acp", "codex-acp", "opencode-acp"} {
		t.Run(agentID, func(t *testing.T) {
			meta := probeHandshakeMeta(t, agentID)
			if _, ok := meta[acpcompat.ParameterizedModelPickerMetaKey]; ok {
				t.Errorf("%s probe carries %q; it is Cursor-only (meta: %v)",
					agentID, acpcompat.ParameterizedModelPickerMetaKey, meta)
			}
		})
	}
}
