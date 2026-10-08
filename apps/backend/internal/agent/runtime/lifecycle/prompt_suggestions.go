package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/common/mcpmode"
)

// PromptSuggestionsPreference reports whether the user enabled next-prompt
// suggestions. Wired to the user settings service.
type PromptSuggestionsPreference func(ctx context.Context) bool

// SetPromptSuggestionsPreference wires the user preference read at launch.
func (m *Manager) SetPromptSuggestionsPreference(fn PromptSuggestionsPreference) {
	m.promptSuggestionsPreference = fn
}

// promptSuggestionsForLaunch decides whether a launch asks its agent for native
// next-prompt suggestions: interactive task sessions only, and only when the
// user opted in. agentctl sends the request only to agents that advertise
// Claude Code on initialize, so other agents never see it.
func (m *Manager) promptSuggestionsForLaunch(ctx context.Context, mode string) bool {
	if m.promptSuggestionsPreference == nil {
		return false
	}
	switch mode {
	case "", mcpmode.Task, mcpmode.TaskTitlePending:
		return m.promptSuggestionsPreference(ctx)
	}
	return false
}
