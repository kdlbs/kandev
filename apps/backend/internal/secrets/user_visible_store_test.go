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
