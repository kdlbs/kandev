// Package acpcompat normalizes observed agent-specific ACP extensions into
// provider-neutral Kandev session shapes.
package acpcompat

import (
	"fmt"
	"math"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	GrokAgentID = "grok-acp"

	GrokReasoningEffortOptionID = "reasoning_effort"
	GrokReasoningEffortMetaKey  = "reasoningEffort"

	grokReasoningCategory  = "thought_level"
	grokReasoningName      = "Reasoning Effort"
	grokSupportsReasoning  = "supportsReasoningEffort"
	grokReasoningEfforts   = "reasoningEfforts"
	grokTotalContextTokens = "totalContextTokens"
	grokTotalTokens        = "totalTokens"
	modelOptionID          = "model"
)

// Model is the provider-neutral subset of an ACP model entry needed by
// compatibility normalization.
type Model struct {
	ID   string
	Name string
	Meta map[string]any
}

// ContextWindow is a context-usage sample derived from agent metadata.
type ContextWindow struct {
	Used int64
	Size int64
}

// NormalizeSessionConfig merges observed Grok model metadata into canonical
// session options without discarding typed or _meta options already parsed by
// the caller. Other agents pass through unchanged.
func NormalizeSessionConfig(
	agentID string,
	base []streams.ConfigOption,
	models []Model,
	currentModelID string,
) []streams.ConfigOption {
	if agentID != GrokAgentID {
		return base
	}
	out := append([]streams.ConfigOption(nil), base...)
	if currentModelID == "" {
		currentModelID = currentModelFromConfig(out)
	}
	if currentModelID == "" && len(models) == 1 {
		currentModelID = models[0].ID
	}
	if !hasModelOption(out) {
		if modelOption := buildModelOption(models, currentModelID); modelOption != nil {
			out = append(out, *modelOption)
		}
	}
	if !hasOption(out, GrokReasoningEffortOptionID) {
		if effort := buildReasoningOption(models, currentModelID, ""); effort != nil {
			out = append(out, *effort)
		}
	}
	return out
}

// ConfigAfterModelChange rebuilds model-dependent Grok options after a model
// switch. Other agents pass through unchanged.
func ConfigAfterModelChange(
	agentID string,
	config []streams.ConfigOption,
	models []Model,
	modelID string,
) []streams.ConfigOption {
	if agentID != GrokAgentID {
		return config
	}
	previousEffort := ""
	out := make([]streams.ConfigOption, 0, len(config))
	for _, option := range config {
		if option.ID == GrokReasoningEffortOptionID {
			previousEffort = option.CurrentValue
			continue
		}
		out = append(out, option)
	}
	if effort := buildReasoningOption(models, modelID, previousEffort); effort != nil {
		out = append(out, *effort)
	}
	return out
}

// ValidateGrokReasoningEffort resolves a UI selection to Grok's canonical
// wire value for the active model.
func ValidateGrokReasoningEffort(models []Model, currentModelID, effort string) (string, error) {
	if currentModelID == "" {
		return "", fmt.Errorf("no active model to apply reasoning effort")
	}
	info := findModel(models, currentModelID)
	if info == nil {
		return "", fmt.Errorf("unknown model %q; cannot set reasoning effort", currentModelID)
	}
	if !getBool(info.Meta, grokSupportsReasoning) {
		return "", fmt.Errorf("model %q does not support reasoning effort", currentModelID)
	}
	options, _ := parseReasoningEfforts(info.Meta)
	if len(options) == 0 {
		return "", fmt.Errorf("model %q does not advertise reasoning effort values", currentModelID)
	}
	if value := matchEffort(effort, options); value != "" {
		return value, nil
	}
	offered := make([]string, 0, len(options))
	for _, option := range options {
		offered = append(offered, option.Value)
	}
	return "", fmt.Errorf("unsupported reasoning effort %q for model %q; use one of: %s",
		effort, currentModelID, strings.Join(offered, ", "))
}

// GrokContextWindow reads Grok's notification total and active model context
// size as one immutable sample.
func GrokContextWindow(
	meta map[string]any,
	models []Model,
	config []streams.ConfigOption,
) (ContextWindow, bool) {
	used, ok := nonNegativeInt64(meta, grokTotalTokens)
	if !ok {
		return ContextWindow{}, false
	}
	model := findModel(models, currentModelFromConfig(config))
	if model == nil {
		return ContextWindow{}, false
	}
	size, ok := positiveInt64(model.Meta, grokTotalContextTokens)
	if !ok {
		return ContextWindow{}, false
	}
	return ContextWindow{Used: used, Size: size}, true
}

// GrokReasoningTokens reads Grok's nested prompt usage extension.
func GrokReasoningTokens(meta map[string]any) int64 {
	usage, _ := meta["usage"].(map[string]any)
	value, _ := nonNegativeInt64(usage, "reasoningTokens")
	return value
}

func buildModelOption(models []Model, currentModelID string) *streams.ConfigOption {
	values := make([]streams.ConfigOptionValue, 0, len(models))
	for _, model := range models {
		name := model.Name
		if name == "" {
			name = model.ID
		}
		values = append(values, streams.ConfigOptionValue{Value: model.ID, Name: name})
	}
	if len(values) == 0 {
		return nil
	}
	return &streams.ConfigOption{
		Type:         "select",
		ID:           modelOptionID,
		Category:     modelOptionID,
		Name:         "Model",
		CurrentValue: currentModelID,
		Options:      values,
	}
}

func buildReasoningOption(models []Model, modelID, selected string) *streams.ConfigOption {
	model := findModel(models, modelID)
	if model == nil || !getBool(model.Meta, grokSupportsReasoning) {
		return nil
	}
	options, defaultEffort := parseReasoningEfforts(model.Meta)
	if len(options) == 0 {
		return nil
	}
	current := resolveEffort(selected, options, defaultEffort, model.Meta)
	return &streams.ConfigOption{
		Type:         "select",
		ID:           GrokReasoningEffortOptionID,
		Category:     grokReasoningCategory,
		Name:         grokReasoningName,
		CurrentValue: current,
		Options:      options,
	}
}

func parseReasoningEfforts(meta map[string]any) ([]streams.ConfigOptionValue, string) {
	items, ok := meta[grokReasoningEfforts].([]any)
	if !ok {
		return nil, ""
	}
	var out []streams.ConfigOptionValue
	defaultEffort := ""
	for _, item := range items {
		switch value := item.(type) {
		case string:
			value = strings.TrimSpace(value)
			if value != "" {
				out = append(out, streams.ConfigOptionValue{Value: value, Name: humanize(value)})
			}
		case map[string]any:
			wireValue := getString(value, "value")
			if wireValue == "" {
				wireValue = getString(value, "id")
			}
			if wireValue == "" {
				continue
			}
			name := getString(value, "label")
			if name == "" {
				name = humanize(wireValue)
			}
			out = append(out, streams.ConfigOptionValue{Value: wireValue, Name: name})
			if getBool(value, "default") {
				defaultEffort = wireValue
			}
		}
	}
	return out, defaultEffort
}

func resolveEffort(selected string, options []streams.ConfigOptionValue, fallback string, meta map[string]any) string {
	for _, candidate := range []string{selected, getString(meta, GrokReasoningEffortMetaKey), fallback, "medium"} {
		if value := matchEffort(candidate, options); value != "" {
			return value
		}
	}
	return options[0].Value
}

func matchEffort(token string, options []streams.ConfigOptionValue) string {
	for _, option := range options {
		if option.Value == token || option.Name == token {
			return option.Value
		}
	}
	return ""
}

func currentModelFromConfig(options []streams.ConfigOption) string {
	for _, option := range options {
		if option.ID == modelOptionID || option.Category == modelOptionID {
			return option.CurrentValue
		}
	}
	return ""
}

func hasModelOption(options []streams.ConfigOption) bool {
	for _, option := range options {
		if option.ID == modelOptionID || option.Category == modelOptionID {
			return true
		}
	}
	return false
}

func hasOption(options []streams.ConfigOption, id string) bool {
	for _, option := range options {
		if option.ID == id {
			return true
		}
	}
	return false
}

func findModel(models []Model, id string) *Model {
	for i := range models {
		if models[i].ID == id {
			return &models[i]
		}
	}
	return nil
}

func humanize(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func getString(meta map[string]any, key string) string {
	value, _ := meta[key].(string)
	return value
}

func getBool(meta map[string]any, key string) bool {
	value, _ := meta[key].(bool)
	return value
}

func positiveInt64(meta map[string]any, key string) (int64, bool) {
	value, ok := nonNegativeInt64(meta, key)
	return value, ok && value > 0
}

func nonNegativeInt64(meta map[string]any, key string) (int64, bool) {
	if meta == nil {
		return 0, false
	}
	switch value := meta[key].(type) {
	case float64:
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) ||
			value != math.Trunc(value) || value >= math.Exp2(63) {
			return 0, false
		}
		return int64(value), true
	case int64:
		return value, value >= 0
	case int:
		return int64(value), value >= 0
	default:
		return 0, false
	}
}
