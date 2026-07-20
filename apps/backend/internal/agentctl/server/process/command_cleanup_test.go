package process

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKillAndWaitStartedCommandReapsProcess(t *testing.T) {
	cmd := fixtureCmd("sleep 30")
	require.NoError(t, cmd.Start())

	require.NoError(t, killAndWaitStartedCommand(cmd))
	require.NotNil(t, cmd.ProcessState)
}
