package sqlite

import (
	"github.com/kandev/kandev/internal/db/dialect"
)

// postgresSessionSummaryLateralFrom parses each JSON text column of
// task_sessions exactly once per row. The projection's per-row expressions
// reference the parsed jsonb through these lateral aliases instead of
// re-casting the raw text in every extraction; the model-label lookup
// re-evaluates its filter per models-array element, so a raw-text cast there
// re-parsed the whole metadata document once per element. The OFFSET 0 fences
// keep the planner from flattening the laterals back into per-reference
// expressions (a flattened lateral re-casts the raw text everywhere it was
// referenced, restoring the per-element cost).
const postgresSessionSummaryLateralFrom = `
		CROSS JOIN LATERAL (SELECT CASE WHEN ts.metadata IS NULL OR ts.metadata = 'null' OR ts.metadata = '' THEN '{}'::jsonb ELSE ts.metadata::jsonb END AS meta OFFSET 0) mm
		CROSS JOIN LATERAL (SELECT
				CASE WHEN jsonb_typeof(mm.meta -> 'acp_model_state' -> 'models') = 'array'
					THEN mm.meta -> 'acp_model_state' -> 'models' ELSE '[]'::jsonb END AS models,
				CASE WHEN mm.meta -> 'acp_model_state' ->> 'settings_policy' = 'provider_restored'
					THEN COALESCE(mm.meta -> 'acp_model_state' ->> 'current_model_id', '')
					ELSE COALESCE(NULLIF(mm.meta -> 'runtime_config_overrides' ->> 'model', ''),
						NULLIF(mm.meta -> 'runtime_config' ->> 'model', ''),
						mm.meta -> 'acp_model_state' ->> 'current_model_id', '')
				END AS model_id OFFSET 0) mi
		CROSS JOIN LATERAL (SELECT ts.agent_profile_snapshot::jsonb AS prof, ts.repository_snapshot::jsonb AS repo OFFSET 0) ms`

// postgresSessionSummaryModelColumns returns the model label columns that
// reference the lateral aliases defined by postgresSessionSummaryLateralFrom.
// The model_id CASE is evaluated once per row instead of once per candidate
// element of the models array.
func postgresSessionSummaryModelColumns() (string, string) {
	modelName := `(SELECT choice->>'name' FROM jsonb_array_elements(mi.models) choice WHERE choice->>'model_id' = mi.model_id LIMIT 1)`
	return "mi.model_id", modelName
}

// sessionSummaryModelColumns returns the model label columns for the SQLite
// path, where json_extract parses the raw text in place. The Postgres path
// uses postgresSessionSummaryModelColumns against the lateral aliases instead.
func sessionSummaryModelColumns(metadata string) (string, string) {
	modelID := "COALESCE(NULLIF(" + dialect.JSONExtractPath(dialect.SQLite3, metadata, "runtime_config_overrides", "model") + ", ''), NULLIF(" +
		dialect.JSONExtractPath(dialect.SQLite3, metadata, "runtime_config", "model") + ", ''), " +
		dialect.JSONExtractPath(dialect.SQLite3, metadata, "acp_model_state", "current_model_id") + ", '')"
	modelID = "CASE WHEN " + dialect.JSONExtractPath(dialect.SQLite3, metadata, "acp_model_state", "settings_policy") + " = 'provider_restored' THEN COALESCE(" + dialect.JSONExtractPath(dialect.SQLite3, metadata, "acp_model_state", "current_model_id") + ", '') ELSE " + modelID + " END"

	choices := "CASE WHEN json_type(" + metadata + ", '$.acp_model_state.models') = 'array' THEN json_extract(" + metadata + ", '$.acp_model_state.models') ELSE '[]' END"
	return modelID, "(SELECT CASE WHEN choice.type = 'object' THEN json_extract(choice.value, '$.name') END FROM json_each(" + choices + ") choice WHERE CASE WHEN choice.type = 'object' THEN json_extract(choice.value, '$.model_id') END = " + modelID + " LIMIT 1)"
}
