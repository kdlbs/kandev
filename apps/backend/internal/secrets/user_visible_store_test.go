package secrets

import (
	"context"
	"errors"
	"testing"
)

func TestUserVisibleStoreHidesInternalGitHubSecrets(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	internal := &SecretWithValue{
		Secret: Secret{ID: "github:user:workspace-1:user-1:access", Name: "personal token"},
		Value:  "personal-secret",
	}
	visible := &SecretWithValue{Secret: Secret{ID: "user-secret", Name: "user token"}, Value: "user-value"}
	if err := store.Create(ctx, internal); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, visible); err != nil {
		t.Fatal(err)
	}

	wrapped := NewUserVisibleStore(store)
	items, err := wrapped.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != visible.ID {
		t.Fatalf("visible secrets = %+v, want only %q", items, visible.ID)
	}
	for _, operation := range []func() error{
		func() error { _, err := wrapped.Get(ctx, internal.ID); return err },
		func() error { _, err := wrapped.Reveal(ctx, internal.ID); return err },
		func() error { return wrapped.Update(ctx, internal.ID, &UpdateSecretRequest{}) },
		func() error { return wrapped.Delete(ctx, internal.ID) },
	} {
		if err := operation(); !errors.Is(err, ErrNotFound) {
			t.Fatalf("internal operation error = %v, want not found", err)
		}
	}
	if got, err := store.Reveal(ctx, internal.ID); err != nil || got != "personal-secret" {
		t.Fatalf("raw internal reveal = %q, %v", got, err)
	}
}

func TestUserVisibleStoreRejectsInternalCreate(t *testing.T) {
	store := newTestSQLiteStore(t)
	err := NewUserVisibleStore(store).Create(context.Background(), &SecretWithValue{
		Secret: Secret{ID: "github:workspace:workspace-1:pat", Name: "PAT"}, Value: "secret",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Create() error = %v, want not found", err)
	}
}

func TestIsInternalIDRecognizesKubernetesRuntimeSecrets(t *testing.T) {
	t.Parallel()

	if !IsInternalID("kandev-runtime:execution-1:agentctl-auth-token") {
		t.Fatal("Kubernetes runtime secret ID must be internal")
	}
	if IsInternalID("legacy-random-runtime-secret-id") {
		t.Fatal("legacy random secret ID must remain user-visible")
	}
}

func TestUserVisibleStoreWorkspaceOperationsFilterInternalSecrets(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	global := &SecretWithValue{Secret: Secret{ID: "global-visible", Name: "global"}, Value: "global-value"}
	workspace := &SecretWithValue{Secret: Secret{
		ID: "workspace-visible", Name: "workspace", Scope: ScopeWorkspace, WorkspaceID: "workspace-a",
	}, Value: "workspace-value"}
	internal := &SecretWithValue{Secret: Secret{
		ID: "github:internal-workspace", Name: "internal", Scope: ScopeWorkspace, WorkspaceID: "workspace-a",
	}, Value: "internal-value"}
	for _, secret := range []*SecretWithValue{global, workspace, internal} {
		if err := store.Create(ctx, secret); err != nil {
			t.Fatalf("create %s: %v", secret.ID, err)
		}
	}

	wrapped := NewUserVisibleStore(store)
	scoped, ok := wrapped.(ScopedSecretStore)
	if !ok {
		t.Fatal("user-visible store is not scoped")
	}
	items, err := scoped.ListScoped(ctx, SecretListOptions{
		Scope: ScopeWorkspace, WorkspaceID: "workspace-a", IncludeGlobal: true,
	})
	if err != nil {
		t.Fatalf("list workspace options: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("visible workspace items = %#v, want global and workspace entries", items)
	}
	if _, err := scoped.GetForWorkspace(ctx, workspace.ID, "workspace-a"); err != nil {
		t.Fatalf("get workspace secret: %v", err)
	}
	if _, err := scoped.GetForWorkspace(ctx, internal.ID, "workspace-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get internal workspace secret = %v, want not found", err)
	}
	if got, err := scoped.RevealForWorkspace(ctx, workspace.ID, "workspace-a"); err != nil || got != workspace.Value {
		t.Fatalf("reveal workspace secret = %q, %v", got, err)
	}

	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin cleanup transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	transactional, ok := wrapped.(WorkspaceSecretTransactionalDeleter)
	if !ok {
		t.Fatal("user-visible store is not transactional")
	}
	if err := transactional.DeleteWorkspaceSecretsTx(ctx, tx, "workspace-a"); err != nil {
		t.Fatalf("transactional workspace cleanup: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit cleanup transaction: %v", err)
	}
	if _, err := store.Get(ctx, workspace.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("workspace secret after cleanup = %v, want not found", err)
	}
	if _, err := store.Get(ctx, internal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("internal workspace secret after cleanup = %v, want not found", err)
	}
}
