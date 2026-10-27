package process

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxNativeFailedSpawnReturnsNonReplayableReceipt(t *testing.T) {
	launch, _ := processSandboxFixture(t)
	runner := NewInteractiveRunner(nil, newTestLogger(t), 4096)
	request := InteractiveStartRequest{SessionID: "session", Command: []string{filepath.Join(t.TempDir(), "missing")}, WorkingDir: t.TempDir(), Sandbox: launch, ImmediateStart: true}
	_, err := runner.Start(t.Context(), request)
	require.Error(t, err)
	var failure *sandbox.StartFailure
	require.True(t, errors.As(err, &failure), "a completed failed native spawn needs its exact receipt")
	require.NoError(t, failure.Receipt.ValidateStopped(launch))
	require.Equal(t, sandbox.StartNotStarted, failure.Receipt.Outcome)
	request.Command = []string{"/bin/sh", "-c", "read line"}
	_, err = runner.Start(t.Context(), request)
	require.Error(t, err, "the receipt must fence later starts of the same attempt")
	next, wrapperReceipt := processSandboxFixture(t)
	next.AttemptID = "replacement"
	request.Sandbox = next
	request.Env = map[string]string{"SANDBOX_RECEIPT": wrapperReceipt}
	info, err := runner.Start(t.Context(), request)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, runner.Stop(ctx, info.ID))
	})
}
