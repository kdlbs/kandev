package acp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	grokAgentID                      = acpcompat.GrokAgentID
	grokReasoningEffortMetaKey       = acpcompat.GrokReasoningEffortMetaKey
	configOptionIDReasoningEffort    = acpcompat.GrokReasoningEffortOptionID
	configOptionCategoryThoughtLevel = "thought_level"
)

func newGrokACPDialect() acpDialect {
	return acpDialect{
		normalizeSessionConfig: grokNormalizeSessionConfig,
		configAfterModelChange: grokConfigAfterModelChange,
		modelChangeRequest:     grokModelChangeRequest,
		configChangeRequest:    grokConfigChangeRequest,
		suppressNotification: func(notification acp.SessionNotification) bool {
			return notification.Update.UserMessageChunk != nil
		},
		contextWindow:        grokContextWindow,
		normalizePromptUsage: normalizeGrokPromptUsage,
	}
}

func grokNormalizeSessionConfig(
	base []streams.ConfigOption,
	models []modelInfo,
	currentModelID string,
) []streams.ConfigOption {
	return acpcompat.NormalizeSessionConfig(
		grokAgentID,
		base,
		toCompatModels(models),
		currentModelID,
	)
}

// grokSessionConfigOptions is retained as a focused parser entry point for
// captured-wire tests. Runtime selection goes through acpDialect.
func grokSessionConfigOptions(
	typed []streams.ConfigOption,
	meta map[string]any,
	models []modelInfo,
	currentModelID string,
) []streams.ConfigOption {
	base := typed
	if len(base) == 0 {
		base = extractConfigOptions(meta)
	}
	return grokNormalizeSessionConfig(base, models, currentModelID)
}

func grokConfigAfterModelChange(
	config []streams.ConfigOption,
	models []modelInfo,
	modelID string,
) []streams.ConfigOption {
	return acpcompat.ConfigAfterModelChange(grokAgentID, config, toCompatModels(models), modelID)
}

func grokModelChangeRequest(change dialectConfigChange) (*dialectRPC, error) {
	if len(change.models) > 0 && findModelInfo(change.models, change.value) == nil {
		return nil, fmt.Errorf("model %q is not in the agent's available models", change.value)
	}
	req := acp.UnstableSetSessionModelRequest{
		SessionId: acp.SessionId(change.sessionID),
		ModelId:   change.value,
	}
	if previous := currentGrokEffortFromConfig(change.config); previous != "" {
		if effort, err := acpcompat.ValidateGrokReasoningEffort(
			toCompatModels(change.models), change.value, previous,
		); err == nil {
			req.Meta = map[string]any{grokReasoningEffortMetaKey: effort}
		}
	}
	return &dialectRPC{request: req, formatError: formatGrokSetModelError}, nil
}

func grokConfigChangeRequest(change dialectConfigChange) (*dialectRPC, error) {
	if isModelConfigID(change.configID, change.config) {
		return grokModelChangeRequest(change)
	}
	if change.configID != configOptionIDReasoningEffort {
		return nil, nil
	}
	modelID := currentModelFromConfig(change.config)
	effort, err := acpcompat.ValidateGrokReasoningEffort(
		toCompatModels(change.models), modelID, change.value,
	)
	if err != nil {
		return nil, err
	}
	return &dialectRPC{
		request: acp.UnstableSetSessionModelRequest{
			SessionId: acp.SessionId(change.sessionID),
			ModelId:   modelID,
			Meta:      map[string]any{grokReasoningEffortMetaKey: effort},
		},
		formatError: formatGrokReasoningError,
	}, nil
}

func grokContextWindow(
	meta map[string]any,
	models []modelInfo,
	config []streams.ConfigOption,
) (contextWindowSample, bool) {
	sample, ok := acpcompat.GrokContextWindow(meta, toCompatModels(models), config)
	return contextWindowSample{used: sample.Used, size: sample.Size}, ok
}

func normalizeGrokPromptUsage(
	usage *streams.PromptUsage,
	meta map[string]any,
) *streams.PromptUsage {
	if usage != nil && usage.ThoughtTokens == 0 {
		usage.ThoughtTokens = acpcompat.GrokReasoningTokens(meta)
	}
	return usage
}

func toCompatModels(models []modelInfo) []acpcompat.Model {
	out := make([]acpcompat.Model, 0, len(models))
	for _, model := range models {
		out = append(out, acpcompat.Model{ID: model.ModelId, Name: model.Name, Meta: toStringMap(model.Meta)})
	}
	return out
}

func findModelInfo(models []modelInfo, modelID string) *modelInfo {
	for i := range models {
		if models[i].ModelId == modelID {
			return &models[i]
		}
	}
	return nil
}

func currentGrokEffortFromConfig(config []streams.ConfigOption) string {
	for _, option := range config {
		if option.ID == configOptionIDReasoningEffort {
			return option.CurrentValue
		}
	}
	return ""
}

func formatGrokReasoningError(err error) error {
	return fmt.Errorf("set grok reasoning effort via session/set_model failed: %w", err)
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
