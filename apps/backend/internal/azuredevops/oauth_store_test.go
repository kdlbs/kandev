package azuredevops

import (
	"context"
	"testing"
	"time"
)

func newTestOAuthFlow(stateHash, workspaceID string, expiresAt time.Time) *oauthFlow {
	return &oauthFlow{
		StateHash: stateHash, WorkspaceID: workspaceID, UserID: "user-1",
		OrganizationURL: "https://dev.azure.com/acme", TenantID: "tenant-1",
		PKCEVerifier: "verifier", RedirectURL: "http://localhost/cb", ExpiresAt: expiresAt,
	}
}

func TestOAuthFlowConsumeIsSingleUseAndExpires(t *testing.T) {
	ctx := context.Background()
	_, store, _ := newTestService(t, nil)
	now := time.Now().UTC()
	if err := store.InsertOAuthFlow(ctx, newTestOAuthFlow("live", "ws-a", now.Add(time.Minute)), now); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertOAuthFlow(ctx, newTestOAuthFlow("expired", "ws-a", now.Add(-time.Second)), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	flow, err := store.ConsumeOAuthFlow(ctx, "live", now)
	if err != nil || flow == nil || flow.WorkspaceID != "ws-a" || flow.PKCEVerifier != "verifier" {
		t.Fatalf("first consume = %+v, %v", flow, err)
	}
	if again, err := store.ConsumeOAuthFlow(ctx, "live", now); err != nil || again != nil {
		t.Fatalf("second consume = %+v, %v; want nil", again, err)
	}
	if expired, err := store.ConsumeOAuthFlow(ctx, "expired", now); err != nil || expired != nil {
		t.Fatalf("expired consume = %+v, %v; want nil", expired, err)
	}
	if unknown, err := store.ConsumeOAuthFlow(ctx, "unknown", now); err != nil || unknown != nil {
		t.Fatalf("unknown consume = %+v, %v; want nil", unknown, err)
	}
}

func TestOAuthFlowInsertPurgesSpentFlowsAndWorkspaceDeleteRemovesRest(t *testing.T) {
	ctx := context.Background()
	_, store, _ := newTestService(t, nil)
	now := time.Now().UTC()
	for _, flow := range []*oauthFlow{
		newTestOAuthFlow("old", "ws-a", now.Add(-time.Second)),
		newTestOAuthFlow("other", "ws-b", now.Add(time.Minute)),
	} {
		if err := store.InsertOAuthFlow(ctx, flow, now.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.InsertOAuthFlow(ctx, newTestOAuthFlow("new", "ws-a", now.Add(time.Minute)), now); err != nil {
		t.Fatal(err)
	}
	if count := countOAuthFlows(t, store); count != 2 {
		t.Fatalf("flows after purge = %d, want 2 (expired flow removed)", count)
	}
	if err := store.DeleteOAuthFlowsByWorkspace(ctx, "ws-a"); err != nil {
		t.Fatal(err)
	}
	if count := countOAuthFlows(t, store); count != 1 {
		t.Fatalf("flows after workspace delete = %d, want only ws-b", count)
	}
}

func TestOAuthCredentialStateRoundTripsWithoutTouchingConfigFields(t *testing.T) {
	ctx := context.Background()
	_, store, _ := newTestService(t, nil)
	if err := store.UpsertConfig(ctx, &Config{
		WorkspaceID: "ws-a", OrganizationURL: "https://dev.azure.com/acme",
		DefaultProjectName: "Platform", AuthMethod: AuthMethodOAuth,
	}); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if err := store.SetOAuthCredentialState(ctx, "ws-a", oauthCredentialState{
		Account: "Ana Silva", TenantID: "tenant-1", TokenExpiresAt: &expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetOAuthReauthRequired(ctx, "ws-a", true); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.GetConfig(ctx, "ws-a")
	if err != nil || cfg == nil {
		t.Fatalf("config = %+v, %v", cfg, err)
	}
	if cfg.OAuthAccount != "Ana Silva" || cfg.OAuthTenantID != "tenant-1" || !cfg.ReauthRequired ||
		cfg.TokenExpiresAt == nil || !cfg.TokenExpiresAt.Equal(expiresAt) || cfg.DefaultProjectName != "Platform" {
		t.Fatalf("config = %+v", cfg)
	}
	renewed := expiresAt.Add(time.Hour)
	if err := store.UpdateOAuthTokenExpiry(ctx, "ws-a", renewed); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearOAuthCredentialState(ctx, "ws-a"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = store.GetConfig(ctx, "ws-a")
	if cfg.OAuthAccount != "" || cfg.OAuthTenantID != "" || cfg.TokenExpiresAt != nil || cfg.ReauthRequired {
		t.Fatalf("cleared config = %+v", cfg)
	}
}

func TestOAuthSchemaReplaysOnSameDatabase(t *testing.T) {
	database := newTestDB(t)
	if _, err := NewStore(database, database); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("schema replay: %v", err)
	}
	if err := store.InsertOAuthFlow(context.Background(), newTestOAuthFlow("s", "ws", time.Now().Add(time.Minute)), time.Now()); err != nil {
		t.Fatalf("insert after replay: %v", err)
	}
}

func countOAuthFlows(t *testing.T, store *Store) int {
	t.Helper()
	var count int
	if err := store.ro.Get(&count, `SELECT COUNT(*) FROM azure_devops_oauth_flows`); err != nil {
		t.Fatal(err)
	}
	return count
}
