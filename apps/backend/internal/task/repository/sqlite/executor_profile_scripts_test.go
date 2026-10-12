package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.11
func TestExecutorProfileScriptsStorage(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedExecutorForProfiles(t, repo, "script-storage-executor")
	profile := &models.ExecutorProfile{ID: "script-storage-profile", ExecutorID: "script-storage-executor", Name: "Before", PrepareScript: "prepare", CleanupScript: "cleanup"}
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), profile))
	before := *profile
	prepare := "not-committed"
	intent := models.ExecutorProfileScriptIntent{PrepareScript: &prepare}
	_, err := repo.DB().ExecContext(t.Context(), `CREATE TABLE script_commit_guard (profile_id TEXT REFERENCES executor_profiles(id) DEFERRABLE INITIALLY DEFERRED)`)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(t.Context(), `CREATE TRIGGER reject_script_commit AFTER UPDATE ON executor_profiles BEGIN INSERT INTO script_commit_guard(profile_id) VALUES ('missing-commit-profile'); END`)
	require.NoError(t, err)
	err = repo.UpdateExecutorProfileWithScriptIntent(t.Context(), profile, intent)
	require.ErrorContains(t, err, "FOREIGN KEY", "deferred constraint fails commit after UPDATE RETURNING")
	require.Equal(t, before, *profile, "failed commit cannot install an acknowledged pair or timestamp")
	stored, err := repo.GetExecutorProfile(t.Context(), profile.ID)
	require.NoError(t, err)
	require.Equal(t, before.PrepareScript, stored.PrepareScript)
	require.Equal(t, before.CleanupScript, stored.CleanupScript)
	require.True(t, before.UpdatedAt.Equal(stored.UpdatedAt))
	_, err = repo.DB().ExecContext(t.Context(), `DROP TRIGGER reject_script_commit`)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateExecutorProfileWithScriptIntent(t.Context(), profile, intent), "connection reused after actual failed commit")
	require.Equal(t, prepare, profile.PrepareScript)
	require.Equal(t, "cleanup", profile.CleanupScript)
	missing := models.ExecutorProfile{ID: "missing-script-storage", PrepareScript: "untouched"}
	beforeMissing := missing
	require.Error(t, repo.UpdateExecutorProfileWithScriptIntent(t.Context(), &missing, intent))
	require.Equal(t, beforeMissing, missing)
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, repo.UpdateExecutorProfileWithScriptIntent(cancelled, profile, intent), context.Canceled)
}
