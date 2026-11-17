package secrets

import (
	"context"
	"errors"
	"testing"
)

// secretStoreOnly exercises the service's legacy fallback without advertising
// the optional scope-aware extension implemented by the underlying SQLite store.
type secretStoreOnly struct{ SecretStore }

func TestWorkspaceUpdateThroughLegacySecretStore(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scope  SecretScope
		update func(*Service, context.Context, string, string, *UpdateSecretRequest) (*SecretListItem, error)
	}{
		{"workspace CRUD", ScopeWorkspace, (*Service).UpdateWorkspaceSecret},
		{"workspace selection", ScopeWorkspace, (*Service).UpdateForWorkspace},
		{"global selection", ScopeGlobal, (*Service).UpdateForWorkspace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store := newTestSQLiteStore(t)
			svc := NewService(secretStoreOnly{store}, nil)
			allowed, calls := true, 0
			denied := errors.New("workspace denied")
			svc.SetWorkspaceAuthorizer(func(context.Context, string) error {
				calls++
				if !allowed {
					return denied
				}
				return nil
			})
			workspaceID := ""
			if tc.scope == ScopeWorkspace {
				workspaceID = "workspace-a"
			}
			created, err := svc.Create(ctx, &CreateSecretRequest{
				Name: "original", Value: "original-value", Scope: tc.scope, WorkspaceID: workspaceID,
			})
			if err != nil {
				t.Fatal(err)
			}
			calls = 0
			updated, updateErr := tc.update(svc, ctx, created.ID, "workspace-a", &UpdateSecretRequest{
				Name: stringPtr("renamed"), Value: stringPtr("rotated-value"),
			})
			stored, err := store.Get(ctx, created.ID)
			if err != nil || stored.Name != "renamed" {
				t.Fatalf("metadata was not persisted: %v", err)
			}
			value, err := store.Reveal(ctx, created.ID)
			if err != nil || value != "rotated-value" {
				t.Fatalf("rotation was not persisted: %v", err)
			}
			if updateErr != nil {
				t.Fatalf("write persisted but update returned an error: %v", updateErr)
			}
			if calls != 1 || updated == nil || updated.ID != created.ID || updated.Name != stored.Name ||
				updated.Scope != created.Scope || updated.WorkspaceID != created.WorkspaceID {
				t.Fatal("read-back lost metadata or repeated workspace authorization")
			}
			if tc.scope == ScopeWorkspace {
				if _, err := tc.update(svc, ctx, created.ID, "workspace-b", &UpdateSecretRequest{
					Name: stringPtr("wrong workspace"), Value: stringPtr("wrong-workspace-value"),
				}); !errors.Is(err, ErrNotFound) {
					t.Fatalf("cross-workspace update = %v, want ErrNotFound", err)
				}
			}
			allowed = false
			if _, err := tc.update(svc, ctx, created.ID, "workspace-a", &UpdateSecretRequest{
				Name: stringPtr("denied"), Value: stringPtr("denied-value"),
			}); !errors.Is(err, denied) {
				t.Fatalf("initial authorization denial = %v", err)
			}
			stored, err = store.Get(ctx, created.ID)
			if err != nil || stored.Name != "renamed" {
				t.Fatalf("denied update changed metadata: %v", err)
			}
			value, err = store.Reveal(ctx, created.ID)
			if err != nil || value != "rotated-value" {
				t.Fatalf("denied update changed value: %v", err)
			}
			control, err := svc.Update(ctx, created.ID, &UpdateSecretRequest{Name: stringPtr("global control")})
			if tc.scope == ScopeGlobal && (err != nil || control == nil || control.Name != "global control") {
				t.Fatalf("ordinary global update failed: %v", err)
			} else if tc.scope == ScopeWorkspace && !errors.Is(err, ErrNotFound) {
				t.Fatalf("ordinary global update accepted workspace secret: %v", err)
			}
		})
	}
}

func TestWorkspaceUpdateThroughUnsupportedVisibleStore(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t)
	svc := NewService(NewUserVisibleStore(secretStoreOnly{store}), nil)
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	created := createWorkspaceSecret(t, svc)
	_, wantErr := svc.GetForWorkspace(ctx, created.ID, "workspace-a")
	if wantErr == nil || wantErr.Error() != "workspace-scoped secret storage is unavailable" {
		t.Fatalf("unsupported wrapper read = %v", wantErr)
	}
	for _, update := range []func(context.Context, string, string, *UpdateSecretRequest) (*SecretListItem, error){
		svc.UpdateWorkspaceSecret, svc.UpdateForWorkspace,
	} {
		if _, err := update(ctx, created.ID, "workspace-a", &UpdateSecretRequest{
			Name: stringPtr("unexpected"), Value: stringPtr("unexpected-value"),
		}); err == nil || err.Error() != wantErr.Error() {
			t.Fatalf("unsupported wrapper update = %v, want unavailable error", err)
		}
		stored, err := store.Get(ctx, created.ID)
		if err != nil || stored.Name != created.Name {
			t.Fatalf("unsupported wrapper changed metadata: %v", err)
		}
		value, err := store.Reveal(ctx, created.ID)
		if err != nil || value != "original" {
			t.Fatalf("unsupported wrapper changed value: %v", err)
		}
	}
	capable := NewService(NewUserVisibleStore(store), nil)
	capable.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	if _, err := capable.UpdateWorkspaceSecret(ctx, created.ID, "workspace-a", &UpdateSecretRequest{
		Name: stringPtr("supported"), Value: stringPtr("supported-value"),
	}); err != nil {
		t.Fatalf("capable wrapper positive control: %v", err)
	}
	value, err := store.Reveal(ctx, created.ID)
	if err != nil || value != "supported-value" {
		t.Fatalf("capable wrapper did not persist rotation: %v", err)
	}
}
