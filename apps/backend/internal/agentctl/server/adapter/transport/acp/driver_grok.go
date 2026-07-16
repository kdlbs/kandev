package acp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const grokAgentID = "grok-acp"

// Grok private meta keys / option IDs.
const (
	grokTotalTokensMetaKey     = "totalTokens"
	grokTotalContextTokensKey  = "totalContextTokens"
	grokSupportsReasoningKey   = "supportsReasoningEffort"
	grokReasoningEffortsKey    = "reasoningEfforts"
	grokReasoningEffortMetaKey = "reasoningEffort"

	// Normalized Kandev config option for reasoning effort. Frontend already
	// understands this id/category pair; category must NOT be "mode" because
	// usableConfigOptions filters mode options out of the model selector.
	configOptionIDReasoningEffort    = "reasoning_effort"
	configOptionCategoryThoughtLevel = "thought_level"
	configOptionNameReasoningEffort  = "Reasoning Effort"
)

type grokContextSample struct {
	used int64
	size int64
}

type grokACPDriver struct {
	mu               sync.Mutex
	contextSessionID string
	contextSample    grokContextSample
	hasContextSample bool
}

func newGrokACPDriver() *grokACPDriver {
	return &grokACPDriver{}
}

func (*grokACPDriver) sessionConfigOptions(
	meta map[string]any,
	options []acp.SessionConfigOption,
	models []modelInfo,
	currentModelID string,
) []streams.ConfigOption {
	return grokSessionConfigOptions(convertACPConfigOptions(options), meta, models, currentModelID)
}

func (*grokACPDriver) modelConfigAfterChange(
	config []streams.ConfigOption,
	models []modelInfo,
	modelID string,
) []streams.ConfigOption {
	previousEffort := ""
	filtered := make([]streams.ConfigOption, 0, len(config))
	for _, option := range config {
		if option.ID == configOptionIDReasoningEffort {
			previousEffort = option.CurrentValue
			continue
		}
		filtered = append(filtered, option)
	}
	if effort := buildGrokReasoningEffortOption(models, modelID, previousEffort); effort != nil {
		filtered = append(filtered, *effort)
	}
	return filtered
}

func (d *grokACPDriver) handleConfigOption(
	ctx context.Context,
	adapter *Adapter,
	conn driverSessionConn,
	change driverConfigChange,
) (bool, error) {
	if change.configID == configOptionIDReasoningEffort {
		return true, d.setReasoningEffort(ctx, adapter, conn, change)
	}
	if isModelConfigID(change.configID, change.config) {
		return d.handleModelChange(ctx, adapter, conn, change)
	}
	return false, nil
}

func (d *grokACPDriver) handleModelChange(
	ctx context.Context,
	adapter *Adapter,
	conn driverSessionConn,
	change driverConfigChange,
) (bool, error) {
	return true, d.setModel(ctx, adapter, conn, change)
}

func (*grokACPDriver) suppressNotification(notification acp.SessionNotification) bool {
	return notification.Update.UserMessageChunk != nil
}

func (d *grokACPDriver) onModelChanged(sessionID string) {
	d.mu.Lock()
	if d.contextSessionID == sessionID {
		d.hasContextSample = false
	}
	d.mu.Unlock()
}

func (d *grokACPDriver) supplementalEvents(
	adapter *Adapter,
	sessionID string,
	meta map[string]any,
) []AgentEvent {
	used, ok := parseGrokTotalTokens(meta)
	if !ok {
		return nil
	}
	event := d.contextWindowFromUsed(adapter, sessionID, used)
	if event == nil {
		return nil
	}
	return []AgentEvent{*event}
}

func (*grokACPDriver) normalizePromptUsage(
	usage *streams.PromptUsage,
	meta map[string]any,
) *streams.PromptUsage {
	if usage == nil || usage.ThoughtTokens != 0 {
		return usage
	}
	usageMeta := toStringMap(meta["usage"])
	usage.ThoughtTokens = getInt64(usageMeta, "reasoningTokens")
	return usage
}

// grokSessionConfigOptions exposes Grok's legacy model catalog through the
// generic ConfigOption shape used by the frontend.
func grokSessionConfigOptions(
	typed []streams.ConfigOption,
	meta map[string]any,
	models []modelInfo,
	currentModelID string,
) []streams.ConfigOption {
	if len(typed) > 0 {
		return typed
	}
	if opts := extractConfigOptions(meta); len(opts) > 0 {
		return opts
	}
	return buildGrokConfigOptions(models, currentModelID)
}

func buildGrokConfigOptions(models []modelInfo, currentModelID string) []streams.ConfigOption {
	var out []streams.ConfigOption
	if currentModelID == "" {
		currentModelID = currentModelFromModels(models)
	}
	if modelOpt := buildGrokModelConfigOption(models, currentModelID); modelOpt != nil {
		out = append(out, *modelOpt)
	}
	if effortOpt := buildGrokReasoningEffortOption(models, currentModelID, ""); effortOpt != nil {
		out = append(out, *effortOpt)
	}
	return out
}

func currentModelFromModels(models []modelInfo) string {
	if len(models) == 1 {
		return models[0].ModelId
	}
	return ""
}

func buildGrokModelConfigOption(models []modelInfo, currentModelID string) *streams.ConfigOption {
	values := make([]streams.ConfigOptionValue, 0, len(models))
	for _, m := range models {
		name := m.Name
		if name == "" {
			name = m.ModelId
		}
		values = append(values, streams.ConfigOptionValue{Value: m.ModelId, Name: name})
	}
	if len(values) == 0 {
		return nil
	}
	return &streams.ConfigOption{
		Type:         "select",
		ID:           configOptionIDModel,
		Category:     configOptionIDModel,
		Name:         "Model",
		CurrentValue: currentModelID,
		Options:      values,
	}
}

// buildGrokReasoningEffortOption builds the reasoning_effort select for the
// current model. Returns nil when the model does not support reasoning effort.
func buildGrokReasoningEffortOption(models []modelInfo, currentModelID, selectedEffort string) *streams.ConfigOption {
	info := findModelInfo(models, currentModelID)
	if info == nil {
		// No catalog entry: cannot know supportsReasoningEffort safely.
		return nil
	}
	meta := toStringMap(info.Meta)
	if !getBool(meta, grokSupportsReasoningKey) {
		return nil
	}

	options, defaultEffort := parseGrokReasoningEfforts(meta)
	if len(options) == 0 {
		return nil
	}
	current := resolveGrokEffortSelection(selectedEffort, options, defaultEffort, meta)
	return &streams.ConfigOption{
		Type:         "select",
		ID:           configOptionIDReasoningEffort,
		Category:     configOptionCategoryThoughtLevel,
		Name:         configOptionNameReasoningEffort,
		CurrentValue: current,
		Options:      options,
	}
}

func findModelInfo(models []modelInfo, modelID string) *modelInfo {
	if modelID == "" {
		return nil
	}
	for i := range models {
		if models[i].ModelId == modelID {
			return &models[i]
		}
	}
	return nil
}

// parseGrokReasoningEfforts reads the model-advertised wire values verbatim.
func parseGrokReasoningEfforts(meta map[string]any) ([]streams.ConfigOptionValue, string) {
	if meta == nil {
		return nil, ""
	}
	raw, ok := meta[grokReasoningEffortsKey]
	if !ok {
		return nil, ""
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, ""
	}
	var (
		out           []streams.ConfigOptionValue
		defaultEffort string
	)
	for _, item := range items {
		switch v := item.(type) {
		case string:
			value := strings.TrimSpace(v)
			if value == "" {
				continue
			}
			out = append(out, streams.ConfigOptionValue{
				Value: value,
				Name:  humanizeGrokEffort(value),
			})
		case map[string]any:
			value := getString(v, "value")
			if value == "" {
				value = getString(v, "id")
			}
			if value == "" {
				continue
			}
			name := getString(v, "label")
			if name == "" {
				name = humanizeGrokEffort(value)
			}
			out = append(out, streams.ConfigOptionValue{Value: value, Name: name})
			if getBool(v, "default") {
				defaultEffort = value
			}
		}
	}
	return out, defaultEffort
}

func humanizeGrokEffort(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func resolveGrokEffortSelection(selected string, options []streams.ConfigOptionValue, defaultEffort string, modelMeta map[string]any) string {
	// Prefer explicit session selection (may be menu id — map via options).
	if v := matchEffortInOptions(selected, options); v != "" {
		return v
	}
	// Model default from meta.reasoningEffort.
	if v := matchEffortInOptions(getString(modelMeta, grokReasoningEffortMetaKey), options); v != "" {
		return v
	}
	if v := matchEffortInOptions(defaultEffort, options); v != "" {
		return v
	}
	if len(options) > 0 {
		// Prefer medium when present, else first option.
		if v := matchEffortInOptions("medium", options); v != "" {
			return v
		}
		return options[0].Value
	}
	return ""
}

func matchEffortInOptions(token string, options []streams.ConfigOptionValue) string {
	if token == "" {
		return ""
	}
	for _, o := range options {
		if o.Value == token || o.Name == token {
			return o.Value
		}
	}
	return ""
}

// validateGrokReasoningEffort returns the canonical wire effort for the given
// selection against the current model, or an actionable error.
func validateGrokReasoningEffort(models []modelInfo, currentModelID, effort string) (string, error) {
	if currentModelID == "" {
		return "", fmt.Errorf("no active model to apply reasoning effort")
	}
	info := findModelInfo(models, currentModelID)
	if info == nil {
		return "", fmt.Errorf("unknown model %q; cannot set reasoning effort", currentModelID)
	}
	meta := toStringMap(info.Meta)
	if !getBool(meta, grokSupportsReasoningKey) {
		return "", fmt.Errorf("model %q does not support reasoning effort", currentModelID)
	}
	options, _ := parseGrokReasoningEfforts(meta)
	if len(options) == 0 {
		return "", fmt.Errorf("model %q does not advertise reasoning effort values", currentModelID)
	}
	if v := matchEffortInOptions(effort, options); v != "" {
		return v, nil
	}
	offered := make([]string, 0, len(options))
	for _, o := range options {
		offered = append(offered, o.Value)
	}
	return "", fmt.Errorf("unsupported reasoning effort %q for model %q; use one of: %s",
		effort, currentModelID, strings.Join(offered, ", "))
}

// modelTotalContextTokens reads _meta.totalContextTokens from a model entry.
func modelTotalContextTokens(m *modelInfo) int64 {
	if m == nil {
		return 0
	}
	return getInt64(toStringMap(m.Meta), grokTotalContextTokensKey)
}

// parseGrokTotalTokens reads notification-level _meta.totalTokens.
// Returns (value, ok). ok is false for missing, negative, or non-numeric.
func parseGrokTotalTokens(meta map[string]any) (int64, bool) {
	if meta == nil {
		return 0, false
	}
	raw, ok := meta[grokTotalTokensMetaKey]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		if v < 0 {
			return 0, false
		}
		return int64(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return v, true
	case int:
		if v < 0 {
			return 0, false
		}
		return int64(v), true
	default:
		return 0, false
	}
}

// getBool safely extracts a bool from a map (JSON bool).
func getBool(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, ok := m[key].(bool)
	return ok && v
}

func (d *grokACPDriver) setModel(
	ctx context.Context,
	adapter *Adapter,
	conn driverSessionConn,
	change driverConfigChange,
) error {
	if len(change.models) > 0 && findModelInfo(change.models, change.value) == nil {
		return fmt.Errorf("model %q is not in the agent's available models", change.value)
	}

	req := acp.UnstableSetSessionModelRequest{
		SessionId: acp.SessionId(change.sessionID),
		ModelId:   change.value,
	}
	if previous := currentGrokEffortFromConfig(change.config); previous != "" {
		if effort, err := validateGrokReasoningEffort(change.models, change.value, previous); err == nil {
			req.Meta = map[string]any{grokReasoningEffortMetaKey: effort}
		}
	}
	if _, err := conn.UnstableSetSessionModel(ctx, req); err != nil {
		return formatGrokSetModelError(err)
	}

	adapter.resetContextWindowMaxSize(change.sessionID)
	d.onModelChanged(change.sessionID)
	adapter.emitSetModelEvent(change.sessionID, change.value, change.models, change.config)
	return nil
}

func (*grokACPDriver) setReasoningEffort(
	ctx context.Context,
	adapter *Adapter,
	conn driverSessionConn,
	change driverConfigChange,
) error {
	modelID := currentModelFromConfig(change.config)
	effort, err := validateGrokReasoningEffort(change.models, modelID, change.value)
	if err != nil {
		return err
	}
	_, err = conn.UnstableSetSessionModel(ctx, acp.UnstableSetSessionModelRequest{
		SessionId: acp.SessionId(change.sessionID),
		ModelId:   modelID,
		Meta:      map[string]any{grokReasoningEffortMetaKey: effort},
	})
	if err != nil {
		return fmt.Errorf("set grok reasoning effort via session/set_model failed: %w", err)
	}
	adapter.emitSetConfigOptionEvent(
		change.sessionID,
		configOptionIDReasoningEffort,
		effort,
		change.models,
		change.config,
	)
	return nil
}

func (d *grokACPDriver) contextWindowFromUsed(
	adapter *Adapter,
	sessionID string,
	used int64,
) *AgentEvent {
	adapter.mu.RLock()
	activeSessionID := adapter.sessionID
	modelID := currentModelFromConfig(adapter.availableConfigOptions)
	models := adapter.availableModels
	adapter.mu.RUnlock()
	if sessionID != activeSessionID {
		return nil
	}

	size := modelTotalContextTokens(findModelInfo(models, modelID))
	if size <= 0 {
		return nil
	}

	d.mu.Lock()
	if d.hasContextSample && d.contextSessionID == sessionID &&
		d.contextSample.used == used && d.contextSample.size == size {
		d.mu.Unlock()
		return nil
	}
	d.contextSessionID = sessionID
	d.contextSample = grokContextSample{used: used, size: size}
	d.hasContextSample = true
	d.mu.Unlock()

	remaining := size - used
	if remaining < 0 {
		remaining = 0
	}
	return &AgentEvent{
		Type:                   streams.EventTypeContextWindow,
		SessionID:              sessionID,
		ContextWindowSize:      size,
		ContextWindowUsed:      used,
		ContextWindowRemaining: remaining,
		ContextEfficiency:      float64(used) / float64(size) * 100,
	}
}

func formatGrokSetModelError(err error) error {
	if err == nil {
		return nil
	}
	var reqErr *acp.RequestError
	if errors.As(err, &reqErr) && reqErr.Message != "" {
		if isGrokIncompatibleAgentSwitchError(err) {
			message := reqErr.Message
			if !strings.Contains(message, "Start a new session") {
				message += ". Start a new session to use this model."
			}
			return fmt.Errorf("%s [MODEL_SWITCH_INCOMPATIBLE_AGENT]: %w", message, err)
		}
		return fmt.Errorf("set grok model via session/set_model failed: %s: %w", reqErr.Message, err)
	}
	return fmt.Errorf("set grok model via session/set_model failed: %w", err)
}

func isGrokIncompatibleAgentSwitchError(err error) bool {
	if err == nil {
		return false
	}
	var reqErr *acp.RequestError
	if errors.As(err, &reqErr) && grokErrorDataCode(reqErr.Data) == "MODEL_SWITCH_INCOMPATIBLE_AGENT" {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "MODEL_SWITCH_INCOMPATIBLE_AGENT") ||
		(strings.Contains(message, "requires agent") && strings.Contains(message, "Start a new session"))
}

func grokErrorDataCode(data any) string {
	switch value := data.(type) {
	case map[string]any:
		code, _ := value["code"].(string)
		return code
	case map[string]string:
		return value["code"]
	default:
		return ""
	}
}

func currentGrokEffortFromConfig(config []streams.ConfigOption) string {
	for _, option := range config {
		if option.ID == configOptionIDReasoningEffort {
			return option.CurrentValue
		}
	}
	return ""
}
