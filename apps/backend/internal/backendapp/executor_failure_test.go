package backendapp

import (
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestExecutorFailureLocalControllerAuthority(t *testing.T) {
	available := agentctl.NewAvailability(nil, nil)
	attached := time.Now().UTC()
	read := localExecutorObservationReader(available, 42, attached)
	target := models.ExecutorObservationTarget{LocalPID: 42, ResourceKey: "local-pid:42", ExpectedExecutorUpdatedAt: attached.Add(time.Second)}
	require.Nil(t, read(target))
	available.MarkAvailable()
	require.Equal(t, "healthy", read(target).Outcome, "authenticated attached runtime remains available until its exit monitor proves loss")
	available.MarkUnavailable()
	lost := read(target)
	require.Equal(t, "terminated", lost.Outcome)
	require.Equal(t, "ControllerExited", lost.Reason)
	require.NotNil(t, lost.OccurredAt)
	target.LocalPID = 43
	require.Nil(t, read(target), "controller evidence cannot describe a different process")
	target.LocalPID = 42
	target.ExpectedExecutorUpdatedAt = attached.Add(-time.Second)
	require.Nil(t, read(target), "backend restart cannot attribute old rows to its new controller")
}
