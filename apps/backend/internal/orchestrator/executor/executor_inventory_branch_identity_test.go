package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.12
func TestAttestedBranchEvolutionRejectsStaleOriginalBranch(t *testing.T) {
	assertBranchEvolutionRefused(t, func(t *testing.T, f *branchContinuityFixture) {
		require.NoError(t, f.repo.UpdateTaskSessionWorktreeBranchByWorktree(t.Context(),
			f.session.ID, f.row.WorktreeID, f.receipt.Preservation.ObservedBranch))
	})
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.11
func TestAttestedBranchEvolutionDoesNotReadIgnoredContent(t *testing.T) {
	f := newBranchContinuityFixture(t)
	f.changeBranch(t, false)
	ignored := filepath.Join(f.row.WorktreePath, "build-output")
	require.NoError(t, os.WriteFile(filepath.Join(f.row.WorktreePath, ".gitignore"), []byte("build-output\n"), 0o600))
	require.NoError(t, os.WriteFile(ignored, []byte("unreadable build output\n"), 0o600))
	require.NoError(t, os.Chmod(ignored, 0))
	t.Cleanup(func() { _ = os.Chmod(ignored, 0o600) })
	if _, err := os.ReadFile(ignored); err == nil {
		t.Skip("filesystem permits reading files without read permission")
	}
	_, err := f.executor.ResumeSessionWithOptions(t.Context(), f.session, true, ResumeOptions{})
	require.NoError(t, err, "branch identity admission must not scan checkout content")
	require.Equal(t, 1, f.manager.launchAgentCallCount)
	stored, err := f.repo.GetWorkspaceInventoryRepairReceipt(t.Context(), "task", "repair")
	require.NoError(t, err)
	require.Equal(t, f.receipt, stored)
}
