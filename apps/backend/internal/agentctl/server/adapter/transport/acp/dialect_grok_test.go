package acp

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestGrokSessionConfig_MergesDerivedReasoningWithTypedOptions(t *testing.T) {
	modelCategory := acp.SessionConfigOptionCategoryModel
	typed := []streams.ConfigOption{{
		Type:         "select",
		ID:           configOptionIDModel,
		Category:     string(modelCategory),
		Name:         "Model",
		CurrentValue: "grok-4.5",
	}}
	models := []modelInfo{{
		ModelId: "grok-4.5",
		Meta: map[string]any{
			"supportsReasoningEffort": true,
			"reasoningEffort":         "high",
			"reasoningEfforts":        []any{"high", "medium", "low"},
		},
	}}

	opts := grokSessionConfigOptions(typed, nil, models, "grok-4.5")

	if len(opts) != 2 {
		t.Fatalf("got %d options, want typed model + derived reasoning", len(opts))
	}
	if opts[1].ID != configOptionIDReasoningEffort {
		t.Fatalf("second option ID = %q, want %q", opts[1].ID, configOptionIDReasoningEffort)
	}
}

type fakeGrokSetModelConn struct {
	reqs       []acp.UnstableSetSessionModelRequest
	configReqs []acp.SetSessionConfigOptionRequest
	err        error
}

type blockingGrokSetModelConn struct {
	mu           sync.Mutex
	started      chan string
	releaseFirst chan struct{}
	calls        int
}

func (f *blockingGrokSetModelConn) SetSessionConfigOption(
	context.Context,
	acp.SetSessionConfigOptionRequest,
) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (f *blockingGrokSetModelConn) UnstableSetSessionModel(
	_ context.Context,
	req acp.UnstableSetSessionModelRequest,
) (acp.UnstableSetSessionModelResponse, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()
	f.started <- req.ModelId
	if call == 1 {
		<-f.releaseFirst
	}
	return acp.UnstableSetSessionModelResponse{}, nil
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
	a.dialect = newGrokACPDialect()
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

	if err := a.setModelWithConn(context.Background(), conn, "sess-grok", "grok-4.5", models, config); err != nil {
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
	rpc, err := grokConfigChangeRequest(dialectConfigChange{
		sessionID: "sess-grok",
		configID:  configOptionIDReasoningEffort,
		value:     "high",
		models:    models,
		config:    config,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rpc == nil {
		t.Fatal("expected Grok session/set_model request")
	}
	req := rpc.request
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

	err := a.setModelWithConn(
		context.Background(), conn, "sess-grok", "grok-composer-2.5-fast", models, config,
	)
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

func TestSetModelWithConn_SerializesConcurrentGrokChanges(t *testing.T) {
	models := []modelInfo{{ModelId: "grok-old"}, {ModelId: "grok-first"}, {ModelId: "grok-latest"}}
	config := []streams.ConfigOption{{
		Type: "select", ID: configOptionIDModel, Category: configOptionIDModel, CurrentValue: "grok-old",
	}}
	adapter := grokAdapterWithModels(t, models, config)
	conn := &blockingGrokSetModelConn{
		started:      make(chan string, 2),
		releaseFirst: make(chan struct{}),
	}
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- adapter.setModelWithConn(
			context.Background(), conn, "sess-grok", "grok-first", models, config,
		)
	}()
	if got := <-conn.started; got != "grok-first" {
		t.Fatalf("first RPC model = %q, want grok-first", got)
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- adapter.setModelWithConn(
			context.Background(), conn, "sess-grok", "grok-latest", models, config,
		)
	}()
	select {
	case got := <-conn.started:
		t.Fatalf("second RPC %q started before the first completed", got)
	case <-time.After(50 * time.Millisecond):
	}

	close(conn.releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first SetModel: %v", err)
	}
	if got := <-conn.started; got != "grok-latest" {
		t.Fatalf("second RPC model = %q, want grok-latest", got)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second SetModel: %v", err)
	}
	if got := currentModelFromConfig(adapter.availableConfigOptions); got != "grok-latest" {
		t.Fatalf("cached model = %q, want latest selection", got)
	}
}

func TestSetModelWithConn_StaleGrokCompletionDoesNotMutateReplacementSession(t *testing.T) {
	models := []modelInfo{{ModelId: "grok-old"}, {ModelId: "grok-stale"}, {ModelId: "grok-new"}}
	oldConfig := []streams.ConfigOption{{
		Type: "select", ID: configOptionIDModel, Category: configOptionIDModel, CurrentValue: "grok-old",
	}}
	adapter := grokAdapterWithModels(t, models, oldConfig)
	conn := &blockingGrokSetModelConn{
		started:      make(chan string, 1),
		releaseFirst: make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() {
		done <- adapter.setModelWithConn(
			context.Background(), conn, "sess-grok", "grok-stale", models, oldConfig,
		)
	}()
	<-conn.started

	newConfig := []streams.ConfigOption{{
		Type: "select", ID: configOptionIDModel, Category: configOptionIDModel, CurrentValue: "grok-new",
	}}
	adapter.mu.Lock()
	adapter.sessionID = "sess-new"
	adapter.availableConfigOptions = newConfig
	adapter.mu.Unlock()
	close(conn.releaseFirst)
	if err := <-done; err != nil {
		t.Fatalf("stale SetModel: %v", err)
	}

	adapter.mu.RLock()
	gotSession := adapter.sessionID
	gotModel := currentModelFromConfig(adapter.availableConfigOptions)
	adapter.mu.RUnlock()
	if gotSession != "sess-new" || gotModel != "grok-new" {
		t.Fatalf("replacement state = session %q model %q, want sess-new/grok-new", gotSession, gotModel)
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
			t.Fatal("usage tracker reset must not duplicate unchanged dialect context")
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
			t.Fatal("stale session must not emit dialect context")
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

func TestGrokContextRetriesAfterFullUpdateChannel(t *testing.T) {
	models := []modelInfo{{
		ModelId: "grok-current",
		Meta:    map[string]any{"totalContextTokens": float64(500_000)},
	}}
	a := grokAdapterWithModels(t, models, []streams.ConfigOption{{
		Type: "select", ID: configOptionIDModel, Category: configOptionIDModel, CurrentValue: "grok-current",
	}})
	a.updatesCh = make(chan AgentEvent, 1)
	a.updatesCh <- AgentEvent{Type: streams.EventTypeMessageChunk}
	meta := map[string]any{"totalTokens": float64(42_000)}

	if event := a.emitDialectContextWindow("sess-grok", meta); event != nil {
		t.Fatalf("emitDialectContextWindow() = %#v with full channel, want nil", event)
	}
	if _, cached := a.contextSamples["sess-grok"]; cached {
		t.Fatal("dropped context sample must not be cached")
	}
	<-a.updatesCh

	if event := a.emitDialectContextWindow("sess-grok", meta); event == nil {
		t.Fatal("same context sample must be retried after channel capacity returns")
	}
}

func TestGrokDialect_NormalizesPrivateReasoningTokens(t *testing.T) {
	response := &acp.PromptResponse{Meta: map[string]any{
		"usage": map[string]any{
			"inputTokens":     float64(5),
			"outputTokens":    float64(3),
			"totalTokens":     float64(8),
			"reasoningTokens": float64(2),
		},
	}}
	dialect := newGrokACPDialect()
	usage := dialect.promptUsage(extractUsage(response), response.Meta)
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
