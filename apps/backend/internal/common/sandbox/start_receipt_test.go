//go:build linux

package sandbox

import (
	"context"
	"github.com/kandev/kandev/internal/common/processcontainment"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxStartReceiptRequiresExactCompletedAttempt(t *testing.T) {
	launch := &Launch{AttemptID: "attempt", Generation: 3, PolicyDigest: strings.Repeat("a", 64)}
	receipt := &StartReceipt{Version: 1, AttemptID: launch.AttemptID, Generation: launch.Generation, PolicyDigest: launch.PolicyDigest, Outcome: StartNotStarted}
	require.NoError(t, receipt.ValidateStopped(launch))
	for _, change := range []func(*StartReceipt){
		func(r *StartReceipt) { r.Version++ },
		func(r *StartReceipt) { r.AttemptID = "other" },
		func(r *StartReceipt) { r.Generation++ },
		func(r *StartReceipt) { r.PolicyDigest = strings.Repeat("b", 64) },
		func(r *StartReceipt) { r.Outcome = "unknown" },
		func(r *StartReceipt) { r.Outcome = StartTerminated },
		func(r *StartReceipt) { r.Owner = &ProcessOwner{} },
	} {
		changed := *receipt
		change(&changed)
		require.Error(t, changed.ValidateStopped(launch))
	}
	require.Error(t, (*StartReceipt)(nil).ValidateStopped(launch))
	require.Error(t, receipt.ValidateStopped(nil))
}

func TestSandboxStartReceiptCannotRetireLiveOrReplacedProcess(t *testing.T) {
	group, err := processcontainment.Create()
	if err != nil && os.Getenv("KANDEV_TEST_SANDBOX_CGROUP") != "1" {
		t.Skip("requires delegated cgroup v2")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, group.Kill(context.Background())); require.NoError(t, group.Remove()) })
	command := exec.Command("sleep", "30")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	lease, err := group.Configure(command)
	require.NoError(t, err)
	require.NoError(t, command.Start())
	require.NoError(t, lease.Close())
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	launch := &Launch{Containment: group, AttemptID: "attempt", Generation: 1, PolicyDigest: strings.Repeat("a", 64)}
	owner, err := CaptureProcessOwner(launch, command.Process.Pid)
	require.NoError(t, err)
	receipt := &StartReceipt{Version: 1, AttemptID: launch.AttemptID, Generation: launch.Generation, PolicyDigest: launch.PolicyDigest, Outcome: StartTerminated, Owner: owner}
	require.Error(t, receipt.ValidateStopped(launch), "a live process is not a cleanup receipt")
	require.NoError(t, command.Process.Kill())
	require.Error(t, command.Wait())
	require.NoError(t, receipt.ValidateStopped(launch))
	uncontained := launch.Clone()
	uncontained.Containment = nil
	require.Error(t, receipt.ValidateStopped(uncontained), "session exit alone does not prove descendant cleanup")
	changed := *owner
	changed.Generation++
	receipt.Owner = &changed
	require.Error(t, receipt.ValidateStopped(launch))
}
