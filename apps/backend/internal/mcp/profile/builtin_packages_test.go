package profile

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-BUILTIN-MCP-003.2
func TestBuiltinManifestCannotDisappearDuringNormalization(t *testing.T) {
	raw := []byte(`{"surface":"kanban-task","builtin_packages":{"version":1,"catalog_version":1,"workspace_id":"workspace","task_id":"task","surface":"kanban-task","workspace_revision":0,"task_revision":0,"packages":["core"],"origins":{"core":"required"}}}`)
	var ctx Context
	require.NoError(t, json.Unmarshal(raw, &ctx))
	encoded, err := json.Marshal(Normalize(ctx))
	require.NoError(t, err)
	var result map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &result))
	require.Contains(t, result, "builtin_packages", "normalization must retain the execution's package restrictions")
}

// @covers AC-AGENTS-BUILTIN-MCP-002.3
func TestBuiltinManifestValidation(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		value       any
	}{
		{"unknown version", "version", 2},
		{"unknown catalog", "catalog_version", 2},
		{"missing workspace", "workspace_id", ""},
		{"missing task", "task_id", ""},
		{"negative revision", "task_revision", -1},
		{"core omitted", "packages", []string{"diagnostics"}},
		{"duplicate", "packages", []string{"core", "core"}},
		{"unknown package", "packages", []string{"core", "all"}},
		{"fixed surface", "surface", "automation"},
		{"configuration missing", "surface", "configuration"},
		{"unknown field", "arbitrary_grant", true},
		{"missing origin", "origins", map[string]string{}},
		{"invalid origin", "origins", map[string]string{"core": "task"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{"version": 1, "catalog_version": 1, "workspace_id": "workspace", "task_id": "task", "surface": "kanban-task", "workspace_revision": 0, "task_revision": 0, "packages": []string{"core"}, "origins": map[string]string{"core": "required"}}
			payload[tc.field] = tc.value
			raw, err := json.Marshal(map[string]any{"surface": "kanban-task", "builtin_packages": payload})
			require.NoError(t, err)
			var profile Context
			require.Error(t, json.Unmarshal(raw, &profile))
		})
	}
}
