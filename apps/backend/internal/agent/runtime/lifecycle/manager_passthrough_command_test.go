package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

// TestPromptForPassthroughCommand asserts the gating used by passthroughAgentCommand:
// AutoInjectPrompt + empty PromptFlag suppresses the positional prompt arg, so the
// description is delivered exclusively via PTY stdin in autoInjectInitialPrompt.
// Without this guard, BuildPassthroughCommand would append the description as a
// positional arg and put Claude (and similar TUIs) into non-interactive `-p` mode.
func TestPromptForPassthroughCommand(t *testing.T) {
	tests := []struct {
		name string
		pt   agents.PassthroughConfig
		desc string
		want string
	}{
		{
			name: "auto-inject suppresses prompt arg",
			pt:   agents.PassthroughConfig{AutoInjectPrompt: true},
			desc: "refactor cron handler",
			want: "",
		},
		{
			name: "auto-inject off keeps prompt",
			pt:   agents.PassthroughConfig{AutoInjectPrompt: false},
			desc: "refactor cron handler",
			want: "refactor cron handler",
		},
		{
			name: "auto-inject with explicit PromptFlag keeps prompt — flag delivery wins",
			pt: agents.PassthroughConfig{
				AutoInjectPrompt: true,
				PromptFlag:       agents.NewParam("--prompt", "{prompt}"),
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
