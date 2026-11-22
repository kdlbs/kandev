package acp

import "github.com/kandev/kandev/internal/agentctl/types/streams"

func configurationResultFromOptions(
	options []streams.ConfigOption,
	configID string,
	requested string,
) streams.ConfigurationResult {
	result := streams.ConfigurationResult{ConfigID: configID, Requested: requested}
	for _, option := range options {
		if option.ID != configID || option.CurrentValue == "" {
			continue
		}
		result.Effective = option.CurrentValue
		result.Confirmed = true
		break
	}
	return result
}
