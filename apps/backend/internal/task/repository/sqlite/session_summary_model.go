package sqlite

import (
	"github.com/kandev/kandev/internal/db/dialect"
)

func sessionSummaryModelColumns(driver, metadata string) (string, string) {
	modelID := "COALESCE(NULLIF(" + dialect.JSONExtractPath(driver, metadata, "runtime_config_overrides", "model") + ", ''), NULLIF(" +
		dialect.JSONExtractPath(driver, metadata, "runtime_config", "model") + ", ''), " +
		dialect.JSONExtractPath(driver, metadata, "acp_model_state", "current_model_id") + ", '')"
	modelID = "CASE WHEN " + dialect.JSONExtractPath(driver, metadata, "acp_model_state", "settings_policy") + " = 'provider_restored' THEN COALESCE(" + dialect.JSONExtractPath(driver, metadata, "acp_model_state", "current_model_id") + ", '') ELSE " + modelID + " END"

	if dialect.IsPostgres(driver) {
		choices := "CASE WHEN jsonb_typeof(" + metadata + "::jsonb->'acp_model_state'->'models') = 'array' THEN " + metadata + "::jsonb->'acp_model_state'->'models' ELSE '[]'::jsonb END"
		return modelID, "(SELECT choice->>'name' FROM jsonb_array_elements(" + choices + ") choice WHERE choice->>'model_id' = " + modelID + " LIMIT 1)"
	}
	choices := "CASE WHEN json_type(" + metadata + ", '$.acp_model_state.models') = 'array' THEN json_extract(" + metadata + ", '$.acp_model_state.models') ELSE '[]' END"
	return modelID, "(SELECT CASE WHEN choice.type = 'object' THEN json_extract(choice.value, '$.name') END FROM json_each(" + choices + ") choice WHERE CASE WHEN choice.type = 'object' THEN json_extract(choice.value, '$.model_id') END = " + modelID + " LIMIT 1)"
}
