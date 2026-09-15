package manifest

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutomationConditionsRequireExplicitPublicContract(t *testing.T) {
	c := AutomationCondition{Key: "push", AdapterKey: "events", Access: "public", ConfigVersion: 1, Label: "Push", ConfigSchema: map[string]any{"type": "object"}}
	m := Manifest{AutomationConditions: []AutomationCondition{c}}
	require.Empty(t, m.validateAutomationConditions())
	m.AutomationConditions[0].Access = ""
	require.NotEmpty(t, m.validateAutomationConditions())
	m.AutomationConditions = []AutomationCondition{c, c}
	require.NotEmpty(t, m.validateAutomationConditions())
}

func TestAutomationSchemaRejectsUnsupportedControls(t *testing.T) {
	for _, field := range []map[string]any{
		{"type": "string", "format": "password"}, {"type": "object"},
		{"type": "array", "items": map[string]any{"type": "object"}},
		{"type": "string", "enum": []any{map[string]any{"bad": "option"}}},
	} {
		require.False(t, validAutomationSchema(map[string]any{"type": "object", "properties": map[string]any{"field": field}}))
	}
	require.True(t, validAutomationSchema(map[string]any{"type": "object", "properties": map[string]any{"branches": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}))
}
