package modelsdev_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/costs/modelsdev"
)

// TestNormalize walks every distinct modelId observed across the
// captured ACP probes at /tmp/acp-probe-*.jsonl. New CLIs and
// re-released probes must extend this corpus when their ids change
// shape.
func TestNormalize(t *testing.T) {
	tests := []struct {
		raw          string
		wantKey      string
		wantStrategy modelsdev.Strategy
		note         string
	}{
		// claude-acp probes — logical aliases only.
		{"default", "default", modelsdev.StrategySkip, "claude-acp alias"},
		{"sonnet", "sonnet", modelsdev.StrategySkip, "claude-acp alias"},
		{"sonnet[1m]", "sonnet[1m]", modelsdev.StrategySkip, "claude-acp 1m variant"},
		{"haiku", "haiku", modelsdev.StrategySkip, "claude-acp alias"},
		{"opus", "opus", modelsdev.StrategySkip, "claude-acp alias"},

		// codex-acp probes — <model>/<effort> form.
		{"gpt-5.1-codex-max/high", "gpt-5.1-codex-max", modelsdev.StrategyLookup, "codex-acp"},
		{"gpt-5.2-codex/xhigh", "gpt-5.2-codex", modelsdev.StrategyLookup, "codex-acp"},
		{"gpt-5.4-mini/medium", "gpt-5.4-mini", modelsdev.StrategyLookup, "codex-acp"},
		{"gpt-5.5", "gpt-5.5", modelsdev.StrategyLookup, "codex-acp top-level"},
		{"gpt-5.2/low", "gpt-5.2", modelsdev.StrategyLookup, "codex-acp"},
		{"gpt-5.3-codex/medium", "gpt-5.3-codex", modelsdev.StrategyLookup, "codex-acp"},

		// opencode-acp probes — <route>/<model>[/<effort>].
		{"github-copilot/claude-haiku-4.5", "claude-haiku-4.5", modelsdev.StrategyLookup, "opencode route stripped"},
		{"github-copilot/claude-haiku-4.5/high", "claude-haiku-4.5", modelsdev.StrategyLookup, "opencode route + effort"},
		{"github-copilot/gpt-5-mini/medium", "gpt-5-mini", modelsdev.StrategyLookup, "opencode route + effort"},
		{"openai/gpt-4.1", "gpt-4.1", modelsdev.StrategyLookup, "opencode openai route"},
		{"openai/gpt-5-codex", "gpt-5-codex", modelsdev.StrategyLookup, "opencode openai route"},
		{"openai/gpt-4o-2024-08-06", "gpt-4o-2024-08-06", modelsdev.StrategyLookup, "opencode dated model"},

		// gemini probes — mostly canonical; auto-* is a router.
		{"gemini-2.5-pro", "gemini-2.5-pro", modelsdev.StrategyLookup, "gemini canonical"},
		{"gemini-2.5-flash", "gemini-2.5-flash", modelsdev.StrategyLookup, "gemini canonical"},
		{"gemini-3-flash-preview", "gemini-3-flash-preview", modelsdev.StrategyLookup, "gemini preview"},
		{"auto-gemini-2.5", "auto-gemini-2.5", modelsdev.StrategyEstimated, "gemini auto-router"},
		{"auto-gemini-3", "auto-gemini-3", modelsdev.StrategyEstimated, "gemini auto-router"},

		// copilot-acp — usually canonical, or "auto".
		{"claude-haiku-4.5", "claude-haiku-4.5", modelsdev.StrategyLookup, "copilot canonical"},
		{"gpt-5-mini", "gpt-5-mini", modelsdev.StrategyLookup, "copilot canonical"},
		{"gpt-4.1", "gpt-4.1", modelsdev.StrategyLookup, "copilot canonical"},
		{"auto", "auto", modelsdev.StrategyEstimated, "copilot auto-router"},

		// auggie probes — canonical with optional context-length suffix
		// and a few proprietary ids.
		{"claude-opus-4-7", "claude-opus-4-7", modelsdev.StrategyLookup, "auggie canonical"},
		{"claude-opus-4-7-500k", "claude-opus-4-7", modelsdev.StrategyLookup, "auggie 500k tier stripped"},
		{"claude-sonnet-4-6-500k", "claude-sonnet-4-6", modelsdev.StrategyLookup, "auggie 500k tier stripped"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6", modelsdev.StrategyLookup, "auggie canonical"},
		{"butler_a", "butler_a", modelsdev.StrategyLookup, "auggie proprietary id"},
		{"butler_b", "butler_b", modelsdev.StrategyLookup, "auggie proprietary id"},
		{"kimi-k2p6", "kimi-k2p6", modelsdev.StrategyLookup, "auggie proprietary id"},
		{"gpt-5-1", "gpt-5-1", modelsdev.StrategyLookup, "auggie canonical"},
		{"gpt-5", "gpt-5", modelsdev.StrategyLookup, "auggie canonical"},
		{"gemini-3-1-pro-preview", "gemini-3-1-pro-preview", modelsdev.StrategyLookup, "auggie canonical"},

		// Edge cases.
		{"", "", modelsdev.StrategyEstimated, "empty"},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			gotKey, gotStrategy := modelsdev.Normalize(tc.raw)
			if gotKey != tc.wantKey {
				t.Errorf("Normalize(%q) key = %q, want %q (%s)",
					tc.raw, gotKey, tc.wantKey, tc.note)
			}
			if gotStrategy != tc.wantStrategy {
				t.Errorf("Normalize(%q) strategy = %d, want %d (%s)",
					tc.raw, gotStrategy, tc.wantStrategy, tc.note)
			}
		})
	}
}
