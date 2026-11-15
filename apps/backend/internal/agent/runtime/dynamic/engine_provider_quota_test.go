package dynamic

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
)

// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.6
// @covers AC-AGENTS-CLAUDE-SESSION-LIMIT-001.11
// @covers AC-AGENTS-CODEX-USAGE-LIMIT-001.7
func TestEngineRoutesProviderQuotaNotices(t *testing.T) {
	const sharedBinding = "credential:shared"
	observedAt := time.Date(2026, time.November, 9, 9, 10, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		provider string
		notice   string
	}{
		{
			name: "claude five-hour", provider: "claude-acp",
			notice: "Internal error: You've hit your 5-hour limit · resets 12:50pm (Europe/Berlin)",
		},
		{
			name: "claude usage", provider: "claude-acp",
			notice: "You've hit your usage limit",
		},
		{
			name: "codex workspace credits", provider: "codex-acp",
			notice: "Your workspace is out of credits. Add credits to continue.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := routingerr.Classify(routingerr.Input{
				Phase: routingerr.PhasePromptSend, ProviderID: tc.provider,
				Stderr: tc.notice, OccurredAt: observedAt,
			})
			if failure.Code != routingerr.CodeQuotaLimited || failure.Class != routingerr.ClassHard ||
				!failure.FallbackAllowed || !failure.AutoRetryable {
				t.Fatalf("classification = %+v, want hard fallback-eligible quota", failure)
			}

			profile := Profile{
				ID: "dynamic-provider-quota", Version: 1,
				Candidates: []Candidate{
					{ID: "primary", Enabled: true, BindingKey: sharedBinding, Policies: routingpolicy.DefaultDocument()},
					{ID: "sibling", Enabled: true, BindingKey: sharedBinding, Policies: routingpolicy.DefaultDocument()},
					{ID: "healthy", Enabled: true, BindingKey: "credential:other", Policies: routingpolicy.DefaultDocument()},
				},
			}
			now := observedAt
			engine := NewEngine(WithClock(func() time.Time { return now }))
			initial, err := engine.Select("provider-quota", profile, 0, "")
			if err != nil || initial.ExecutionProfileID != "primary" {
				t.Fatalf("initial selection = %#v, %v; want primary", initial, err)
			}

			decision, err := engine.ApplyFailure(
				"provider-quota", profile, initial.Generation, initial.ExecutionProfileID, failure,
			)
			if err != nil {
				t.Fatalf("ApplyFailure: %v", err)
			}
			if decision.ExecutionProfileID != "healthy" || decision.Reason != "policy_skip" {
				t.Fatalf("decision = %#v, want the healthy binding after shared siblings are skipped", decision)
			}
			if !engine.Circuits().IsOpen(sharedBinding, now) {
				t.Fatal("shared credential circuit is not open before its deadline")
			}

			otherSession, err := engine.Select("provider-quota-other", profile, 0, "")
			if err != nil || otherSession.ExecutionProfileID != "healthy" {
				t.Fatalf("selection while shared circuit is open = %#v, %v; want healthy", otherSession, err)
			}

			suspension, blocked := engine.Circuits().ActiveSuspension(sharedBinding, now)
			if !blocked || suspension.Until.IsZero() {
				t.Fatalf("shared circuit suspension = %+v, want a timed deadline", suspension)
			}
			now = suspension.Until
			probe, err := engine.Select("provider-quota-probe", profile, 0, "")
			if err != nil || probe.ExecutionProfileID != "primary" {
				t.Fatalf("selection at deadline = %#v, %v; want primary probe", probe, err)
			}
			whileProbeHeld, err := engine.Select("provider-quota-sibling", profile, 0, "")
			if err != nil || whileProbeHeld.ExecutionProfileID != "healthy" {
				t.Fatalf("selection while probe is held = %#v, %v; want healthy", whileProbeHeld, err)
			}
			if !engine.Circuits().IsOpen(sharedBinding, now) {
				t.Fatal("shared circuit closed before the probe succeeded")
			}
			engine.ReleaseProbe(probe, true)
			if engine.Circuits().IsOpen(sharedBinding, now) {
				t.Fatal("shared circuit remained open after a successful probe")
			}
		})
	}
}
