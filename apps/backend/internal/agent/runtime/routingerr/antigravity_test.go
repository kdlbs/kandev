package routingerr

import (
	"testing"
	"time"
)

func TestAntigravityErrorClassification(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	shortReset := now.Add(30 * time.Second)
	longReset := now.Add(2 * time.Hour)

	cases := []struct {
		name      string
		failure   AntigravityFailure
		wantCode  Code
		wantConf  Confidence
		wantReset bool
		kanban    RecoveryDecision
		office    RecoveryDecision
	}{
		{
			name:     "structured authentication",
			failure:  AntigravityFailure{HTTPStatus: 401, Status: "UNAUTHENTICATED", Message: "Request had invalid authentication credentials."},
			wantCode: CodeAuthRequired, wantConf: ConfHigh,
			kanban: DecisionManual, office: DecisionFallback,
		},
		{
			name:     "message-only authentication",
			failure:  AntigravityFailure{Message: "Antigravity authentication required"},
			wantCode: CodeAuthRequired, wantConf: ConfMedium,
			kanban: DecisionManual, office: DecisionFallback,
		},
		{
			name: "generic -32000 with cached auth methods stays unknown",
			failure: AntigravityFailure{
				RPCCode: -32000, AuthMethodsCached: true, Message: "Internal error",
			},
			wantCode: CodeUnknownProvider, wantConf: ConfLow,
			kanban: DecisionManual, office: DecisionPark,
		},
		{
			name: "generic -32000 with real auth wording is authentication",
			failure: AntigravityFailure{
				RPCCode: -32000, AuthMethodsCached: true, Message: "Authentication required",
			},
			wantCode: CodeAuthRequired, wantConf: ConfMedium,
			kanban: DecisionManual, office: DecisionFallback,
		},
		{
			name:     "subscription entitlement",
			failure:  AntigravityFailure{HTTPStatus: 403, Status: "PERMISSION_DENIED", Message: "Your Google AI subscription is not active."},
			wantCode: CodeSubscriptionRequired, wantConf: ConfHigh,
			kanban: DecisionManual, office: DecisionFallback,
		},
		{
			name:     "model access",
			failure:  AntigravityFailure{HTTPStatus: 404, Status: "NOT_FOUND", Message: "Model gemini-3 is not available for this account."},
			wantCode: CodeModelUnavailable, wantConf: ConfHigh,
			kanban: DecisionManual, office: DecisionFallback,
		},
		{
			name: "quota with provider reset",
			failure: AntigravityFailure{
				HTTPStatus: 429, Status: "RESOURCE_EXHAUSTED", Reason: "QUOTA_EXHAUSTED",
				Message: "You have reached your quota.", ResetHint: &shortReset,
			},
			wantCode: CodeQuotaLimited, wantConf: ConfHigh, wantReset: true,
			kanban: DecisionManual, office: DecisionLongRetry,
		},
		{
			name: "quota with provider delay",
			failure: AntigravityFailure{
				Reason: "QUOTA_EXHAUSTED", Message: "Quota reached.",
				ResetDelay: longReset.Sub(now), OccurredAt: now,
			},
			wantCode: CodeQuotaLimited, wantConf: ConfHigh, wantReset: true,
			kanban: DecisionManual, office: DecisionLongRetry,
		},
		{
			name:     "quota refusal without reset",
			failure:  AntigravityFailure{QuotaRefusal: true},
			wantCode: CodeQuotaLimited, wantConf: ConfHigh,
			kanban: DecisionManual, office: DecisionLongRetry,
		},
		{
			name:     "explicit quota token without reset",
			failure:  AntigravityFailure{QuotaExhausted: true},
			wantCode: CodeQuotaLimited, wantConf: ConfHigh,
			kanban: DecisionManual, office: DecisionLongRetry,
		},
		{
			name:     "quota without reset stays unknown",
			failure:  AntigravityFailure{Reason: "QUOTA_EXHAUSTED", Message: "Quota reached."},
			wantCode: CodeQuotaLimited, wantConf: ConfHigh,
			kanban: DecisionManual, office: DecisionLongRetry,
		},
		{
			name:     "transient throttling without reset",
			failure:  AntigravityFailure{HTTPStatus: 429, Message: "Too many requests."},
			wantCode: CodeRateLimited, wantConf: ConfHigh,
			kanban: DecisionShortRetry, office: DecisionLongRetry,
		},
		{
			name: "transient throttling with short reset",
			failure: AntigravityFailure{
				HTTPStatus: 429, Status: "RESOURCE_EXHAUSTED",
				Message: "Rate limit exceeded.", ResetHint: &shortReset,
			},
			wantCode: CodeRateLimited, wantConf: ConfHigh, wantReset: true,
			kanban: DecisionShortRetry, office: DecisionShortRetry,
		},
		{
			name:     "provider failure",
			failure:  AntigravityFailure{HTTPStatus: 503, Status: "UNAVAILABLE", Message: "Service unavailable."},
			wantCode: CodeProviderUnavailable, wantConf: ConfHigh,
			kanban: DecisionShortRetry, office: DecisionShortRetry,
		},
		{
			name:     "provider overloaded",
			failure:  AntigravityFailure{HTTPStatus: 529},
			wantCode: CodeProviderOverloaded, wantConf: ConfHigh,
			kanban: DecisionShortRetry, office: DecisionShortRetry,
		},
		{
			name:     "unknown provider failure",
			failure:  AntigravityFailure{Message: "Something unexpected happened."},
			wantCode: CodeUnknownProvider, wantConf: ConfLow,
			kanban: DecisionManual, office: DecisionPark,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			classified := ClassifyAntigravity(tc.failure)
			if classified.Code != tc.wantCode {
				t.Fatalf("code = %s (rule %s), want %s", classified.Code, classified.ClassifierRule, tc.wantCode)
			}
			if classified.Confidence != tc.wantConf {
				t.Fatalf("confidence = %s, want %s", classified.Confidence, tc.wantConf)
			}
			if tc.wantReset && classified.ResetHint == nil {
				t.Fatal("reset hint = nil, want provider evidence")
			}
			if !tc.wantReset && classified.ResetHint != nil {
				t.Fatalf("reset hint = %v, want unknown", classified.ResetHint)
			}
			if tc.failure.ResetHint != nil && !classified.ResetHint.Equal(*tc.failure.ResetHint) {
				t.Fatalf("reset hint = %v, want %v", classified.ResetHint, tc.failure.ResetHint)
			}
			if tc.failure.ResetDelay > 0 {
				want := tc.failure.OccurredAt.Add(tc.failure.ResetDelay)
				if !classified.ResetHint.Equal(want) {
					t.Fatalf("reset hint = %v, want %v", classified.ResetHint, want)
				}
			}
			if got := Decide(ContextKanban, classified, now); got != tc.kanban {
				t.Errorf("Kanban decision = %q, want %q", got, tc.kanban)
			}
			if got := Decide(ContextOffice, classified, now); got != tc.office {
				t.Errorf("Office decision = %q, want %q", got, tc.office)
			}
		})
	}
}

func TestAntigravityGenericRPCCodeIsNotAuthEvidence(t *testing.T) {
	classified := ClassifyAntigravity(AntigravityFailure{
		RPCCode: -32000, AuthMethodsCached: true, Message: "Internal error",
	})
	if classified.Code == CodeAuthRequired {
		t.Fatalf("generic -32000 with cached auth methods classified as %s via %s", classified.Code, classified.ClassifierRule)
	}
	if classified.Code != CodeUnknownProvider {
		t.Fatalf("generic -32000 classified as %s, want unknown_provider_error", classified.Code)
	}
	if IsAuthenticationFailureDiagnostic(classified.RawExcerpt) {
		t.Fatalf("normalized unknown excerpt %q is still authentication evidence", classified.RawExcerpt)
	}
}

func TestAntigravityNormalizedMessagesReclassify(t *testing.T) {
	cases := []struct {
		code   Code
		detail string
	}{
		{CodeAuthRequired, "Authentication required"},
		{CodeSubscriptionRequired, "Your subscription is not active."},
		{CodeModelUnavailable, "Model gemini-3 is unavailable."},
		{CodeQuotaLimited, "You have reached your quota."},
		{CodeRateLimited, "Too many requests."},
		{CodeProviderOverloaded, "The provider is overloaded."},
		{CodeProviderUnavailable, "Service unavailable."},
		{CodeUnknownProvider, "Something unexpected happened."},
	}
	for _, tc := range cases {
		message := AntigravityFailureMessage(tc.code, tc.detail)
		classified := Classify(Input{
			Phase:      PhasePromptSend,
			ProviderID: "antigravity-acp",
			Stderr:     message,
		})
		if classified.Code != tc.code {
			t.Errorf("%s message %q reclassified as %s via %s", tc.code, message, classified.Code, classified.ClassifierRule)
		}
	}

	unverified := Classify(Input{
		Phase:      PhasePromptSend,
		ProviderID: "antigravity-acp",
		Stderr:     AntigravityOutcomeUnverifiedMessage,
	})
	if unverified.Code != CodeAgentRuntime {
		t.Fatalf("unverified outcome reclassified as %s, want %s", unverified.Code, CodeAgentRuntime)
	}
	if unverified.FallbackAllowed || unverified.AutoRetryable {
		t.Fatalf("unverified outcome invariants = fallback:%v retry:%v, want neither", unverified.FallbackAllowed, unverified.AutoRetryable)
	}
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	if got := Decide(ContextKanban, unverified, now); got != DecisionManual {
		t.Errorf("unverified Kanban decision = %q, want manual", got)
	}
	if got := Decide(ContextOffice, unverified, now); got != DecisionPark {
		t.Errorf("unverified Office decision = %q, want park", got)
	}
}

func TestAntigravityFailureMessagePreservesCauseAndReset(t *testing.T) {
	reset := time.Date(2026, 11, 3, 12, 30, 0, 0, time.UTC)
	classified := ClassifyAntigravity(AntigravityFailure{
		Reason: "QUOTA_EXHAUSTED", Message: "Quota reached.", ResetHint: &reset,
	})
	message := AntigravityFailureMessage(classified.Code, "Quota reached.")
	if message != "Antigravity quota reached: Quota reached." {
		t.Fatalf("message = %q, want a normalized cause with the provider detail", message)
	}
	if IsAuthenticationFailureDiagnostic(message) {
		t.Fatalf("quota message %q must not match authentication guidance", message)
	}
}
