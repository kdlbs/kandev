//go:build linux

package journal

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/processidentity"
)

func TestRetainedRecoveryAcceptsAgentGroupTermination(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	capability := CheckStorage(root, "session")
	j, err := Open(Config{Path: capability.Path})
	require.NoError(t, err)
	payload := []byte(`{"text":"interrupted instruction"}`)
	_, err = j.PutSubmission(ctx, Submission{
		ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Hash: SubmissionHash(payload), Payload: payload, State: SubmissionDispatching,
	})
	require.NoError(t, err)
	require.NoError(t, j.Close())

	runtime, err := processidentity.Capture(os.Getpid())
	require.NoError(t, err)
	agent := exec.Command("sleep", "30")
	agent.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, agent.Start())
	agentIdentity, err := processidentity.Capture(agent.Process.Pid)
	require.NoError(t, err)
	require.NoError(t, syscall.Kill(-agent.Process.Pid, syscall.SIGKILL))
	_ = agent.Wait()

	owner := runtime.WithAgentGroup(agentIdentity)
	evidence, err := ReadRetainedReconstructionEvidence(ctx, RetainedReconstructionEvidenceRequest{
		Root: root, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, OriginalRuntime: &owner,
	})
	require.NoError(t, err)
	require.NotNil(t, evidence.ProcessTerminated)
	require.True(t, *evidence.ProcessTerminated,
		"an exited agent group proves termination while the shared runtime is still alive")

	evidence, err = ReadRetainedReconstructionEvidence(ctx, RetainedReconstructionEvidenceRequest{
		Root: root, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, OriginalRuntime: &runtime,
	})
	require.NoError(t, err)
	require.NotNil(t, evidence.ProcessTerminated)
	require.False(t, *evidence.ProcessTerminated, "a runtime-only binding still waits for the runtime")
}
