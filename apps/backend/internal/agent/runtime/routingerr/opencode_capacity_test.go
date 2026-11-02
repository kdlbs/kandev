package routingerr

import (
	"net/http"
	"testing"
	"time"
)

const openCodeAdmissionRefusedMessage = "Upstream request failed: [ttft_admission_refused] the cell is at capacity"

// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11
func TestOpenCodeAdmissionCapacityClassification(t *testing.T) {
	resetInjection()
	for _, message := range []string{
		"Internal error: " + openCodeAdmissionRefusedMessage + "; retry shortly (after 21 s)",
		"AI_APICallError: " + openCodeAdmissionRefusedMessage + "; retry shortly (after 21 s)",
		"Error: " + openCodeAdmissionRefusedMessage,
		openCodeAdmissionRefusedMessage,
	} {
		t.Run(message, func(t *testing.T) {
			e := Classify(Input{Phase: PhasePromptSend, ProviderID: "opencode-acp", Stderr: message})
			if e.Code != CodeModelCapacity || e.Confidence != ConfHigh || e.Class != ClassTransient {
				t.Fatalf("classification = %+v, want high-confidence transient model_capacity", e)
			}
			if e.ClassifierRule != RuleOpenCodeCapacityAdmission {
				t.Fatalf("classifier rule = %q, want %q", e.ClassifierRule, RuleOpenCodeCapacityAdmission)
			}
			if !e.AutoRetryable || !e.FallbackAllowed || e.UserAction {
				t.Fatalf("capacity recovery flags violated: %+v", e)
			}
		})
	}
}

// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11
func TestOpenCodeAdmissionCapacityRejectsUnrelatedText(t *testing.T) {
	resetInjection()
	for _, message := range []string{
		"the cell is at capacity",
		"Upstream request failed: the cell is at capacity",
		`The log said "Upstream request failed: [ttft_admission_refused] the cell is at capacity"`,
		"The agent explained: Upstream request failed: [ttft_admission_refused] the cell is at capacity",
		"Upstream request failed: [ttft_admission_denied] the cell is at capacity",
		"Upstream request failed: [ttft_admission_refused] the cell is unavailable",
	} {
		if e := Classify(Input{Phase: PhasePromptSend, ProviderID: "opencode-acp", Stderr: message}); e.ClassifierRule == RuleOpenCodeCapacityAdmission {
			t.Fatalf("%q acquired the admission-refusal rule: %+v", message, e)
		}
	}
}

// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11
func TestOpenCodeAdmissionCapacityUnrelatedProvider(t *testing.T) {
	resetInjection()
	const message = "Internal error: " + openCodeAdmissionRefusedMessage + "; retry shortly (after 21 s)"
	for _, provider := range []string{"codex-acp", "claude-acp"} {
		if e := Classify(Input{Phase: PhasePromptSend, ProviderID: provider, Stderr: message}); e.ClassifierRule == RuleOpenCodeCapacityAdmission {
			t.Fatalf("provider %q acquired the OpenCode admission rule: %+v", provider, e)
		}
	}
}

// Structured and account-limit precedence must stay ahead of the new signature.
//
// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11
func TestOpenCodeAdmissionCapacityKeepsExistingPrecedence(t *testing.T) {
	resetInjection()
	const message = openCodeAdmissionRefusedMessage + "; retry shortly (after 21 s)"

	e := Classify(Input{Phase: PhasePromptSend, ProviderID: "opencode-acp", HTTPStatus: http.StatusTooManyRequests, Stderr: message})
	if e.ClassifierRule != "http.429" || e.Code != CodeRateLimited {
		t.Fatalf("structured status lost precedence: %+v", e)
	}

	e = Classify(Input{Phase: PhasePromptSend, ProviderID: "opencode-acp", Stderr: "Go usage limit exceeded\n" + message})
	if e.ClassifierRule != "opencode.acp.go_usage_limit.v1" {
		t.Fatalf("account-limit rule lost precedence: %+v", e)
	}
}

// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11
func TestOpenCodeAdmissionCapacityResetHint(t *testing.T) {
	resetInjection()
	observedAt := time.Date(2026, time.October, 28, 22, 27, 49, 0, time.UTC)
	tests := []struct {
		name      string
		message   string
		wantHint  time.Duration
		wantReset bool
	}{
		{"valid hint", openCodeAdmissionRefusedMessage + "; retry shortly (after 21 s)", 21 * time.Second, true},
		{"minimum hint", openCodeAdmissionRefusedMessage + "; retry shortly (after 1 s)", time.Second, true},
		{"maximum hint", openCodeAdmissionRefusedMessage + "; retry shortly (after 60 s)", time.Minute, true},
		{"zero hint", openCodeAdmissionRefusedMessage + "; retry shortly (after 0 s)", 0, false},
		{"negative hint", openCodeAdmissionRefusedMessage + "; retry shortly (after -5 s)", 0, false},
		{"above maximum", openCodeAdmissionRefusedMessage + "; retry shortly (after 61 s)", 0, false},
		{"unbounded digits", openCodeAdmissionRefusedMessage + "; retry shortly (after 99999999999 s)", 0, false},
		{"missing unit", openCodeAdmissionRefusedMessage + "; retry shortly (after 21)", 0, false},
		{"missing suffix", openCodeAdmissionRefusedMessage, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{Phase: PhasePromptSend, ProviderID: "opencode-acp", OccurredAt: observedAt, Stderr: tc.message})
			if e.Code != CodeModelCapacity || e.ClassifierRule != RuleOpenCodeCapacityAdmission {
				t.Fatalf("classification = %+v, want capacity even when the hint is unusable", e)
			}
			if !tc.wantReset {
				if e.ResetHint != nil {
					t.Fatalf("ResetHint = %v, want none", e.ResetHint)
				}
				return
			}
			if e.ResetHint == nil || !e.ResetHint.Equal(observedAt.Add(tc.wantHint)) {
				t.Fatalf("ResetHint = %v, want %v", e.ResetHint, observedAt.Add(tc.wantHint))
			}
		})
	}
}

// A relative hint belongs to the admission-refusal signature only; other
// OpenCode rules must keep their own reset behavior.
//
// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11
func TestOpenCodeAdmissionCapacityHintStaysRuleScoped(t *testing.T) {
	resetInjection()
	observedAt := time.Date(2026, time.October, 28, 22, 27, 49, 0, time.UTC)
	e := Classify(Input{
		Phase: PhasePromptSend, ProviderID: "opencode-acp", OccurredAt: observedAt,
		Stderr: "AI_APICallError: Rate limit exceeded: free-models-per-min (after 21 s)",
	})
	if e.ResetHint != nil {
		t.Fatalf("non-admission rule acquired a relative reset hint: %+v", e)
	}
}

// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11
func TestParseShortResetHintAt(t *testing.T) {
	now := time.Date(2026, time.October, 28, 22, 27, 49, 0, time.UTC)
	if got := parseShortResetHintAt("no hint here", now); got != nil {
		t.Fatalf("parseShortResetHintAt without a hint = %v, want nil", got)
	}
	got := parseShortResetHintAt("retry shortly (after 24 s)", now)
	if got == nil || !got.Equal(now.Add(24*time.Second)) {
		t.Fatalf("parseShortResetHintAt = %v, want %v", got, now.Add(24*time.Second))
	}
}

// The relative suffix belongs to the diagnostic line that established the
// capacity signature, not to unrelated stderr or stdout text.
//
// @covers AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11
func TestOpenCodeAdmissionCapacityHintIgnoresUnrelatedLines(t *testing.T) {
	resetInjection()
	observedAt := time.Date(2026, time.October, 28, 22, 27, 49, 0, time.UTC)
	const noHint = "Internal error: " + openCodeAdmissionRefusedMessage
	twentyOne := 21 * time.Second
	tests := []struct {
		name   string
		stderr string
		stdout string
		want   *time.Duration
	}{
		{name: "unrelated stderr suffix", stderr: noHint + "\nquoted example (after 60 s)"},
		{name: "unrelated stdout suffix", stderr: noHint, stdout: "quoted example (after 60 s)"},
		{name: "unrelated prefix line", stderr: "quoted example (after 60 s)\n" + noHint},
		{
			name:   "malformed diagnostic hint plus unrelated valid suffix",
			stderr: noHint + "; retry shortly (after 0 s)\nquoted example (after 60 s)",
		},
		{
			name:   "valid diagnostic hint with surrounding output",
			stderr: "quoted example (after 60 s)\n" + noHint + "; retry shortly (after 21 s)\ntrailing output",
			stdout: "other output (after 60 s)",
			want:   &twentyOne,
		},
		{
			name:   "diagnostic hint in stdout",
			stderr: "ordinary stderr line",
			stdout: noHint + "; retry shortly (after 21 s)",
			want:   &twentyOne,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{
				Phase: PhasePromptSend, ProviderID: "opencode-acp", OccurredAt: observedAt,
				Stderr: tc.stderr, Stdout: tc.stdout,
			})
			if e.Code != CodeModelCapacity || e.ClassifierRule != RuleOpenCodeCapacityAdmission {
				t.Fatalf("classification = %+v, want the capacity rule", e)
			}
			if tc.want == nil {
				if e.ResetHint != nil {
					t.Fatalf("ResetHint = %v, want no unrelated hint", e.ResetHint)
				}
				return
			}
			if e.ResetHint == nil || !e.ResetHint.Equal(observedAt.Add(*tc.want)) {
				t.Fatalf("ResetHint = %v, want %v", e.ResetHint, observedAt.Add(*tc.want))
			}
		})
	}
}
