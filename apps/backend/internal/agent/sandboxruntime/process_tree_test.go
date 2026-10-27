//go:build unix

package sandboxruntime

import (
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/processidentity"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func processTreePrepared(t *testing.T) *Prepared {
	t.Helper()
	return &Prepared{manager: &Manager{attempts: map[string]*Prepared{}}, orgID: "org", scope: policy.OwnershipProcessTree, startDispatched: true,
		launch: &contract.Launch{Version: 1, AttemptID: "attempt", Generation: 1, PolicyDigest: "digest"}}
}

func sessionLeader(t *testing.T) (*exec.Cmd, *contract.ProcessOwner) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "sleep", "30")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, command.Start())
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})
	identity, err := processidentity.Capture(command.Process.Pid)
	require.NoError(t, err)
	return command, &contract.ProcessOwner{AttemptID: "attempt", Generation: 1, PolicyDigest: "digest", Supervisor: identity}
}

func TestProcessTreeChildReferencesUseTheReservedTemporaryDirectory(t *testing.T) {
	base := t.TempDir()
	reserved, err := contract.ReserveRuntimeRoot(base)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reserved.Close() })
	p := &Prepared{reserved: reserved, scope: policy.OwnershipProcessTree}
	values := map[string]string{}
	for _, ref := range p.childDirectoryReferences() {
		values[ref.Key] = ref.Value
	}
	require.Equal(t, filepath.Join(reserved.Root(), "tmp"), values["sandbox.child.tmp"])
	require.Len(t, values, 1, "without agent_state only the temporary directory is bound")
	require.Empty(t, (&Prepared{}).childDirectoryReferences())
}

func TestProcessTreeOwnerNeedsASessionLeaderThatIsItsOwnGroup(t *testing.T) {
	_, owner := sessionLeader(t)
	p := processTreePrepared(t)
	require.NoError(t, p.validateOwner(owner), "no containment is consulted for a process tree")
	ordinary := *owner
	ordinary.Supervisor.GroupID = owner.Supervisor.PID + 1
	require.Error(t, p.validateOwner(&ordinary))
	ordinary = *owner
	ordinary.Supervisor.SessionID = owner.Supervisor.PID + 1
	require.Error(t, p.validateOwner(&ordinary))
}

func TestProcessTreeVerifyRequiresTheSupervisorAsTheLiveChild(t *testing.T) {
	_, owner := sessionLeader(t)
	p := processTreePrepared(t)
	p.owner = owner
	attemptOwner := &policy.AttemptOwner{SupervisorPID: owner.Supervisor.PID, SupervisorBirth: owner.Supervisor.BirthToken, ChildPID: owner.Supervisor.PID, ChildBirth: owner.Supervisor.BirthToken}
	require.NoError(t, p.verifyProcesses(t.Context(), attemptOwner))
	other := *attemptOwner
	other.ChildPID++
	require.ErrorIs(t, p.verifyProcesses(t.Context(), &other), policy.ErrConflict)
	other = *attemptOwner
	other.ChildBirth = "other"
	require.ErrorIs(t, p.verifyProcesses(t.Context(), &other), policy.ErrConflict)
}

func TestProcessTreeArtifactsStayUntilTheGroupIsEmpty(t *testing.T) {
	command, owner := sessionLeader(t)
	p := processTreePrepared(t)
	p.owner = owner
	stubGroupMembers(t, func(int) ([]int, error) { return nil, nil })
	require.ErrorIs(t, p.canRemoveArtifacts(), policy.ErrConflict, "a live supervisor blocks removal")
	require.NoError(t, command.Process.Kill())
	_ = command.Wait()
	require.NoError(t, p.canRemoveArtifacts())
	stubGroupMembers(t, func(int) ([]int, error) { return []int{owner.Supervisor.PID + 1}, nil })
	shortenGroupBound(t)
	require.ErrorIs(t, p.canRemoveArtifacts(), policy.ErrConflict, "a surviving group member blocks removal")
}

func TestProcessTreeAttemptsCaptureAndRefreshNoReport(t *testing.T) {
	p := processTreePrepared(t)
	report, err := p.captureReport(t.Context())
	require.NoError(t, err)
	require.Nil(t, report)
	p.manager.attempts["attempt"] = p
	_, err = p.manager.RefreshReport(t.Context(), "org", "attempt")
	require.ErrorIs(t, err, policy.ErrNotFound)
}

func TestProcessTreeRecoveryRequiresAnEmptyGroup(t *testing.T) {
	attempt := &policy.Attempt{Snapshot: policy.AttemptSnapshot{OwnershipScope: policy.OwnershipProcessTree}, Owner: &policy.AttemptOwner{SupervisorPID: 4242}}
	stubGroupMembers(t, func(int) ([]int, error) { return []int{4243}, nil })
	require.ErrorIs(t, releaseRecovered(t.Context(), attempt), policy.ErrConflict)
	stubGroupMembers(t, func(int) ([]int, error) { return nil, nil })
	require.NoError(t, releaseRecovered(t.Context(), attempt))
}
