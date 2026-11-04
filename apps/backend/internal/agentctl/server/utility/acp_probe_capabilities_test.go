package utility

import (
	"context"
	"errors"
	"io"
	"strings"
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

	if _, err := e.probeACPSessionWithContext(ctx, c2aW, a2cR, t.TempDir(), agentID, "", "", nil, nil); err != nil {
		t.Fatalf("probeACPSessionWithContext(%q): %v", agentID, err)
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
		acpcompat.CursorAgentID, "hello", "", nil, "", nil, nil, nil); err != nil {
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

func TestProbeACPSessionWithContextRejectsProviderWithoutModelSelection(t *testing.T) {
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

	_, err := e.probeACPSessionWithContext(
		ctx, c2aW, a2cR, t.TempDir(), "provider-without-models", "model", "", nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "provider does not support model selection") {
		t.Fatalf("error = %v, want unsupported model selection", err)
	}
}

// TestProbeACPSessionWithContextErrorsWhenConfigOptionYieldsNoSnapshot pins that
// when an agent advertises a typed model config option (so the probe applies the
// model via session/set_config_option) but returns neither inline options nor a
// follow-up config-update notification, the probe fails. Keeping the pre-switch
// session/new snapshot would report the previous model's options as the resolved
// configuration for the newly selected model, so this must stay an error. The
// empty-resolution relaxation is scoped to the legacy session/set_model path
// (auggie); see TestApplyProbeModel_LegacyNoConfigOptionsReturnsEmpty.
func TestProbeACPSessionWithContextErrorsWhenConfigOptionYieldsNoSnapshot(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeNoConfigResponseAgent{probeCaptureAgent: &probeCaptureAgent{}}
	acp.NewAgentSideConnection(fake, a2cW, c2aR)
	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := e.probeACPSessionWithContext(
		ctx, c2aW, a2cR, t.TempDir(), "provider-without-config-response", "model", "", nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "returned no configuration options") {
		t.Fatalf("error = %v, want missing configuration snapshot", err)
	}
}

// probeAdvertisedModesAgent advertises build/plan modes on session/new through
// either ACP mode carrier. Its SetSessionMode records every call so tests can
// prove which modes the probe attempted, and can be told to reject the call.
// With emptyModes, the legacy mode state is still present but offers none.
type probeAdvertisedModesAgent struct {
	*probeCaptureAgent
	conn        *acp.AgentSideConnection
	mu          sync.Mutex
	modeCalls   []string
	failSetMode bool
	emptyModes  bool
	configOnly  bool
	configOpts  []acp.SessionConfigOption
}

func (a *probeAdvertisedModesAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	if a.configOnly {
		return acp.NewSessionResponse{
			SessionId:     acp.SessionId("probe-session"),
			ConfigOptions: a.configOpts,
		}, nil
	}
	state := &acp.SessionModeState{
		CurrentModeId: acp.SessionModeId("build"),
		AvailableModes: []acp.SessionMode{
			{Id: acp.SessionModeId("build"), Name: "Build"},
			{Id: acp.SessionModeId("plan"), Name: "Plan"},
		},
	}
	if a.emptyModes {
		state.AvailableModes = nil
	}
	return acp.NewSessionResponse{
		SessionId: acp.SessionId("probe-session"),
		Modes:     state,
	}, nil
}

func (a *probeAdvertisedModesAgent) SetSessionMode(
	ctx context.Context,
	request acp.SetSessionModeRequest,
) (acp.SetSessionModeResponse, error) {
	a.mu.Lock()
	a.modeCalls = append(a.modeCalls, string(request.ModeId))
	a.mu.Unlock()
	if a.failSetMode {
		return acp.SetSessionModeResponse{}, errors.New("mode rejected")
	}
	return acp.SetSessionModeResponse{}, a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: request.SessionId,
		Update: acp.SessionUpdate{
			ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
				ConfigOptions: probeModelConfigOptions("default-model"),
			},
		},
	})
}

func (a *probeAdvertisedModesAgent) recordedModeCalls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.modeCalls...)
}

func probeModeConfigOptions(options acp.SessionConfigSelectOptions) []acp.SessionConfigOption {
	modeCategory := acp.SessionConfigOptionCategoryMode
	return []acp.SessionConfigOption{{Select: &acp.SessionConfigOptionSelect{
		Id:           "mode",
		Name:         "Mode",
		Category:     &modeCategory,
		Type:         "select",
		CurrentValue: "build",
		Options:      options,
	}}}
}

// A mode the agent did not advertise (for example an OpenCode agent defined in
// a repository checkout) must not fail the probe: the probe workspace has no
// repository context, and the real task session applies the mode itself.
func TestProbeACPSessionWithContextSkipsUnadvertisedMode(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeAdvertisedModesAgent{probeCaptureAgent: &probeCaptureAgent{}, failSetMode: true}
	fake.conn = acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := e.probeACPSessionWithContext(
		ctx, c2aW, a2cR, t.TempDir(), "opencode-acp", "", "planner", nil, nil,
	)
	if err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v", err)
	}
	if calls := fake.recordedModeCalls(); len(calls) != 0 {
		t.Fatalf("set_mode calls = %v, want none for unadvertised mode", calls)
	}
	if resp.CurrentModeID != "build" {
		t.Fatalf("current mode = %q, want the advertised default", resp.CurrentModeID)
	}
	if len(resp.Modes) != 2 {
		t.Fatalf("modes = %#v, want the two advertised modes", resp.Modes)
	}
}

// Advertised modes are still applied so their option effects are captured.
func TestProbeACPSessionWithContextAppliesAdvertisedMode(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeAdvertisedModesAgent{probeCaptureAgent: &probeCaptureAgent{}}
	fake.conn = acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := e.probeACPSessionWithContext(
		ctx, c2aW, a2cR, t.TempDir(), "opencode-acp", "", "plan", nil, nil,
	); err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v", err)
	}
	if calls := fake.recordedModeCalls(); len(calls) != 1 || calls[0] != "plan" {
		t.Fatalf("set_mode calls = %v, want [plan]", calls)
	}
}

// A present modes state with no available modes is an authoritative empty
// advertisement, not a missing one: every mode counts as unadvertised.
func TestProbeACPSessionWithContextSkipsModeWhenAdvertisementIsEmpty(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeAdvertisedModesAgent{probeCaptureAgent: &probeCaptureAgent{}, failSetMode: true, emptyModes: true}
	fake.conn = acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := e.probeACPSessionWithContext(
		ctx, c2aW, a2cR, t.TempDir(), "opencode-acp", "", "reviewer", nil, nil,
	)
	if err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v", err)
	}
	if calls := fake.recordedModeCalls(); len(calls) != 0 {
		t.Fatalf("set_mode calls = %v, want none against an empty advertisement", calls)
	}
	if resp.CurrentModeID != "build" {
		t.Fatalf("current mode = %q, want the advertised default", resp.CurrentModeID)
	}
}

func TestProbeACPSessionWithContextUsesConfigOptionsModeAdvertisement(t *testing.T) {
	groupedModes := acp.SessionConfigSelectOptionsGrouped{
		{Group: "work", Name: "Work", Options: []acp.SessionConfigSelectOption{
			{Name: "Build", Value: "build"},
		}},
		{Group: "planning", Name: "Planning", Options: []acp.SessionConfigSelectOption{
			{Name: "Plan", Value: "plan"},
		}},
	}
	emptyModes := acp.SessionConfigSelectOptionsUngrouped{}

	tests := []struct {
		name          string
		mode          string
		configOptions []acp.SessionConfigOption
		failSetMode   bool
		wantCalls     []string
		wantErr       bool
	}{
		{
			name: "grouped mode option authorizes application",
			mode: "plan",
			configOptions: probeModeConfigOptions(acp.SessionConfigSelectOptions{
				Grouped: &groupedModes,
			}),
			wantCalls: []string{"plan"},
		},
		{
			name: "grouped mode option skips an unadvertised mode",
			mode: "reviewer",
			configOptions: probeModeConfigOptions(acp.SessionConfigSelectOptions{
				Grouped: &groupedModes,
			}),
			failSetMode: true,
		},
		{
			name: "empty mode option skips an unadvertised mode",
			mode: "reviewer",
			configOptions: probeModeConfigOptions(acp.SessionConfigSelectOptions{
				Ungrouped: &emptyModes,
			}),
			failSetMode: true,
		},
		{
			name:          "unrelated config options do not advertise modes",
			mode:          "planner",
			configOptions: probeModelConfigOptions("default-model"),
			failSetMode:   true,
			wantCalls:     []string{"planner"},
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c2aR, c2aW := io.Pipe()
			a2cR, a2cW := io.Pipe()
			t.Cleanup(func() {
				_ = c2aR.Close()
				_ = c2aW.Close()
				_ = a2cR.Close()
				_ = a2cW.Close()
			})

			fake := &probeAdvertisedModesAgent{
				probeCaptureAgent: &probeCaptureAgent{},
				failSetMode:       tt.failSetMode,
				configOnly:        true,
				configOpts:        tt.configOptions,
			}
			fake.conn = acp.NewAgentSideConnection(fake, a2cW, c2aR)

			e := NewACPInferenceExecutor(zap.NewNop())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := e.probeACPSessionWithContext(
				ctx, c2aW, a2cR, t.TempDir(), "opencode-acp", "", tt.mode, nil, nil,
			)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "session/set_mode") {
					t.Fatalf("probe error = %v, want session/set_mode failure", err)
				}
			} else if err != nil {
				t.Fatalf("probeACPSessionWithContext(): %v", err)
			}

			if got := fake.recordedModeCalls(); strings.Join(got, ",") != strings.Join(tt.wantCalls, ",") {
				t.Fatalf("set_mode calls = %v, want %v", got, tt.wantCalls)
			}
		})
	}
}

// probeModeRejectAgent advertises no modes and rejects every SetSessionMode.
type probeModeRejectAgent struct {
	*probeCaptureAgent
	mu        sync.Mutex
	modeCalls int
}

func (a *probeModeRejectAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	a.mu.Lock()
	a.modeCalls++
	a.mu.Unlock()
	return acp.SetSessionModeResponse{}, errors.New("mode rejected")
}

// Without a mode advertisement the probe cannot tell a repository-local mode
// from a typo, so it keeps attempting the mode and surfaces the failure.
func TestProbeACPSessionWithContextStillAppliesModeWithoutModeAdvertisement(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeModeRejectAgent{probeCaptureAgent: &probeCaptureAgent{}}
	acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := e.probeACPSessionWithContext(
		ctx, c2aW, a2cR, t.TempDir(), "opencode-acp", "", "planner", nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "session/set_mode") {
		t.Fatalf("error = %v, want session/set_mode failure", err)
	}
}

func TestProbeACPSessionWithModel_UsesReturnedConfigOptions(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeModelConfigAgent{probeCaptureAgent: &probeCaptureAgent{}}
	acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := e.probeACPSessionWithContext(
		ctx, c2aW, a2cR, t.TempDir(), "opencode-acp", "model-with-effort", "", nil, nil,
	)
	if err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v", err)
	}

	var found bool
	for _, option := range resp.ConfigOptions {
		if option.ID == "reasoning_effort" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("config options = %#v, want reasoning_effort", resp.ConfigOptions)
	}
}

func TestProbeACPSessionWithModel_UsesConfigUpdateNotification(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeNotificationConfigAgent{probeCaptureAgent: &probeCaptureAgent{}}
	fake.conn = acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := e.probeACPSessionWithContext(
		ctx, c2aW, a2cR, t.TempDir(), "opencode-acp", "legacy-model-with-effort", "", nil, nil,
	)
	if err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v (calls: %v)", err, fake.recordedCalls())
	}

	var found bool
	for _, option := range resp.ConfigOptions {
		if option.ID == "reasoning_effort" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("config options = %#v, want reasoning_effort", resp.ConfigOptions)
	}
}

func TestProbeACPSessionWithContextReturnsSnapshotAfterRequestedOptions(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeRequestedConfigAgent{probeCaptureAgent: &probeCaptureAgent{}}
	acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := e.probeACPSessionWithContext(
		ctx,
		c2aW,
		a2cR,
		t.TempDir(),
		"opencode-acp",
		"model-with-effort",
		"build",
		map[string]string{"reasoning_effort": "low"},
		nil,
	)
	if err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v", err)
	}

	for _, option := range resp.ConfigOptions {
		if option.ID == "reasoning_effort" {
			if option.CurrentValue != "low" {
				t.Fatalf("reasoning_effort current value = %q, want low", option.CurrentValue)
			}
			return
		}
	}
	t.Fatalf("config options = %#v, want reasoning_effort", resp.ConfigOptions)
}

func TestProbeACPSessionWithContextAppliesModeBeforeModelAndOptions(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeModeConfigAgent{probeCaptureAgent: &probeCaptureAgent{}}
	fake.conn = acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := e.probeACPSessionWithContext(
		ctx,
		c2aW,
		a2cR,
		t.TempDir(),
		"opencode-acp",
		"model-with-effort",
		"smart",
		map[string]string{"reasoning_effort": "low"},
		nil,
	)
	if err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v (calls: %v)", err, fake.recordedCalls())
	}
	if got, want := strings.Join(fake.recordedCalls(), ","), "mode,model,config"; got != want {
		t.Fatalf("ACP mutation order = %q, want %q", got, want)
	}

	for _, option := range resp.ConfigOptions {
		if option.ID == "reasoning_effort" {
			if option.CurrentValue != "low" {
				t.Fatalf("reasoning_effort current value = %q, want low", option.CurrentValue)
			}
			return
		}
	}
	t.Fatalf("config options = %#v, want reasoning_effort", resp.ConfigOptions)
}

func TestProbeACPSessionWithContextRequiresSnapshotAfterFinalConfigMutation(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	fake := &probeTwoConfigAgent{probeCaptureAgent: &probeCaptureAgent{}}
	acp.NewAgentSideConnection(fake, a2cW, c2aR)

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := e.probeACPSessionWithContext(
		ctx,
		c2aW,
		a2cR,
		t.TempDir(),
		"opencode-acp",
		"model-with-options",
		"",
		map[string]string{"first": "one", "second": "two"},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "model config selection returned no configuration options") {
		t.Fatalf("error = %v, want missing final configuration snapshot", err)
	}
}

func TestFilterRequestedConfigOptionsUsesResolvedSnapshot(t *testing.T) {
	category := acp.SessionConfigOptionCategoryThoughtLevel
	got := filterRequestedConfigOptions(
		map[string]string{"effort": "high", "reasoning_effort": "max"},
		[]acp.SessionConfigOption{{Select: &acp.SessionConfigOptionSelect{
			Id:       "reasoning_effort",
			Category: &category,
		}}},
	)
	if len(got) != 1 || got["reasoning_effort"] != "max" {
		t.Fatalf("filtered options = %#v, want only reasoning_effort=max", got)
	}
}

func TestFilterRequestedConfigOptionsDropsValuesMissingFromResolvedSnapshot(t *testing.T) {
	category := acp.SessionConfigOptionCategoryThoughtLevel
	got := filterRequestedConfigOptions(
		map[string]string{"effort": "medium"},
		[]acp.SessionConfigOption{{Select: &acp.SessionConfigOptionSelect{
			Id:       "effort",
			Category: &category,
			Options: acp.SessionConfigSelectOptions{
				Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
					{Value: "low"},
					{Value: "high"},
					{Value: "max"},
				},
			},
		}}},
	)
	if len(got) != 0 {
		t.Fatalf("filtered options = %#v, want stale effort value removed", got)
	}
}

type probeModelConfigAgent struct {
	*probeCaptureAgent
}

type probeNoConfigResponseAgent struct {
	*probeCaptureAgent
}

func (*probeNoConfigResponseAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{
		SessionId:     acp.SessionId("probe-session"),
		ConfigOptions: probeModelConfigOptions("default-model"),
	}, nil
}

func (*probeNoConfigResponseAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (*probeModelConfigAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{
		SessionId:     acp.SessionId("probe-session"),
		ConfigOptions: probeModelConfigOptions("default-model"),
	}, nil
}

func (*probeModelConfigAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{
		ConfigOptions: probeModelConfigOptions("model-with-effort"),
	}, nil
}

type probeNotificationConfigAgent struct {
	*probeCaptureAgent
	conn  *acp.AgentSideConnection
	mu    sync.Mutex
	calls []string
}

type probeRequestedConfigAgent struct {
	*probeCaptureAgent
}

type probeTwoConfigAgent struct {
	*probeCaptureAgent
}

type probeModeConfigAgent struct {
	*probeCaptureAgent
	conn  *acp.AgentSideConnection
	mu    sync.Mutex
	calls []string
}

func (a *probeNotificationConfigAgent) record(call string) {
	a.mu.Lock()
	a.calls = append(a.calls, call)
	a.mu.Unlock()
}

func (a *probeNotificationConfigAgent) recordedCalls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.calls...)
}

func (a *probeModeConfigAgent) record(call string) {
	a.mu.Lock()
	a.calls = append(a.calls, call)
	a.mu.Unlock()
}

func (a *probeModeConfigAgent) recordedCalls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.calls...)
}

func (*probeModeConfigAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{
		SessionId:     acp.SessionId("probe-session"),
		ConfigOptions: probeModelConfigOptions("default-model"),
	}, nil
}

func (a *probeModeConfigAgent) SetSessionMode(
	ctx context.Context,
	request acp.SetSessionModeRequest,
) (acp.SetSessionModeResponse, error) {
	a.record("mode")
	options := probeModelConfigOptions("default-model")
	options[1].Select.Options = acp.SessionConfigSelectOptions{
		Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
			{Name: "Low", Value: "low"},
			{Name: "High", Value: "high"},
		},
	}
	return acp.SetSessionModeResponse{}, a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: request.SessionId,
		Update: acp.SessionUpdate{
			ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{ConfigOptions: options},
		},
	})
}

func (a *probeModeConfigAgent) SetSessionConfigOption(
	_ context.Context,
	request acp.SetSessionConfigOptionRequest,
) (acp.SetSessionConfigOptionResponse, error) {
	if request.ValueId.ConfigId == "model" {
		a.record("model")
	} else {
		a.record("config")
	}
	options := probeModelConfigOptions("model-with-effort")
	options[1].Select.Options = acp.SessionConfigSelectOptions{
		Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
			{Name: "Low", Value: "low"},
			{Name: "High", Value: "high"},
		},
	}
	if request.ValueId.ConfigId == "reasoning_effort" {
		options[1].Select.CurrentValue = request.ValueId.Value
	}
	return acp.SetSessionConfigOptionResponse{ConfigOptions: options}, nil
}

func (*probeRequestedConfigAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{
		SessionId:     acp.SessionId("probe-session"),
		ConfigOptions: probeModelConfigOptions("default-model"),
	}, nil
}

func (*probeTwoConfigAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{
		SessionId:     acp.SessionId("probe-session"),
		ConfigOptions: probeTwoConfigOptions("default-model"),
	}, nil
}

func (*probeTwoConfigAgent) SetSessionConfigOption(
	_ context.Context,
	request acp.SetSessionConfigOptionRequest,
) (acp.SetSessionConfigOptionResponse, error) {
	if request.ValueId.ConfigId == "model" {
		return acp.SetSessionConfigOptionResponse{
			ConfigOptions: probeTwoConfigOptions(string(request.ValueId.Value)),
		}, nil
	}
	if request.ValueId.ConfigId == "first" {
		return acp.SetSessionConfigOptionResponse{
			ConfigOptions: probeTwoConfigOptions("first-applied"),
		}, nil
	}
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (*probeRequestedConfigAgent) SetSessionConfigOption(
	_ context.Context,
	request acp.SetSessionConfigOptionRequest,
) (acp.SetSessionConfigOptionResponse, error) {
	options := probeModelConfigOptions("model-with-effort")
	options[1].Select.Options = acp.SessionConfigSelectOptions{
		Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
			{Name: "Low", Value: "low"},
			{Name: "High", Value: "high"},
		},
	}
	if request.ValueId != nil && request.ValueId.ConfigId == "reasoning_effort" {
		options[1].Select.CurrentValue = request.ValueId.Value
	}
	return acp.SetSessionConfigOptionResponse{ConfigOptions: options}, nil
}

func (*probeNotificationConfigAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{
		SessionId:     acp.SessionId("probe-session"),
		ConfigOptions: probeModelConfigOptions("default-model"),
	}, nil
}

func (a *probeNotificationConfigAgent) SetSessionConfigOption(ctx context.Context, req acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	a.record("config")
	err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: req.ValueId.SessionId,
		Update: acp.SessionUpdate{
			ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
				ConfigOptions: probeModelConfigOptions(string(req.ValueId.Value)),
			},
		},
	})
	return acp.SetSessionConfigOptionResponse{}, err
}

func probeModelConfigOptions(model string) []acp.SessionConfigOption {
	modelCategory := acp.SessionConfigOptionCategoryModel
	effortCategory := acp.SessionConfigOptionCategory("reasoning_effort")
	return []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{
			Id:           "model",
			Name:         "Model",
			Category:     &modelCategory,
			Type:         "select",
			CurrentValue: acp.SessionConfigValueId(model),
			Options: acp.SessionConfigSelectOptions{
				Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
					{Name: model, Value: acp.SessionConfigValueId(model)},
				},
			},
		}},
		{Select: &acp.SessionConfigOptionSelect{
			Id:           "reasoning_effort",
			Name:         "Reasoning effort",
			Category:     &effortCategory,
			Type:         "select",
			CurrentValue: "high",
			Options: acp.SessionConfigSelectOptions{
				Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
					{Name: "High", Value: "high"},
				},
			},
		}},
	}
}

func probeTwoConfigOptions(model string) []acp.SessionConfigOption {
	modelCategory := acp.SessionConfigOptionCategoryModel
	optionCategory := acp.SessionConfigOptionCategoryThoughtLevel
	return []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{
			Id:           "model",
			Name:         "Model",
			Category:     &modelCategory,
			Type:         "select",
			CurrentValue: acp.SessionConfigValueId(model),
			Options: acp.SessionConfigSelectOptions{
				Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
					{Name: model, Value: acp.SessionConfigValueId(model)},
				},
			},
		}},
		{Select: &acp.SessionConfigOptionSelect{
			Id:           "first",
			Name:         "First option",
			Category:     &optionCategory,
			Type:         "select",
			CurrentValue: "one",
			Options: acp.SessionConfigSelectOptions{
				Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{{Name: "One", Value: "one"}},
			},
		}},
		{Select: &acp.SessionConfigOptionSelect{
			Id:           "second",
			Name:         "Second option",
			Category:     &optionCategory,
			Type:         "select",
			CurrentValue: "two",
			Options: acp.SessionConfigSelectOptions{
				Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{{Name: "Two", Value: "two"}},
			},
		}},
	}
}
