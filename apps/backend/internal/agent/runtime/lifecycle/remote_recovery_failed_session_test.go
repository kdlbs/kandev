package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestRemoteRecoverySnapshotFailedSessionReattachRules(t *testing.T) {
	row := func(token string, resumable bool) *models.ExecutorRunning {
		return &models.ExecutorRunning{
			SessionID: "s", TaskID: "t", AgentExecutionID: "e", Runtime: agentruntime.RuntimeSSH,
			ResumeToken: token, Resumable: resumable,
		}
	}
	failed := func(record *models.ExecutorRunning, proof bool) *RemoteRecoverySnapshot {
		snapshot := task08RemoteSnapshot(record)
		snapshot.SessionState = models.TaskSessionStateFailed
		snapshot.RecoverySourcePresent = proof
		if proof {
			snapshot.RecoveryExecutionID = record.AgentExecutionID
			snapshot.RecoverySubmissionID, snapshot.RecoveryStreamID = "sub", "stream"
			snapshot.RecoveryIncarnationID = "inc"
			snapshot.RecoveryHarnessGeneration, snapshot.RecoveryRevision = 1, 1
		}
		return snapshot
	}

	resumable := row("token", true)
	require.NoError(t, validateRemoteRecoverySnapshot(failed(resumable, false), resumable),
		"a resumable FAILED session reattaches its live agent")

	noToken := row("", true)
	require.Error(t, validateRemoteRecoverySnapshot(failed(noToken, false), noToken),
		"a FAILED session that cannot resume and has no delivery proof stays ineligible")

	notResumable := row("token", false)
	notResumable.Runtime = agentruntime.RuntimeRemoteDocker
	require.Error(t, validateRemoteRecoverySnapshot(failed(notResumable, false), notResumable))

	withProof := row("", false)
	require.NoError(t, validateRemoteRecoverySnapshot(failed(withProof, true), withProof),
		"the delivery-proof path is unchanged")
}
