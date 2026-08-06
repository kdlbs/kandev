package secrets

import (
	"context"
	"errors"
	"testing"
)

func TestService_WorkspaceSecretRequiresWorkspaceAuthorization(t *testing.T) {
	store := newTestSQLiteStore(t)
	svc := NewService(store, nil)
	wantErr := errors.New("workspace denied")
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return wantErr })

	_, err := svc.Create(context.Background(), &CreateSecretRequest{
		Name:        "workspace token",
		Value:       "value",
		Scope:       ScopeWorkspace,
		WorkspaceID: "workspace-a",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Create error = %v, want authorization error", err)
	}
}

func TestService_GlobalDefaultAndScopedOperations(t *testing.T) {
	store := newTestSQLiteStore(t)
	svc := NewService(store, nil)
	authorized := map[string]bool{"workspace-a": true}
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if !authorized[workspaceID] {
			return errors.New("workspace denied")
		}
		return nil
	})

	global, err := svc.Create(context.Background(), &CreateSecretRequest{Name: "global", Value: "value"})
	if err != nil {
		t.Fatalf("create default global: %v", err)
	}
	if global.Scope != ScopeGlobal || global.WorkspaceID != "" {
		t.Fatalf("global result = %+v, want global scope", global)
	}

	workspace, err := svc.Create(context.Background(), &CreateSecretRequest{
		Name:        "workspace",
		Value:       "value",
		Scope:       ScopeWorkspace,
		WorkspaceID: "workspace-a",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if workspace.Scope != ScopeWorkspace || workspace.WorkspaceID != "workspace-a" {
		t.Fatalf("workspace result = %+v", workspace)
	}

	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list global: %v", err)
	}
	if len(items) != 1 || items[0].ID != global.ID {
		t.Fatalf("global list = %+v, want only %s", items, global.ID)
	}

	workspaceItems, err := svc.ListScoped(context.Background(), SecretListOptions{
		Scope:       ScopeWorkspace,
		WorkspaceID: "workspace-a",
	})
	if err != nil {
		t.Fatalf("list workspace: %v", err)
	}
	if len(workspaceItems) != 1 || workspaceItems[0].ID != workspace.ID {
		t.Fatalf("workspace list = %+v, want only %s", workspaceItems, workspace.ID)
	}

	if _, err := svc.Get(context.Background(), workspace.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("global get of workspace secret = %v, want ErrNotFound", err)
	}
	if _, err := svc.GetForWorkspace(context.Background(), workspace.ID, "workspace-a"); err != nil {
		t.Fatalf("workspace get: %v", err)
	}
	if _, err := svc.GetForWorkspace(context.Background(), workspace.ID, "workspace-b"); err == nil {
		t.Fatal("workspace get from unauthorized workspace succeeded")
	}
}

func TestService_WorkspaceCRUDDoesNotMutateGlobalSecrets(t *testing.T) {
	store := newTestSQLiteStore(t)
	svc := NewService(store, nil)
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })

	global, err := svc.Create(context.Background(), &CreateSecretRequest{Name: "global", Value: "value"})
	if err != nil {
		t.Fatalf("create global: %v", err)
	}
	name := "changed"
	if _, err := svc.UpdateWorkspaceSecret(context.Background(), global.ID, "workspace-a", &UpdateSecretRequest{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("workspace update of global = %v, want ErrNotFound", err)
	}
	if _, err := svc.GetWorkspaceSecret(context.Background(), global.ID, "workspace-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("workspace get of global = %v, want ErrNotFound", err)
	}
	if _, err := svc.RevealWorkspaceSecret(context.Background(), global.ID, "workspace-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("workspace reveal of global = %v, want ErrNotFound", err)
	}
	if err := svc.DeleteWorkspaceSecret(context.Background(), global.ID, "workspace-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("workspace delete of global = %v, want ErrNotFound", err)
	}

	item, err := svc.Update(context.Background(), global.ID, &UpdateSecretRequest{Name: &name})
	if err != nil {
		t.Fatalf("global update: %v", err)
	}
	if item.Name != name {
		t.Fatalf("global name = %q, want %q", item.Name, name)
	}
}

func TestService_NormalizesLegacyGlobalScopeOnUpdate(t *testing.T) {
	store := newTestSQLiteStore(t)
	svc := NewService(store, nil)
	ctx := context.Background()

	global, err := svc.Create(ctx, &CreateSecretRequest{Name: "legacy", Value: "value"})
	if err != nil {
		t.Fatalf("create global: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, store.db.Rebind(`UPDATE secrets SET scope = '' WHERE id = ?`), global.ID); err != nil {
		t.Fatalf("seed legacy scope: %v", err)
	}

	updated, err := svc.Update(ctx, global.ID, &UpdateSecretRequest{Name: stringPtr("renamed")})
	if err != nil {
		t.Fatalf("update legacy global: %v", err)
	}
	if updated.Scope != ScopeGlobal {
		t.Fatalf("updated scope = %q, want %q", updated.Scope, ScopeGlobal)
	}
}

func stringPtr(value string) *string { return &value }
