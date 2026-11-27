package main

import (
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// mockUsageDirective makes the prompt result report token usage, as real
// agents such as OpenCode do at the end of every turn.
const mockUsageDirective = "/with-usage"

func mockPromptUsage(prompt string) *acp.Usage {
	if !strings.Contains(prompt, mockUsageDirective) {
		return nil
	}
	return &acp.Usage{InputTokens: 120, OutputTokens: 30, TotalTokens: 150}
}
