package automation

import "fmt"

func validateTriggerCombination(kinds []TriggerType) error {
	var scheduled, plugin bool
	for _, kind := range kinds {
		scheduled = scheduled || kind == TriggerTypeScheduled
		plugin = plugin || kind == TriggerTypePluginEvent
	}
	if scheduled && plugin {
		return fmt.Errorf("scheduled and plugin event conditions cannot be combined")
	}
	return nil
}
