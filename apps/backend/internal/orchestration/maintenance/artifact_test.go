package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantMaintenanceReviewArtifact(t *testing.T) {
	sandbox, source, grant := sandboxFixture(t)
	ctx := context.Background()
	guard := func(context.Context) error { return nil }
	require.NoError(t, sandbox.Prepare(ctx, source, grant, guard))
	file, err := sandbox.Read(ctx, grant, "scripts/example.js")
	require.NoError(t, err)
	file.Content = "module.exports = 2;\n"
	patched, err := sandbox.Patch(ctx, grant, file, guard)
	require.NoError(t, err)
	job, err := sandbox.jobPath(grant)
	require.NoError(t, err)
	validation := models.MaintenanceValidation{TreeOID: patched.TreeOID, GrantRevision: grant.Revision, Passed: true, Checks: []models.MaintenanceCheck{{Kind: "positive"}, {Kind: "negative"}}}
	require.NoError(t, writePrivateJSON(filepath.Join(job, "validation.json"), validation))
	committed, err := sandbox.Commit(ctx, grant, guard)
	require.NoError(t, err)
	// A new process reconstructs the receipt without creating another commit.
	restarted := New(sandbox.root)
	artifact, err := restarted.Review(ctx, grant)
	require.NoError(t, err)
	require.Equal(t, committed.CommitOID, artifact.CommitOID)
	require.Equal(t, committed.TreeOID, artifact.TreeOID)
	require.Contains(t, artifact.Patch, "+module.exports = 2;")
	require.NotEmpty(t, artifact.PatchSHA256)
	require.True(t, artifact.Validation.Passed)
	patchPath := filepath.Join(t.TempDir(), "repair.patch")
	require.NoError(t, os.WriteFile(patchPath, []byte(artifact.Patch), 0600))
	fixtureGit(t, source, "apply", "--check", patchPath)
	wrongOwner := grant
	wrongOwner.OwnerUserID = "foreign"
	_, err = restarted.Review(ctx, wrongOwner)
	require.Error(t, err)
}

func TestAssistantMaintenanceRejectsTruncatedCommandEvidence(t *testing.T) {
	_, _, err := runCommand(context.Background(), t.TempDir(), "printf", "%s", strings.Repeat("x", 70000))
	require.Error(t, err, "a truncated changed-file list or check log is not complete evidence")
}
