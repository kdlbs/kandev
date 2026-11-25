package sshconnections

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
	"golang.org/x/crypto/ssh"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityCreationIsAuthorizedAndIdempotent(t *testing.T) {
	s := testStore(t)
	key, err := secrets.NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = secrets.Provide(s.db, s.db, key)
	if err != nil {
		t.Fatal(err)
	}
	vault := secrets.NewSSHIdentityVault(key)
	service := NewService(true, s, vault)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
	req := CreateIdentityRequest{RequestID: uuid.NewString(), Label: "deployment", Kind: "generate"}
	first, err := service.CreateIdentity(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	public, _, _, _, err := ssh.ParseAuthorizedKey([]byte(first.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	if public.Type() != ssh.KeyAlgoED25519 || first.Fingerprint != ssh.FingerprintSHA256(public) {
		t.Fatal("generated key metadata mismatch")
	}
	second, err := service.CreateIdentity(ctx, req)
	if err != nil || second.ID != first.ID {
		t.Fatalf("retry changed identity: %v", err)
	}
	req.Label = "changed"
	if _, err = service.CreateIdentity(ctx, req); !errors.Is(err, ErrConflict) {
		t.Fatalf("idempotency conflict = %v", err)
	}
	var data string
	if err = s.db.Get(&data, `SELECT data FROM ssh_identities`); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(data, "PRIVATE KEY") {
		t.Fatal("private key persisted in metadata")
	}
}
func TestIdentityManagementGatesBeforeDependencies(t *testing.T) {
	for _, tt := range []struct {
		enabled  bool
		identity *authn.Identity
		want     error
	}{
		{false, nil, ErrDisabled}, {true, nil, ErrForbidden}, {true, &authn.Identity{UserID: "member", Role: authn.RoleMember}, ErrForbidden},
	} {
		ctx := context.Background()
		if tt.identity != nil {
			ctx = authn.WithIdentity(ctx, *tt.identity)
		}
		_, err := NewService(tt.enabled, nil, nil).CreateIdentity(ctx, CreateIdentityRequest{})
		if !errors.Is(err, tt.want) {
			t.Fatalf("gate = %v", err)
		}
	}
}

func TestImportProtectedIdentity(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "test", []byte("correct"))
	if err != nil {
		t.Fatal(err)
	}
	input := string(pem.EncodeToMemory(block))
	for _, pass := range []string{"wrong", "correct"} {
		out, key, err := makeIdentity(CreateIdentityRequest{Label: "imported", Kind: "import", PrivateKey: input, Passphrase: pass})
		if pass == "wrong" {
			if !errors.Is(err, ErrInvalidIdentity) {
				t.Fatalf("wrong passphrase: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		if ssh.FingerprintSHA256(signer.PublicKey()) != out.Fingerprint {
			t.Fatal("import changed identity")
		}
	}
	if _, _, err := makeIdentity(CreateIdentityRequest{Kind: "import", PrivateKey: strings.Repeat("x", 65537)}); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatal("unbounded input accepted")
	}
}

func TestExternalIdentityReferencesDoNotCopyKeys(t *testing.T) {
	for _, kind := range []string{"file", "agent"} {
		reference := filepath.Join(t.TempDir(), "credential")
		out, key, err := makeIdentity(CreateIdentityRequest{Label: "existing", Kind: kind, Reference: reference})
		if err != nil {
			t.Fatal(err)
		}
		if out.Kind != kind || out.Reference != reference || len(key) != 0 {
			t.Fatal("external identity was copied")
		}
		if _, _, err = makeIdentity(CreateIdentityRequest{Kind: kind, Reference: "relative"}); !errors.Is(err, ErrInvalidIdentity) {
			t.Fatal("relative identity reference accepted")
		}
	}
}

func identityService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	store := testStore(t)
	key, err := secrets.NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = secrets.Provide(store.db, store.db, key); err != nil {
		t.Fatal(err)
	}
	return NewService(true, store, secrets.NewSSHIdentityVault(key)), authn.WithIdentity(context.Background(), authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
}
func TestIdentityDeletionAndAuditAreTransactional(t *testing.T) {
	s, ctx := identityService(t)
	identity, err := s.CreateIdentity(ctx, CreateIdentityRequest{RequestID: uuid.NewString(), Label: "key", Kind: "generate"})
	if err != nil {
		t.Fatal(err)
	}
	identities, err := s.ListIdentities(ctx)
	if err != nil || len(identities) != 1 {
		t.Fatalf("list: %v", err)
	}
	c, err := s.store.SaveConnection(ctx, Connection{ID: "connection", IdentityID: identity.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteIdentity(ctx, identity.ID, identity.Version); !errors.Is(err, ErrInUse) {
		t.Fatalf("deleted referenced key: %v", err)
	}
	if err = s.store.DeleteConnection(ctx, c.ID, c.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.db.Exec(`DROP TABLE ssh_connection_audit`); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteIdentity(ctx, identity.ID, identity.Version); err == nil {
		t.Fatal("delete succeeded without audit")
	}
	tx, err := s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = s.vault.Reveal(ctx, tx, "ssh-identity:"+identity.ID); err != nil {
		t.Fatal("audit failure deleted key")
	}
	if _, err = identityIn(ctx, tx, identity.ID); err != nil {
		t.Fatal("audit failure deleted metadata")
	}
}

func TestImportRejectsMultipleKeysAndExtraneousMaterial(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}
	key := string(pem.EncodeToMemory(block))
	for _, input := range []string{key + key, key + "unexpected", "unexpected\n" + key} {
		if _, _, err := makeIdentity(CreateIdentityRequest{Kind: "import", PrivateKey: input}); !errors.Is(err, ErrInvalidIdentity) {
			t.Errorf("extraneous material accepted: %v", err)
		}
	}
}
