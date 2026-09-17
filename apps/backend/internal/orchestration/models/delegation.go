package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

func DelegationContext(agent *AgentInstance) string {
	var settings struct {
		Context string `json:"delegation_context"`
	}
	_ = json.Unmarshal([]byte(agent.Settings), &settings)
	return settings.Context
}

func SetDelegationContext(agent *AgentInstance, value string) error {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 2000 {
		return fmt.Errorf("delegation context must be valid text of at most 2000 characters")
	}
	settings := map[string]json.RawMessage{}
	if agent.Settings != "" {
		if err := json.Unmarshal([]byte(agent.Settings), &settings); err != nil {
			return err
		}
	}
	if settings == nil {
		settings = map[string]json.RawMessage{}
	}
	raw, _ := json.Marshal(strings.TrimSpace(value))
	settings["delegation_context"] = raw
	data, err := json.Marshal(settings)
	if err == nil {
		agent.Settings = string(data)
	}
	return err
}
