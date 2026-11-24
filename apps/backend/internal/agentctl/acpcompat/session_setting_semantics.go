package acpcompat

import "github.com/kandev/kandev/internal/agentctl/types/streams"

const PiAgentID = "pi-acp"
const ThoughtLevelCategory = "thought_level"

// PiThinkingConfigID is the Pi bridge's stable setting ID for saved profiles.
const PiThinkingConfigID = "thought_level"

// ThinkingOption returns only an unambiguous, explicitly categorized select.
func ThinkingOption(options []streams.ConfigOption) (streams.ConfigOption, bool) {
	var selected streams.ConfigOption
	count := 0
	for _, option := range options {
		if option.Type == "select" && option.Category == ThoughtLevelCategory {
			selected = option
			count++
		}
	}
	return selected, count == 1
}

// SessionModeOption preserves permission options. Only the built-in Pi binding
// aliases its legacy session mode to the thinking configuration.
func SessionModeOption(agentID string, options []streams.ConfigOption) (streams.ConfigOption, bool) {
	if agentID == PiAgentID {
		return ThinkingOption(options)
	}
	for _, option := range options {
		if option.Type == "select" && option.Category == "mode" {
			return option, true
		}
	}
	return streams.ConfigOption{}, false
}
