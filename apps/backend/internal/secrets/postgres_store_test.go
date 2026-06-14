package secrets

import (
	"context"
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
}
