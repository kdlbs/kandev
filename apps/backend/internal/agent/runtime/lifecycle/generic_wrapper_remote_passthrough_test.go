package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-WRAPPERS-004.1
func TestGenericWrapperRemotePassthroughRefusesBeforeStarting(t *testing.T) {
	shell, err := exec.LookPath("sh")
	require.NoError(t, err)
	for _, wrapped := range []bool{false, true} {
		t.Run(map[bool]string{false: "unwrapped control", true: "wrapped refusal"}[wrapped], func(t *testing.T) {
			mgr, _, execution := newPassthroughRunnerManager(t)
			controller, root := sshPassthroughTestController(t, execution)
			cmd := passthroughCommand{Command: agents.NewCommand(shell, "-c", "printf started > launch-marker; cat")}
			if wrapped {
				cmd.wrapper = launchwrapper.Profile([]string{"required-wrapper"})
			}
			info, startErr := mgr.startInteractiveProcess(context.Background(), execution, agents.PassthroughConfig{}, nil, cmd, nil, nil)
			marker := filepath.Join(root, "launch-marker")
			if wrapped {
				require.ErrorContains(t, startErr, "remote terminal transport does not support launch wrappers")
				require.Nil(t, info)
				require.Zero(t, controller.Passthrough().Snapshot().OSPID)
				require.NoFileExists(t, marker)
				return
			}
			require.NoError(t, startErr)
			require.NotNil(t, info)
			require.Eventually(t, func() bool {
				_, err := os.Stat(marker)
				return err == nil
			}, 5*time.Second, 10*time.Millisecond)
		})
	}
}
