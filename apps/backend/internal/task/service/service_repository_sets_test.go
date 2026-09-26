package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// seedSetWorkspace creates one workspace with three repositories, the fixture
// every repository-set service test builds on.
func seedSetWorkspace(t *testing.T, svc *Service, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateRepository(context.Context, *models.Repository) error
}) {
	t.Helper()
	_ = svc
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	for _, id := range []string{"repo-web", "repo-gateway", "repo-orders"} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-1", Name: id, SourceType: sourceTypeLocal, LocalPath: t.TempDir(),
		}); err != nil {
			t.Fatalf("CreateRepository %s: %v", id, err)
		}
	}
}

func createFullStackSet(t *testing.T, svc *Service) *models.RepositorySet {
	t.Helper()
	set, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          "Full-stack",
		Description:   "web + gateway",
		RepositoryIDs: []string{"repo-web", "repo-gateway"},
	})
	if err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}
	return set
}

func TestCreateRepositorySetPersistsAndPublishes(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)

	set := createFullStackSet(t, svc)

	if set.ID == "" || set.WorkspaceID != "ws-1" || set.Name != "Full-stack" {
		t.Fatalf("created set = %+v", set)
	}
	if got := set.RepositoryIDs(); len(got) != 2 || got[0] != "repo-web" || got[1] != "repo-gateway" {
		t.Fatalf("membership = %v", got)
	}

	events := eventBus.GetPublishedEvents()
	if len(events) != 1 || events[0].Type != "repository_set.created" {
		t.Fatalf("published events = %#v, want one repository_set.created", events)
	}
	data, ok := events[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data = %#v", events[0].Data)
	}
	if data["workspace_id"] != "ws-1" || data["id"] != set.ID {
		t.Fatalf("event data = %#v, want id + workspace_id for workspace routing", data)
	}
}

func TestCreateRepositorySetTrimsNameAndRejectsBlank(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	ctx := context.Background()

	set, err := svc.CreateRepositorySet(ctx, &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          "  Full-stack  ",
		RepositoryIDs: []string{"repo-web"},
	})
	if err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}
	if set.Name != "Full-stack" {
		t.Fatalf("name = %q, want trimmed", set.Name)
	}

	_, err = svc.CreateRepositorySet(ctx, &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          "   ",
		RepositoryIDs: []string{"repo-web"},
	})
	if !errors.Is(err, ErrInvalidRepositorySet) {
		t.Fatalf("blank name error = %v, want ErrInvalidRepositorySet", err)
	}
}

func TestCreateRepositorySetRejectsOverlongName(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)

	_, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          strings.Repeat("x", 101),
		RepositoryIDs: []string{"repo-web"},
	})
	if !errors.Is(err, ErrInvalidRepositorySet) {
		t.Fatalf("overlong name error = %v, want ErrInvalidRepositorySet", err)
	}

	// The boundary itself is allowed.
	if _, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          strings.Repeat("x", 100),
		RepositoryIDs: []string{"repo-web"},
	}); err != nil {
		t.Fatalf("100-character name rejected: %v", err)
	}
}

func TestCreateRepositorySetRequiresAtLeastOneRepository(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)

	_, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID: "ws-1",
		Name:        "Empty",
	})
	if !errors.Is(err, ErrInvalidRepositorySet) {
		t.Fatalf("empty membership error = %v, want ErrInvalidRepositorySet", err)
	}
}

func TestCreateRepositorySetRejectsDuplicateRepositoryID(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)

	_, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          "Twice",
		RepositoryIDs: []string{"repo-web", "repo-web"},
	})
	if !errors.Is(err, ErrInvalidRepositorySet) {
		t.Fatalf("duplicate id error = %v, want ErrInvalidRepositorySet", err)
	}
}

func TestCreateRepositorySetConflictNamesExistingSet(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	createFullStackSet(t, svc)

	_, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          "full-STACK",
		RepositoryIDs: []string{"repo-orders"},
	})
	if !errors.Is(err, ErrRepositorySetNameConflict) {
		t.Fatalf("conflict error = %v, want ErrRepositorySetNameConflict", err)
	}
	if !strings.Contains(err.Error(), "Full-stack") {
		t.Fatalf("conflict error %q does not name the existing set", err)
	}
}

func TestCreateRepositorySetRejectsUnknownAndForeignRepositories(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Other"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-foreign", WorkspaceID: "ws-2", Name: "foreign", SourceType: sourceTypeLocal,
		LocalPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	_, err := svc.CreateRepositorySet(ctx, &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          "Mixed",
		RepositoryIDs: []string{"repo-web", "repo-foreign", "repo-missing"},
	})
	if !errors.Is(err, ErrUnknownRepositorySetMembers) {
		t.Fatalf("member error = %v, want ErrUnknownRepositorySetMembers", err)
	}
	// Both offenders are named so the client can point at the right rows, and
	// the cross-workspace id is not distinguished from a nonexistent one.
	if !strings.Contains(err.Error(), "repo-foreign") || !strings.Contains(err.Error(), "repo-missing") {
		t.Fatalf("member error %q does not list the offending ids", err)
	}

	sets, err := svc.ListRepositorySets(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositorySets: %v", err)
	}
	if len(sets) != 0 {
		t.Fatalf("rejected create still wrote a set: %d", len(sets))
	}
}

func TestCreateRepositorySetRejectsSoftDeletedRepository(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	ctx := context.Background()
	if err := repo.DeleteRepository(ctx, "repo-gateway"); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}

	_, err := svc.CreateRepositorySet(ctx, &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          "Stale",
		RepositoryIDs: []string{"repo-web", "repo-gateway"},
	})
	if !errors.Is(err, ErrUnknownRepositorySetMembers) {
		t.Fatalf("deleted member error = %v, want ErrUnknownRepositorySetMembers", err)
	}
}

func TestUpdateRepositorySetReplacesMembershipAndPublishes(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	set := createFullStackSet(t, svc)
	eventBus.ClearEvents()

	name := "Backend"
	ids := []string{"repo-orders", "repo-gateway"}
	updated, err := svc.UpdateRepositorySet(context.Background(), set.ID, &UpdateRepositorySetRequest{
		Name:          &name,
		RepositoryIDs: &ids,
	})
	if err != nil {
		t.Fatalf("UpdateRepositorySet: %v", err)
	}
	if updated.Name != "Backend" {
		t.Fatalf("name = %q", updated.Name)
	}
	if got := updated.RepositoryIDs(); len(got) != 2 || got[0] != "repo-orders" || got[1] != "repo-gateway" {
		t.Fatalf("membership = %v, want the supplied order", got)
	}

	events := eventBus.GetPublishedEvents()
	if len(events) != 1 || events[0].Type != "repository_set.updated" {
		t.Fatalf("published events = %#v, want one repository_set.updated", events)
	}
}

func TestUpdateRepositorySetLeavesOmittedFieldsAlone(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	set := createFullStackSet(t, svc)

	description := "just the description"
	updated, err := svc.UpdateRepositorySet(context.Background(), set.ID, &UpdateRepositorySetRequest{
		Description: &description,
	})
	if err != nil {
		t.Fatalf("UpdateRepositorySet: %v", err)
	}
	if updated.Name != "Full-stack" {
		t.Fatalf("name changed to %q", updated.Name)
	}
	if updated.Description != "just the description" {
		t.Fatalf("description = %q", updated.Description)
	}
	// An absent RepositoryIDs must not be read as "remove every member".
	if got := updated.RepositoryIDs(); len(got) != 2 {
		t.Fatalf("membership = %v, want unchanged", got)
	}
}

func TestUpdateRepositorySetRejectsEmptyMembership(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	set := createFullStackSet(t, svc)

	empty := []string{}
	_, err := svc.UpdateRepositorySet(context.Background(), set.ID, &UpdateRepositorySetRequest{
		RepositoryIDs: &empty,
	})
	if !errors.Is(err, ErrInvalidRepositorySet) {
		t.Fatalf("empty membership error = %v, want ErrInvalidRepositorySet", err)
	}
	loaded, err := svc.GetRepositorySet(context.Background(), set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	if len(loaded.Items) != 2 {
		t.Fatalf("rejected update changed membership: %+v", loaded.Items)
	}
}

func TestUpdateRepositorySetKeepingItsOwnNameIsNotAConflict(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	set := createFullStackSet(t, svc)

	name := "Full-stack"
	description := "same name, new description"
	if _, err := svc.UpdateRepositorySet(context.Background(), set.ID, &UpdateRepositorySetRequest{
		Name:        &name,
		Description: &description,
	}); err != nil {
		t.Fatalf("UpdateRepositorySet with its own name: %v", err)
	}
}

func TestUpdateAndDeleteMissingRepositorySetReportNotFound(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	ctx := context.Background()

	name := "Nope"
	_, err := svc.UpdateRepositorySet(ctx, "missing", &UpdateRepositorySetRequest{Name: &name})
	if !errors.Is(err, repoerrors.ErrRepositorySetNotFound) {
		t.Fatalf("update error = %v, want ErrRepositorySetNotFound", err)
	}
	if err := svc.DeleteRepositorySet(ctx, "missing"); !errors.Is(err, repoerrors.ErrRepositorySetNotFound) {
		t.Fatalf("delete error = %v, want ErrRepositorySetNotFound", err)
	}
	if _, err := svc.GetRepositorySet(ctx, "missing"); !errors.Is(err, repoerrors.ErrRepositorySetNotFound) {
		t.Fatalf("get error = %v, want ErrRepositorySetNotFound", err)
	}
}

func TestDeleteRepositorySetPublishesAndKeepsRepositories(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	set := createFullStackSet(t, svc)
	eventBus.ClearEvents()
	ctx := context.Background()

	if err := svc.DeleteRepositorySet(ctx, set.ID); err != nil {
		t.Fatalf("DeleteRepositorySet: %v", err)
	}
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 || events[0].Type != "repository_set.deleted" {
		t.Fatalf("published events = %#v, want one repository_set.deleted", events)
	}
	if _, err := repo.GetRepository(ctx, "repo-web"); err != nil {
		t.Fatalf("set deletion touched a repository: %v", err)
	}
}

func TestListRepositorySetsIsWorkspaceScoped(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	createFullStackSet(t, svc)
	ctx := context.Background()

	sets, err := svc.ListRepositorySets(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositorySets: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("ListRepositorySets returned %d sets", len(sets))
	}

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Other"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	other, err := svc.ListRepositorySets(ctx, "ws-2")
	if err != nil {
		t.Fatalf("ListRepositorySets other workspace: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("other workspace saw %d sets", len(other))
	}
}
