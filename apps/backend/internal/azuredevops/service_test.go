package azuredevops

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

type fakeSecretStore struct {
	mu             sync.Mutex
	values         map[string]string
	setFailures    int
	deleteFailures int
}

func newFakeSecretStore() *fakeSecretStore {
	return &fakeSecretStore{values: make(map[string]string)}
}

func (f *fakeSecretStore) Reveal(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.values[id]
	if !ok {
		return "", errors.New("secret not found")
	}
	return value, nil
}

func (f *fakeSecretStore) Set(_ context.Context, id, _ string, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setFailures > 0 {
		f.setFailures--
		return errors.New("injected secret set failure")
	}
	f.values[id] = value
	return nil
}

func (f *fakeSecretStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteFailures > 0 {
		f.deleteFailures--
		return errors.New("injected secret delete failure")
	}
	delete(f.values, id)
	return nil
}

func (f *fakeSecretStore) Exists(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.values[id]
	return ok, nil
}

type fakeClient struct {
	invalidClient
	result *TestConnectionResult
	err    error
}

func (f *fakeClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	return f.result, f.err
}

type capturedCredentials struct {
	config Config
	pat    string
}

func newTestService(t *testing.T, factory ClientFactory) (*Service, *Store, *fakeSecretStore) {
	t.Helper()
	db := newTestDB(t)
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	secrets := newFakeSecretStore()
	return NewService(store, secrets, factory, logger.Default()), store, secrets
}

func TestOrganizationURLValidation(t *testing.T) {
	valid := []string{
		"https://dev.azure.com/acme",
		"https://dev.azure.com/team-42",
	}
	for _, raw := range valid {
		if got, err := ValidateOrganizationURL(raw); err != nil || got != raw {
			t.Errorf("ValidateOrganizationURL(%q) = %q, %v", raw, got, err)
		}
	}
	invalid := []string{
		"", "http://dev.azure.com/acme", "https://example.com/acme",
		"https://dev.azure.com", "https://dev.azure.com/acme/project",
		"https://user@dev.azure.com/acme", "https://dev.azure.com:443/acme",
		"https://dev.azure.com/acme?x=1", "https://dev.azure.com/acme#fragment",
		"https://dev.azure.com/-acme", "https://dev.azure.com/acme_works",
	}
	for _, raw := range invalid {
		if _, err := ValidateOrganizationURL(raw); err == nil {
			t.Errorf("ValidateOrganizationURL(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestConfigWorkspaceIsolationAndReconstruction(t *testing.T) {
	svc, store, secrets := newTestService(t, nil)
	ctx := context.Background()
	for _, tc := range []struct{ workspace, org, pat string }{
		{"ws-a", "https://dev.azure.com/acme", "pat-a"},
		{"ws-b", "https://dev.azure.com/other", "pat-b"},
	} {
		if _, err := svc.SetConfigForWorkspace(ctx, tc.workspace, &SetConfigRequest{
			OrganizationURL: tc.org, PAT: tc.pat,
		}); err != nil {
			t.Fatalf("set %s: %v", tc.workspace, err)
		}
	}
	reconstructed := NewService(store, secrets, nil, logger.Default())
	for _, tc := range []struct{ workspace, org, pat string }{
		{"ws-a", "https://dev.azure.com/acme", "pat-a"},
		{"ws-b", "https://dev.azure.com/other", "pat-b"},
	} {
		cfg, err := reconstructed.GetConfigForWorkspace(ctx, tc.workspace)
		if err != nil || cfg == nil || cfg.OrganizationURL != tc.org || !cfg.HasSecret {
			t.Fatalf("get %s: cfg=%+v err=%v", tc.workspace, cfg, err)
		}
		gotPAT, err := secrets.Reveal(ctx, SecretKeyForWorkspace(tc.workspace))
		if err != nil || gotPAT != tc.pat {
			t.Fatalf("secret %s: value=%q err=%v", tc.workspace, gotPAT, err)
		}
	}
}

func TestConfigTestUsesSubmittedAndStoredCredentialsWithoutPersistence(t *testing.T) {
	var captures []capturedCredentials
	factory := func(cfg *Config, pat string) Client {
		captures = append(captures, capturedCredentials{config: *cfg, pat: pat})
		return &fakeClient{result: &TestConnectionResult{OK: true, DisplayName: "Alice"}}
	}
	svc, _, _ := newTestService(t, factory)
	ctx := context.Background()
	res, err := svc.TestConnectionForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "submitted",
	})
	if err != nil || !res.OK {
		t.Fatalf("submitted probe: result=%+v err=%v", res, err)
	}
	if cfg, _ := svc.GetConfigForWorkspace(ctx, "ws-a"); cfg != nil {
		t.Fatalf("test connection persisted config: %+v", cfg)
	}
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "stored",
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	res, err = svc.TestConnectionForWorkspace(ctx, "ws-a", &SetConfigRequest{})
	if err != nil || !res.OK {
		t.Fatalf("stored probe: result=%+v err=%v", res, err)
	}
	if len(captures) != 2 || captures[0].pat != "submitted" || captures[1].pat != "stored" {
		t.Fatalf("captured credentials: %+v", captures)
	}
}

func TestCopyConfigCopiesCredentialAndDeleteIsScoped(t *testing.T) {
	svc, store, secrets := newTestService(t, nil)
	ctx := context.Background()
	if _, err := svc.SetConfigForWorkspace(ctx, "source", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", DefaultProjectID: "p1",
		DefaultProjectName: "Platform", PAT: "source-pat",
	}); err != nil {
		t.Fatalf("set source: %v", err)
	}
	if _, err := svc.SetConfigForWorkspace(ctx, "target", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/old", PAT: "old-pat",
	}); err != nil {
		t.Fatalf("set target: %v", err)
	}
	if err := store.UpdateAuthHealth(ctx, "target", true, "", time.Now().UTC()); err != nil {
		t.Fatalf("set target health: %v", err)
	}
	if _, err := svc.CopyConfigToWorkspace(ctx, "source", "target"); err != nil {
		t.Fatalf("copy: %v", err)
	}
	target, err := svc.GetConfigForWorkspace(ctx, "target")
	if err != nil || target == nil || target.DefaultProjectID != "p1" || !target.HasSecret {
		t.Fatalf("target config: %+v err=%v", target, err)
	}
	if target.LastCheckedAt != nil || target.LastOK || target.LastError != "" {
		t.Fatalf("copied config inherited health state: %+v", target)
	}
	if pat, _ := secrets.Reveal(ctx, SecretKeyForWorkspace("target")); pat != "source-pat" {
		t.Fatalf("target PAT = %q", pat)
	}
	if err := svc.DeleteConfigForWorkspace(ctx, "target"); err != nil {
		t.Fatalf("delete target: %v", err)
	}
	if source, _ := svc.GetConfigForWorkspace(ctx, "source"); source == nil {
		t.Fatal("deleting target removed source config")
	}
	if exists, _ := secrets.Exists(ctx, SecretKeyForWorkspace("target")); exists {
		t.Fatal("target secret still exists after delete")
	}
}

func TestConfigRecordAuthHealthPersistsPerWorkspace(t *testing.T) {
	clients := map[string]*fakeClient{
		"acme":  {result: &TestConnectionResult{OK: true}},
		"other": {result: &TestConnectionResult{OK: false, Error: "401 unauthorized"}},
	}
	factory := func(cfg *Config, _ string) Client {
		org := cfg.OrganizationURL[len("https://dev.azure.com/"):]
		return clients[org]
	}
	svc, _, _ := newTestService(t, factory)
	ctx := context.Background()
	for _, tc := range []struct{ workspace, org string }{
		{"ws-a", "acme"}, {"ws-b", "other"},
	} {
		if _, err := svc.SetConfigForWorkspace(ctx, tc.workspace, &SetConfigRequest{
			OrganizationURL: "https://dev.azure.com/" + tc.org, PAT: "pat",
		}); err != nil {
			t.Fatalf("set %s: %v", tc.workspace, err)
		}
	}
	svc.RecordAuthHealth(ctx)
	a, _ := svc.GetConfigForWorkspace(ctx, "ws-a")
	b, _ := svc.GetConfigForWorkspace(ctx, "ws-b")
	if !a.LastOK || a.LastCheckedAt == nil || b.LastOK || b.LastError != "401 unauthorized" {
		t.Fatalf("health rows: a=%+v b=%+v", a, b)
	}
}

func TestSetConfigCompensatesSecretFailure(t *testing.T) {
	svc, store, secrets := newTestService(t, nil)
	ctx := context.Background()
	secrets.setFailures = 1
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "new-pat",
	}); err == nil {
		t.Fatal("set config unexpectedly succeeded")
	}
	config, err := store.GetConfig(ctx, "ws-a")
	if err != nil || config != nil {
		t.Fatalf("config after secret failure = %+v, err = %v", config, err)
	}
	if exists, _ := secrets.Exists(ctx, SecretKeyForWorkspace("ws-a")); exists {
		t.Fatal("secret remains after failed config creation")
	}
}

func TestSetConfigKeepsPreviousConfigWhenSecretReplacementFails(t *testing.T) {
	svc, store, secrets := newTestService(t, nil)
	ctx := context.Background()
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/old", PAT: "old-pat",
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	secrets.setFailures = 1
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/new", PAT: "new-pat",
	}); err == nil {
		t.Fatal("replace config unexpectedly succeeded")
	}
	config, err := store.GetConfig(ctx, "ws-a")
	if err != nil || config.OrganizationURL != "https://dev.azure.com/old" {
		t.Fatalf("config after secret replacement failure = %+v, err = %v", config, err)
	}
	if pat, _ := secrets.Reveal(ctx, SecretKeyForWorkspace("ws-a")); pat != "old-pat" {
		t.Fatalf("PAT after replacement failure = %q", pat)
	}
}

func TestSetConfigRestoresPATWhenStoreFails(t *testing.T) {
	svc, store, secrets := newTestService(t, nil)
	ctx := context.Background()
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "old-pat",
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "new-pat",
	}); err == nil {
		t.Fatal("set config unexpectedly succeeded")
	}
	if pat, _ := secrets.Reveal(ctx, SecretKeyForWorkspace("ws-a")); pat != "old-pat" {
		t.Fatalf("PAT after store failure = %q", pat)
	}
}

func TestSetConfigResetsHealthOnlyWhenCredentialsChange(t *testing.T) {
	svc, store, _ := newTestService(t, nil)
	ctx := context.Background()
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "old-pat",
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	setHealthy := func() {
		t.Helper()
		if err := store.UpdateAuthHealth(ctx, "ws-a", true, "", time.Now().UTC()); err != nil {
			t.Fatalf("set auth health: %v", err)
		}
	}
	assertHealth := func(wantChecked, wantOK bool) {
		t.Helper()
		config, err := svc.GetConfigForWorkspace(ctx, "ws-a")
		if err != nil {
			t.Fatalf("get config: %v", err)
		}
		if (config.LastCheckedAt != nil) != wantChecked || config.LastOK != wantOK {
			t.Fatalf("health = %+v, want checked=%t ok=%t", config, wantChecked, wantOK)
		}
	}

	setHealthy()
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", DefaultProjectID: "project-a",
	}); err != nil {
		t.Fatalf("update project: %v", err)
	}
	assertHealth(true, true)

	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/other",
	}); err != nil {
		t.Fatalf("update organization: %v", err)
	}
	assertHealth(false, false)

	setHealthy()
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/other", PAT: "new-pat",
	}); err != nil {
		t.Fatalf("update PAT: %v", err)
	}
	assertHealth(false, false)
}

func TestDeleteConfigLeavesStateWhenSecretDeleteFails(t *testing.T) {
	svc, store, secrets := newTestService(t, nil)
	ctx := context.Background()
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	secrets.deleteFailures = 1
	if err := svc.DeleteConfigForWorkspace(ctx, "ws-a"); err == nil {
		t.Fatal("delete config unexpectedly succeeded")
	}
	config, err := store.GetConfig(ctx, "ws-a")
	if err != nil || config == nil {
		t.Fatalf("config after secret delete failure = %+v, err = %v", config, err)
	}
	if pat, _ := secrets.Reveal(ctx, SecretKeyForWorkspace("ws-a")); pat != "pat" {
		t.Fatalf("PAT after failed deletion = %q", pat)
	}
}

func TestDeleteConfigRestoresPATWhenStoreFails(t *testing.T) {
	svc, store, secrets := newTestService(t, nil)
	ctx := context.Background()
	if _, err := svc.SetConfigForWorkspace(ctx, "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if err := svc.DeleteConfigForWorkspace(ctx, "ws-a"); err == nil {
		t.Fatal("delete config unexpectedly succeeded")
	}
	if pat, _ := secrets.Reveal(ctx, SecretKeyForWorkspace("ws-a")); pat != "pat" {
		t.Fatalf("PAT after store deletion failure = %q", pat)
	}
}

func TestRecordAuthHealthIgnoresParentCancellation(t *testing.T) {
	svc, _, _ := newTestService(t, func(*Config, string) Client {
		return &fakeClient{err: context.Canceled}
	})
	if _, err := svc.SetConfigForWorkspace(t.Context(), "ws-a", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	svc.RecordAuthHealthForWorkspace(ctx, "ws-a")
	config, err := svc.GetConfigForWorkspace(t.Context(), "ws-a")
	if err != nil || config.LastCheckedAt != nil {
		t.Fatalf("health after cancellation = %+v, err = %v", config, err)
	}
}
