package backendapp

import (
	"encoding/json"

	shared "github.com/kandev/kandev/internal/orchestration/models"
)

func nativeAssistantCapabilities(workspace string) []shared.Capability {
	operations := []struct {
		name, effect string
		fields       []string
	}{
		{"workspace.catalog", capabilityEffectRead, nil}, {"task.details", capabilityEffectRead, []string{taskIDPayloadKey}},
		{"context.read", capabilityEffectRead, []string{"objective_id", "profile_id"}}, {"memory.read", capabilityEffectRead, nil},
		{"capabilities.read", capabilityEffectRead, nil}, {"task.create", capabilityEffectWrite, []string{"objective_id", "context_ref", "workflow_id", capabilityTitleKey}},
		{"task.message", capabilityEffectWrite, []string{taskIDPayloadKey, capabilityMessageKey, "context_ref"}}, {"task.start", capabilityEffectWrite, []string{taskIDPayloadKey}},
		{"task.stop", capabilityEffectWrite, []string{taskIDPayloadKey}}, {"task.status", capabilityEffectWrite, []string{taskIDPayloadKey, statusKey}},
		{"objective.write", "receipt", []string{"operation_id"}}, {"conversation.reply", "receipt", []string{"body"}},
		{"memory.write", capabilityEffectWrite, []string{"key", workspaceResultContentKey}},
	}
	rows := make([]shared.Capability, 0, len(operations))
	for _, op := range operations {
		entry := catalogCapability("native", op.name, op.name, workspace, "1")
		properties := map[string]any{}
		for _, field := range op.fields {
			properties[field] = map[string]string{capabilitySchemaType: "string"}
		}
		entry.InputSchema, _ = json.Marshal(map[string]any{capabilitySchemaType: "object", "properties": properties})
		entry.SchemaPartial = true
		entry.Effect, entry.Health, entry.Reason = op.effect, capabilityReady, "native_catalog"
		rows = append(rows, entry)
	}
	return rows
}
