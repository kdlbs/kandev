package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkspaceOwnershipRevisionAdvancesOnlyOnOwnershipTransfer(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	created := &models.Workspace{
		ID:                "workspace-owner-revision",
		Name:              "Owner revision",
		OwnerID:           "owner-a",
		OwnershipRevision: 42,
	}
	if err := repo.CreateWorkspace(ctx, created); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if created.OwnershipRevision != 1 {
		t.Fatalf("created model ownership revision = %d, want 1", created.OwnershipRevision)
	}
	if revision := readWorkspaceOwnershipRevision(t, repo, "workspace-owner-revision"); revision != 1 {
		t.Fatalf("new workspace ownership revision = %d, want 1", revision)
	}
	if err := repo.TransferWorkspaceOwnership(ctx, "workspace-owner-revision", "owner-a", "owner-a"); err != nil {
		t.Fatalf("same-owner TransferWorkspaceOwnership: %v", err)
	}
	if revision := readWorkspaceOwnershipRevision(t, repo, "workspace-owner-revision"); revision != 1 {
		t.Fatalf("same-owner transfer changed ownership revision to %d, want 1", revision)
	}

	workspace, err := repo.GetWorkspace(ctx, "workspace-owner-revision")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if workspace.OwnershipRevision != 1 {
		t.Fatalf("loaded workspace ownership revision = %d, want 1", workspace.OwnershipRevision)
	}
	workspace.Name = "Renamed workspace"
	if err := repo.UpdateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("UpdateWorkspace: %v", err)
	}
	description := "Changed through fields"
	if _, err := repo.UpdateWorkspaceFields(ctx, workspace.ID, models.WorkspaceFieldUpdate{
		Description: &description,
	}, nil); err != nil {
		t.Fatalf("UpdateWorkspaceFields: %v", err)
	}
	if revision := readWorkspaceOwnershipRevision(t, repo, workspace.ID); revision != 1 {
		t.Fatalf("ordinary workspace edits changed ownership revision to %d, want 1", revision)
	}

	if err := repo.TransferWorkspaceOwnership(ctx, workspace.ID, "owner-a", "owner-b"); err != nil {
		t.Fatalf("TransferWorkspaceOwnership to owner-b: %v", err)
	}
	if revision := readWorkspaceOwnershipRevision(t, repo, workspace.ID); revision != 2 {
		t.Fatalf("ownership revision after first transfer = %d, want 2", revision)
	}
	workspace, err = repo.GetWorkspace(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace after first transfer: %v", err)
	}
	if workspace.OwnerID != "owner-b" || workspace.OwnershipRevision != 2 {
		t.Fatalf("workspace after first transfer = owner %q revision %d, want owner-b/2", workspace.OwnerID, workspace.OwnershipRevision)
	}
	if err := repo.TransferWorkspaceOwnership(ctx, workspace.ID, "owner-b", "owner-a"); err != nil {
		t.Fatalf("TransferWorkspaceOwnership back to owner-a: %v", err)
	}
	if revision := readWorkspaceOwnershipRevision(t, repo, workspace.ID); revision != 3 {
		t.Fatalf("ownership revision after transfer back = %d, want 3", revision)
	}
	if err := repo.TransferWorkspaceOwnership(ctx, workspace.ID, "owner-b", "owner-c"); !errors.Is(err, ErrOwnershipChanged) {
		t.Fatalf("stale ownership transfer error = %v, want ErrOwnershipChanged", err)
	}
	if revision := readWorkspaceOwnershipRevision(t, repo, workspace.ID); revision != 3 {
		t.Fatalf("failed ownership transfer changed revision to %d, want 3", revision)
	}
}

func TestWorkspaceOwnershipRevisionMigrationDefaultsLegacyRows(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID:      "workspace-owner-revision-legacy",
		Name:    "Legacy owner revision",
		OwnerID: "owner-a",
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `ALTER TABLE workspaces DROP COLUMN ownership_revision`); err != nil {
		t.Fatalf("simulate legacy workspace table: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	if revision := readWorkspaceOwnershipRevision(t, repo, "workspace-owner-revision-legacy"); revision != 1 {
		t.Fatalf("legacy workspace ownership revision = %d, want 1", revision)
	}
	workspace, err := repo.GetWorkspace(ctx, "workspace-owner-revision-legacy")
	if err != nil {
		t.Fatalf("GetWorkspace after migration: %v", err)
	}
	if workspace.OwnershipRevision != 1 {
		t.Fatalf("loaded legacy ownership revision = %d, want 1", workspace.OwnershipRevision)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("replay runMigrations: %v", err)
	}
	if revision := readWorkspaceOwnershipRevision(t, repo, "workspace-owner-revision-legacy"); revision != 1 {
		t.Fatalf("ownership revision after migration replay = %d, want 1", revision)
	}
}

func readWorkspaceOwnershipRevision(t *testing.T, repo *Repository, workspaceID string) int64 {
	t.Helper()
	var revision int64
	if err := repo.db.QueryRowContext(t.Context(), `
		SELECT ownership_revision FROM workspaces WHERE id = ?
	`, workspaceID).Scan(&revision); err != nil {
		t.Fatalf("read workspace ownership revision: %v", err)
	}
	return revision
}
