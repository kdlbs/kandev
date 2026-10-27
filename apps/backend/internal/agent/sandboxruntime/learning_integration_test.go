//go:build linux

package sandboxruntime

import (
	"context"
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/stretchr/testify/require"
)

func assertRuntimeLearning(t *testing.T, ctx context.Context, manager *Manager, policies *policy.Service, report *policy.Report, target string) {
	t.Helper()
	policies.SetLearningCompiler(manager.CompileLearning)
	var selected string
	for _, record := range report.Data.Records {
		if record.Target == target && record.Operation == "read" {
			selected = record.ID
			break
		}
	}
	require.NotEmpty(t, selected)
	proposal, err := policies.ProposeLearning(ctx, report.SessionID, report.AttemptID, policy.LearningRequest{
		ReportDigest: report.Digest, RecordIDs: []string{selected}, Name: "Reviewed access", Scope: "workspace"})
	require.NoError(t, err)
	require.Len(t, proposal.AddedGrants.Filesystem, 1)
	require.Equal(t, target, proposal.AddedGrants.Filesystem[0].Path)
	require.False(t, proposal.AddedGrants.Filesystem[0].Writable)
	revision, err := policies.PublishLearning(ctx, proposal.ID, proposal.Digest)
	require.NoError(t, err)
	require.Equal(t, "workspace", revision.Policy.WorkspaceID)
	require.Equal(t, proposal.ID, revision.LearningProposalID)
}
