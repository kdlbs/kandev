package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-EXACT-PROFILE-LAUNCH-001.2
func TestPostgresExactProfileAssignmentSerializesTaskAndChecksCurrentToken(t *testing.T) {
	testutil.PostgresDSNFromEnv(t)
	fixture := newPGPolicyPatchFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, fixture.primary.CreateWorkspace(ctx, &models.Workspace{ID: "exact-pg-workspace", Name: "Exact PG"}))
	require.NoError(t, fixture.primary.CreateTask(ctx, &models.Task{
		ID: "exact-pg-task", WorkspaceID: "exact-pg-workspace", Title: "Exact PG task",
	}))
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS agent_profiles (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, enabled BOOLEAN NOT NULL, deleted_at TIMESTAMPTZ)`,
		`CREATE TABLE IF NOT EXISTS agent_profile_revisions (profile_id TEXT PRIMARY KEY, revision BIGINT NOT NULL)`,
		`INSERT INTO agent_profiles (id, workspace_id, enabled) VALUES ('exact-pg-profile', 'exact-pg-workspace', TRUE)`,
		`INSERT INTO agent_profile_revisions (profile_id, revision) VALUES ('exact-pg-profile', 2)`,
	} {
		_, err := fixture.primary.db.ExecContext(ctx, stmt)
		require.NoError(t, err)
	}
	assignment := &models.ExactProfileAssignment{
		TaskID: "exact-pg-task", WorkspaceID: "exact-pg-workspace", AgentProfileID: "exact-pg-profile",
		ProfileRevision: 2, Generation: 1,
	}
	stale := *assignment
	stale.ProfileRevision = 1
	changed, err := fixture.primary.AssignExactProfileAssignmentIfProfileCurrent(ctx, &stale)
	require.False(t, changed)
	require.ErrorIs(t, err, models.ErrExactProfileAssignmentInvalidInput)

	holder, err := fixture.primary.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"exact-profile-assignment:"+assignment.TaskID)
	require.NoError(t, err)
	type outcome struct {
		changed bool
		err     error
	}
	done := make(chan struct{})
	var got outcome
	go func() {
		got.changed, got.err = fixture.worker.AssignExactProfileAssignmentIfProfileCurrent(ctx, assignment)
		close(done)
	}()
	require.NoError(t, waitForPostgresLock(ctx, fixture.observer.db, fixture.workerPID, done))
	require.NoError(t, holder.Commit())
	<-done
	require.NoError(t, got.err)
	require.True(t, got.changed)

	changed, err = fixture.primary.AssignExactProfileAssignmentIfProfileCurrent(ctx, assignment)
	require.NoError(t, err)
	require.False(t, changed)
	replacement := *assignment
	replacement.Generation = 2
	changed, err = fixture.primary.AssignExactProfileAssignmentIfProfileCurrent(ctx, &replacement)
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = fixture.primary.AssignExactProfileAssignmentIfProfileCurrent(ctx, assignment)
	require.False(t, changed)
	require.True(t, errors.Is(err, models.ErrExactProfileAssignmentGeneration))
}
