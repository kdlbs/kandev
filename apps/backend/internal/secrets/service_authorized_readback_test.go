package secrets

import (
	"context"
	"errors"
	"testing"
)

func TestWorkspaceUpdateAcknowledgesAuthorizedPartialWrite(t *testing.T) {
	updates := map[string]func(*Service, context.Context, string, string, *UpdateSecretRequest) (*SecretListItem, error){
		"workspace CRUD":      (*Service).UpdateWorkspaceSecret,
		"workspace selection": (*Service).UpdateForWorkspace,
	}
	for method, update := range updates {
		for _, field := range []string{"name", "value"} {
			t.Run(method+"/"+field, func(t *testing.T) {
				svc := newProductionService(t)
				ctx := context.Background()
				created := createWorkspaceSecret(t, svc)
				calls := 0
				denied := errors.New("workspace authorization unavailable")
				svc.SetWorkspaceAuthorizer(func(context.Context, string) error {
					calls++
					if calls > 1 {
						return denied
					}
					return nil
				})
				req := &UpdateSecretRequest{Name: stringPtr("renamed token")}
				wantName, wantValue := "renamed token", "original"
				if field == "value" {
					req = &UpdateSecretRequest{Value: stringPtr("rotated")}
					wantName, wantValue = created.Name, "rotated"
				}
				updated, err := update(svc, ctx, created.ID, "workspace-a", req)
				if err != nil {
					t.Fatalf("authorized update was not acknowledged: %v", err)
				}
				if calls != 1 {
					t.Fatalf("authorization calls = %d, want 1", calls)
				}
				if updated.Name != wantName || updated.ID != created.ID ||
					updated.Scope != created.Scope || updated.WorkspaceID != created.WorkspaceID ||
					!updated.CreatedAt.Equal(created.CreatedAt) || !updated.HasValue {
					t.Fatal("update did not preserve identity and unchanged metadata")
				}
				scoped := svc.store.(ScopedSecretStore)
				stored, err := scoped.GetForWorkspace(ctx, created.ID, "workspace-a")
				if err != nil || stored.Name != wantName {
					t.Fatalf("persisted metadata mismatch: %v", err)
				}
				value, err := scoped.RevealForWorkspace(ctx, created.ID, "workspace-a")
				if err != nil || value != wantValue {
					t.Fatalf("persisted value mismatch: %v", err)
				}
				svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return denied })
				if _, err := update(svc, ctx, created.ID, "workspace-a", &UpdateSecretRequest{
					Name: stringPtr("denied rename"), Value: stringPtr("denied rotation"),
				}); !errors.Is(err, denied) {
					t.Fatalf("denied update = %v, want authorization error", err)
				}
				stored, err = scoped.GetForWorkspace(ctx, created.ID, "workspace-a")
				if err != nil || stored.Name != wantName {
					t.Fatalf("denied update changed metadata: %v", err)
				}
				value, err = scoped.RevealForWorkspace(ctx, created.ID, "workspace-a")
				if err != nil || value != wantValue {
					t.Fatalf("denied update changed value: %v", err)
				}
			})
		}
	}
}
