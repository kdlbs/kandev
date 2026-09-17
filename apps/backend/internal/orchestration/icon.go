package orchestration

import (
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func orchestratorIcon(a *models.AgentInstance) string {
	var settings struct {
		Icon string `json:"orchestrator_icon"`
	}
	_ = json.Unmarshal([]byte(a.Settings), &settings)
	return settings.Icon
}
func setOrchestratorIcon(a *models.AgentInstance, icon string) error {
	switch icon {
	case "", "💼", "🧭", "🤖", "🛠️", "🌱", "⭐":
	default:
		return fmt.Errorf("select an available persona icon")
	}
	settings := map[string]json.RawMessage{}
	if a.Settings != "" {
		if err := json.Unmarshal([]byte(a.Settings), &settings); err != nil {
			return err
		}
	}
	if settings == nil {
		settings = map[string]json.RawMessage{}
	}
	raw, _ := json.Marshal(icon)
	settings["orchestrator_icon"] = raw
	data, err := json.Marshal(settings)
	if err == nil {
		a.Settings = string(data)
	}
	return err
}

func setPersonaPresentation(a *models.AgentInstance, icon, context string) error {
	if err := setOrchestratorIcon(a, icon); err != nil {
		return err
	}
	return models.SetDelegationContext(a, context)
}
