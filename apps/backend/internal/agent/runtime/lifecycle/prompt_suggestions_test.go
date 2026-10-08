package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/mcpmode"
)

// TestPromptSuggestionsForLaunch verifies suggestions are requested only for
// interactive sessions when the user opted in.
func TestPromptSuggestionsForLaunch(t *testing.T) {
	enabled := func(context.Context) bool { return true }
	disabled := func(context.Context) bool { return false }
	cases := []struct {
		name string
		pref PromptSuggestionsPreference
		mode string
		want bool
	}{
		{name: "task", pref: enabled, mode: mcpmode.Task, want: true},
		{name: "default mode", pref: enabled, mode: "", want: true},
		{name: "pending title", pref: enabled, mode: mcpmode.TaskTitlePending, want: true},
		{name: "preference off", pref: disabled, mode: mcpmode.Task},
		{name: "no preference reader", mode: mcpmode.Task},
		{name: "office run", pref: enabled, mode: mcpmode.Office},
		{name: "automation", pref: enabled, mode: mcpmode.Automation},
		{name: "coordinator", pref: enabled, mode: mcpmode.Coordinator},
		{name: "configuration chat", pref: enabled, mode: mcpmode.Config},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manager{}
			if tc.pref != nil {
				m.SetPromptSuggestionsPreference(tc.pref)
			}
			if got := m.promptSuggestionsForLaunch(context.Background(), tc.mode); got != tc.want {
				t.Fatalf("promptSuggestionsForLaunch = %v, want %v", got, tc.want)
			}
		})
	}
}
