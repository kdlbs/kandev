package acpcompat

import (
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-001.1, AC-AGENTS-COPILOT-CONTEXT-TIER-001.3, AC-AGENTS-COPILOT-CONTEXT-TIER-001.4
func TestNormalizeSessionConfig_CopilotContextTier(t *testing.T) {
	wireOptions := []streams.ConfigOption{
		{Type: "select", ID: "mode", Name: "Session Mode", Category: "mode", CurrentValue: "agent"},
		{Type: "select", ID: "model", Name: "Model", Category: "model", CurrentValue: "gpt-5.4"},
		{Type: "select", ID: "reasoning_effort", Name: "Reasoning Effort", Category: "thought_level", CurrentValue: "medium"},
		{Type: "select", ID: "allow_all", Name: "Permissions", CurrentValue: "false"},
	}

	got := NormalizeSessionConfig("copilot-acp", wireOptions, nil, "gpt-5.4")

	gotIDs := make([]string, 0, len(got))
	for _, opt := range got {
		gotIDs = append(gotIDs, opt.ID)
	}
	wantIDs := []string{"mode", "model", "reasoning_effort", "context_tier", "allow_all"}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("option order = %v, want %v", gotIDs, wantIDs)
	}

	ctxOpt := got[3]
	if ctxOpt.Type != "select" || ctxOpt.Name != "Context Window" || ctxOpt.Category != "context_window" {
		t.Fatalf("unexpected context_tier metadata: %+v", ctxOpt)
	}
	if ctxOpt.CurrentValue != "default" {
		t.Fatalf("CurrentValue = %q, want %q", ctxOpt.CurrentValue, "default")
	}
	if len(ctxOpt.Options) != 2 ||
		ctxOpt.Options[0].Value != "default" || ctxOpt.Options[0].Name != "Default" ||
		ctxOpt.Options[1].Value != "long_context" || ctxOpt.Options[1].Name != "Long Context" {
		t.Fatalf("unexpected context_tier choices: %+v", ctxOpt.Options)
	}

	// Non-Copilot agents remain untouched.
	claudeGot := NormalizeSessionConfig("claude-acp", wireOptions, nil, "claude-sonnet-4-6")
	if len(claudeGot) != len(wireOptions) {
		t.Fatalf("non-Copilot options mutated: %+v", claudeGot)
	}
}

// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-001.1, AC-AGENTS-COPILOT-CONTEXT-TIER-001.3, AC-AGENTS-COPILOT-CONTEXT-TIER-001.4
func TestNormalizeCopilotConfigOptions_EdgeCases(t *testing.T) {
	t.Run("preserves native context_tier without duplicating", func(t *testing.T) {
		native := []streams.ConfigOption{
			{Type: "select", ID: "model", Name: "Model", Category: "model", CurrentValue: "gpt-5.4"},
			{
				Type:         "select",
				ID:           "context_tier",
				Name:         "Native Context",
				Category:     "context_window",
				Description:  "Native wire description.",
				CurrentValue: "long_context",
				Options: []streams.ConfigOptionValue{
					{Value: "default", Name: "Default"},
					{Value: "long_context", Name: "1M Context"},
				},
			},
		}
		got, synthesized := NormalizeCopilotConfigOptions(native, "default")
		if synthesized {
			t.Fatal("expected synthesized=false when native context_tier is present")
		}
		if len(got) != 2 || got[1].Name != "Native Context" || got[1].CurrentValue != "long_context" {
			t.Fatalf("native option mutated or duplicated: %+v", got)
		}
		if IsSynthesizedCopilotContextOption(got[1].ID, got[1].Description) {
			t.Fatal("native option must not be classified as synthesized")
		}
	})

	t.Run("inserts after model when reasoning_effort is absent and reflects activeContextTier", func(t *testing.T) {
		withoutReasoning := []streams.ConfigOption{
			{Type: "select", ID: "mode", Name: "Session Mode", Category: "mode", CurrentValue: "agent"},
			{Type: "select", ID: "model", Name: "Model", Category: "model", CurrentValue: "claude-haiku-4.5"},
			{Type: "select", ID: "allow_all", Name: "Permissions", CurrentValue: "true"},
		}
		got, synthesized := NormalizeCopilotConfigOptions(withoutReasoning, CopilotContextTierLong)
		if !synthesized {
			t.Fatal("expected synthesized=true")
		}
		gotIDs := []string{got[0].ID, got[1].ID, got[2].ID, got[3].ID}
		wantIDs := []string{"mode", "model", "context_tier", "allow_all"}
		if !slices.Equal(gotIDs, wantIDs) {
			t.Fatalf("option order = %v, want %v", gotIDs, wantIDs)
		}
		if got[2].CurrentValue != CopilotContextTierLong {
			t.Fatalf("CurrentValue = %q, want %q", got[2].CurrentValue, CopilotContextTierLong)
		}
		if !IsSynthesizedCopilotContextOption(got[2].ID, got[2].Description) {
			t.Fatal("synthesized option must be classified as synthesized")
		}
	})
}

// @covers AC-AGENTS-COPILOT-CONTEXT-TIER-002.1, AC-AGENTS-COPILOT-CONTEXT-TIER-002.2
func TestParseCopilotContextTierFromArgs(t *testing.T) {
	cases := []struct {
		name         string
		args         []string
		wantTier     string
		wantExplicit bool
		wantHasFlag  bool
	}{
		{
			name:         "no context flag defaults to default",
			args:         []string{"--yes", "@github/copilot@1.0.114", "--acp"},
			wantTier:     CopilotContextTierDefault,
			wantExplicit: false,
			wantHasFlag:  false,
		},
		{
			name:         "space-separated long_context",
			args:         []string{"--acp", "--context", "long_context", "--allow-all-tools"},
			wantTier:     CopilotContextTierLong,
			wantExplicit: true,
			wantHasFlag:  true,
		},
		{
			name:         "equals-separated long_context after wrapper separator",
			args:         []string{"--", "npx", "@github/copilot", "--acp", "--context=long_context"},
			wantTier:     CopilotContextTierLong,
			wantExplicit: true,
			wantHasFlag:  true,
		},
		{
			name:         "last valid flag wins",
			args:         []string{"--acp", "--context", "long_context", "--context=default"},
			wantTier:     CopilotContextTierDefault,
			wantExplicit: true,
			wantHasFlag:  true,
		},
		{
			name:         "custom flag value counts as explicit CLI flag",
			args:         []string{"--acp", "--context", "experimental_tier"},
			wantTier:     CopilotContextTierDefault,
			wantExplicit: false,
			wantHasFlag:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotTier, gotExplicit := ParseCopilotContextTierFromArgs(tc.args)
			if gotTier != tc.wantTier || gotExplicit != tc.wantExplicit {
				t.Fatalf("ParseCopilotContextTierFromArgs(%v) = (%q, %v), want (%q, %v)",
					tc.args, gotTier, gotExplicit, tc.wantTier, tc.wantExplicit)
			}
			if gotHas := HasCopilotContextCLIFlag(tc.args); gotHas != tc.wantHasFlag {
				t.Fatalf("HasCopilotContextCLIFlag(%v) = %v, want %v", tc.args, gotHas, tc.wantHasFlag)
			}
		})
	}
}
