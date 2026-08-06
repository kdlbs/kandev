package acp

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/logger"
)

// initCaptureAgent records the InitializeRequest it was handed so a test can
// assert on what actually crossed the wire, not just on what a helper returned.
type initCaptureAgent struct {
	burstAgent
	mu  sync.Mutex
	req acp.InitializeRequest
}

func (a *initCaptureAgent) Initialize(_ context.Context, req acp.InitializeRequest) (acp.InitializeResponse, error) {
	a.mu.Lock()
	a.req = req
	a.mu.Unlock()
	return acp.InitializeResponse{ProtocolVersion: req.ProtocolVersion}, nil
}

func (a *initCaptureAgent) captured() acp.InitializeRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.req
}

func newTestAdapterForAgent(agentID string) *Adapter {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "json",
		OutputPath: "stderr",
	})
	return NewAdapter(&shared.Config{AgentID: agentID, WorkDir: "/tmp/test"}, log)
}

// handshakeMeta runs a real Initialize against a fake agent over pipes and
// returns the client capability meta the agent received.
func handshakeMeta(t *testing.T, agentID string) map[string]any {
	t.Helper()

	a := newTestAdapterForAgent(agentID)
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})
	if err := a.Connect(c2aW, a2cR); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fake := &initCaptureAgent{}
	acp.NewAgentSideConnection(fake, a2cW, c2aR)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	drainEvents(a)

	return fake.captured().ClientCapabilities.Meta
}

// Cursor decides its model picker mode from this one key, and it decides once
// per session. Without it a Cursor session is locked to the exploded
// fast=true model rows and cannot run at the regular tier.
func TestInitialize_AdvertisesParameterizedModelPickerToCursor(t *testing.T) {
	meta := handshakeMeta(t, cursorAgentID)

	if got := meta[parameterizedModelPickerMetaKey]; got != true {
		t.Fatalf("cursor handshake meta[%q] = %v, want true (meta: %v)",
			parameterizedModelPickerMetaKey, got, meta)
	}
	if got := meta["terminal_output"]; got != true {
		t.Errorf("cursor handshake dropped terminal_output: meta = %v", meta)
	}
}

// The opt-in is Cursor-specific: no other agent's handshake may change shape,
// since an unrecognized meta key is at best ignored and at worst a behavior
// change we did not ask for.
func TestInitialize_LeavesOtherAgentsHandshakeUnchanged(t *testing.T) {
	for _, agentID := range []string{"claude-acp", codexAgentID, grokAgentID, "opencode-acp"} {
		t.Run(agentID, func(t *testing.T) {
			meta := handshakeMeta(t, agentID)

			if _, ok := meta[parameterizedModelPickerMetaKey]; ok {
				t.Errorf("%s handshake carries %q; it is Cursor-only (meta: %v)",
					agentID, parameterizedModelPickerMetaKey, meta)
			}
			if got := meta["terminal_output"]; got != true {
				t.Errorf("%s handshake meta = %v, want terminal_output true", agentID, meta)
			}
		})
	}
}

func TestClientCapabilitiesForAgent(t *testing.T) {
	cursor := clientCapabilitiesForAgent(cursorAgentID)
	if cursor.Meta[parameterizedModelPickerMetaKey] != true {
		t.Errorf("cursor meta = %v, want %q true", cursor.Meta, parameterizedModelPickerMetaKey)
	}

	other := clientCapabilitiesForAgent("claude-acp")
	if _, ok := other.Meta[parameterizedModelPickerMetaKey]; ok {
		t.Errorf("non-cursor meta = %v, want no %q", other.Meta, parameterizedModelPickerMetaKey)
	}

	// Distinct maps per call: a shared map would let one agent's session leak
	// the key into another's handshake.
	if &cursor.Meta == &other.Meta {
		t.Error("clientCapabilitiesForAgent returned a shared meta map")
	}
}
