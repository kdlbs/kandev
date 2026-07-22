package gitlab

import (
	"context"
	"errors"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

func newWorkspaceConfigService(t *testing.T, store *Store, secrets WorkspaceSecretStore) *Service {
	t.Helper()
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	svc.SetWorkspaceSecretStore(secrets)
	return svc
}

func TestResolveGitLabExecutionCredentialsByAuthMethod(t *testing.T) {
	store := newTestStore(t)
	secrets := &configTestSecrets{values: map[string]string{"gitlab_workspace_pat": "pat-token"}}
	seedWorkspace(t, store, "pat")
	seedWorkspace(t, store, "environment")
	seedWorkspace(t, store, "glab")
	for _, cfg := range []*GitLabConfig{
		{WorkspaceID: "pat", Host: "https://gitlab.pat.example", AuthMethod: AuthMethodPAT},
		{WorkspaceID: "environment", Host: DefaultHost, AuthMethod: AuthMethodEnvironment},
		{WorkspaceID: "glab", Host: "https://gitlab.glab.example", AuthMethod: AuthMethodGLab},
	} {
		if err := store.SaveConfigForWorkspace(context.Background(), cfg.WorkspaceID, cfg); err != nil {
			t.Fatalf("save %s config: %v", cfg.WorkspaceID, err)
		}
	}
	secrets.values[SecretKeyForWorkspace("pat")] = "pat-token"
	t.Setenv(secretNameToken, "environment-token")
	svc := newWorkspaceConfigService(t, store, secrets)
	var resolvedHost string
	svc.glabTokenFn = func(_ context.Context, host string) (string, error) {
		resolvedHost = host
		return "glab-token", nil
	}

	tests := []struct{ workspace, host, token string }{
		{"pat", "https://gitlab.pat.example", "pat-token"},
		{"environment", DefaultHost, "environment-token"},
		{"glab", "https://gitlab.glab.example", "glab-token"},
	}
	for _, test := range tests {
		host, token, err := svc.ResolveGitLabExecutionCredentials(context.Background(), test.workspace)
		if err != nil {
			t.Fatalf("resolve %s: %v", test.workspace, err)
		}
		if host != test.host || token != test.token {
			t.Fatalf("resolve %s = (%q, %q)", test.workspace, host, token)
		}
	}
	if resolvedHost != "gitlab.glab.example" {
		t.Fatalf("glab host = %q", resolvedHost)
	}
	if _, exists := secrets.values[SecretKeyForWorkspace("glab")]; exists {
		t.Fatal("glab execution credential was persisted")
	}
}

func TestServiceConfigInvalidCredentialPreservesWorkingConnection(t *testing.T) {
	server := httptest.NewServer(userHandler(func(token string) bool { return token == "valid" }))
	t.Cleanup(server.Close)
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-a")
	secrets := &configTestSecrets{values: make(map[string]string)}
	svc := newWorkspaceConfigService(t, store, secrets)
	ctx := context.Background()

	if _, err := svc.SetConfigForWorkspace(ctx, "workspace-a", &SetConfigRequest{
		Host: server.URL, AuthMethod: AuthMethodPAT, Token: "valid",
	}); err != nil {
		t.Fatalf("save valid config: %v", err)
	}
	if _, err := svc.SetConfigForWorkspace(ctx, "workspace-a", &SetConfigRequest{
		Host: server.URL, AuthMethod: AuthMethodPAT, Token: "invalid",
	}); err == nil {
		t.Fatal("invalid credential unexpectedly saved")
	}
	cfg, err := svc.GetConfigForWorkspace(ctx, "workspace-a")
	if err != nil {
		t.Fatalf("get preserved config: %v", err)
	}
	if cfg == nil || cfg.Host != server.URL {
		t.Fatalf("preserved config = %#v", cfg)
	}
	if got := secrets.values[SecretKeyForWorkspace("workspace-a")]; got != "valid" {
		t.Fatalf("preserved secret = %q, want valid", got)
	}
}

func TestServiceCopyConfigCopiesPATWithoutWatchRows(t *testing.T) {
	server := httptest.NewServer(userHandler(func(token string) bool { return token == "valid" }))
	t.Cleanup(server.Close)
	store := newTestStore(t)
	seedWorkspace(t, store, "source")
	seedWorkspace(t, store, "target")
	secrets := &configTestSecrets{values: make(map[string]string)}
	svc := newWorkspaceConfigService(t, store, secrets)
	ctx := context.Background()
	if _, err := svc.SetConfigForWorkspace(ctx, "source", &SetConfigRequest{
		Host: server.URL, AuthMethod: AuthMethodPAT, Token: "valid",
	}); err != nil {
		t.Fatalf("save source: %v", err)
	}

	copied, err := svc.CopyConfigToWorkspace(ctx, "source", "target")
	if err != nil {
		t.Fatalf("copy config: %v", err)
	}
	if copied.Host != server.URL || !copied.HasSecret {
		t.Fatalf("copied config = %#v", copied)
	}
	if got := secrets.values[SecretKeyForWorkspace("target")]; got != "valid" {
		t.Fatalf("copied secret = %q", got)
	}
	var watches int
	if err := store.ro.Get(&watches, `SELECT COUNT(*) FROM gitlab_review_watches WHERE workspace_id = 'target'`); err != nil {
		t.Fatalf("count target watches: %v", err)
	}
	if watches != 0 {
		t.Fatalf("target watches = %d, want 0", watches)
	}
}

func TestServiceConfigSecretSetFailureRestoresWorkingConnection(t *testing.T) {
	store, secrets, svc, before := setupWorkingWorkspaceConfig(t, "workspace-a")
	secrets.setFailures = 1
	secrets.mutateBeforeSetError = true

	_, err := svc.SetConfigForWorkspace(context.Background(), "workspace-a", &SetConfigRequest{
		Host: "https://replacement.gitlab.example", AuthMethod: AuthMethodPAT, Token: "replacement-token",
	})
	if err == nil {
		t.Fatal("expected secret set failure")
	}
	assertWorkingWorkspaceConfig(t, store, secrets, "workspace-a", before, "working-token")
}

func TestServiceConfigUpsertFailureRestoresWorkingConnection(t *testing.T) {
	store, secrets, svc, before := setupWorkingWorkspaceConfig(t, "workspace-a")
	installConfigFailureTrigger(t, store, "fail_gitlab_config_upsert", `
		BEFORE UPDATE OF host ON gitlab_configs
		BEGIN SELECT RAISE(FAIL, 'injected config upsert failure'); END`)

	_, err := svc.SetConfigForWorkspace(context.Background(), "workspace-a", &SetConfigRequest{
		Host: "https://replacement.gitlab.example", AuthMethod: AuthMethodPAT, Token: "replacement-token",
	})
	if err == nil {
		t.Fatal("expected config upsert failure")
	}
	assertWorkingWorkspaceConfig(t, store, secrets, "workspace-a", before, "working-token")
}

func TestServiceConfigHealthFailureRestoresWorkingConnection(t *testing.T) {
	store, secrets, svc, before := setupWorkingWorkspaceConfig(t, "workspace-a")
	installConfigFailureTrigger(t, store, "fail_gitlab_config_health", `
		BEFORE UPDATE OF last_ok ON gitlab_configs
		BEGIN SELECT RAISE(FAIL, 'injected config health failure'); END`)

	_, err := svc.SetConfigForWorkspace(context.Background(), "workspace-a", &SetConfigRequest{
		Host: "https://replacement.gitlab.example", AuthMethod: AuthMethodPAT, Token: "replacement-token",
	})
	if err == nil {
		t.Fatal("expected config health failure")
	}
	assertWorkingWorkspaceConfig(t, store, secrets, "workspace-a", before, "working-token")
}

func TestServiceCopyFailureRestoresTargetConnection(t *testing.T) {
	store, secrets, svc, before := setupWorkingWorkspaceConfig(t, "target")
	seedWorkspace(t, store, "source")
	if err := store.UpsertConfigForWorkspace(context.Background(), "source", &GitLabConfig{
		Host: "https://source.gitlab.example", AuthMethod: AuthMethodPAT, Username: "source-user",
	}); err != nil {
		t.Fatalf("seed source config: %v", err)
	}
	secrets.values[SecretKeyForWorkspace("source")] = "source-token"
	installConfigFailureTrigger(t, store, "fail_gitlab_copy_health", `
		BEFORE UPDATE OF last_ok ON gitlab_configs
		BEGIN SELECT RAISE(FAIL, 'injected copy health failure'); END`)

	if _, err := svc.CopyConfigToWorkspace(context.Background(), "source", "target"); err == nil {
		t.Fatal("expected copy failure")
	}
	assertWorkingWorkspaceConfig(t, store, secrets, "target", before, "working-token")
}

func TestServiceDeleteConfigFailureRestoresCredential(t *testing.T) {
	store, secrets, svc, before := setupWorkingWorkspaceConfig(t, "workspace-a")
	installConfigFailureTrigger(t, store, "fail_gitlab_config_delete", `
		BEFORE DELETE ON gitlab_configs
		BEGIN SELECT RAISE(FAIL, 'injected config delete failure'); END`)

	if err := svc.DeleteConfigForWorkspace(context.Background(), "workspace-a"); err == nil {
		t.Fatal("expected config delete failure")
	}
	assertWorkingWorkspaceConfig(t, store, secrets, "workspace-a", before, "working-token")
}

func TestServiceDeleteSecretFailureReportsErrorAndRestoresConnection(t *testing.T) {
	store, secrets, svc, before := setupWorkingWorkspaceConfig(t, "workspace-a")
	secrets.deleteFailures = 1
	secrets.mutateBeforeDeleteError = true

	if err := svc.DeleteConfigForWorkspace(context.Background(), "workspace-a"); err == nil {
		t.Fatal("expected secret delete failure")
	}
	assertWorkingWorkspaceConfig(t, store, secrets, "workspace-a", before, "working-token")
}

func TestServiceCopyConfigSnapshotsSourceMetadataAndPATAtomically(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "source")
	seedWorkspace(t, store, "target")
	if err := store.UpsertConfigForWorkspace(context.Background(), "source", &GitLabConfig{
		Host: "https://old.gitlab.example", AuthMethod: AuthMethodPAT, Username: "old-user",
	}); err != nil {
		t.Fatalf("seed source config: %v", err)
	}
	secrets := newBarrierConfigSecrets(SecretKeyForWorkspace("source"), "old-token")
	svc := newWorkspaceConfigService(t, store, secrets)
	svc.workspaceClientFn = func(_ context.Context, cfg *GitLabConfig, _ string) (Client, error) {
		client := NewMockClient(cfg.Host)
		client.SetUser("new-user")
		return client, nil
	}

	updateDone := make(chan error, 1)
	go func() {
		_, err := svc.SetConfigForWorkspace(context.Background(), "source", &SetConfigRequest{
			Host: "https://new.gitlab.example", AuthMethod: AuthMethodPAT, Token: "new-token",
		})
		updateDone <- err
	}()
	<-secrets.setMutated

	type copyResult struct {
		cfg *GitLabConfig
		err error
	}
	copyDone := make(chan copyResult, 1)
	go func() {
		cfg, err := svc.CopyConfigToWorkspace(context.Background(), "source", "target")
		copyDone <- copyResult{cfg: cfg, err: err}
	}()

	select {
	case <-secrets.sourceRevealed:
		// The old implementation can read the newly-mutated token while the
		// source metadata is still old. Let it reach target persistence.
	case <-time.After(250 * time.Millisecond):
		// The fixed implementation is waiting at the config mutation boundary.
	}
	close(secrets.releaseSet)
	if err := <-updateDone; err != nil {
		t.Fatalf("update source config: %v", err)
	}
	copied := <-copyDone
	if copied.err != nil {
		t.Fatalf("copy config: %v", copied.err)
	}
	if copied.cfg.Host != "https://new.gitlab.example" {
		t.Fatalf("copied host = %q, want new source host", copied.cfg.Host)
	}
	if got := secrets.value(SecretKeyForWorkspace("target")); got != "new-token" {
		t.Fatalf("copied token = %q, want new-token", got)
	}
}

func TestExpectedHostActionWaitsForConfigMutationAndFailsClosed(t *testing.T) {
	const workspaceID = "workspace-a"
	store := newTestStore(t)
	seedWorkspace(t, store, workspaceID)
	if err := store.UpsertConfigForWorkspace(t.Context(), workspaceID, &GitLabConfig{
		Host: "https://old.gitlab.example", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	secrets := newBarrierConfigSecrets(SecretKeyForWorkspace(workspaceID), "old-token")
	svc := newWorkspaceConfigService(t, store, secrets)
	svc.workspaceClientFn = func(_ context.Context, cfg *GitLabConfig, _ string) (Client, error) {
		return NewMockClient(cfg.Host), nil
	}

	mutationDone := make(chan error, 1)
	go func() {
		mutationDone <- svc.persistWorkspaceConfig(context.Background(), workspaceID, &GitLabConfig{
			Host: "https://new.gitlab.example", AuthMethod: AuthMethodPAT,
		}, "new-token")
	}()
	<-secrets.setMutated

	actionCalls := 0
	actionStarted := make(chan struct{})
	actionDone := make(chan error, 1)
	go func() {
		close(actionStarted)
		actionDone <- svc.RunWithWorkspaceClient(
			context.Background(), workspaceID, "https://old.gitlab.example",
			func(Client) error {
				actionCalls++
				return nil
			},
		)
	}()
	<-actionStarted
	close(secrets.releaseSet)
	if err := <-mutationDone; err != nil {
		t.Fatalf("persist replacement: %v", err)
	}
	if err := <-actionDone; !errors.Is(err, ErrWorkspaceHostMismatch) {
		t.Fatalf("action error = %v, want host mismatch", err)
	}
	if actionCalls != 0 {
		t.Fatalf("wrong-host upstream calls = %d, want 0", actionCalls)
	}
}

func TestConfigMutationInvalidatesClientBuiltFromPreviousRevision(t *testing.T) {
	const workspaceID = "workspace-a"
	store := newTestStore(t)
	seedWorkspace(t, store, workspaceID)
	if err := store.UpsertConfigForWorkspace(t.Context(), workspaceID, &GitLabConfig{
		Host: "https://old.gitlab.example", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	secrets := &configTestSecrets{values: map[string]string{
		SecretKeyForWorkspace(workspaceID): "old-token",
	}}
	svc := newWorkspaceConfigService(t, store, secrets)
	buildStarted := make(chan struct{})
	releaseBuild := make(chan struct{})
	svc.workspaceClientFn = func(_ context.Context, cfg *GitLabConfig, _ string) (Client, error) {
		if cfg.Host == "https://old.gitlab.example" {
			close(buildStarted)
			<-releaseBuild
		}
		return NewMockClient(cfg.Host), nil
	}

	oldClientDone := make(chan error, 1)
	go func() {
		_, err := svc.ClientForWorkspaceHost(context.Background(), workspaceID, "https://old.gitlab.example")
		oldClientDone <- err
	}()
	<-buildStarted
	mutationDone := make(chan error, 1)
	go func() {
		mutationDone <- svc.persistWorkspaceConfig(context.Background(), workspaceID, &GitLabConfig{
			Host: "https://new.gitlab.example", AuthMethod: AuthMethodPAT,
		}, "new-token")
	}()
	if err := <-mutationDone; err != nil {
		t.Fatalf("persist new revision: %v", err)
	}
	close(releaseBuild)
	if err := <-oldClientDone; !errors.Is(err, ErrWorkspaceHostMismatch) {
		t.Fatalf("stale resolver error = %v, want host mismatch", err)
	}
	client, err := svc.ClientForWorkspaceHost(t.Context(), workspaceID, "https://new.gitlab.example")
	if err != nil {
		t.Fatalf("resolve new revision: %v", err)
	}
	if client.Host() != "https://new.gitlab.example" {
		t.Fatalf("cached client host = %q, want new revision", client.Host())
	}
}

func TestBlockedWorkspaceActionDoesNotBlockOtherWorkspaceOrConfigMutation(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-a")
	seedWorkspace(t, store, "workspace-b")
	for workspaceID, host := range map[string]string{
		"workspace-a": "https://a.gitlab.example",
		"workspace-b": "https://b.gitlab.example",
	} {
		if err := store.UpsertConfigForWorkspace(t.Context(), workspaceID, &GitLabConfig{
			Host: host, AuthMethod: AuthMethodPAT,
		}); err != nil {
			t.Fatalf("seed %s config: %v", workspaceID, err)
		}
	}
	secrets := newBarrierConfigSecrets("unused", "")
	secrets.values[SecretKeyForWorkspace("workspace-a")] = "token-a"
	secrets.values[SecretKeyForWorkspace("workspace-b")] = "token-b"
	svc := newWorkspaceConfigService(t, store, secrets)
	svc.workspaceClientFn = func(_ context.Context, cfg *GitLabConfig, _ string) (Client, error) {
		return NewMockClient(cfg.Host), nil
	}

	actionAStarted := make(chan struct{})
	releaseActionA := make(chan struct{})
	actionADone := make(chan error, 1)
	go func() {
		actionADone <- svc.RunWithWorkspaceClient(
			context.Background(), "workspace-a", "https://a.gitlab.example",
			func(client Client) error {
				if client.Host() != "https://a.gitlab.example" {
					return errors.New("workspace A resolved wrong host")
				}
				close(actionAStarted)
				<-releaseActionA
				return nil
			},
		)
	}()
	<-actionAStarted

	actionBDone := make(chan error, 1)
	go func() {
		actionBDone <- svc.RunWithWorkspaceClient(
			context.Background(), "workspace-b", "https://b.gitlab.example",
			func(Client) error { return nil },
		)
	}()
	if err := receiveBarrierResult(t, "workspace B action", actionBDone); err != nil {
		t.Fatalf("workspace B action: %v", err)
	}

	mutationDone := make(chan error, 1)
	go func() {
		mutationDone <- svc.persistWorkspaceConfig(context.Background(), "workspace-a", &GitLabConfig{
			Host: "https://a-new.gitlab.example", AuthMethod: AuthMethodPAT,
		}, "token-a-new")
	}()
	if err := receiveBarrierResult(t, "workspace A config mutation", mutationDone); err != nil {
		t.Fatalf("workspace A config mutation: %v", err)
	}
	close(releaseActionA)
	if err := receiveBarrierResult(t, "workspace A action", actionADone); err != nil {
		t.Fatalf("workspace A action: %v", err)
	}
}

func receiveBarrierResult(t *testing.T, name string, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not cross the barrier", name)
		return nil
	}
}

func TestServiceHealthProbeDoesNotOverwriteNewerConfigRevision(t *testing.T) {
	store, secrets, svc, _ := setupWorkingWorkspaceConfig(t, "workspace-a")
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	svc.workspaceClientFn = func(_ context.Context, cfg *GitLabConfig, _ string) (Client, error) {
		client := NewMockClient(cfg.Host)
		if cfg.Host == "https://working.gitlab.example" {
			return &blockingAuthenticatedUserClient{
				Client: client, started: probeStarted, release: releaseProbe, username: "stale-user",
			}, nil
		}
		client.SetUser("current-user")
		return client, nil
	}

	healthDone := make(chan struct{})
	go func() {
		svc.recordWorkspaceAuthHealth(context.Background(), store, "workspace-a")
		close(healthDone)
	}()
	<-probeStarted

	if _, err := svc.SetConfigForWorkspace(context.Background(), "workspace-a", &SetConfigRequest{
		Host: "https://current.gitlab.example", AuthMethod: AuthMethodPAT, Token: "current-token",
	}); err != nil {
		t.Fatalf("save newer config while probe is blocked: %v", err)
	}
	close(releaseProbe)
	<-healthDone

	cfg, err := store.GetConfigForWorkspace(context.Background(), "workspace-a")
	if err != nil {
		t.Fatalf("get config after stale probe: %v", err)
	}
	if cfg.Host != "https://current.gitlab.example" || cfg.Username != "current-user" {
		t.Fatalf("newer config overwritten by stale health result: %#v", cfg)
	}
	if got := secrets.values[SecretKeyForWorkspace("workspace-a")]; got != "current-token" {
		t.Fatalf("newer token = %q, want current-token", got)
	}
}

type barrierConfigSecrets struct {
	mu             sync.Mutex
	values         map[string]string
	blockedKey     string
	setMutated     chan struct{}
	releaseSet     chan struct{}
	sourceRevealed chan struct{}
	setOnce        sync.Once
	revealOnce     sync.Once
}

func newBarrierConfigSecrets(key, value string) *barrierConfigSecrets {
	return &barrierConfigSecrets{
		values:         map[string]string{key: value},
		blockedKey:     key,
		setMutated:     make(chan struct{}),
		releaseSet:     make(chan struct{}),
		sourceRevealed: make(chan struct{}),
	}
}

func (s *barrierConfigSecrets) Reveal(_ context.Context, id string) (string, error) {
	s.mu.Lock()
	value := s.values[id]
	s.mu.Unlock()
	if id == s.blockedKey && value == "new-token" {
		s.revealOnce.Do(func() { close(s.sourceRevealed) })
	}
	return value, nil
}

func (s *barrierConfigSecrets) Set(_ context.Context, id, _ string, value string) error {
	s.mu.Lock()
	s.values[id] = value
	s.mu.Unlock()
	if id == s.blockedKey && value == "new-token" {
		s.setOnce.Do(func() { close(s.setMutated) })
		<-s.releaseSet
	}
	return nil
}

func (s *barrierConfigSecrets) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, id)
	return nil
}

func (s *barrierConfigSecrets) Exists(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.values[id]
	return ok, nil
}

func (s *barrierConfigSecrets) value(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[id]
}

type blockingAuthenticatedUserClient struct {
	Client
	started  chan struct{}
	release  chan struct{}
	username string
}

func (c *blockingAuthenticatedUserClient) GetAuthenticatedUser(context.Context) (string, error) {
	close(c.started)
	<-c.release
	return c.username, nil
}

func setupWorkingWorkspaceConfig(t *testing.T, workspaceID string) (*Store, *configTestSecrets, *Service, *GitLabConfig) {
	t.Helper()
	store := newTestStore(t)
	seedWorkspace(t, store, workspaceID)
	secrets := &configTestSecrets{values: map[string]string{
		SecretKeyForWorkspace(workspaceID): "working-token",
	}}
	checkedAt := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	cfg := &GitLabConfig{
		Host:       "https://working.gitlab.example",
		AuthMethod: AuthMethodPAT,
		Username:   "working-user",
		CreatedAt:  checkedAt.Add(-time.Hour),
	}
	if err := store.UpsertConfigForWorkspace(context.Background(), workspaceID, cfg); err != nil {
		t.Fatalf("seed working config: %v", err)
	}
	updated, err := store.UpdateConfigHealthForRevision(
		context.Background(), workspaceID, cfg.Username, true, "", checkedAt, cfg.Revision,
	)
	if err != nil {
		t.Fatalf("seed working health: %v", err)
	}
	if !updated {
		t.Fatal("seed working health did not match config revision")
	}
	before, err := store.GetConfigForWorkspace(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("read working config: %v", err)
	}
	svc := newWorkspaceConfigService(t, store, secrets)
	svc.workspaceClientFn = func(_ context.Context, cfg *GitLabConfig, _ string) (Client, error) {
		return NewMockClient(cfg.Host), nil
	}
	return store, secrets, svc, before
}

func installConfigFailureTrigger(t *testing.T, store *Store, name, body string) {
	t.Helper()
	if _, err := store.db.Exec(`CREATE TRIGGER ` + name + ` ` + body); err != nil {
		t.Fatalf("install failure trigger %s: %v", name, err)
	}
}

func assertWorkingWorkspaceConfig(t *testing.T, store *Store, secrets *configTestSecrets, workspaceID string, want *GitLabConfig, wantToken string) {
	t.Helper()
	got, err := store.GetConfigForWorkspace(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("get preserved config: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preserved config = %#v, want %#v", got, want)
	}
	if gotToken := secrets.values[SecretKeyForWorkspace(workspaceID)]; gotToken != wantToken {
		t.Fatalf("preserved secret = %q, want %q", gotToken, wantToken)
	}
}
