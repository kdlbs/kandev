package main

import (
	"context"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// mockPromptSuggestion is the deterministic next-prompt suggestion the mock
// agent emits for sessions that requested native suggestions.
const mockPromptSuggestion = "Yes, run the tests"

// mockNoSuggestionDirective suppresses the suggestion for one prompt.
const mockNoSuggestionDirective = "/no-suggestion"

// mockSuggestionDelay mirrors Claude Code, which emits the suggestion after
// the turn result rather than inside it.
const mockSuggestionDelay = 150 * time.Millisecond

type extensionNotifier interface {
	NotifyExtension(ctx context.Context, method string, params any) error
}

// promptSuggestionsRequested reports whether session _meta asks for Claude
// Code prompt suggestions, mirroring claude-agent-acp's option passthrough.
func promptSuggestionsRequested(meta map[string]any) bool {
	claudeCode, _ := meta["claudeCode"].(map[string]any)
	options, _ := claudeCode["options"].(map[string]any)
	return options["promptSuggestions"] == true
}

func (a *mockAgent) recordPromptSuggestionRequest(sid acp.SessionId, meta map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.promptSuggestionSessions == nil {
		a.promptSuggestionSessions = make(map[acp.SessionId]bool)
	}
	a.promptSuggestionSessions[sid] = promptSuggestionsRequested(meta)
}

// emitPromptSuggestion sends the forwarded prompt_suggestion message the way
// claude-agent-acp does for emitRawSDKMessages.
func (a *mockAgent) emitPromptSuggestion(ctx context.Context, sid acp.SessionId, prompt string) {
	a.mu.Lock()
	requested := a.promptSuggestionSessions[sid]
	notifier, ok := a.conn.(extensionNotifier)
	a.mu.Unlock()
	if !requested || !ok || strings.Contains(prompt, mockNoSuggestionDirective) {
		return
	}
	_ = notifier.NotifyExtension(ctx, "_claude/sdkMessage", map[string]any{
		"sessionId": string(sid),
		"message":   map[string]any{"type": "prompt_suggestion", "suggestion": mockPromptSuggestion},
	})
}

type connectionCloser interface {
	Done() <-chan struct{}
}

// emitPromptSuggestionAfterResponse waits for the turn result to reach the
// client, then emits the suggestion. It returns early when the connection closes.
func (a *mockAgent) emitPromptSuggestionAfterResponse(sid acp.SessionId, prompt string) {
	a.mu.Lock()
	closer, _ := a.conn.(connectionCloser)
	a.mu.Unlock()
	var done <-chan struct{}
	if closer != nil {
		done = closer.Done()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.NewTimer(mockSuggestionDelay)
	defer timer.Stop()
	select {
	case <-done:
		return
	case <-timer.C:
	}
	go func() {
		select {
		case <-done:
			cancel()
		case <-ctx.Done():
		}
	}()
	a.emitPromptSuggestion(ctx, sid, prompt)
}
