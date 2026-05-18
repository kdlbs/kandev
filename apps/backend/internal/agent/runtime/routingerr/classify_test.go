package routingerr

import (
	"net/http"
	"sync"
	"testing"
)

func resetInjection() {
	injectOnce = sync.Once{}
	injectMap = nil
}

func TestClassify_HTTPStatusMapping(t *testing.T) {
	resetInjection()
	cases := []struct {
		status int
		want   Code
	}{
		{http.StatusUnauthorized, CodeAuthRequired},
		{http.StatusForbidden, CodePermissionDeniedByUser},
		{http.StatusPaymentRequired, CodeSubscriptionRequired},
		{http.StatusTooManyRequests, CodeRateLimited},
		{http.StatusServiceUnavailable, CodeProviderUnavailable},
	}
	for _, c := range cases {
		got := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", HTTPStatus: c.status})
		if got.Code != c.want {
			t.Fatalf("status %d → %s, want %s", c.status, got.Code, c.want)
		}
		if got.Confidence != ConfHigh {
			t.Fatalf("status %d confidence %s want high", c.status, got.Confidence)
		}
	}
}

func TestClassify_ExitCode127(t *testing.T) {
	resetInjection()
	exit := 127
	e := Classify(Input{Phase: PhaseProcessStart, ProviderID: "claude-acp", ExitCode: &exit})
	if e.Code != CodeProviderNotConfigured {
		t.Fatalf("got %s, want provider_not_configured", e.Code)
	}
	if !e.UserAction || !e.FallbackAllowed || e.AutoRetryable {
		t.Fatalf("invariants violated: %+v", e)
	}
}

func TestClassify_ProviderRules(t *testing.T) {
	resetInjection()
	cases := []struct {
		name       string
		providerID string
		stderr     string
		wantCode   Code
	}{
		{"claude quota", "claude-acp", "Error: anthropic_quota_exceeded for user", CodeQuotaLimited},
		{"claude rate", "claude-acp", "you hit the rate-limit", CodeRateLimited},
		{"claude auth", "claude-acp", "you are not authenticated", CodeAuthRequired},
		{"claude model", "claude-acp", "model claude-foo not found here", CodeModelUnavailable},
		{"codex quota", "codex-acp", "insufficient_quota for project", CodeQuotaLimited},
		{"codex rate", "codex-acp", "rate_limit_exceeded", CodeRateLimited},
		{"codex apikey", "codex-acp", "invalid api key provided", CodeMissingCredentials},
		{"opencode auth", "opencode-acp", "Unauthorized request", CodeAuthRequired},
		{"copilot subscription", "copilot-acp", "user is not entitled to Copilot", CodeSubscriptionRequired},
		{"copilot signin", "copilot-acp", "please sign in via gh auth login", CodeAuthRequired},
		{"amp auth", "amp-acp", "Invalid Token returned by server", CodeAuthRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := Classify(Input{Phase: PhaseSessionInit, ProviderID: c.providerID, Stderr: c.stderr})
			if e.Code != c.wantCode {
				t.Fatalf("%s → %s, want %s (rule=%s)", c.name, e.Code, c.wantCode, e.ClassifierRule)
			}
		})
	}
}

func TestClassify_PrestartPhaseFallback(t *testing.T) {
	resetInjection()
	for _, p := range []Phase{PhaseAuthCheck, PhaseProcessStart, PhaseSessionInit} {
		e := Classify(Input{Phase: p, ProviderID: "unknown-provider", Stderr: "weird gibberish"})
		if e.Code != CodeUnknownProvider {
			t.Fatalf("phase %s → %s, want unknown_provider_error", p, e.Code)
		}
		if !e.FallbackAllowed || !e.AutoRetryable {
			t.Fatalf("phase %s invariants violated: %+v", p, e)
		}
		if e.Confidence != ConfLow {
			t.Fatalf("phase %s confidence %s want low", p, e.Confidence)
		}
	}
}

func TestClassify_PoststartAmbiguousNoFallback(t *testing.T) {
	resetInjection()
	for _, p := range []Phase{PhasePromptSend, PhaseStreaming, PhaseToolExecution, PhaseShutdown} {
		e := Classify(Input{Phase: p, ProviderID: "claude-acp", Stderr: "unexpected glitch"})
		if e.Code != CodeAgentRuntime {
			t.Fatalf("phase %s → %s, want agent_runtime_error", p, e.Code)
		}
		if e.FallbackAllowed {
			t.Fatalf("phase %s must not fall back", p)
		}
	}
}

func TestClassify_InvariantsAuthRequired(t *testing.T) {
	resetInjection()
	e := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", Stderr: "not authenticated"})
	if e.Code != CodeAuthRequired {
		t.Fatalf("got %s", e.Code)
	}
	if !e.FallbackAllowed || e.AutoRetryable || !e.UserAction {
		t.Fatalf("auth_required invariants violated: %+v", e)
	}
}

func TestClassify_InvariantsModelUnavailable(t *testing.T) {
	resetInjection()
	e := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", Stderr: "model claude-x not found"})
	if e.Code != CodeModelUnavailable {
		t.Fatalf("got %s", e.Code)
	}
	if !e.FallbackAllowed {
		t.Fatalf("model_unavailable must allow fallback: %+v", e)
	}
}

func TestClassify_InvariantsQuotaLimitedAutoRetryable(t *testing.T) {
	resetInjection()
	e := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", Stderr: "anthropic_quota_exceeded"})
	if e.Code != CodeQuotaLimited {
		t.Fatalf("got %s", e.Code)
	}
	if !e.AutoRetryable || !e.FallbackAllowed {
		t.Fatalf("quota_limited invariants violated: %+v", e)
	}
}

func TestClassify_InjectionShortCircuit(t *testing.T) {
	resetInjection()
	t.Setenv("KANDEV_PROVIDER_FAILURES", "claude-acp:quota_limited,codex-acp:auth_required")
	resetInjection()
	e := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", Stderr: "completely unrelated text"})
	if e.Code != CodeQuotaLimited {
		t.Fatalf("injection ignored: got %s", e.Code)
	}
	if e.ClassifierRule != "inject.env" {
		t.Fatalf("expected inject.env rule, got %s", e.ClassifierRule)
	}
}

func TestClassify_SanitizesRawExcerpt(t *testing.T) {
	resetInjection()
	e := Classify(Input{Phase: PhaseSessionInit, ProviderID: "claude-acp", Stderr: "Bearer abcdefghijklmnopqrstuvwxyz1234567890"})
	if e.RawExcerpt == "" {
		t.Fatal("expected raw excerpt set")
	}
	if containsSubstring(e.RawExcerpt, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("raw excerpt not sanitized: %q", e.RawExcerpt)
	}
}

func TestError_String(t *testing.T) {
	e := &Error{Code: CodeAuthRequired, ClassifierRule: "claude.stderr.auth.v1"}
	if got := e.Error(); got != "auth_required: claude.stderr.auth.v1" {
		t.Fatalf("got %q", got)
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
