package models

import "context"

type assistantClarificationKey struct{}
type assistantClarificationSource struct {
	AgentID   string
	MemoryIDs []string
}

// WithAssistantClarification is set only after the assistant broker validates
// explicit question delegation and current confirmed worker-scoped context.
func WithAssistantClarification(ctx context.Context, agentID string, memoryIDs []string) context.Context {
	return context.WithValue(ctx, assistantClarificationKey{}, assistantClarificationSource{AgentID: agentID, MemoryIDs: append([]string(nil), memoryIDs...)})
}

// ApplyClarificationAttribution runs inside the native response claim. A later
// human claim clears old attribution left by an abandoned delivery attempt.
func ApplyClarificationAttribution(ctx context.Context, metadata map[string]any) {
	for _, key := range []string{"response_author_type", "response_author_id", "response_memory_ids"} {
		delete(metadata, key)
	}
	source, ok := ctx.Value(assistantClarificationKey{}).(assistantClarificationSource)
	if !ok || source.AgentID == "" || len(source.MemoryIDs) == 0 || len(source.MemoryIDs) > 20 {
		return
	}
	metadata["response_author_type"] = "agent"
	metadata["response_author_id"] = source.AgentID
	metadata["response_memory_ids"] = append([]string(nil), source.MemoryIDs...)
}
