package acp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestGrokSessionConfig_BecomesModelAndReasoningSelect(t *testing.T) {
	models := []modelInfo{{
		ModelId: "grok-4.5",
		Name:    "Grok 4.5",
		Meta: map[string]any{
			"supportsReasoningEffort": true,
			"totalContextTokens":      float64(500_000),
			"reasoningEffort":         "high",
			"reasoningEfforts":        []any{"high", "medium", "low"},
		},
	}}
	// No typed ACP configOptions / no extractable meta options → build from catalog.
	opts := grokSessionConfigOptions(nil, nil, models, "grok-4.5")
	if len(opts) != 2 {
		t.Fatalf("got %d options, want 2 (model + reasoning_effort)", len(opts))
	}

	var model, effort *streams.ConfigOption
	for i := range opts {
		switch opts[i].ID {
		case configOptionIDModel:
			model = &opts[i]
		case configOptionIDReasoningEffort:
			effort = &opts[i]
		}
	}
	if model == nil || model.CurrentValue != "grok-4.5" {
		t.Fatalf("model option = %#v", model)
	}
	if effort == nil {
		t.Fatal("missing reasoning_effort option")
	}
	if effort.Category != configOptionCategoryThoughtLevel {
		t.Fatalf("effort category = %q, want %q (frontend filters mode)", effort.Category, configOptionCategoryThoughtLevel)
	}
	if effort.CurrentValue != "high" {
		t.Fatalf("effort current = %q, want high", effort.CurrentValue)
	}
}

func TestGrokSessionConfig_UnsupportedModelHidesEffort(t *testing.T) {
	models := []modelInfo{{
		ModelId: "grok-build",
		Name:    "Grok Build",
		Meta:    map[string]any{"totalContextTokens": float64(500_000)},
	}}
	opts := grokSessionConfigOptions(nil, nil, models, "grok-build")
	for _, o := range opts {
		if o.ID == configOptionIDReasoningEffort {
			t.Fatalf("reasoning_effort must be hidden for unsupported model; got %#v", o)
		}
	}
}

type fakeGrokSetModelConn struct {
	reqs       []acp.UnstableSetSessionModelRequest
	configReqs []acp.SetSessionConfigOptionRequest
	err        error
}

func (f *fakeGrokSetModelConn) SetSessionConfigOption(
	_ context.Context,
	req acp.SetSessionConfigOptionRequest,
) (acp.SetSessionConfigOptionResponse, error) {
	f.configReqs = append(f.configReqs, req)
	return acp.SetSessionConfigOptionResponse{}, f.err
}

func (f *fakeGrokSetModelConn) UnstableSetSessionModel(_ context.Context, req acp.UnstableSetSessionModelRequest) (acp.UnstableSetSessionModelResponse, error) {
	f.reqs = append(f.reqs, req)
	return acp.UnstableSetSessionModelResponse{}, f.err
}

func grokAdapterWithModels(t *testing.T, models []modelInfo, config []streams.ConfigOption) *Adapter {
	t.Helper()
	a := newTestAdapter()
	a.agentID = grokAgentID
	a.driver = newGrokACPDriver()
	a.sessionID = "sess-grok"
	a.availableModels = models
	a.availableConfigOptions = config
	return a
}

func TestSetGrokModel_UsesSessionSetModel(t *testing.T) {
	models := []modelInfo{
		{ModelId: "grok-build", Name: "Grok Build"},
		{ModelId: "grok-4.5", Name: "Grok 4.5", Meta: map[string]any{
			"supportsReasoningEffort": true,
			"reasoningEfforts":        []any{"high", "medium", "low"},
		}},
	}
	config := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", CurrentValue: "grok-build",
			Options: []streams.ConfigOptionValue{
				{Value: "grok-build", Name: "Grok Build"},
				{Value: "grok-4.5", Name: "Grok 4.5"},
			}},
		{Type: "select", ID: configOptionIDReasoningEffort, Category: configOptionCategoryThoughtLevel,
			CurrentValue: "high", Options: []streams.ConfigOptionValue{
				{Value: "low", Name: "Low"}, {Value: "medium", Name: "Medium"}, {Value: "high", Name: "High"},
			}},
	}
	a := grokAdapterWithModels(t, models, config)
	conn := &fakeGrokSetModelConn{}

	driver := a.driver.(*grokACPDriver)
	if err := driver.setModel(context.Background(), a, conn, driverConfigChange{
		sessionID: "sess-grok",
		value:     "grok-4.5",
		models:    models,
		config:    config,
	}); err != nil {
		t.Fatalf("setModel: %v", err)
	}
	if len(conn.reqs) != 1 {
		t.Fatalf("RPC count = %d, want 1", len(conn.reqs))
	}
	if conn.reqs[0].ModelId != "grok-4.5" {
		t.Fatalf("modelId = %q, want grok-4.5", conn.reqs[0].ModelId)
	}
	// Prior effort still valid for target model → carried in meta.
	if conn.reqs[0].Meta[grokReasoningEffortMetaKey] != "high" {
		t.Fatalf("meta.reasoningEffort = %v, want high", conn.reqs[0].Meta[grokReasoningEffortMetaKey])
	}
}

func TestSetGrokReasoningEffort_UsesNormalizedModelConfig(t *testing.T) {
	models := []modelInfo{{
		ModelId: "grok-4.5",
		Meta: map[string]any{
			"supportsReasoningEffort": true,
			"reasoningEfforts":        []any{"high", "medium", "low"},
		},
	}}
	config := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", CurrentValue: "grok-4.5"},
		{Type: "select", ID: configOptionIDReasoningEffort, Category: configOptionCategoryThoughtLevel,
			CurrentValue: "medium", Options: []streams.ConfigOptionValue{
				{Value: "low", Name: "Low"}, {Value: "medium", Name: "Medium"}, {Value: "high", Name: "High"},
			}},
	}
	a := grokAdapterWithModels(t, models, config)
	conn := &fakeGrokSetModelConn{}

	driver := a.driver.(*grokACPDriver)
	if err := driver.setReasoningEffort(context.Background(), a, conn, driverConfigChange{
		sessionID: "sess-grok",
		value:     "high",
		models:    models,
		config:    config,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.reqs) != 1 {
		t.Fatalf("RPC count = %d, want 1", len(conn.reqs))
	}
	req := conn.reqs[0]
	if req.ModelId != "grok-4.5" {
		t.Fatalf("modelId = %q, want current model preserved", req.ModelId)
	}
	if req.Meta[grokReasoningEffortMetaKey] != "high" {
		t.Fatalf("meta.reasoningEffort = %v, want high", req.Meta[grokReasoningEffortMetaKey])
	}
}

func TestSetGrokModel_IncompatibleHarnessAsksForNewSession(t *testing.T) {
	models := []modelInfo{
		{ModelId: "grok-build", Meta: map[string]any{"agentType": "grok-build-plan"}},
		{ModelId: "grok-composer-2.5-fast", Name: "Composer 2.5", Meta: map[string]any{"agentType": "cursor"}},
	}
	config := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", CurrentValue: "grok-build"},
	}
	a := grokAdapterWithModels(t, models, config)
	conn := &fakeGrokSetModelConn{
		err: &acp.RequestError{
			Code:    -32600,
			Message: "Cannot switch model harness after first turn",
			Data:    map[string]any{"code": "MODEL_SWITCH_INCOMPATIBLE_AGENT"},
		},
	}

	driver := a.driver.(*grokACPDriver)
	err := driver.setModel(context.Background(), a, conn, driverConfigChange{
		sessionID: "sess-grok",
		value:     "grok-composer-2.5-fast",
		models:    models,
		config:    config,
	})
	if err == nil || !strings.Contains(err.Error(), "Start a new session") {
		t.Fatalf("setModel error = %v, want actionable new-session instruction", err)
	}
	if len(conn.reqs) != 1 {
		t.Fatalf("set_model RPC count = %d, want 1", len(conn.reqs))
	}
	if a.sessionID != "sess-grok" {
		t.Fatalf("session ID = %q, want unchanged sess-grok", a.sessionID)
	}
	if currentModelFromConfig(a.availableConfigOptions) != "grok-build" {
		t.Fatalf("current model config changed after failed switch: %#v", a.availableConfigOptions)
	}
}

func TestSetModelWithConn_GrokDriverAsksForNewSession(t *testing.T) {
	models := []modelInfo{
		{ModelId: "grok-build", Meta: map[string]any{"agentType": "grok-build-plan"}},
		{ModelId: "grok-composer-2.5-fast", Meta: map[string]any{"agentType": "cursor"}},
	}
	config := []streams.ConfigOption{{
		Type: "select", ID: "model", Category: "model", CurrentValue: "grok-build",
	}}
	adapter := grokAdapterWithModels(t, models, config)
	conn := &fakeGrokSetModelConn{
		err: &acp.RequestError{
			Code:    -32600,
			Message: "Cannot switch model harness after first turn",
			Data:    map[string]any{"code": "MODEL_SWITCH_INCOMPATIBLE_AGENT"},
		},
	}

	err := adapter.setModelWithConn(
		context.Background(),
		conn,
		"sess-grok",
		"grok-composer-2.5-fast",
		models,
		config,
	)
	if err == nil || !strings.Contains(err.Error(), "Start a new session") {
		t.Fatalf("SetModel error = %v, want actionable new-session instruction", err)
	}
	if len(conn.configReqs) != 0 {
		t.Fatalf("session/set_config_option calls = %d, want 0", len(conn.configReqs))
	}
	if len(conn.reqs) != 1 {
		t.Fatalf("session/set_model calls = %d, want 1", len(conn.reqs))
	}
	if adapter.sessionID != "sess-grok" {
		t.Fatalf("session ID = %q, want unchanged sess-grok", adapter.sessionID)
	}
	if currentModelFromConfig(adapter.availableConfigOptions) != "grok-build" {
		t.Fatalf("current model config changed after failed switch: %#v", adapter.availableConfigOptions)
	}
}

func TestGrokContextFromNotificationMeta(t *testing.T) {
	models := []modelInfo{{
		ModelId: "grok-4.5",
		Meta:    map[string]any{"totalContextTokens": float64(500_000)},
	}}
	a := grokAdapterWithModels(t, models, []streams.ConfigOption{{
		Type: "select", ID: configOptionIDModel, Category: configOptionIDModel, CurrentValue: "grok-4.5",
	}})

	a.handleACPUpdate(acp.SessionNotification{
		SessionId: "sess-grok",
		Meta:      map[string]any{"totalTokens": float64(42_000)},
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
				Content: acp.TextBlock("hi"),
			},
		},
	})

	events := drainEvents(a)
	var ctx *AgentEvent
	for i := range events {
		if events[i].Type == streams.EventTypeContextWindow {
			ctx = &events[i]
		}
	}
	if ctx == nil {
		t.Fatalf("expected context_window event; got %#v", events)
	}
	if ctx.ContextWindowSize != 500_000 || ctx.ContextWindowUsed != 42_000 {
		t.Fatalf("size/used = %d/%d, want 500000/42000", ctx.ContextWindowSize, ctx.ContextWindowUsed)
	}

	a.handleACPUpdate(acp.SessionNotification{
		SessionId: "sess-grok",
		Meta:      map[string]any{"totalTokens": float64(42_000)},
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("again")},
		},
	})
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeContextWindow {
			t.Fatal("unchanged totalTokens must not emit duplicate context_window")
		}
	}

	// Prompt completion consumes the shared ACP usage tracker. Driver-owned
	// context dedupe must remain intact across that unrelated reset.
	a.consumeUsageDelta("sess-grok")
	a.handleACPUpdate(acp.SessionNotification{
		SessionId: "sess-grok",
		Meta:      map[string]any{"totalTokens": float64(42_000)},
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("next turn")},
		},
	})
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeContextWindow {
			t.Fatal("usage tracker reset must not duplicate unchanged driver context")
		}
	}

	// Compaction legitimately lowers context usage and must still emit.
	a.handleACPUpdate(acp.SessionNotification{
		SessionId: "sess-grok",
		Meta:      map[string]any{"totalTokens": float64(40_000)},
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("compacted")},
		},
	})
	compactedUsed := int64(0)
	compactedFound := false
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeContextWindow {
			compactedUsed = event.ContextWindowUsed
			compactedFound = true
		}
	}
	if !compactedFound || compactedUsed != 40_000 {
		t.Fatalf("compaction context used = %d (found=%t), want 40000", compactedUsed, compactedFound)
	}

	// A delayed notification from a replaced session cannot update current UI.
	a.handleACPUpdate(acp.SessionNotification{
		SessionId: "stale-session",
		Meta:      map[string]any{"totalTokens": float64(99_000)},
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("stale")},
		},
	})
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeContextWindow {
			t.Fatal("stale session must not emit driver context")
		}
	}
}

func TestGrokUserMessageEchoIsSuppressedWithoutDroppingContext(t *testing.T) {
	models := []modelInfo{{
		ModelId: "grok-current",
		Meta:    map[string]any{"totalContextTokens": float64(500_000)},
	}}
	a := grokAdapterWithModels(t, models, []streams.ConfigOption{{
		Type: "select", ID: configOptionIDModel, Category: configOptionIDModel, CurrentValue: "grok-current",
	}})

	a.handleACPUpdate(acp.SessionNotification{
		SessionId: "sess-grok",
		Meta:      map[string]any{"totalTokens": float64(42_000)},
		Update: acp.SessionUpdate{
			UserMessageChunk: &acp.SessionUpdateUserMessageChunk{
				Content: acp.TextBlock("echoed prompt"),
			},
		},
	})

	contextFound := false
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeMessageChunk {
			t.Fatalf("Grok user echo must not become a message event: %#v", event)
		}
		if event.Type == streams.EventTypeContextWindow {
			contextFound = true
		}
	}
	if !contextFound {
		t.Fatal("suppressed user echo must still contribute context metadata")
	}
}

func TestGrokDriver_NormalizesPrivateReasoningTokens(t *testing.T) {
	response := &acp.PromptResponse{Meta: map[string]any{
		"usage": map[string]any{
			"inputTokens":     float64(5),
			"outputTokens":    float64(3),
			"totalTokens":     float64(8),
			"reasoningTokens": float64(2),
		},
	}}
	driver := newGrokACPDriver()
	usage := driver.normalizePromptUsage(extractUsage(response), response.Meta)
	if usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if usage.ThoughtTokens != 2 {
		t.Fatalf("ThoughtTokens = %d, want 2", usage.ThoughtTokens)
	}
}

func TestIsGrokIncompatibleAgentSwitchError(t *testing.T) {
	reqErr := &acp.RequestError{
		Code:    -32600,
		Message: "Cannot switch to model 'composer': it requires agent 'cursor'. Start a new session to use this model.",
		Data:    map[string]any{"code": "MODEL_SWITCH_INCOMPATIBLE_AGENT"},
	}
	if !isGrokIncompatibleAgentSwitchError(reqErr) {
		t.Fatal("expected structured error to match")
	}
	wrapped := formatGrokSetModelError(reqErr)
	if !isGrokIncompatibleAgentSwitchError(wrapped) {
		t.Fatal("expected wrapped error to still match")
	}
	if !strings.Contains(wrapped.Error(), "MODEL_SWITCH_INCOMPATIBLE_AGENT") {
		t.Fatalf("wrapped should retain code tag, got %q", wrapped.Error())
	}
	if isGrokIncompatibleAgentSwitchError(errors.New("method not found")) {
		t.Fatal("unrelated error must not match")
	}
}
