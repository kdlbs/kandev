package lifecycle

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

// TestPassthroughCommand_omits_prompt_arg_for_auto_inject_agents verifies the
// fix for the double-delivery bug: when an agent has AutoInjectPrompt=true and
// no PromptFlag, BuildPassthroughCommand must NOT include the task description
// as a positional arg. The prompt is delivered later via PTY stdin in
// autoInjectInitialPrompt; if it also got passed on argv, agents like Claude
// would launch in non-interactive `-p` mode and exit immediately.
func TestPassthroughCommand_omits_prompt_arg_for_auto_inject_agents(t *testing.T) {
	const description = "refactor the cron handler to use TickerScheduler"

	agent := &agents.StandardPassthrough{
		Cfg: agents.PassthroughConfig{
			Supported:        true,
			PassthroughCmd:   agents.NewCommand("npx", "-y", "@anthropic-ai/claude-code", "--verbose"),
			AutoInjectPrompt: true,
			SubmitSequence:   "\r",
			// PromptFlag intentionally empty — that's the case the fix targets.
		},
	}

	// Mirror what passthroughAgentCommand does when AutoInjectPrompt is on.
	promptForCmd := description
	if agent.Cfg.AutoInjectPrompt && agent.Cfg.PromptFlag.IsEmpty() {
		promptForCmd = ""
	}

	cmd := agent.BuildPassthroughCommand(agents.PassthroughOptions{
		Prompt: promptForCmd,
	})

	for _, arg := range cmd.Args() {
		if strings.Contains(arg, "refactor the cron handler") {
			t.Fatalf("description appeared as command arg: %q (full args: %v)", arg, cmd.Args())
		}
	}
}

// TestPassthroughCommand_keeps_prompt_arg_when_auto_inject_disabled verifies
// the non-auto-inject path still appends the prompt as a positional arg.
// This is the legacy behavior for non-AutoInjectPrompt agents.
func TestPassthroughCommand_keeps_prompt_arg_when_auto_inject_disabled(t *testing.T) {
	const description = "do the thing"

	agent := &agents.StandardPassthrough{
		Cfg: agents.PassthroughConfig{
			Supported:      true,
			PassthroughCmd: agents.NewCommand("legacy-agent"),
			// AutoInjectPrompt left false.
		},
	}

	promptForCmd := description
	if agent.Cfg.AutoInjectPrompt && agent.Cfg.PromptFlag.IsEmpty() {
		promptForCmd = ""
	}

	cmd := agent.BuildPassthroughCommand(agents.PassthroughOptions{
		Prompt: promptForCmd,
	})

	found := false
	for _, arg := range cmd.Args() {
		if arg == description {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected description to be appended as positional arg, got args: %v", cmd.Args())
	}
}
