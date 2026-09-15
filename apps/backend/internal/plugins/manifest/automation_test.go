package manifest

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutomationConditionsRequireExplicitPublicContract(t *testing.T) {
	c := AutomationCondition{Key: "push", Access: "public", ConfigVersion: 1, Label: "Push", ConfigSchema: map[string]any{"type": "object"}}
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

func TestAutomationDefaultsValidateOnlySuppliedValues(t *testing.T) {
	c := AutomationCondition{Key: "push", Access: "public", ConfigVersion: 1, Label: "Push", ConfigSchema: map[string]any{"type": "object", "properties": map[string]any{"repository": map[string]any{"type": "string"}, "event": map[string]any{"type": "string", "enum": []any{"push"}}}, "required": []any{"repository"}}}
	for _, defaults := range []map[string]any{nil, {}, {"repository": ""}, {"repository": "repo", "event": "push"}} {
		c.DefaultConfig = defaults
		require.True(t, validAutomationDefaults(c))
	}
	for _, defaults := range []map[string]any{{"unknown": "x"}, {"event": "merge"}, {"repository": true}} {
		c.DefaultConfig = defaults
		require.False(t, validAutomationDefaults(c))
		m := Manifest{AutomationConditions: []AutomationCondition{c}}
		require.NotEmpty(t, m.validateAutomationConditions())
	}
}
