package azuredevops

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresOAuthFlowAndCredentialState(t *testing.T) {
	ctx := context.Background()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, archived_at TIMESTAMP)`); err != nil {
		t.Fatalf("create task prerequisite: %v", err)
	}
	if _, err := NewStore(database, database); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("schema replay: %v", err)
	}
	now := time.Now().UTC()
	if err := store.InsertOAuthFlow(ctx, newTestOAuthFlow("state", "ws-1", now.Add(time.Minute)), now); err != nil {
		t.Fatal(err)
	}
	flow, err := store.ConsumeOAuthFlow(ctx, "state", now)
	if err != nil || flow == nil || flow.TenantID != "tenant-1" {
		t.Fatalf("consume = %+v, %v", flow, err)
	}
	if again, err := store.ConsumeOAuthFlow(ctx, "state", now); err != nil || again != nil {
		t.Fatalf("second consume = %+v, %v", again, err)
	}
	if err := store.UpsertConfig(ctx, &Config{
		WorkspaceID: "ws-1", OrganizationURL: "https://dev.azure.com/acme", AuthMethod: AuthMethodOAuth,
	}); err != nil {
		t.Fatal(err)
	}
	expiresAt := now.Add(time.Hour)
	if err := store.SetOAuthCredentialState(ctx, "ws-1", oauthCredentialState{
		Account: "Ana", TenantID: "tenant-1", TokenExpiresAt: &expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetOAuthReauthRequired(ctx, "ws-1", true); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.GetConfig(ctx, "ws-1")
	if err != nil || cfg.OAuthAccount != "Ana" || !cfg.ReauthRequired || cfg.TokenExpiresAt == nil {
		t.Fatalf("config = %+v, %v", cfg, err)
	}
	if err := store.ClearOAuthCredentialState(ctx, "ws-1"); err != nil {
		t.Fatal(err)
	}
}
