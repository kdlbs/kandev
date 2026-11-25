package secrets

import (
	"context"
	"errors"
	"testing"
)

func TestSSHIdentitySecretsAreNotUserVisible(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	id := "ssh-identity:one"
	if err := store.Create(ctx, &SecretWithValue{Secret: Secret{ID: id, Name: "identity"}, Value: "private-key"}); err != nil {
		t.Fatal(err)
	}
	visible := NewUserVisibleStore(store)
	if _, err := visible.Reveal(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reveal internal = %v", err)
	}
	if !IsInternalID(" SSH-IDENTITY:one ") {
		t.Fatal("SSH identity prefix must be reserved")
	}
}

func TestSSHIdentityVaultTransactionsAndOwnership(t *testing.T) {
	testSSHIdentityVault(t, newTestSQLiteStore(t))
}
func testSSHIdentityVault(t *testing.T, store *sqliteStore) {
	ctx := context.Background()
	vault := NewSSHIdentityVault(store.crypto)
	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := vault.Create(ctx, tx, "ssh-identity:rollback", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "ssh-identity:rollback"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rollback: %v", err)
	}
	tx, err = store.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := vault.Create(ctx, tx, "ssh-identity:keep", []byte("kept-key")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimUnowned(ctx, "first-admin"); err != nil {
		t.Fatal(err)
	}
	tx, err = store.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	key, err := vault.Reveal(ctx, tx, "ssh-identity:keep")
	if err != nil || string(key) != "kept-key" {
		t.Fatalf("reveal after claim: %v", err)
	}
	for _, id := range []string{"user-secret", "github:token", "ssh-identity:", " SSH-IDENTITY:keep "} {
		if err := vault.Create(ctx, tx, id, []byte("x")); err == nil {
			t.Fatalf("created invalid internal ID %q", id)
		}
		if _, err := vault.Reveal(ctx, tx, id); err == nil {
			t.Fatalf("revealed invalid ID %q", id)
		}
		if err := vault.Delete(ctx, tx, id); err == nil {
			t.Fatalf("deleted invalid ID %q", id)
		}
	}
	other, err := NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSSHIdentityVault(other).Reveal(ctx, tx, "ssh-identity:keep"); err == nil {
		t.Fatal("wrong master key accepted")
	}
	if err := vault.Delete(ctx, tx, "ssh-identity:keep"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "ssh-identity:keep"); err != nil {
		t.Fatalf("delete rollback: %v", err)
	}
}
