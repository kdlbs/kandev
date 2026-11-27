package lifecycle

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/stretchr/testify/require"
)

func TestValidateBuiltAgentCommandsPreservesTypedFailure(t *testing.T) {
	for _, tc := range []struct {
		name               string
		args, continueArgs []string
		command            string
	}{
		{name: "empty initial", command: "agent"},
		{name: "flag executable", args: []string{"--flag"}, command: "agent"},
		{name: "empty continue", args: []string{"agent"}, continueArgs: []string{}, command: "continue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBuiltAgentCommands(tc.args, tc.continueArgs)
			require.Error(t, err)
			wrapped := routingerr.SanitizeError(fmt.Errorf("failed to resume session: %w", err))
			var validation *routingerr.BuiltCommandValidationError
			require.ErrorAs(t, wrapped, &validation)
			require.Equal(t, tc.command, validation.Command)
			require.True(t, errors.Is(wrapped, validation.Cause))
			require.Contains(t, wrapped.Error(), "validate "+tc.command+" command:")
		})
	}
	require.NoError(t, validateBuiltAgentCommands([]string{"agent", ""}, nil))
	require.NoError(t, validateBuiltAgentCommands([]string{"agent"}, []string{"agent", "resume"}))
}
