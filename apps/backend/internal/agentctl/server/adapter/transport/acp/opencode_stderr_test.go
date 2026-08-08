package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestNormalizeOpenCodeActionURLAcceptsOnlyAllowlistedRoute(t *testing.T) {
	const want = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go"
	accepted := []string{
		want,
		"https://opencode.ai/workspace/a/go",
		"https://opencode.ai/workspace/WRK-build_run-42/go",
		"https://opencode.ai/workspace/" + strings.Repeat("x", 128) + "/go",
	}
	for _, raw := range accepted {
		if got := NormalizeOpenCodeActionURL(raw); got != raw {
			t.Errorf("NormalizeOpenCodeActionURL(%q) = %q, want the input itself", raw, got)
		}
	}

	rejected := []string{
		"",
		"http://opencode.ai/workspace/wrk_123/go",
		"https://opencode.example.com/workspace/wrk_123/go",
		"https://opencode.ai/workspace/wrk_123",
		"https://opencode.ai/workspace/wrk_123/go/",
		"https://opencode.ai/workspace/wrk_123/go/extra",
		"https://opencode.ai/workspace/wrk_123/other",
		"https://opencode.ai/workspace/wrk_123/go?source=email",
		"https://opencode.ai/workspace/wrk_123/go?",
		"https://opencode.ai/workspace/wrk_123/go#fragment",
		"https://opencode.ai/workspace/wrk_123/go#",
		"https://opencode.ai/workspace/wrk_123/go?#",
		"https://user:pass@opencode.ai/workspace/wrk_123/go",
		"https://opencode.ai:443/workspace/wrk_123/go",
		"https://opencode.ai/workspace/wrk_123%2F..%2Fgo",
		"https://opencode.ai/workspace/%77rk_123/go",
		"https://opencode.ai/workspace/../go",
		"https://opencode.ai/workspace/../workspace/wrk_123/go",
		"https://opencode.ai/workspace/wrk_123/go extra",
		"https://opencode.ai/workspace/wrk_123/go.",
		"https://opencode.ai/workspace/" + strings.Repeat("w", 200) + "/go",
		"https://example.test/workspace/wrk_123/go",
		"not a url",
	}
	for _, raw := range rejected {
		if got := NormalizeOpenCodeActionURL(raw); got != "" {
			t.Errorf("NormalizeOpenCodeActionURL(%q) = %q, want empty", raw, got)
		}
	}
}

func TestExtractOpenCodeActionURLFindsValidTokenInMessage(t *testing.T) {
	const want = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go"
	line := "AI_APICallError: 5-hour usage limit reached. To continue using this model now, enable usage from your available balance: " + want
	if got := extractOpenCodeActionURL(line); got != want {
		t.Fatalf("extractOpenCodeActionURL() = %q, want %q", got, want)
	}
	for _, raw := range []string{
		"provider failed",
		"visit https://example.test/workspace/wrk_123/go instead",
		"balance: https://opencode.ai/workspace/wrk_123/go.",
		"https://opencode.ai/workspace/wrk_123",
	} {
		if got := extractOpenCodeActionURL(raw); got != "" {
			t.Errorf("extractOpenCodeActionURL(%q) = %q, want empty", raw, got)
		}
	}
}

func TestProviderErrorFromACPRequestErrorReadsOnlyStructuredActionURL(t *testing.T) {
	const wantURL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go"

	t.Run("structured action_url projects a safe provider error", func(t *testing.T) {
		err := &sdk.RequestError{
			Code:    -32603,
			Message: "AI_APICallError: 5-hour usage limit reached: " + wantURL,
			Data:    map[string]any{"action_url": wantURL},
		}
		got := ProviderErrorFromError(err)
		if got == nil {
			t.Fatal("ProviderErrorFromError() = nil, want provider error")
		}
		if got.Source != streams.ProviderErrorSourceOpenCodeACP {
			t.Fatalf("source = %q", got.Source)
		}
		if got.RemediationURL != wantURL {
			t.Fatalf("remediation URL = %q, want %q", got.RemediationURL, wantURL)
		}
		if strings.Contains(got.Message, "https://") ||
			strings.Contains(got.Message, "wrk_01KQM7K5CYT715264YKKFB17ZY") {
			t.Fatalf("message contains private URL or identifier: %q", got.Message)
		}
		if got.Message == "" {
			t.Fatal("message is empty")
		}
	})

	t.Run("missing or malformed action_url keeps the generic error path", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			err  error
		}{
			{name: "no data", err: &sdk.RequestError{Code: -32603, Message: "provider failed"}},
			{name: "wrong shape", err: &sdk.RequestError{Code: -32603, Message: "provider failed", Data: "action_url"}},
			{name: "wrong host", err: &sdk.RequestError{
				Code: -32603, Message: "provider failed",
				Data: map[string]any{"action_url": "https://example.test/workspace/wrk_123/go"},
			}},
			{name: "query", err: &sdk.RequestError{
				Code: -32603, Message: "provider failed",
				Data: map[string]any{"action_url": wantURL + "?source=email"},
			}},
			{name: "malformed id", err: &sdk.RequestError{
				Code: -32603, Message: "provider failed",
				Data: map[string]any{"action_url": "https://opencode.ai/workspace/../go"},
			}},
			{name: "oversized", err: &sdk.RequestError{
				Code: -32603, Message: "provider failed",
				Data: map[string]any{"action_url": "https://opencode.ai/workspace/" + strings.Repeat("w", 300) + "/go"},
			}},
			{name: "wrapped generic error", err: fmt.Errorf("provider failed")},
		} {
			t.Run(tt.name, func(t *testing.T) {
				if got := ProviderErrorFromError(tt.err); got != nil {
					t.Fatalf("ProviderErrorFromError() = %+v, want nil", got)
				}
			})
		}
	})
}

func TestParseOpenCodeStderrLineAcceptsForegroundStreamError(t *testing.T) {
	line := `timestamp=2026-08-02T15:15:44.504Z level=ERROR run=a93db686 message="stream error" providerID=opencode-go modelID=kimi-k3 session.id=ses_03d1ac324ffeA2eAGeBfuq80dq small=false agent=build mode=primary error.error="AI_APICallError: 5-hour usage limit reached. Resets in 4hr 19min. To continue using this model now, enable usage from your available balance: https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go"`

	diagnostic, ok := parseOpenCodeStderrLine(line)
	if !ok {
		t.Fatal("parseOpenCodeStderrLine() rejected a foreground stream error")
	}
	if diagnostic.SessionID != "ses_03d1ac324ffeA2eAGeBfuq80dq" {
		t.Fatalf("session ID = %q", diagnostic.SessionID)
	}
	if diagnostic.ProviderError.Source != "opencode_stderr" {
		t.Fatalf("source = %q", diagnostic.ProviderError.Source)
	}
	if diagnostic.ProviderError.ProviderID != "opencode-go" {
		t.Fatalf("provider ID = %q", diagnostic.ProviderError.ProviderID)
	}
	if diagnostic.ProviderError.ModelID != "kimi-k3" {
		t.Fatalf("model ID = %q", diagnostic.ProviderError.ModelID)
	}
	wantOccurred := time.Date(2026, 8, 2, 15, 15, 44, 504000000, time.UTC)
	if !diagnostic.ProviderError.OccurredAt.Equal(wantOccurred) {
		t.Fatalf("occurred at = %s, want %s", diagnostic.ProviderError.OccurredAt, wantOccurred)
	}
	wantReset := wantOccurred.Add(4*time.Hour + 19*time.Minute)
	if diagnostic.ProviderError.ResetAt == nil || !diagnostic.ProviderError.ResetAt.Equal(wantReset) {
		t.Fatalf("reset at = %v, want %s", diagnostic.ProviderError.ResetAt, wantReset)
	}
	if strings.Contains(diagnostic.ProviderError.Message, "https://") ||
		strings.Contains(diagnostic.ProviderError.Message, "wrk_01KQM7K5CYT715264YKKFB17ZY") {
		t.Fatalf("message contains private URL or identifier: %q", diagnostic.ProviderError.Message)
	}
	if diagnostic.ProviderError.Message == "" {
		t.Fatal("message is empty")
	}
	wantURL := "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go"
	if diagnostic.ProviderError.RemediationURL != wantURL {
		t.Fatalf("remediation URL = %q, want %q", diagnostic.ProviderError.RemediationURL, wantURL)
	}
}

func TestParseOpenCodeStderrLineCapturesExplicitActionURLField(t *testing.T) {
	line := `timestamp=2026-08-02T15:15:44Z level=ERROR message="stream error" providerID=opencode-go modelID=kimi-k3 session.id=ses_123 small=false agent=build action_url="https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go" error.error="AI_APICallError: usage limit reached"`

	diagnostic, ok := parseOpenCodeStderrLine(line)
	if !ok {
		t.Fatal("parseOpenCodeStderrLine() rejected an action_url record")
	}
	wantURL := "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go"
	if diagnostic.ProviderError.RemediationURL != wantURL {
		t.Fatalf("remediation URL = %q, want %q", diagnostic.ProviderError.RemediationURL, wantURL)
	}
	if strings.Contains(diagnostic.ProviderError.Message, "https://") ||
		strings.Contains(diagnostic.ProviderError.Message, "wrk_") {
		t.Fatalf("message contains private URL or identifier: %q", diagnostic.ProviderError.Message)
	}
}

func TestParseOpenCodeStderrLineRejectsUntrustedActionURLs(t *testing.T) {
	base := `timestamp=2026-08-02T15:15:44Z level=ERROR message="stream error" providerID=opencode-go modelID=kimi-k3 session.id=ses_123 small=false agent=build error.error="usage limit reached"`
	for _, tt := range []struct {
		name string
		line string
	}{
		{
			name: "wrong host",
			line: strings.Replace(base, `error.error=`, `action_url="https://example.test/workspace/wrk_123/go" error.error=`, 1),
		},
		{
			name: "query",
			line: strings.Replace(base, `error.error=`, `action_url="https://opencode.ai/workspace/wrk_123/go?x=1" error.error=`, 1),
		},
		{
			name: "malformed id",
			line: strings.Replace(base, `error.error=`, `action_url="https://opencode.ai/workspace/../go" error.error=`, 1),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			diagnostic, ok := parseOpenCodeStderrLine(tt.line)
			if !ok {
				t.Fatal("parseOpenCodeStderrLine() rejected the record")
			}
			if diagnostic.ProviderError.RemediationURL != "" {
				t.Fatalf("remediation URL = %q, want empty", diagnostic.ProviderError.RemediationURL)
			}
		})
	}
}

func TestPromptDiagnosticSettlesOnlyMatchingSession(t *testing.T) {
	a := newTestAdapter()
	turn := &promptTurnState{
		rpcDone:         make(chan struct{}),
		abortCh:         make(chan struct{}),
		providerErrorCh: make(chan openCodeStderrDiagnostic, 2),
	}
	var cancelCause error
	turn.endTurn = func(err error) {
		cancelCause = err
		close(turn.rpcDone)
	}

	done := make(chan error, 1)
	go func() { done <- a.waitForPromptRPCAfterUserCancel(turn, "ses-current") }()
	turn.providerErrorCh <- openCodeStderrDiagnostic{SessionID: "ses-other"}
	turn.providerErrorCh <- openCodeStderrDiagnostic{
		SessionID: "ses-current",
		ProviderError: streams.ProviderError{
			Source:     streams.ProviderErrorSourceOpenCodeStderr,
			Message:    "5-hour usage limit reached",
			OccurredAt: time.Date(2026, 8, 2, 15, 15, 44, 0, time.UTC),
		},
	}

	select {
	case err := <-done:
		var providerErr *providerPromptError
		if !errors.As(err, &providerErr) {
			t.Fatalf("error = %v, want providerPromptError", err)
		}
		if providerErr.ProviderError.Message != "5-hour usage limit reached" {
			t.Fatalf("provider message = %q", providerErr.ProviderError.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("matching provider diagnostic did not settle prompt")
	}
	var cause *providerPromptError
	if !errors.As(cancelCause, &cause) {
		t.Fatalf("cancel cause = %v, want provider prompt error", cancelCause)
	}
}

type providerErrorFakeAgent struct {
	concurrencyFakeAgent
	entered chan struct{}
}

func (f *providerErrorFakeAgent) Prompt(ctx context.Context, _ sdk.PromptRequest) (sdk.PromptResponse, error) {
	select {
	case f.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return sdk.PromptResponse{}, ctx.Err()
}

func (*providerErrorFakeAgent) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	return sdk.NewSessionResponse{SessionId: "ses_current"}, nil
}

func TestOpenCodeDiagnosticPromptSettlesFromCorrelatedStderr(t *testing.T) {
	a := newTestAdapter()
	a.agentID = opencodeAgentID
	a.normalizer = NewNormalizer(opencodeAgentID)
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	fake := &providerErrorFakeAgent{
		concurrencyFakeAgent: concurrencyFakeAgent{
			initialized: make(chan sdk.InitializeRequest, 1),
		},
		entered: make(chan struct{}, 1),
	}
	if err := a.Connect(clientToAgentW, agentToClientR); err != nil {
		t.Fatalf("connect adapter: %v", err)
	}
	_ = sdk.NewAgentSideConnection(fake, agentToClientW, clientToAgentR)
	t.Cleanup(func() {
		_ = a.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
	})

	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	sessionID, err := a.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	promptDone := make(chan error, 1)
	go func() { promptDone <- a.Prompt(ctx, "continue", nil, 17) }()
	select {
	case <-fake.entered:
	case <-time.After(time.Second):
		t.Fatal("fake prompt did not start")
	}

	line := fmt.Sprintf(`timestamp=2026-08-02T15:15:44Z level=ERROR message="stream error" providerID=opencode-go modelID=kimi-k3 session.id=%s small=false agent=build error.error="AI_APICallError: 5-hour usage limit reached. Resets in 4hr 19min."`, sessionID)
	a.ConsumeStderrLine(line)

	select {
	case err := <-promptDone:
		var providerErr *providerPromptError
		if !errors.As(err, &providerErr) {
			t.Fatalf("prompt error = %v, want providerPromptError", err)
		}
		if providerErr.ProviderError.ModelID != "kimi-k3" {
			t.Fatalf("provider error = %+v", providerErr.ProviderError)
		}
	case <-time.After(time.Second):
		t.Fatal("correlated stderr did not settle the held prompt")
	}
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeComplete {
			t.Fatalf("provider failure emitted a complete event: %+v", event)
		}
	}
}

func TestParseOpenCodeStderrLineRejectsUntrustedRecords(t *testing.T) {
	base := `timestamp=2026-08-02T15:15:44.504Z level=ERROR message="stream error" providerID=opencode-go modelID=kimi-k3 session.id=ses_123 small=false agent=build error.error="provider failed"`
	tests := []struct {
		name string
		line string
	}{
		{name: "small", line: strings.Replace(base, "small=false", "small=true", 1)},
		{name: "title agent", line: strings.Replace(base, "agent=build", "agent=title", 1)},
		{name: "wrong level", line: strings.Replace(base, "level=ERROR", "level=INFO", 1)},
		{name: "wrong message", line: strings.Replace(base, `message="stream error"`, `message="stream"`, 1)},
		{name: "missing session", line: strings.Replace(base, " session.id=ses_123", "", 1)},
		{name: "missing error", line: strings.Replace(base, ` error.error="provider failed"`, "", 1)},
		{name: "malformed", line: "level=ERROR message=stream error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := parseOpenCodeStderrLine(tt.line); ok {
				t.Fatal("parseOpenCodeStderrLine() accepted an untrusted record")
			}
		})
	}
}

func TestParseOpenCodeStderrLineSanitizesIdentifiersAndBoundsMessage(t *testing.T) {
	line := `timestamp=2026-08-02T15:15:44Z level=ERROR message="stream error" providerID=opencode-go modelID=kimi-k3 session.id=ses_123 small=false agent=build error.error="failed run=run_123 session.id=ses_456 workspace=wrk_789 https://example.test/secret"`
	diagnostic, ok := parseOpenCodeStderrLine(line)
	if !ok {
		t.Fatal("expected diagnostic")
	}
	for _, secret := range []string{"run_123", "ses_456", "wrk_789", "https://example.test/secret"} {
		if strings.Contains(diagnostic.ProviderError.Message, secret) {
			t.Fatalf("message contains %q: %q", secret, diagnostic.ProviderError.Message)
		}
	}
}

func TestOpenCodeStderrConsumerDropsWhenPromptChannelIsBacklogged(t *testing.T) {
	a := newTestAdapter()
	a.agentID = opencodeAgentID
	a.promptTurn = &promptTurnState{
		providerErrorCh: make(chan openCodeStderrDiagnostic, 1),
	}
	a.promptTurn.providerErrorCh <- openCodeStderrDiagnostic{}

	line := `timestamp=2026-08-02T15:15:44Z level=ERROR message="stream error" providerID=opencode-go modelID=kimi-k3 session.id=ses_123 small=false agent=build error.error="provider failed"`
	done := make(chan struct{})
	go func() {
		a.ConsumeStderrLine(line)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("backlogged provider diagnostic channel blocked stderr consumption")
	}
}

func TestSanitizeOpenCodeStderrLineProjectsOnlySafeProviderMessage(t *testing.T) {
	a := newTestAdapter()
	a.agentID = opencodeAgentID
	line := `timestamp=2026-08-02T15:15:44Z level=ERROR message="stream error" providerID=opencode-go modelID=kimi-k3 session.id=ses_123 small=false agent=build error.error="usage limit reached: https://opencode.ai/workspace/wrk_123"`

	safe, keep := a.SanitizeStderrLine(line)
	if !keep || safe == "" {
		t.Fatalf("safe line = %q, keep = %t", safe, keep)
	}
	if strings.Contains(safe, "https://") || strings.Contains(safe, "wrk_123") {
		t.Fatalf("safe line contains private details: %q", safe)
	}

	if safe, keep := a.SanitizeStderrLine("level=ERROR message=title"); keep || safe != "" {
		t.Fatalf("unrecognized line = %q, keep = %t", safe, keep)
	}
}
