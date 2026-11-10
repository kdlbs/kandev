package secrets

import (
	"context"
	"errors"
	"testing"
)

// The production wiring wraps the store with NewUserVisibleStore before the
// service consumes it. The wrapper's global Update intentionally hides
// workspace-owned ids, so the workspace update paths must route through the
// workspace-aware surface the way deleteStored does.
func newProductionService(t *testing.T) *Service {
	t.Helper()
	store := newTestSQLiteStore(t)
	svc := NewService(NewUserVisibleStore(store), nil)
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	return svc
}

func createWorkspaceSecret(t *testing.T, svc *Service) *SecretListItem {
	t.Helper()
	created, err := svc.Create(context.Background(), &CreateSecretRequest{
		Name:        "deploy token",
		Value:       "original",
		Scope:       ScopeWorkspace,
		WorkspaceID: "workspace-a",
	})
	if err != nil {
		t.Fatalf("create workspace secret: %v", err)
	}
	return created
}

func TestService_UpdateWorkspaceSecretThroughUserVisibleStore(t *testing.T) {
	svc := newProductionService(t)
	ctx := context.Background()
	created := createWorkspaceSecret(t, svc)

	updated, err := svc.UpdateWorkspaceSecret(ctx, created.ID, "workspace-a", &UpdateSecretRequest{
		Value: stringPtr("rotated"),
	})
	if err != nil {
		t.Fatalf("update workspace secret: %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("updated id = %q, want %q", updated.ID, created.ID)
	}

	value, err := svc.RevealWorkspaceSecret(ctx, created.ID, "workspace-a")
	if err != nil {
		t.Fatalf("reveal updated secret: %v", err)
	}
	if value != "rotated" {
		t.Fatalf("revealed value = %q, want %q", value, "rotated")
	}

	if _, err := svc.UpdateWorkspaceSecret(ctx, created.ID, "workspace-b", &UpdateSecretRequest{
		Value: stringPtr("hijack"),
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update from unauthorized workspace = %v, want ErrNotFound", err)
	}
}

func TestService_UpdateForWorkspaceThroughUserVisibleStore(t *testing.T) {
	svc := newProductionService(t)
	ctx := context.Background()
	created := createWorkspaceSecret(t, svc)

	updated, err := svc.UpdateForWorkspace(ctx, created.ID, "workspace-a", &UpdateSecretRequest{
		Name: stringPtr("renamed token"),
	})
	if err != nil {
		t.Fatalf("update workspace secret: %v", err)
	}
	if updated.Name != "renamed token" {
		t.Fatalf("updated name = %q, want %q", updated.Name, "renamed token")
	}

	global, err := svc.Create(ctx, &CreateSecretRequest{Name: "global", Value: "value"})
	if err != nil {
		t.Fatalf("create global secret: %v", err)
	}
	if _, err := svc.UpdateForWorkspace(ctx, global.ID, "workspace-a", &UpdateSecretRequest{
		Name: stringPtr("renamed global"),
	}); err != nil {
		t.Fatalf("update global through workspace endpoint: %v", err)
	}
}

func TestService_GlobalUpdateThroughUserVisibleStore(t *testing.T) {
	svc := newProductionService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, &CreateSecretRequest{Name: "global", Value: "value"})
	if err != nil {
		t.Fatalf("create global secret: %v", err)
	}
	if _, err := svc.Update(ctx, created.ID, &UpdateSecretRequest{Name: stringPtr("renamed")}); err != nil {
		t.Fatalf("update global secret: %v", err)
	}
}
