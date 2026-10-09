package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestApplyRepositoryUpdatesRejectsInvalidProviderScopeWithoutClearingIdentity(t *testing.T) {
	repository := &models.Repository{ProviderScope: "existing-scope"}
	invalid := strings.Repeat("x", maxProviderScopeBytes+1)

	err := applyRepositoryUpdates(repository, &UpdateRepositoryRequest{ProviderScope: &invalid})

	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("applyRepositoryUpdates error = %v, want ErrInvalidRepositorySettings", err)
	}
	if repository.ProviderScope != "existing-scope" {
		t.Fatalf("invalid scope changed repository identity to %q", repository.ProviderScope)
	}
}

// TestApplyRepositoryUpdatesAcceptsProviderRepoIDWithoutScope guards
// repoclone.Cloner.WorkspaceProviderRepositoryPath's actual invariant: only
// a non-empty provider_scope requires a paired provider_repo_id. A bare
// provider_repo_id with no scope is the normal shape for every built-in
// provider (GitHub, GitLab, Azure DevOps) — none of which resolve a
// provider connection scope — and must stay accepted.
func TestApplyRepositoryUpdatesAcceptsProviderRepoIDWithoutScope(t *testing.T) {
	repository := &models.Repository{}
	repoID := "1131388506"

	if err := applyRepositoryUpdates(repository, &UpdateRepositoryRequest{ProviderRepoID: &repoID}); err != nil {
		t.Fatalf("applyRepositoryUpdates error = %v, want nil", err)
	}
	if repository.ProviderRepoID != repoID {
		t.Fatalf("ProviderRepoID = %q, want %q", repository.ProviderRepoID, repoID)
	}
}

// TestApplyRepositoryUpdatesRejectsProviderScopeWithoutRepoID guards the
// direction that actually breaks WorkspaceProviderRepositoryPath: a scope
// with no repository ID can't build a unique scoped clone path.
func TestApplyRepositoryUpdatesRejectsProviderScopeWithoutRepoID(t *testing.T) {
	repository := &models.Repository{}
	scope := "forge-instance-a"

	err := applyRepositoryUpdates(repository, &UpdateRepositoryRequest{ProviderScope: &scope})

	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("applyRepositoryUpdates error = %v, want ErrInvalidRepositorySettings", err)
	}
}

// TestApplyRepositoryUpdatesRejectsWhitespaceOnlyRepoIDWithScope guards
// against a whitespace-only provider_repo_id slipping past the pairing
// check unnoticed: WorkspaceProviderRepositoryPath trims both fields before
// deciding whether they are present, so an untrimmed "   " paired with a
// real provider_scope would pass this validator but still trip the
// "supplied together" clone-path error once trimmed.
func TestApplyRepositoryUpdatesRejectsWhitespaceOnlyRepoIDWithScope(t *testing.T) {
	repository := &models.Repository{ProviderScope: "existing-scope"}
	repoID := "   "

	err := applyRepositoryUpdates(repository, &UpdateRepositoryRequest{ProviderRepoID: &repoID})

	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("applyRepositoryUpdates error = %v, want ErrInvalidRepositorySettings", err)
	}
}

// TestService_CreateRepositoryAcceptsProviderRepoIDWithoutScope guards the
// actual repoclone.Cloner.WorkspaceProviderRepositoryPath invariant: a bare
// provider_repo_id with no provider_scope must be accepted. This is the
// normal shape for built-in providers, which do not resolve a connection
// scope.
func TestService_CreateRepositoryAcceptsProviderRepoIDWithoutScope(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:    "ws-1",
		Name:           "kdlbs/kandev",
		SourceType:     sourceTypeProvider,
		Provider:       "github",
		ProviderRepoID: "1131388506",
		ProviderOwner:  "kdlbs",
		ProviderName:   "kandev",
	})
	if err != nil {
		t.Fatalf("CreateRepository error = %v, want nil", err)
	}
	if created.ProviderRepoID != "1131388506" {
		t.Fatalf("ProviderRepoID = %q, want %q", created.ProviderRepoID, "1131388506")
	}
}

// TestService_CreateRepositoryRejectsProviderScopeWithoutRepoID guards the
// direction that breaks the scope-isolated clone layout.
func TestService_CreateRepositoryRejectsProviderScopeWithoutRepoID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	_, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Name:          "TEAM/widgets",
		SourceType:    sourceTypeProvider,
		Provider:      "custom-provider",
		ProviderHost:  "https://forge.example.test",
		ProviderScope: "forge-instance-a",
		ProviderOwner: "TEAM",
		ProviderName:  "widgets",
	})
	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("CreateRepository error = %v, want ErrInvalidRepositorySettings", err)
	}
}

// TestService_FindOrCreateRepositoryBackfillsRepoIDWithoutScope covers the
// existing-row backfill path. A bare provider_repo_id remains valid.
func TestService_FindOrCreateRepositoryBackfillsRepoIDWithoutScope(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Name:          "kdlbs/kandev",
		SourceType:    sourceTypeProvider,
		Provider:      "github",
		ProviderHost:  "https://github.com",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	resolved, wasCreated, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:    "ws-1",
		Provider:       "github",
		ProviderHost:   "https://github.com",
		ProviderRepoID: "1131388506",
		ProviderOwner:  "kdlbs",
		ProviderName:   "kandev",
	})
	if err != nil {
		t.Fatalf("FindOrCreateRepository: %v", err)
	}
	if wasCreated || resolved.ID != created.ID {
		t.Fatalf("resolved repository = %q (created=%t), want existing %q", resolved.ID, wasCreated, created.ID)
	}
	if resolved.ProviderRepoID != "1131388506" {
		t.Fatalf("ProviderRepoID = %q, want backfilled %q", resolved.ProviderRepoID, "1131388506")
	}
	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.ProviderRepoID != "1131388506" {
		t.Fatalf("persisted ProviderRepoID = %q, want %q", stored.ProviderRepoID, "1131388506")
	}
}
