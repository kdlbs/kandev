package acp

import (
	"context"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

// requireClaudeSuggestionMeta asserts the support/request configuration
// contract on a session/new, session/load, or session/resume request.
func requireClaudeSuggestionMeta(t *testing.T, meta map[string]any, requested bool) {
	t.Helper()
	claudeCode, ok := meta["claudeCode"].(map[string]any)
	if !ok {
		t.Fatalf("claudeCode meta = %#v, want an object", meta)
	}
	options, ok := claudeCode["options"].(map[string]any)
	if !ok {
		t.Fatalf("claudeCode options = %#v, want an object", claudeCode)
	}
	if got := options["promptSuggestions"]; got != requested {
		t.Fatalf("promptSuggestions = %#v, want %v", got, requested)
	}
	_, hasRawFilter := claudeCode["emitRawSDKMessages"]
	if hasRawFilter != requested {
		t.Fatalf("raw filter present = %v, want %v", hasRawFilter, requested)
	}
}

func requireNoSuggestionMeta(t *testing.T, meta map[string]any) {
	t.Helper()
	if len(meta) != 0 {
		t.Fatalf("session meta = %#v, want none for an unsupported integration", meta)
	}
}

// TestPromptSuggestionSessionConfiguration verifies every support/request pair
// on new, load, and advertised resume, including the method-not-found fallback
// to load. Supported but unrequested sessions send an explicit disable and no
// raw filter; unsupported sessions receive no suggestion configuration.
func TestPromptSuggestionSessionConfiguration(t *testing.T) {
	cases := []struct {
		name      string
		supported bool
		requested bool
	}{
		{name: "supported and requested", supported: true, requested: true},
		{name: "supported but not requested", supported: true, requested: false},
		{name: "unsupported but requested", supported: false, requested: true},
		{name: "unsupported and not requested", supported: false, requested: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			a, fake := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
			a.promptSuggestionSupport = tc.supported
			a.cfg.PromptSuggestions = tc.requested

			if _, err := a.NewSession(ctx, nil); err != nil {
				t.Fatalf("NewSession: %v", err)
			}
			if err := a.LoadSession(ctx, "saved-session", nil); err != nil {
				t.Fatalf("LoadSession: %v", err)
			}
			a.capabilities.SessionCapabilities.Resume = &acpsdk.SessionResumeCapabilities{}
			if err := a.ResumeSession(ctx, "saved-session", nil); err != nil {
				t.Fatalf("ResumeSession: %v", err)
			}
			for _, request := range []struct {
				name string
				meta map[string]any
			}{
				{name: "session/new", meta: fake.newRequest.Meta},
				{name: "session/load", meta: fake.loadRequest.Meta},
				{name: "session/resume", meta: fake.resumeRequest.Meta},
			} {
				if tc.supported {
					requireClaudeSuggestionMeta(t, request.meta, tc.requested)
				} else {
					requireNoSuggestionMeta(t, request.meta)
				}
			}

			resumeAdapter, resumeFake := newSessionResumeAdapter(t, true, true)
			resumeAdapter.promptSuggestionSupport = tc.supported
			resumeAdapter.cfg.PromptSuggestions = tc.requested
			if err := resumeAdapter.LoadSession(ctx, "saved-session", nil); err != nil {
				t.Fatalf("resuming LoadSession: %v", err)
			}
			resumeFake.resumeError = acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionResume)
			if err := resumeAdapter.LoadSession(ctx, "saved-session", nil); err != nil {
				t.Fatalf("fallback LoadSession: %v", err)
			}
			for _, request := range []struct {
				name string
				meta map[string]any
			}{
				{name: "session/resume", meta: resumeFake.resumeRequest.Meta},
				{name: "session/load fallback", meta: resumeFake.loadRequest.Meta},
			} {
				if tc.supported {
					requireClaudeSuggestionMeta(t, request.meta, tc.requested)
				} else {
					requireNoSuggestionMeta(t, request.meta)
				}
			}
		})
	}
}

func suggestForCurrentSession(a *Adapter, text string) []AgentEvent {
	a.handleClaudeSDKMessage(claudeSDKMessage(a.GetSessionID(), "prompt_suggestion", text))
	return drainEvents(a)
}

// TestPromptSuggestionSessionOriginReset verifies a suggestion window from the
// prior provider conversation is discarded at a session boundary. A delayed
// notification carrying the successor session ID must not claim it, while the
// successor's own finished turn still yields a suggestion.
func TestPromptSuggestionSessionOriginReset(t *testing.T) {
	transitions := []struct {
		name    string
		session func(*testing.T, *Adapter)
	}{
		{
			name: "new session",
			session: func(t *testing.T, a *Adapter) {
				t.Helper()
				if _, err := a.NewSession(context.Background(), nil); err != nil {
					t.Fatalf("NewSession: %v", err)
				}
			},
		},
		{
			name: "loaded session",
			session: func(t *testing.T, a *Adapter) {
				t.Helper()
				if err := a.LoadSession(context.Background(), "saved-session", nil); err != nil {
					t.Fatalf("LoadSession: %v", err)
				}
			},
		},
		{
			name: "reset session",
			session: func(t *testing.T, a *Adapter) {
				t.Helper()
				if _, err := a.ResetSession(context.Background(), nil); err != nil {
					t.Fatalf("ResetSession: %v", err)
				}
			},
		},
	}
	for _, tc := range transitions {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
			a.cfg.PromptSuggestions = true
			a.promptSuggestionSupport = true
			a.sessionID = "prior-session"
			a.finishPromptTurn(startPromptForSuggestion(a, 7))
			drainEvents(a)

			tc.session(t, a)
			drainEvents(a)

			if events := suggestForCurrentSession(a, "delayed from the prior conversation"); len(events) != 0 {
				t.Fatalf("events = %+v, want the prior window dropped after the session boundary", events)
			}

			a.finishPromptTurn(startPromptForSuggestion(a, 8))
			requireSuggestionEvent(t, suggestForCurrentSession(a, "fresh"), "fresh", 8)
		})
	}
}

// TestPromptSuggestionOriginSurvivesFailedTransition verifies a failed new,
// load, or reset transition keeps the surviving conversation's open suggestion
// window. The successor prompt must stay ambiguous, so the prior prompt's
// delayed suggestion is not emitted with the successor's generation.
func TestPromptSuggestionOriginSurvivesFailedTransition(t *testing.T) {
	transitions := []struct {
		name string
		fail func(*testing.T, *Adapter, *sessionRequestCaptureAgent)
	}{
		{
			name: "failed new session",
			fail: func(t *testing.T, a *Adapter, fake *sessionRequestCaptureAgent) {
				t.Helper()
				fake.failNewSessionOnAttempt = 1
				if _, err := a.NewSession(context.Background(), nil); err == nil {
					t.Fatal("NewSession succeeded, want the injected failure")
				}
			},
		},
		{
			name: "failed reset",
			fail: func(t *testing.T, a *Adapter, fake *sessionRequestCaptureAgent) {
				t.Helper()
				fake.failNewSessionOnAttempt = 1
				if _, err := a.ResetSession(context.Background(), nil); err == nil {
					t.Fatal("ResetSession succeeded, want the injected failure")
				}
			},
		},
		{
			name: "failed load",
			fail: func(t *testing.T, a *Adapter, fake *sessionRequestCaptureAgent) {
				t.Helper()
				fake.loadErr = errors.New("injected load failure")
				if err := a.LoadSession(context.Background(), "prior-session", nil); err == nil {
					t.Fatal("LoadSession succeeded, want the injected failure")
				}
			},
		},
	}
	for _, tc := range transitions {
		t.Run(tc.name, func(t *testing.T) {
			a, fake := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
			a.cfg.PromptSuggestions = true
			a.promptSuggestionSupport = true
			a.sessionID = "prior-session"
			a.finishPromptTurn(startPromptForSuggestion(a, 7))
			drainEvents(a)

			tc.fail(t, a, fake)
			drainEvents(a)
			if a.GetSessionID() != "prior-session" {
				t.Fatalf("session ID = %q, want the surviving prior-session", a.GetSessionID())
			}

			// Turn B runs on the surviving conversation. A's still-open window
			// makes B's origin ambiguous, so a delayed A notification is dropped.
			a.finishPromptTurn(startPromptForSuggestion(a, 8))
			if events := suggestForCurrentSession(a, "delayed from turn A"); len(events) != 0 {
				t.Fatalf("events = %+v, want the delayed prior suggestion dropped", events)
			}
		})
	}
}

// TestPromptSuggestionOriginSurvivesSameSessionRestore verifies a successful
// restore that keeps the same provider session ID preserves the prior
// conversation's open window instead of pretending the successor prompt starts
// clean.
func TestPromptSuggestionOriginSurvivesSameSessionRestore(t *testing.T) {
	restores := []struct {
		name    string
		restore func(*testing.T, *Adapter, *sessionRequestCaptureAgent)
	}{
		{
			name: "same-ID new session",
			restore: func(t *testing.T, a *Adapter, fake *sessionRequestCaptureAgent) {
				t.Helper()
				fake.newResponse = &acpsdk.NewSessionResponse{SessionId: "prior-session"}
				if _, err := a.NewSession(context.Background(), nil); err != nil {
					t.Fatalf("NewSession: %v", err)
				}
			},
		},
		{
			name: "same-ID load",
			restore: func(t *testing.T, a *Adapter, _ *sessionRequestCaptureAgent) {
				t.Helper()
				if err := a.LoadSession(context.Background(), "prior-session", nil); err != nil {
					t.Fatalf("LoadSession: %v", err)
				}
			},
		},
		{
			name: "same-ID advertised resume",
			restore: func(t *testing.T, a *Adapter, _ *sessionRequestCaptureAgent) {
				t.Helper()
				a.capabilities.SessionCapabilities.Resume = &acpsdk.SessionResumeCapabilities{}
				if err := a.LoadSession(context.Background(), "prior-session", nil); err != nil {
					t.Fatalf("LoadSession with resume: %v", err)
				}
			},
		},
	}
	for _, tc := range restores {
		t.Run(tc.name, func(t *testing.T) {
			a, fake := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
			a.cfg.PromptSuggestions = true
			a.promptSuggestionSupport = true
			a.sessionID = "prior-session"
			a.finishPromptTurn(startPromptForSuggestion(a, 7))
			drainEvents(a)

			tc.restore(t, a, fake)
			drainEvents(a)
			if a.GetSessionID() != "prior-session" {
				t.Fatalf("session ID = %q, want prior-session", a.GetSessionID())
			}

			a.finishPromptTurn(startPromptForSuggestion(a, 8))
			if events := suggestForCurrentSession(a, "delayed from turn A"); len(events) != 0 {
				t.Fatalf("events = %+v, want the delayed prior suggestion dropped", events)
			}
		})
	}
}
