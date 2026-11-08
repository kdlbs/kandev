package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
)

func ownerGrantCtx(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, Role: authn.RoleMember})
}

func seedOwnerGrantSecrets(t *testing.T, store *sqliteStore) {
	t.Helper()
	seeds := []struct {
		owner string
		value SecretWithValue
	}{
		{"owner-a", SecretWithValue{Secret: Secret{ID: "owned-global", Name: "a", Scope: ScopeGlobal}, Value: "synthetic-owned-value"}},
		{"owner-a", SecretWithValue{Secret: Secret{ID: "owned-workspace", Name: "w", Scope: ScopeWorkspace, WorkspaceID: "ws-1"}, Value: "synthetic-ws-value"}},
		{"owner-a", SecretWithValue{Secret: Secret{ID: "github:user:ws:owner-a:access", Name: "gh"}, Value: "synthetic-internal-value"}},
		{"", SecretWithValue{Secret: Secret{ID: "unowned-global", Name: "u", Scope: ScopeGlobal}, Value: "synthetic-unowned-value"}},
	}
	for _, seed := range seeds {
		ctx := context.Background()
		if seed.owner != "" {
			ctx = ownerGrantCtx(seed.owner)
		}
		value := seed.value
		if err := store.Create(ctx, &value); err != nil {
			t.Fatalf("create %s: %v", seed.value.ID, err)
		}
	}
}

func TestOwnerGrantedSecretStoreMatchesOnlyOwnedGlobalSecrets(t *testing.T) {
	store := newTestSQLiteStore(t)
	seedOwnerGrantSecrets(t, store)
	wrapped := NewUserVisibleStore(store).(OwnerGrantedSecretStore)
	// The caller identity is irrelevant: a foreign caller resolves through the
	// owner binding, and the binding alone decides the match.
	ctx := ownerGrantCtx("collaborator-b")

	for name, granted := range map[string]OwnerGrantedSecretStore{"sqlite": store, "user-visible": wrapped} {
		t.Run(name, func(t *testing.T) {
			got, err := granted.RevealGlobalOwnedBy(ctx, "owned-global", "owner-a")
			if err != nil || got != "synthetic-owned-value" {
				t.Fatalf("reveal owned global = %q, %v", got, err)
			}
			if meta, err := granted.GetGlobalOwnedBy(ctx, "owned-global", "owner-a"); err != nil || meta.ID != "owned-global" {
				t.Fatalf("get owned global = %+v, %v", meta, err)
			}
		})
	}
}

func TestOwnerGrantedSecretStoreDeniesEveryOtherBinding(t *testing.T) {
	store := newTestSQLiteStore(t)
	seedOwnerGrantSecrets(t, store)
	wrapped := NewUserVisibleStore(store).(OwnerGrantedSecretStore)
	ctx := ownerGrantCtx("owner-a")
	cases := []struct{ name, id, owner string }{
		{"wrong owner", "owned-global", "collaborator-b"},
		{"blank owner", "owned-global", ""},
		{"whitespace owner", "owned-global", "  "},
		{"workspace scope", "owned-workspace", "owner-a"},
		{"internal id", "github:user:ws:owner-a:access", "owner-a"},
		{"unowned secret", "unowned-global", "owner-a"},
		{"missing secret", "missing", "owner-a"},
	}
	for _, tc := range cases {
		for name, granted := range map[string]OwnerGrantedSecretStore{"sqlite": store, "user-visible": wrapped} {
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				value, err := granted.RevealGlobalOwnedBy(ctx, tc.id, tc.owner)
				if !errors.Is(err, ErrNotFound) || value != "" {
					t.Fatalf("reveal = %q, %v; want not found", value, err)
				}
				if strings.Contains(err.Error(), "synthetic-") {
					t.Fatalf("error leaks a value: %v", err)
				}
				if _, err := granted.GetGlobalOwnedBy(ctx, tc.id, tc.owner); !errors.Is(err, ErrNotFound) {
					t.Fatalf("get = %v; want not found", err)
				}
			})
		}
	}
}
