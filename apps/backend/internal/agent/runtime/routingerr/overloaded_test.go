package routingerr

import "testing"

// realOverloaded529 is the exact transient provider error surfaced by the
// claude-agent-acp adapter when the Anthropic API returns 529 Overloaded
// mid-turn. Unlike resume-corruption this is a server-side, temporary
// condition — retrying after a short backoff is expected to succeed.
const realOverloaded529 = `{"code":-32603,"message":"Internal error: API Error: 529 Overloaded. ` +
	`This is a server-side issue, usually temporary — try again in a moment. If it persists, ` +
	`check https://status.claude.com.","data":{"errorKind":"server_error"}}`

func TestMatchRuntimeEnvironmentRules_Overloaded(t *testing.T) {
	got, ok := matchRuntimeEnvironmentRules(realOverloaded529)
	if !ok {
		t.Fatalf("expected match, got none")
	}
	if got.Code != CodeProviderOverloaded {
		t.Fatalf("Code = %q, want %q", got.Code, CodeProviderOverloaded)
	}
	if got.ClassifierRule != overloadedRuleID {
		t.Fatalf("ClassifierRule = %q, want %q", got.ClassifierRule, overloadedRuleID)
	}
	if got.Confidence != ConfHigh {
		t.Fatalf("Confidence = %q, want %q", got.Confidence, ConfHigh)
	}
}

func TestMatchRuntimeEnvironmentRules_OverloadedErrorToken(t *testing.T) {
	// The Anthropic error-type token alone (no numeric code) must also fire.
	msg := `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`
	got, ok := matchRuntimeEnvironmentRules(msg)
	if !ok || got.Code != CodeProviderOverloaded {
		t.Fatalf("expected provider_overloaded match, got ok=%v err=%v", ok, got)
	}
}

func TestClassify_Overloaded_Invariants(t *testing.T) {
	resetInjection()
	e := Classify(Input{
		Phase:      PhasePromptSend,
		ProviderID: "claude-acp",
		Stderr:     realOverloaded529,
	})
	if e == nil {
		t.Fatal("expected non-nil Error")
	}
	if e.Code != CodeProviderOverloaded {
		t.Fatalf("Code = %q, want %q", e.Code, CodeProviderOverloaded)
	}
	if !e.AutoRetryable {
		t.Errorf("AutoRetryable = false, want true (transient server-side overload)")
	}
	if !e.FallbackAllowed {
		t.Errorf("FallbackAllowed = false, want true")
	}
	if e.UserAction {
		t.Errorf("UserAction = true, want false (no user action needed)")
	}
	if e.Phase != PhasePromptSend {
		t.Errorf("Phase = %q, want %q", e.Phase, PhasePromptSend)
	}
}

func TestClassify_HTTPStatus529(t *testing.T) {
	// Callers that surface the status code separately from the body text (e.g.
	// a structured HTTP error) must also classify 529 as transient/overloaded.
	resetInjection()
	e := Classify(Input{Phase: PhasePromptSend, ProviderID: "claude-acp", HTTPStatus: statusOverloaded})
	if e == nil {
		t.Fatal("expected non-nil Error")
	}
	if e.Code != CodeProviderOverloaded {
		t.Fatalf("Code = %q, want %q", e.Code, CodeProviderOverloaded)
	}
	if !e.AutoRetryable {
		t.Errorf("AutoRetryable = false, want true")
	}
	if e.ClassifierRule != "http.529" {
		t.Errorf("ClassifierRule = %q, want http.529", e.ClassifierRule)
	}
}

func TestIsTransientProviderError(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"real 529 envelope", realOverloaded529, true},
		{"overloaded_error token", `"type":"overloaded_error"`, true},
		{"529 then overloaded on one line", "API Error: 529 Overloaded.", true},
		{"prefixed overloaded error token", `"type":"not_overloaded_error"`, false},
		{"unrelated 401", "API Error: 401 authentication_error", false},
		{"plain overloaded word, no code", "the disk is overloaded with files", false},
		{"bare 529 no overloaded", "got status 529 from upstream", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTransientProviderError(tc.msg); got != tc.want {
				t.Errorf("IsTransientProviderError(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}
