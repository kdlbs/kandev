package acpcompat

import (
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// CopilotNativeCompactionCapability is the optional agentctl peer capability
// for Copilot's opt-in native compaction and attributed usage stream. It is not
// a required survival capability; older retained peers remain adoptable and
// report native counting unavailable.
const CopilotNativeCompactionCapability = "copilot-native-compaction.v1"

const (
	CopilotAgentID = "copilot-acp"

	CopilotContextTierOptionID               = "context_tier"
	CopilotContextTierCategory               = "context_window"
	CopilotContextTierDefault                = "default"
	CopilotContextTierLong                   = "long_context"
	CopilotContextTierAllowID                = "allow_all"
	CopilotSynthesizedContextTierDescription = "Controls whether Copilot launches with the model's standard or extended context window."
)

// IsValidCopilotContextTier reports whether value is a supported Copilot
// CLI --context tier.
func IsValidCopilotContextTier(value string) bool {
	return value == CopilotContextTierDefault || value == CopilotContextTierLong
}

// HasCopilotContextCLIFlag reports whether CLI flag tokens explicitly specify
// a --context or --context=... argument.
func HasCopilotContextCLIFlag(args []string) bool {
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "--context" || strings.HasPrefix(trimmed, "--context=") {
			return true
		}
	}
	return false
}

// ParseCopilotContextTierFromArgs returns the effective Copilot context tier
// parsed from CLI arguments and whether a valid --context tier was explicitly
// provided.
func ParseCopilotContextTierFromArgs(args []string) (tier string, explicit bool) {
	tier = CopilotContextTierDefault
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		var candidate string
		switch {
		case arg == "--context" && i+1 < len(args):
			candidate = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--context="):
			candidate = strings.TrimSpace(strings.TrimPrefix(arg, "--context="))
		default:
			continue
		}
		if IsValidCopilotContextTier(candidate) {
			tier = candidate
			explicit = true
		}
	}
	return tier, explicit
}

// HasNativeCopilotContextOption reports whether an ACP configOptions snapshot
// already advertises a native context-window option.
func HasNativeCopilotContextOption(options []streams.ConfigOption) bool {
	for _, option := range options {
		if option.ID == CopilotContextTierOptionID ||
			option.ID == "contextTier" ||
			option.ID == "context" ||
			option.Category == CopilotContextTierCategory {
			return true
		}
	}
	return false
}

// IsSynthesizedCopilotContextOption reports whether an option entry matches
// Kandev's synthesized Copilot context_tier fallback rather than a native
// provider wire option.
func IsSynthesizedCopilotContextOption(id, description string) bool {
	return id == CopilotContextTierOptionID && description == CopilotSynthesizedContextTierDescription
}

// NormalizeCopilotConfigOptions synthesizes the Copilot context_tier select
// option when omitted from wire configOptions, leaving native context-window
// options untouched. The bool reports whether synthesis occurred.
const configOptionTypeSelect = "select"

func NormalizeCopilotConfigOptions(
	options []streams.ConfigOption,
	activeContextTier string,
) ([]streams.ConfigOption, bool) {
	out := append([]streams.ConfigOption(nil), options...)
	if HasNativeCopilotContextOption(out) {
		return out, false
	}
	currentValue := CopilotContextTierDefault
	if IsValidCopilotContextTier(activeContextTier) {
		currentValue = activeContextTier
	}
	synthesized := streams.ConfigOption{
		Type:         configOptionTypeSelect,
		ID:           CopilotContextTierOptionID,
		Name:         "Context Window",
		Category:     CopilotContextTierCategory,
		Description:  CopilotSynthesizedContextTierDescription,
		CurrentValue: currentValue,
		Options: []streams.ConfigOptionValue{
			{
				Value:       CopilotContextTierDefault,
				Name:        "Default",
				Description: "Standard context window.",
			},
			{
				Value:       CopilotContextTierLong,
				Name:        "Long Context",
				Description: "Extended context window for supported models.",
			},
		},
	}
	insertIdx := copilotContextTierInsertIndex(out)
	return slices.Insert(out, insertIdx, synthesized), true
}

func copilotContextTierInsertIndex(options []streams.ConfigOption) int {
	for i, opt := range options {
		if opt.ID == GrokReasoningEffortOptionID || opt.Category == grokReasoningCategory {
			return i + 1
		}
	}
	for i, opt := range options {
		if opt.ID == modelOptionID || opt.Category == modelOptionID {
			return i + 1
		}
	}
	for i, opt := range options {
		if opt.ID == CopilotContextTierAllowID {
			return i
		}
	}
	return len(options)
}
