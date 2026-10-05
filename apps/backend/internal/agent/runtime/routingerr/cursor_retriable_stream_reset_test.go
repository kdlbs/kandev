package routingerr

import (
	"strings"
	"testing"
	"time"
)

const realCursorRetriableStreamReset = "Error: RetriableError: HTTP/2 stream closed with error code CANCEL (0x8)"

const leadingCanceledCursorRetriableStreamReset = "Error: RetriableError: [canceled] HTTP/2 stream closed with error code CANCEL (0x8)"

const cursorRetriablePingTimeout = "Error: RetriableError: [unavailable] PING timed out"

const cursorRetriableConnectionStalled = "Error: RetriableError: Connection stalled"

const realCursorRetriableResourceExhausted = "Error: RetriableError: [resource_exhausted] Error"

const wantCursorRetriableStreamResetRuleID = "cursor.retriable_stream_reset.v1"
const wantCursorRetriableResourceExhaustedRuleID = "cursor.retriable_resource_exhausted.v1"
const wantCursorRetriableUnavailableRuleID = "cursor.retriable_unavailable.v1"
const wantCursorRetriableConnectionStalledRuleID = "cursor.retriable_connection_stalled.v1"

func matchCursorRuntimeEnvironmentRules(text string) (*Error, bool) {
	return matchRuntimeEnvironmentRulesForProvider(cursorRetriableProviderID, text, text)
}

func TestMatchRuntimeEnvironmentRules_CursorRetriable(t *testing.T) {
	got, ok := matchCursorRuntimeEnvironmentRules(realCursorRetriableStreamReset)
	if !ok {
		t.Fatal("expected Cursor stream-reset match")
	}
	if got.Code != CodeAgentTransportLost {
		t.Fatalf("Code = %q, want %q", got.Code, CodeAgentTransportLost)
	}
	if got.ClassifierRule != wantCursorRetriableStreamResetRuleID {
		t.Fatalf("ClassifierRule = %q, want %q", got.ClassifierRule, wantCursorRetriableStreamResetRuleID)
	}
	if got.Confidence != ConfHigh {
		t.Fatalf("Confidence = %q, want %q", got.Confidence, ConfHigh)
	}
	for _, tc := range []struct {
		message  string
		wantCode Code
		wantRule string
	}{
		{cursorRetriablePingTimeout, CodeProviderUnavailable, wantCursorRetriableUnavailableRuleID},
		{cursorRetriableConnectionStalled, CodeNetworkUnavailable, wantCursorRetriableConnectionStalledRuleID},
		{realCursorRetriableResourceExhausted, CodeProviderResourceExhausted, wantCursorRetriableResourceExhaustedRuleID},
	} {
		t.Run(tc.message, func(t *testing.T) {
			got, ok := matchCursorRuntimeEnvironmentRules(tc.message)
			if !ok {
				t.Fatalf("expected Cursor RetriableError match for %q", tc.message)
			}
			if got.Code != tc.wantCode || got.ClassifierRule != tc.wantRule {
				t.Fatalf("match for %q = %+v, want code %q with rule %q", tc.message, got, tc.wantCode, tc.wantRule)
			}
		})
	}

	for _, tc := range []struct {
		name string
		text string
	}{
		{
			name: "prose before marker",
			text: "provider said: " + realCursorRetriableStreamReset,
		},
		{
			name: "partial RetriableError marker",
			text: "Error: Retriable: HTTP/2 stream closed with error code CANCEL (0x8)",
		},
		{
			name: "empty diagnostic suffix",
			text: "Error: RetriableError:",
		},
		{
			name: "context cancellation",
			text: "context canceled: " + realCursorRetriableStreamReset,
		},
		{
			name: "underscore-delimited context cancellation",
			text: "Error: RetriableError: context canceled_by_client",
		},
		{
			name: "context deadline exceeded",
			text: "Error: RetriableError: context deadline exceeded",
		},
		{
			name: "cancel escalation",
			text: "Error: RetriableError: cancel escalated",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := matchCursorRuntimeEnvironmentRules(tc.text); ok {
				t.Fatalf("unexpected match: %+v", got)
			}
		})
	}

	withCursorCancellationToken := realCursorRetriableStreamReset + " [canceled]"
	got, ok = matchCursorRuntimeEnvironmentRules(withCursorCancellationToken)
	if !ok || got.Code != CodeAgentTransportLost {
		t.Fatalf("bracketed cancellation token classified as ok=%v err=%v, want transport loss", ok, got)
	}
}

func TestMatchRuntimeEnvironmentRules_CursorRetriableUsesAdapterBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		suffix string
		want   bool
	}{
		{name: "256 ASCII bytes", suffix: "[resource_exhausted] " + strings.Repeat("x", 256-len("[resource_exhausted] ")), want: true},
		{name: "257 ASCII bytes", suffix: "[resource_exhausted] " + strings.Repeat("x", 257-len("[resource_exhausted] ")), want: false},
		{name: "128 multibyte characters at 256 bytes", suffix: "[resource_exhausted] " + strings.Repeat("é", (256-len("[resource_exhausted] "))/2), want: true},
		{name: "129 multibyte characters over 256 bytes", suffix: "[resource_exhausted] " + strings.Repeat("é", (256-len("[resource_exhausted] "))/2+1), want: false},
		{name: "unicode whitespace only", suffix: "\u2003\u2003", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := matchCursorRuntimeEnvironmentRules("Error: RetriableError: " + tc.suffix)
			if ok != tc.want {
				t.Fatalf("matchRuntimeEnvironmentRules(%q) = %v, want %v", tc.suffix, ok, tc.want)
			}
		})
	}
}

func TestClassifyCursorRetriable(t *testing.T) {
	resetInjection()

	e := Classify(Input{
		Phase:      PhasePromptSend,
		ProviderID: "cursor-acp",
		Stderr:     realCursorRetriableStreamReset,
	})
	if e == nil {
		t.Fatal("expected non-nil Error")
	}
	if e.Code != CodeAgentTransportLost {
		t.Fatalf("Code = %q, want %q", e.Code, CodeAgentTransportLost)
	}
	if e.Class != ClassTransient {
		t.Fatalf("Class = %q, want %q", e.Class, ClassTransient)
	}
	if e.CatalogueVersion != CatalogueVersion {
		t.Fatalf("CatalogueVersion = %q, want %q", e.CatalogueVersion, CatalogueVersion)
	}
	if e.Confidence != ConfHigh {
		t.Fatalf("Confidence = %q, want %q", e.Confidence, ConfHigh)
	}
	if !e.AutoRetryable {
		t.Fatal("AutoRetryable = false, want true")
	}
	if e.FallbackAllowed {
		t.Fatal("FallbackAllowed = true, want false")
	}
	if e.UserAction {
		t.Fatal("UserAction = true, want false")
	}
	if e.Phase != PhasePromptSend {
		t.Fatalf("Phase = %q, want %q", e.Phase, PhasePromptSend)
	}

	overlapped := Classify(Input{
		Phase:  PhasePromptSend,
		Stderr: "API Error: 529 Overloaded. " + realCursorRetriableStreamReset,
	})
	if overlapped.Code != CodeProviderOverloaded {
		t.Fatalf("overlapping overload classified as %q, want %q", overlapped.Code, CodeProviderOverloaded)
	}
}

func TestClassifyCursorRetriableLeadingCanceled(t *testing.T) {
	resetInjection()

	e := Classify(Input{
		Phase:      PhasePromptSend,
		ProviderID: "cursor-acp",
		Stderr:     leadingCanceledCursorRetriableStreamReset,
	})
	if e == nil {
		t.Fatal("expected non-nil Error")
	}
	if e.Code != CodeAgentTransportLost {
		t.Fatalf("Code = %q, want %q", e.Code, CodeAgentTransportLost)
	}
	if !e.AutoRetryable {
		t.Fatal("AutoRetryable = false, want true")
	}
	if e.FallbackAllowed {
		t.Fatal("FallbackAllowed = true, want false")
	}
	if got := Decide(ContextKanban, e, time.Time{}); got != DecisionShortRetry {
		t.Fatalf("Decide(ContextKanban, ...) = %q, want %q", got, DecisionShortRetry)
	}
	if e.Class != ClassTransient {
		t.Fatalf("Class = %q, want %q", e.Class, ClassTransient)
	}
	if e.Confidence != ConfHigh {
		t.Fatalf("Confidence = %q, want %q", e.Confidence, ConfHigh)
	}
	if e.UserAction {
		t.Fatal("UserAction = true, want false")
	}
}

func TestClassifyCursorRetriableVariants(t *testing.T) {
	for _, tc := range []struct {
		message  string
		wantCode Code
		wantRule string
	}{
		{cursorRetriablePingTimeout, CodeProviderUnavailable, wantCursorRetriableUnavailableRuleID},
		{cursorRetriableConnectionStalled, CodeNetworkUnavailable, wantCursorRetriableConnectionStalledRuleID},
	} {
		t.Run(tc.message, func(t *testing.T) {
			resetInjection()
			e := Classify(Input{Phase: PhasePromptSend, ProviderID: "cursor-acp", Stderr: tc.message})
			if e.Code != tc.wantCode || e.Class != ClassTransient || !e.AutoRetryable {
				t.Fatalf("Classify(%q) = %+v, want transient auto-retryable %q", tc.message, e, tc.wantCode)
			}
			if e.ClassifierRule != tc.wantRule {
				t.Fatalf("ClassifierRule = %q, want %q", e.ClassifierRule, tc.wantRule)
			}
		})
	}
}

func TestClassifyCursorRetriableCategory(t *testing.T) {
	resetInjection()

	e := Classify(Input{
		Phase:      PhasePromptSend,
		ProviderID: "cursor-acp",
		Stderr:     realCursorRetriableResourceExhausted,
	})
	if e == nil {
		t.Fatal("expected non-nil Error")
	}
	if e.Code != CodeProviderResourceExhausted {
		t.Fatalf("Code = %q, want %q", e.Code, CodeProviderResourceExhausted)
	}
	if e.Class != ClassTransient {
		t.Fatalf("Class = %q, want %q", e.Class, ClassTransient)
	}
	if e.CatalogueVersion != CatalogueVersion {
		t.Fatalf("CatalogueVersion = %q, want %q", e.CatalogueVersion, CatalogueVersion)
	}
	if e.Confidence != ConfHigh {
		t.Fatalf("Confidence = %q, want %q", e.Confidence, ConfHigh)
	}
	if !e.AutoRetryable {
		t.Fatal("AutoRetryable = false, want true")
	}
	if e.FallbackAllowed {
		t.Fatal("FallbackAllowed = true, want false")
	}
	if e.UserAction {
		t.Fatal("UserAction = true, want false")
	}
	if e.ClassifierRule != wantCursorRetriableResourceExhaustedRuleID {
		t.Fatalf("ClassifierRule = %q, want %q", e.ClassifierRule, wantCursorRetriableResourceExhaustedRuleID)
	}
	if got := Decide(ContextKanban, e, time.Time{}); got != DecisionShortRetry {
		t.Fatalf("Decide(ContextKanban, ...) = %q, want %q", got, DecisionShortRetry)
	}
	if got := Decide(ContextOffice, e, time.Time{}); got != DecisionShortRetry {
		t.Fatalf("Decide(ContextOffice, ...) = %q, want %q", got, DecisionShortRetry)
	}
	if !IsTransientProviderErrorForProvider("cursor-acp", realCursorRetriableResourceExhausted) {
		t.Fatal("IsTransientProviderErrorForProvider(cursor-acp, resourceExhausted) = false, want true")
	}
}

func TestClassifyCursorRetriableDoesNotCrossProviders(t *testing.T) {
	for _, providerID := range []string{"claude-acp", "codex-acp", "opencode-acp", "unknown-provider"} {
		t.Run(providerID, func(t *testing.T) {
			resetInjection()
			e := Classify(Input{
				Phase:      PhasePromptSend,
				ProviderID: providerID,
				Stderr:     cursorRetriablePingTimeout,
			})
			if e.Code == CodeProviderUnavailable || e.ClassifierRule == wantCursorRetriableUnavailableRuleID {
				t.Fatalf("Classify(%q) = %+v, want no Cursor unavailable match", providerID, e)
			}
			eResource := Classify(Input{
				Phase:      PhasePromptSend,
				ProviderID: providerID,
				Stderr:     realCursorRetriableResourceExhausted,
			})
			if eResource.Code == CodeProviderResourceExhausted || eResource.ClassifierRule == wantCursorRetriableResourceExhaustedRuleID {
				t.Fatalf("Classify(%q) = %+v, want no Cursor resource match", providerID, eResource)
			}
		})
	}
}

func TestClassifyCursorRetriableRejectsOversizedSanitizedSuffix(t *testing.T) {
	resetInjection()
	message := "Error: RetriableError: " + strings.Repeat("x", 257)
	e := Classify(Input{
		Phase:      PhasePromptSend,
		ProviderID: "cursor-acp",
		Stderr:     message,
	})
	if e.Code == CodeAgentTransportLost || e.ClassifierRule == wantCursorRetriableStreamResetRuleID {
		t.Fatalf("Classify(oversized redacted suffix) = %+v, want no Cursor transport-loss match", e)
	}
}

func TestClassifyCursorRetriableCancellationRemainsManual(t *testing.T) {
	resetInjection()

	for _, message := range []string{
		"context canceled: " + leadingCanceledCursorRetriableStreamReset,
		"cancel escalated: " + leadingCanceledCursorRetriableStreamReset,
		"Error: RetriableError: context canceled",
		"Error: RetriableError: context canceled_by_client",
		"Error: RetriableError: context deadline exceeded",
		"Error: RetriableError: cancel escalated",
	} {
		t.Run(message, func(t *testing.T) {
			e := Classify(Input{Phase: PhasePromptSend, ProviderID: cursorRetriableProviderID, Stderr: message})
			if e != nil && e.AutoRetryable {
				t.Fatalf("Classify(%q) = %+v, want non-auto-retryable cancellation", message, e)
			}
			if got := Decide(ContextKanban, e, time.Time{}); got != DecisionManual {
				t.Fatalf("Decide(ContextKanban, Classify(%q)) = %q, want %q", message, got, DecisionManual)
			}
		})
	}
}

func TestIsTransientProviderError_Cursor(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{
			name: "exact diagnostic",
			text: realCursorRetriableStreamReset,
			want: true,
		},
		{
			name: "Cursor cancellation token",
			text: realCursorRetriableStreamReset + " [canceled]",
			want: true,
		},
		{
			name: "PING timed out",
			text: cursorRetriablePingTimeout,
			want: true,
		},
		{
			name: "connection stalled",
			text: cursorRetriableConnectionStalled,
			want: true,
		},
		{
			name: "resource exhausted",
			text: realCursorRetriableResourceExhausted,
			want: true,
		},
		{
			name: "context canceled",
			text: "context canceled: " + realCursorRetriableStreamReset,
			want: false,
		},
		{
			name: "cancel escalated",
			text: "cancel escalated: " + realCursorRetriableStreamReset,
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTransientProviderErrorForProvider(cursorRetriableProviderID, tc.text); got != tc.want {
				t.Errorf("IsTransientProviderErrorForProvider(%q, %q) = %v, want %v", cursorRetriableProviderID, tc.text, got, tc.want)
			}
		})
	}
}
