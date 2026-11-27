package main

import "testing"

// TestMockPromptUsageFollowsDirective verifies the mock reports token usage in
// the prompt result only when the prompt asks for it, as real agents such as
// OpenCode do on every turn.
func TestMockPromptUsageFollowsDirective(t *testing.T) {
	if usage := mockPromptUsage("hello"); usage != nil {
		t.Fatalf("usage without directive = %+v, want nil", usage)
	}
	usage := mockPromptUsage("hello " + mockUsageDirective)
	if usage == nil || usage.InputTokens <= 0 || usage.OutputTokens <= 0 ||
		usage.TotalTokens != usage.InputTokens+usage.OutputTokens {
		t.Fatalf("usage with directive = %+v, want positive consistent token counts", usage)
	}
}
