package models

// SessionModelLabelMetadata retains the current display identity without model
// alternative choices, descriptions, configuration options, or runtime settings.
func SessionModelLabelMetadata(modelID, modelName string) map[string]interface{} {
	if modelID == "" {
		return nil
	}
	if modelName == "" {
		modelName = modelID
	}
	return map[string]interface{}{
		"current_model_id": modelID,
		"models":           []interface{}{map[string]interface{}{"model_id": modelID, "name": modelName}},
	}
}

func ProjectSessionModelLabel(metadata map[string]interface{}) map[string]interface{} {
	snapshot, _ := metadata[SessionMetaKeyACPModelState].(map[string]interface{})
	modelID, _ := snapshot["current_model_id"].(string)
	if snapshot["settings_policy"] != "provider_restored" {
		for _, key := range []string{"runtime_config", "runtime_config_overrides"} {
			values, _ := metadata[key].(map[string]interface{})
			if value, _ := values["model"].(string); value != "" {
				modelID = value
			}
		}
	}

	modelName := ""
	choices, _ := snapshot["models"].([]interface{})
	for _, choice := range choices {
		model, _ := choice.(map[string]interface{})
		if model["model_id"] == modelID {
			modelName, _ = model["name"].(string)
			break
		}
	}
	return SessionModelLabelMetadata(modelID, modelName)
}
