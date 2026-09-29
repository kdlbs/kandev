package utility

import "testing"

// A built-in agent's command is compiled into the binary, so it keeps
// resolving to the allow-list literal the taint tracker can follow.
func TestResolveSpawnCommand_BuiltinUsesAllowlistLiteral(t *testing.T) {
	t.Parallel()

	cmd, errMsg := resolveSpawnCommand(&InferenceConfigDTO{Command: []string{"goose", "acp"}})

	if errMsg != "" {
		t.Fatalf("errMsg = %q, want empty", errMsg)
	}
	if cmd != "goose" {
		t.Errorf("cmd = %q, want %q", cmd, "goose")
	}
}

// A command that is neither allow-listed nor operator-defined is still
// refused: for a built-in that means the allow-list and the agent definition
// have drifted apart, which is a bug rather than a configuration choice.
func TestResolveSpawnCommand_RejectsUnlistedBuiltin(t *testing.T) {
	t.Parallel()

	cmd, errMsg := resolveSpawnCommand(&InferenceConfigDTO{Command: []string{"my-agent", "--acp"}})

	if errMsg == "" {
		t.Fatal("errMsg is empty; an unlisted built-in command must be refused")
	}
	if cmd != "" {
		t.Errorf("cmd = %q, want empty", cmd)
	}
}

// A custom agent's command is typed by the install operator in Settings. The
// same string is already spawned unrestricted when a session starts, so
// refusing it here only denies the agent its models and modes.
func TestResolveSpawnCommand_AllowsOperatorDefined(t *testing.T) {
	t.Parallel()

	cmd, errMsg := resolveSpawnCommand(&InferenceConfigDTO{
		Command:         []string{"my-agent", "--acp"},
		OperatorDefined: true,
	})

	if errMsg != "" {
		t.Fatalf("errMsg = %q, want empty", errMsg)
	}
	if cmd != "my-agent" {
		t.Errorf("cmd = %q, want %q", cmd, "my-agent")
	}
}
