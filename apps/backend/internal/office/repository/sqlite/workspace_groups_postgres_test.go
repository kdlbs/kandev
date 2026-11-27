package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresWorkspaceGroupOwnershipRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatal(err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, owned := range []bool{false, true} {
		group := &models.WorkspaceGroup{WorkspaceID: "ownership-workspace", OwnerTaskID: "ownership-task",
			MaterializedKind: models.WorkspaceGroupKindPlainFolder, OwnedByKandev: owned}
		if err := repo.CreateWorkspaceGroup(ctx, group); err != nil {
			t.Fatal(err)
		}
		assertPostgresWorkspaceGroupOwned(t, repo, group.ID, owned)
		assertPostgresWorkspaceGroupMembership(t, repo, group.ID)
		for _, materializedOwned := range []bool{true, false} {
			if err := repo.MarkWorkspaceMaterialized(ctx, group.ID, models.MaterializedWorkspace{
				Kind: models.WorkspaceGroupKindPlainFolder, Path: "/recorded/workspace", OwnedByKandev: materializedOwned,
			}); err != nil {
				t.Fatal(err)
			}
			assertPostgresWorkspaceGroupOwned(t, repo, group.ID, materializedOwned)
		}
	}
}

func assertPostgresWorkspaceGroupMembership(t *testing.T, repo *sqlite.Repository, groupID string) {
	t.Helper()
	ctx := context.Background()
	const taskID = "ownership-member"
	for _, role := range []string{"", models.WorkspaceMemberRoleOwner} {
		if err := repo.AddWorkspaceGroupMember(ctx, groupID, taskID, role); err != nil {
			t.Fatal(err)
		}
	}
	members, err := repo.ListWorkspaceGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].TaskID != taskID || members[0].Role != models.WorkspaceMemberRoleMember {
		t.Fatalf("duplicate admission changed membership: %+v", members)
	}
	if err := repo.ReleaseWorkspaceGroupMember(ctx, groupID, taskID, models.WorkspaceReleaseReasonArchived, "ownership-cascade"); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddWorkspaceGroupMember(ctx, groupID, taskID, ""); err != nil {
		t.Fatal(err)
	}
	members, err = repo.ListActiveWorkspaceGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("duplicate admission revived released membership: %+v", members)
	}
}

func assertPostgresWorkspaceGroupOwned(t *testing.T, repo *sqlite.Repository, id string, owned bool) {
	t.Helper()
	group, err := repo.GetWorkspaceGroup(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if group == nil || group.OwnedByKandev != owned {
		t.Fatalf("workspace ownership did not round trip: group=%+v want owned=%v", group, owned)
	}
}
