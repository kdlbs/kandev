package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// Regressions for PR review findings on the repository-set service.

func TestDeletingARepositoryPublishesTheSetsThatHeldIt(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	ctx := context.Background()

	fullStack := createFullStackSet(t, svc)
	ordersOnly, err := svc.CreateRepositorySet(ctx, &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          "Orders only",
		RepositoryIDs: []string{"repo-orders"},
	})
	if err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}
	eventBus.ClearEvents()

	if err := svc.DeleteRepository(ctx, "repo-gateway"); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}

	// A client that already loaded the workspace only reacts to repository_set.*
	// events, so the pruned set has to be republished or it keeps offering the
	// deleted member until reload.
	var updatedIDs []string
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != "repository_set.updated" {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data = %#v", event.Data)
		}
		id, _ := data["id"].(string)
		updatedIDs = append(updatedIDs, id)
	}
	if len(updatedIDs) != 1 || updatedIDs[0] != fullStack.ID {
		t.Fatalf("republished sets = %v, want only %s", updatedIDs, fullStack.ID)
	}

	// The set that never held the repository is untouched.
	unaffected, err := svc.GetRepositorySet(ctx, ordersOnly.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	if len(unaffected.Items) != 1 {
		t.Fatalf("unaffected set membership = %+v", unaffected.Items)
	}
}

func TestDeletingARepositoryInNoSetPublishesNoSetUpdate(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	ctx := context.Background()
	createFullStackSet(t, svc)
	eventBus.ClearEvents()

	// repo-orders is in no set, so nothing about sets changed.
	if err := svc.DeleteRepository(ctx, "repo-orders"); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == "repository_set.updated" {
			t.Fatalf("published a set update for a repository in no set: %#v", event.Data)
		}
	}
}

func TestUpdateRepositorySetRejectsMemberAndLeavesMetadataAlone(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	ctx := context.Background()
	set := createFullStackSet(t, svc)

	// Validation runs before the write, so neither half lands.
	name := "Renamed"
	bad := []string{"repo-web", "repo-missing"}
	if _, err := svc.UpdateRepositorySet(ctx, set.ID, &UpdateRepositorySetRequest{
		Name:          &name,
		RepositoryIDs: &bad,
	}); err == nil {
		t.Fatal("UpdateRepositorySet accepted an unknown member")
	}

	loaded, err := svc.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	if loaded.Name != "Full-stack" {
		t.Fatalf("name = %q, want the pre-update value", loaded.Name)
	}
	if len(loaded.Items) != 2 {
		t.Fatalf("membership = %+v, want unchanged", loaded.Items)
	}
}

func TestCreateRepositorySetSurvivesAWorkspaceWithNoSetStore(t *testing.T) {
	// repositorySetIDsHolding must tolerate a service built without the set
	// store, because repository deletion runs on every deployment path.
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-bare", Name: "Bare"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-bare", WorkspaceID: "ws-bare", Name: "bare", SourceType: sourceTypeLocal,
		LocalPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := svc.DeleteRepository(ctx, "repo-bare"); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}
}
