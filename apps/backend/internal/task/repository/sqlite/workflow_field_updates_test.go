package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// @covers AC-TASKS-FIELD-UPDATES-002.3
func TestWorkflowFieldUpdatesFailures(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	ctx := context.Background()
	workflow := &models.Workflow{ID: "field-failures", WorkspaceID: "ws", Name: "before", Prompt: "before prompt"}
	require.NoError(t, repo.CreateWorkflow(ctx, workflow))
	before, err := repo.GetWorkflow(ctx, workflow.ID)
	require.NoError(t, err)
	name, prompt := "rejected", "rejected prompt"
	update := models.WorkflowFieldUpdate{Name: &name, Prompt: &prompt}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = repo.UpdateWorkflowFields(cancelled, workflow.ID, update)
	require.ErrorIs(t, err, context.Canceled)
	stored, err := repo.GetWorkflow(ctx, workflow.ID)
	require.NoError(t, err)
	require.Equal(t, before, stored)
	_, err = repo.db.ExecContext(ctx, `CREATE TRIGGER reject_workflow_fields BEFORE UPDATE ON workflows BEGIN SELECT RAISE(ABORT, 'reject workflow fields'); END`)
	require.NoError(t, err)
	_, err = repo.UpdateWorkflowFields(ctx, workflow.ID, update)
	require.ErrorContains(t, err, "reject workflow fields")
	stored, err = repo.GetWorkflow(ctx, workflow.ID)
	require.NoError(t, err)
	require.Equal(t, before, stored, "aborting the statement rolls back all assignments and its version")
	_, err = repo.UpdateWorkflowFields(ctx, "missing", update)
	require.ErrorIs(t, err, repoerrors.ErrWorkflowNotFound)
}

// @covers AC-TASKS-FIELD-UPDATES-002.2, AC-TASKS-FIELD-UPDATES-002.5
func TestWorkflowFieldUpdatesPresence(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	ctx := context.Background()
	seed := &models.Workflow{ID: "field-presence", WorkspaceID: "ws", Name: "before", Description: "keep", Prompt: "keep prompt", Hidden: true, Style: models.WorkflowStyleOffice, Source: models.WorkflowSourceGitHub, SourcePath: "keep.yaml"}
	require.NoError(t, repo.CreateWorkflow(ctx, seed))
	_, err := repo.db.ExecContext(ctx, `UPDATE workflows SET agent_profile_id=NULL,workflow_template_id=NULL WHERE id=?`, seed.ID)
	require.NoError(t, err)
	before, err := repo.GetWorkflow(ctx, seed.ID)
	require.NoError(t, err)
	empty := ""
	updated, err := repo.UpdateWorkflowFields(ctx, seed.ID, models.WorkflowFieldUpdate{Name: &empty})
	require.NoError(t, err)
	require.Empty(t, updated.Name)
	updated.Name, updated.UpdatedAt = before.Name, before.UpdatedAt
	require.Equal(t, before, updated, "all omitted columns retain their persisted values")
	var nulls bool
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT agent_profile_id IS NULL AND workflow_template_id IS NULL FROM workflows WHERE id=?`, seed.ID).Scan(&nulls))
	require.True(t, nulls, "scan normalization must not rewrite omitted NULL columns")
	hidden := false
	updated, err = repo.UpdateWorkflowFields(ctx, seed.ID, models.WorkflowFieldUpdate{Hidden: &hidden, Source: &empty, SourcePath: &empty, AgentProfileID: &empty})
	require.NoError(t, err)
	require.False(t, updated.Hidden)
	require.Equal(t, models.WorkflowSourceManual, updated.Source)
	require.Empty(t, updated.SourcePath)
	require.Empty(t, updated.AgentProfileID)
	require.Equal(t, before.Style, updated.Style)
	require.Equal(t, before.SortOrder, updated.SortOrder)
	require.Equal(t, before.CreatedAt, updated.CreatedAt)
}
