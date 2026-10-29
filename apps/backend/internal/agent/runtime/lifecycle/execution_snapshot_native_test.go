package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutionSnapshotBeforeNativeStart(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		t.Run(strconv.FormatBool(refuse), func(t *testing.T) {
			manager, runner, execution := newPassthroughRunnerManager(t)
			root := t.TempDir()
			target := filepath.Join(root, "child-started")
			captured := false
			persistenceErr := errors.New("snapshot write failed")
			manager.SetExecutionSnapshotRecorder(func(_ context.Context, _ *AgentExecution, snapshot models.ExecutionSnapshot, sandbox *contract.Launch) error {
				require.NoFileExists(t, target)
				require.Nil(t, sandbox)
				require.Equal(t, "native_terminal", snapshot.Transport)
				require.Equal(t, "sh", snapshot.Agent.Executable)
				require.Nil(t, snapshot.Wrapper)
				captured = true
				if refuse {
					return persistenceErr
				}
				return nil
			})
			request := process.InteractiveStartRequest{SessionID: execution.SessionID, WorkingDir: root, Command: []string{"/bin/sh", "-c", `printf allowed > "$1"; read ignored`, "--", target}, ImmediateStart: true}
			info, err := manager.startSandboxInteractive(t.Context(), execution, runner, request)
			if info != nil {
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					require.NoError(t, runner.Stop(ctx, info.ID))
				})
			}
			if refuse {
				require.ErrorIs(t, err, persistenceErr)
				require.Nil(t, info)
				require.NoFileExists(t, target)
			} else {
				require.NoError(t, err)
				require.Eventually(t, func() bool { _, err := os.Stat(target); return err == nil }, time.Second, 10*time.Millisecond)
			}
			require.True(t, captured)
		})
	}
}
