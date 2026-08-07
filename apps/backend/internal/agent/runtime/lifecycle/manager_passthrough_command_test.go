package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

// TestPromptForPassthroughCommand asserts the gating used by passthroughAgentCommand:
// a prompt is passed to BuildPassthroughCommand only when the agent declares a
// PromptFlag. Without one, the prompt is suppressed (returned as "") so it is
// delivered exclusively via PTY stdin in autoInjectInitialPrompt — appending it
// as a positional arg instead drops it for interactive TUIs (e.g. zsh -ic
// "fuelclaude --model opus" "<prompt>" ignores the positional), which is the
// bug custom TUI agents hit. AutoInjectPrompt no longer gates suppression: the
// no-PromptFlag condition is what actually determines CLI-vs-stdin delivery.
func TestPromptForPassthroughCommand(t *testing.T) {
	tests := []struct {
		name string
		pt   agents.PassthroughConfig
		desc string
		want string
	}{
		{
			name: "no prompt flag suppresses prompt arg (auto-inject agent)",
			pt:   agents.PassthroughConfig{AutoInjectPrompt: true},
			desc: "refactor cron handler",
			want: "",
		},
		{
			name: "no prompt flag suppresses prompt arg (custom TUI agent)",
			pt:   agents.PassthroughConfig{AutoInjectPrompt: false},
			desc: "refactor cron handler",
			want: "",
		},
		{
			name: "explicit PromptFlag keeps prompt — flag delivery wins",
			pt: agents.PassthroughConfig{
				AutoInjectPrompt: true,
				PromptFlag:       agents.NewParam("--prompt", "{prompt}"),
			},
			desc: "refactor cron handler",
			want: "refactor cron handler",
		},
		{
			name: "explicit PromptFlag without auto-inject keeps prompt",
			pt: agents.PassthroughConfig{
				PromptFlag: agents.NewParam("--prompt", "{prompt}"),
			},
			desc: "refactor cron handler",
			want: "refactor cron handler",
		},
		{
			name: "empty description always empty",
			pt:   agents.PassthroughConfig{AutoInjectPrompt: true},
			desc: "",
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := promptForPassthroughCommand(tc.pt, tc.desc)
			if got != tc.want {
				t.Errorf("promptForPassthroughCommand = %q, want %q", got, tc.want)
			}
		})
	}
}
