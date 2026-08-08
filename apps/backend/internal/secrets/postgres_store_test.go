package secrets

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStoreRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	crypto, err := NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	store, cleanup, err := Provide(db, db, crypto)
	if err != nil {
		t.Fatalf("provide store: %v", err)
	}
	t.Cleanup(func() {
		_ = cleanup()
	})

	ctx := context.Background()
	secret := &SecretWithValue{Secret: Secret{Name: "token"}, Value: "s3cr3t"}
	if err := store.Create(ctx, secret); err != nil {
		t.Fatalf("create secret: %v", err)
	}
	got, err := store.Reveal(ctx, secret.ID)
	if err != nil {
		t.Fatalf("reveal secret: %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("revealed value = %q, want %q", got, "s3cr3t")
	}

	metadata, err := store.Get(ctx, secret.ID)
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if metadata.Name != "token" {
		t.Fatalf("secret name = %q, want %q", metadata.Name, "token")
	}

	nameOnly := "renamed-token"
	if err := store.Update(ctx, secret.ID, &UpdateSecretRequest{Name: &nameOnly}); err != nil {
		t.Fatalf("update secret name: %v", err)
	}
	metadata, err = store.Get(ctx, secret.ID)
	if err != nil {
		t.Fatalf("get renamed secret: %v", err)
	}
	if metadata.Name != nameOnly {
		t.Fatalf("renamed secret name = %q, want %q", metadata.Name, nameOnly)
	}
	got, err = store.Reveal(ctx, secret.ID)
	if err != nil {
		t.Fatalf("reveal renamed secret: %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("renamed secret value = %q, want %q", got, "s3cr3t")
	}

	newValue := "n3w-s3cr3t"
	valueName := "renamed-token-with-value"
	if err := store.Update(ctx, secret.ID, &UpdateSecretRequest{Name: &valueName, Value: &newValue}); err != nil {
		t.Fatalf("update secret value: %v", err)
	}
	metadata, err = store.Get(ctx, secret.ID)
	if err != nil {
		t.Fatalf("get updated secret: %v", err)
	}
	if metadata.Name != valueName {
		t.Fatalf("updated secret name = %q, want %q", metadata.Name, valueName)
	}
	got, err = store.Reveal(ctx, secret.ID)
	if err != nil {
		t.Fatalf("reveal updated secret: %v", err)
	}
	if got != newValue {
		t.Fatalf("updated secret value = %q, want %q", got, newValue)
	}

	if err := store.Delete(ctx, secret.ID); err != nil {
		t.Fatalf("delete secret: %v", err)
	}
	if _, err := store.Get(ctx, secret.ID); err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted secret error = %v, want secrets.ErrNotFound", err)
	}
}

func TestPostgresScopedSecretSchemaReplay(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	crypto, err := NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	store, cleanup, err := Provide(db, db, crypto)
	if err != nil {
		t.Fatalf("provide store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	ctx := context.Background()
	secret := &SecretWithValue{
		Secret: Secret{ID: "postgres-scoped-secret-replay", Name: "replay-token"},
		Value:  "replay-value",
	}
	if err := store.Create(ctx, secret); err != nil {
		t.Fatalf("create secret: %v", err)
	}

	// Simulate a legacy database that predates the scoped columns. Running the
	// current schema initializer twice must restore both columns without
	// changing the encrypted payload or row identity.
	for _, column := range []string{"workspace_id", "scope"} {
		if _, err := db.Exec("ALTER TABLE secrets DROP COLUMN " + column); err != nil {
			t.Fatalf("drop legacy column %s: %v", column, err)
		}
	}
	if err := store.initSchema(); err != nil {
		t.Fatalf("replay scoped secret schema: %v", err)
	}
	if err := store.initSchema(); err != nil {
		t.Fatalf("replay scoped secret schema twice: %v", err)
	}

	metadata, err := store.Get(ctx, secret.ID)
	if err != nil {
		t.Fatalf("get migrated secret: %v", err)
	}
	if metadata.Scope != ScopeGlobal || metadata.WorkspaceID != "" {
		t.Fatalf("migrated scope = (%q, %q), want (global, empty)", metadata.Scope, metadata.WorkspaceID)
	}
	value, err := store.Reveal(ctx, secret.ID)
	if err != nil {
		t.Fatalf("reveal migrated secret: %v", err)
	}
	if value != "replay-value" {
		t.Fatalf("migrated secret value = %q, want %q", value, "replay-value")
	}
}
