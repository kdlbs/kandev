package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Regressions for PR review findings on repository sets: atomic combined
// updates, not-found on a deleted parent, and case-insensitive name uniqueness.

func TestUpdateRepositorySetRollsBackMetadataWhenMembershipFails(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	// A membership list naming a repository that does not exist fails the item
	// insert on its foreign key, after the metadata UPDATE has already run inside
	// the same transaction.
	renamed := *set
	renamed.Name = "Renamed"
	renamed.Description = "changed"
	members := []models.RepositorySetItem{
		{RepositoryID: "repo-web"},
		{RepositoryID: "repo-does-not-exist"},
	}
	if err := repo.UpdateRepositorySet(ctx, &renamed, &members); err == nil {
		t.Fatal("UpdateRepositorySet accepted a member that does not exist")
	}

	loaded, err := repo.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	// Neither half may survive: a rename that lands while membership fails leaves
	// the set renamed but still holding the old repositories.
	if loaded.Name != "Full-stack" {
		t.Fatalf("name = %q, want the pre-update value", loaded.Name)
	}
	if loaded.Description != "web + gateway + orders" {
		t.Fatalf("description = %q, want the pre-update value", loaded.Description)
	}
	assertRepositoryIDs(t, repositoryIDsOf(loaded), "repo-web", "repo-gateway", "repo-orders")
}

func TestUpdateRepositorySetAppliesBothHalvesTogether(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	set.Name = "Backend"
	members := []models.RepositorySetItem{{RepositoryID: "repo-orders"}}
	if err := repo.UpdateRepositorySet(ctx, set, &members); err != nil {
		t.Fatalf("UpdateRepositorySet: %v", err)
	}

	loaded, err := repo.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	if loaded.Name != "Backend" {
		t.Fatalf("name = %q", loaded.Name)
	}
	assertRepositoryIDs(t, repositoryIDsOf(loaded), "repo-orders")
}

func TestUpdateRepositorySetOnDeletedSetReportsNotFound(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}
	if _, err := repo.DeleteRepositorySet(ctx, set.ID); err != nil {
		t.Fatalf("DeleteRepositorySet: %v", err)
	}

	// An empty membership list touches no item rows, so without checking the
	// metadata UPDATE's RowsAffected this used to commit successfully and let the
	// service publish an update for a set that no longer exists.
	empty := []models.RepositorySetItem{}
	if err := repo.UpdateRepositorySet(ctx, set, &empty); !errors.Is(err, repoerrors.ErrRepositorySetNotFound) {
		t.Fatalf("UpdateRepositorySet error = %v, want ErrRepositorySetNotFound", err)
	}
	if err := repo.UpdateRepositorySet(ctx, set, nil); !errors.Is(err, repoerrors.ErrRepositorySetNotFound) {
		t.Fatalf("metadata-only update error = %v, want ErrRepositorySetNotFound", err)
	}
}

func TestCreateRepositorySetRejectsNameDifferingOnlyByCase(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	// The service compares names case-insensitively, so the database has to be
	// the backstop for two concurrent creates that differ only in case.
	duplicate := &models.RepositorySet{
		WorkspaceID: "ws-sets",
		Name:        "full-STACK",
		Items:       []models.RepositorySetItem{{RepositoryID: "repo-web"}},
	}
	if err := repo.CreateRepositorySet(ctx, duplicate); err == nil {
		t.Fatal("CreateRepositorySet accepted a name differing only by case")
	}

	sets, err := repo.ListRepositorySets(ctx, "ws-sets")
	if err != nil {
		t.Fatalf("ListRepositorySets: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("rejected duplicate still wrote a row: %d sets", len(sets))
	}
}

func TestListRepositorySetIDsByRepository(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}
	other := &models.RepositorySet{
		WorkspaceID: "ws-sets",
		Name:        "Orders only",
		Items:       []models.RepositorySetItem{{RepositoryID: "repo-orders"}},
	}
	if err := repo.CreateRepositorySet(ctx, other); err != nil {
		t.Fatalf("CreateRepositorySet second: %v", err)
	}

	ids, err := repo.ListRepositorySetIDsByRepository(ctx, "repo-orders")
	if err != nil {
		t.Fatalf("ListRepositorySetIDsByRepository: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("sets holding repo-orders = %v, want both", ids)
	}

	ids, err = repo.ListRepositorySetIDsByRepository(ctx, "repo-web")
	if err != nil {
		t.Fatalf("ListRepositorySetIDsByRepository: %v", err)
	}
	if len(ids) != 1 || ids[0] != set.ID {
		t.Fatalf("sets holding repo-web = %v, want only %s", ids, set.ID)
	}
}
