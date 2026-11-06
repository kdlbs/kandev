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
	t.Cleanup(func() { _ = j.Close() })
	payload := []byte(`{"text":"interrupted instruction"}`)
	_, err = j.PutSubmission(ctx, Submission{
		ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Hash: SubmissionHash(payload), Payload: payload, State: SubmissionDispatching,
	})
	require.NoError(t, err)
	require.NoError(t, j.Close())

	runtime, owner := exitedRetainedAgentOwner(t)
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

	for _, test := range []struct {
		name       string
		identity   processidentity.Identity
		terminated bool
	}{
		{name: "agent group", identity: owner, terminated: true},
		{name: "runtime only", identity: runtime},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := ReadRetainedRecovery(ctx, RetainedRecoveryRequest{
				Root: root, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
				StreamID: "stream", SubmissionID: "prompt:one", OriginalRuntime: test.identity, Limit: 4,
			})
			require.NoError(t, err)
			require.Equal(t, test.terminated, result.ProcessTerminated)
		})
	}
}

func exitedRetainedAgentOwner(t *testing.T) (processidentity.Identity, processidentity.Identity) {
	t.Helper()
	runtime, err := processidentity.Capture(os.Getpid())
	require.NoError(t, err)
	agent := exec.Command("sleep", "30")
	agent.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, agent.Start())
	t.Cleanup(func() {
		if agent.ProcessState == nil {
			_ = syscall.Kill(-agent.Process.Pid, syscall.SIGKILL)
			_ = agent.Wait()
		}
	})
	agentIdentity, err := processidentity.Capture(agent.Process.Pid)
	require.NoError(t, err)
	require.NoError(t, syscall.Kill(-agent.Process.Pid, syscall.SIGKILL))
	_ = agent.Wait()
	return runtime, runtime.WithAgentGroup(agentIdentity)
}

func TestRetainedGapAcceptsAgentGroupTermination(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	j, err := Open(Config{Path: CheckStorage(root, "session").Path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = j.Close() })
	payload := []byte(`{"text":"interrupted instruction"}`)
	_, err = j.PutSubmission(ctx, Submission{
		ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Hash: SubmissionHash(payload), Payload: payload, State: SubmissionDispatching,
	})
	require.NoError(t, err)
	_, err = j.Append(ctx, Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", SubmissionID: "prompt:one", Type: "context_window", Payload: []byte(`{}`)})
	require.NoError(t, err)
	require.NoError(t, j.Acknowledge(ctx, "stream", 1))
	require.NoError(t, j.Close())
	runtime, owner := exitedRetainedAgentOwner(t)
	request := RetainedRecoveryRequest{
		Root: root, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", SubmissionID: "prompt:one", OriginalRuntime: runtime,
		Limit: 4, PrunedThrough: 1, ExpectedHash: SubmissionHash(payload),
	}
	_, err = ReadRetainedRecovery(ctx, request)
	require.ErrorIs(t, err, ErrOwnerMismatch, "runtime-only evidence cannot authorize history-gap recovery")

	request.OriginalRuntime = owner
	result, err := ReadRetainedRecovery(ctx, request)
	require.NoError(t, err)
	require.True(t, result.HistoryGapVerified)
	require.True(t, result.ProcessTerminated)
	require.Equal(t, SubmissionInterruptedUnknown, result.Submission.State)
	require.Zero(t, result.Submission.TerminalSequence)
}
