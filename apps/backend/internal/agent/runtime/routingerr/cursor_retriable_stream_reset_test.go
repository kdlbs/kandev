package routingerr

import "testing"

const realCursorRetriableStreamReset = "Error: RetriableError: HTTP/2 stream closed with error code CANCEL (0x8)"

const wantCursorRetriableStreamResetRuleID = "cursor.retriable_stream_reset.v1"

func TestMatchRuntimeEnvironmentRules_CursorRetriable(t *testing.T) {
	got, ok := matchRuntimeEnvironmentRules(realCursorRetriableStreamReset)
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
			name: "partial transport fingerprint",
			text: "Error: RetriableError: HTTP/2 stream reset with error code CANCEL",
		},
		{
			name: "unrelated explanation before fingerprint",
			text: "Error: RetriableError: explanation for CANCEL (0x8)",
		},
		{
			name: "missing transport fingerprint",
			text: "Error: RetriableError: provider is busy",
		},
		{
			name: "context cancellation",
			text: "context canceled: " + realCursorRetriableStreamReset,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := matchRuntimeEnvironmentRules(tc.text); ok {
				t.Fatalf("unexpected match: %+v", got)
			}
		})
	}

	withCursorCancellationToken := realCursorRetriableStreamReset + " [canceled]"
	got, ok = matchRuntimeEnvironmentRules(withCursorCancellationToken)
	if !ok || got.Code != CodeAgentTransportLost {
		t.Fatalf("bracketed cancellation token classified as ok=%v err=%v, want transport loss", ok, got)
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
			if got := IsTransientProviderError(tc.text); got != tc.want {
				t.Errorf("IsTransientProviderError(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}
