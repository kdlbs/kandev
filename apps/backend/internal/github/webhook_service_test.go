package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

type githubWebhookMemoryStore struct {
	workspaces       map[int64][]*WorkspaceConnection
	users            map[int64][]*UserConnection
	deliveries       map[string]*WebhookDelivery
	updates          int
	updateErr        error
	beforeTransition func()
}

func (s *githubWebhookMemoryStore) ClaimWebhookDelivery(
	_ context.Context, delivery *WebhookDelivery, staleBefore time.Time,
) (WebhookDeliveryClaim, error) {
	if s.deliveries == nil {
		s.deliveries = make(map[string]*WebhookDelivery)
	}
	existing, exists := s.deliveries[delivery.DeliveryID]
	if exists && existing.Status != WebhookDeliveryStatusFailed &&
		(existing.Status != WebhookDeliveryStatusReceived || existing.ReceivedAt.After(staleBefore)) {
		return WebhookDeliveryClaim{Status: existing.Status}, nil
	}
	copy := *delivery
	s.deliveries[delivery.DeliveryID] = &copy
	return WebhookDeliveryClaim{Acquired: true, Status: WebhookDeliveryStatusReceived}, nil
}

func (s *githubWebhookMemoryStore) CompleteWebhookDelivery(
	_ context.Context, deliveryID string, status WebhookDeliveryStatus, result string, processedAt time.Time,
) error {
	delivery := s.deliveries[deliveryID]
	delivery.Status = status
	delivery.Result = result
	delivery.ProcessedAt = &processedAt
	return nil
}

func (s *githubWebhookMemoryStore) ListWorkspaceConnectionsByInstallation(
	_ context.Context, installationID int64,
) ([]*WorkspaceConnection, error) {
	return s.workspaces[installationID], nil
}

func (s *githubWebhookMemoryStore) TransitionWorkspaceInstallationConnection(
	_ context.Context,
	expected, next *WorkspaceConnection,
) (bool, error) {
	if s.beforeTransition != nil {
		s.beforeTransition()
		s.beforeTransition = nil
	}
	if s.updateErr != nil {
		err := s.updateErr
		s.updateErr = nil
		return false, err
	}
	current := s.workspaces[*expected.InstallationID][0]
	if current.Source != expected.Source || current.InstallationID == nil ||
		*current.InstallationID != *expected.InstallationID ||
		current.CredentialGeneration != expected.CredentialGeneration || current.Status != expected.Status {
		return false, nil
	}
	*current = *next
	s.updates++
	return true, nil
}

func (s *githubWebhookMemoryStore) ListUserConnectionsByGitHubUser(
	_ context.Context, githubUserID int64,
) ([]*UserConnection, error) {
	return s.users[githubUserID], nil
}

type webhookPersonalRevoker struct {
	revoked []string
}

type webhookInstallationVerifier struct {
	installation AppInstallation
	err          error
}

func (v *webhookInstallationVerifier) GetInstallation(
	context.Context,
	int64,
) (AppInstallation, error) {
	return v.installation, v.err
}

type webhookPersonalReconciler struct {
	revoked bool
	calls   int
}

func (r *webhookPersonalReconciler) ReconcileAuthorizationRevocation(
	context.Context,
	*UserConnection,
) (bool, error) {
	r.calls++
	return r.revoked, nil
}

func (r *webhookPersonalRevoker) RevokePersonalConnection(_ context.Context, workspaceID, userID string) error {
	r.revoked = append(r.revoked, workspaceID+":"+userID)
	return nil
}

type webhookRepoUpdater struct {
	updates []InstallationRepositoriesChange
	err     error
}

func (u *webhookRepoUpdater) ApplyInstallationRepositories(
	_ context.Context, change InstallationRepositoriesChange,
) (bool, error) {
	if u.err != nil {
		err := u.err
		u.err = nil
		return false, err
	}
	u.updates = append(u.updates, change)
	return true, nil
}

func TestGitHubWebhookRejectsInvalidSignatureBeforeDedupe(t *testing.T) {
	store := &githubWebhookMemoryStore{}
	service := NewGitHubWebhookService("webhook-secret", store, nil, nil)
	_, err := service.Handle(context.Background(), GitHubWebhookRequest{
		DeliveryID: "delivery-1", Event: "installation", Signature: "sha256=invalid",
		Payload: []byte(`{"action":"suspend","installation":{"id":42}}`),
	})
	if !errors.Is(err, ErrInvalidWebhookSignature) || len(store.deliveries) != 0 {
		t.Fatalf("Handle() error = %v, deliveries = %v", err, store.deliveries)
	}
}

func TestGitHubWebhookDeduplicatesInstallationTransition(t *testing.T) {
	installationID := int64(42)
	connection := &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID, Status: ConnectionStatusActive, CredentialGeneration: 2,
	}
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{42: {connection}}}
	service := NewGitHubWebhookService("webhook-secret", store, nil, nil)
	payload := []byte(`{"action":"suspend","installation":{"id":42}}`)
	request := signedWebhookRequest("webhook-secret", "delivery-1", "installation", payload)

	first, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("first Handle() error = %v", err)
	}
	second, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate Handle() error = %v", err)
	}
	if first.Affected != 1 || !second.Duplicate || store.updates != 1 ||
		connection.Status != ConnectionStatusSuspended || connection.CredentialGeneration != 3 {
		t.Fatalf("first = %+v, second = %+v, connection = %+v, updates = %d", first, second, connection, store.updates)
	}
}

func TestGitHubWebhookUnsuspendsAndDeletesKnownInstallation(t *testing.T) {
	installationID := int64(42)
	connection := &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID, Status: ConnectionStatusSuspended, CredentialGeneration: 3,
	}
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{42: {connection}}}
	service := NewGitHubWebhookService("webhook-secret", store, nil, nil)

	unsuspend := []byte(`{"action":"unsuspend","installation":{"id":42}}`)
	if _, err := service.Handle(
		context.Background(), signedWebhookRequest("webhook-secret", "delivery-unsuspend", "installation", unsuspend),
	); err != nil {
		t.Fatalf("unsuspend Handle() error = %v", err)
	}
	if connection.Status != ConnectionStatusActive || connection.CredentialGeneration != 4 {
		t.Fatalf("unsuspended connection = %+v", connection)
	}

	deleted := []byte(`{"action":"deleted","installation":{"id":42}}`)
	if _, err := service.Handle(
		context.Background(), signedWebhookRequest("webhook-secret", "delivery-delete", "installation", deleted),
	); err != nil {
		t.Fatalf("delete Handle() error = %v", err)
	}
	if connection.Status != ConnectionStatusRevoked || connection.CredentialGeneration != 5 {
		t.Fatalf("deleted connection = %+v", connection)
	}
}

func TestGitHubWebhookLateUnsuspendCannotReactivateDeletedInstallation(t *testing.T) {
	installationID := int64(42)
	connection := &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID, Status: ConnectionStatusRevoked, CredentialGeneration: 5,
	}
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{42: {connection}}}
	service := NewGitHubWebhookService("webhook-secret", store, nil, nil)
	payload := []byte(`{"action":"unsuspend","installation":{"id":42}}`)
	result, err := service.Handle(
		context.Background(), signedWebhookRequest("webhook-secret", "delivery-late", "installation", payload),
	)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 0 || connection.Status != ConnectionStatusRevoked || connection.CredentialGeneration != 5 {
		t.Fatalf("Handle() = %+v, connection = %+v", result, connection)
	}
}

func TestGitHubWebhookReconcilesOutOfOrderInstallationEventsWithProvider(t *testing.T) {
	installationID := int64(42)
	connection := &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID, InstallationAccountLogin: "acme",
		InstallationAccountType: "Organization", Status: ConnectionStatusSuspended,
		CredentialGeneration: 5,
	}
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{42: {connection}}}
	verifier := &webhookInstallationVerifier{installation: AppInstallation{
		ID: 42, AccountLogin: "acme", AccountType: "Organization",
	}}
	service := NewGitHubWebhookService(
		"webhook-secret", store, nil, nil,
		GitHubWebhookReconciliation{Installations: verifier},
	)

	result, err := service.Handle(context.Background(), signedWebhookRequest(
		"webhook-secret", "delivery-delayed-suspend", "installation",
		[]byte(`{"action":"suspend","installation":{"id":42}}`),
	))
	if err != nil || result.Affected != 1 || connection.Status != ConnectionStatusActive {
		t.Fatalf("delayed suspend result=%+v err=%v connection=%+v", result, err, connection)
	}
	suspendedAt := time.Now().UTC()
	verifier.installation.SuspendedAt = &suspendedAt
	result, err = service.Handle(context.Background(), signedWebhookRequest(
		"webhook-secret", "delivery-delayed-unsuspend", "installation",
		[]byte(`{"action":"unsuspend","installation":{"id":42}}`),
	))
	if err != nil || result.Affected != 1 || connection.Status != ConnectionStatusSuspended {
		t.Fatalf("delayed unsuspend result=%+v err=%v connection=%+v", result, err, connection)
	}
}

func TestGitHubWebhookUnknownInstallationDoesNotCreateBinding(t *testing.T) {
	store := &githubWebhookMemoryStore{}
	service := NewGitHubWebhookService("webhook-secret", store, nil, nil)
	payload := []byte(`{"action":"created","installation":{"id":999}}`)
	result, err := service.Handle(
		context.Background(), signedWebhookRequest("webhook-secret", "delivery-2", "installation", payload),
	)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 0 || result.Status != WebhookDeliveryStatusIgnored || store.updates != 0 {
		t.Fatalf("Handle() = %+v, updates = %d", result, store.updates)
	}
}

func TestGitHubWebhookStaleInstallationRowCannotReplacePAT(t *testing.T) {
	installationID := int64(42)
	connection := &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID, Status: ConnectionStatusActive, CredentialGeneration: 2,
	}
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{42: {connection}}}
	store.beforeTransition = func() {
		connection.Source = ConnectionSourcePAT
		connection.InstallationID = nil
		connection.Login = "octocat"
		connection.CredentialGeneration = 3
	}
	service := NewGitHubWebhookService("webhook-secret", store, nil, nil)
	payload := []byte(`{"action":"suspend","installation":{"id":42}}`)

	result, err := service.Handle(
		context.Background(), signedWebhookRequest("webhook-secret", "delivery-stale-binding", "installation", payload),
	)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 0 || store.updates != 0 || connection.Source != ConnectionSourcePAT ||
		connection.InstallationID != nil || connection.Status != ConnectionStatusActive ||
		connection.CredentialGeneration != 3 {
		t.Fatalf("Handle() = %+v, connection = %+v, updates = %d", result, connection, store.updates)
	}
}

func TestGitHubWebhookAppliesRepositoryChangesOnlyToKnownBindings(t *testing.T) {
	installationID := int64(42)
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{42: {{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID, Status: ConnectionStatusActive, CredentialGeneration: 3,
	}}}}
	repos := &webhookRepoUpdater{}
	service := NewGitHubWebhookService("webhook-secret", store, repos, nil)
	payload := []byte(`{
		"action":"added","installation":{"id":42},
		"repositories_added":[{"id":7,"full_name":"acme/repo"}],"repositories_removed":[]
	}`)
	result, err := service.Handle(
		context.Background(), signedWebhookRequest("webhook-secret", "delivery-3", "installation_repositories", payload),
	)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 1 || len(repos.updates) != 1 || repos.updates[0].WorkspaceID != "workspace-1" ||
		repos.updates[0].CredentialGeneration != 3 || len(repos.updates[0].Added) != 1 ||
		repos.updates[0].Added[0].FullName != "acme/repo" {
		t.Fatalf("Handle() = %+v, repository updates = %+v", result, repos.updates)
	}
}

func TestInstallationRepositoryUpdateIgnoresReplacedAppBinding(t *testing.T) {
	service, _ := newWorkspaceConnectionService(t, "octocat")
	installationID := int64(42)
	appConnection := activeAppWorkspace("ws-1", installationID)
	appConnection.CredentialGeneration = 2
	if err := service.store.UpsertWorkspaceConnection(context.Background(), appConnection); err != nil {
		t.Fatal(err)
	}
	if err := service.store.UpsertWorkspaceSettings(context.Background(), &WorkspaceSettings{
		WorkspaceID: "ws-1", RepoScopeMode: RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{{Owner: "acme", Name: "repo"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourcePAT, Login: "octocat",
		GitHubHost: "github.com", Status: ConnectionStatusActive, CredentialGeneration: 3,
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := (&installationRepositorySettingsUpdater{service: service}).ApplyInstallationRepositories(
		context.Background(),
		InstallationRepositoriesChange{
			WorkspaceID: "ws-1", InstallationID: installationID,
			ConnectionSource: ConnectionSourceGitHubAppInstallation, CredentialGeneration: 2,
			Action: "removed", Removed: []InstallationRepository{{ID: 7, FullName: "acme/repo"}},
		},
	)
	if err != nil || updated {
		t.Fatalf("ApplyInstallationRepositories() updated=%v, err=%v", updated, err)
	}
	settings, err := service.store.GetWorkspaceSettings(context.Background(), "ws-1")
	if err != nil || len(settings.RepoScopeRepos) != 1 || settings.RepoScopeRepos[0].Name != "repo" {
		t.Fatalf("workspace settings after stale webhook = %+v, err=%v", settings, err)
	}
}

func TestGitHubWebhookAuthorizationRevocationRemovesExactUsers(t *testing.T) {
	store := &githubWebhookMemoryStore{users: map[int64][]*UserConnection{11: {
		{WorkspaceID: "workspace-1", UserID: "user-1", GitHubUserID: 11},
		{WorkspaceID: "workspace-2", UserID: "user-9", GitHubUserID: 11},
	}}}
	revoker := &webhookPersonalRevoker{}
	service := NewGitHubWebhookService("webhook-secret", store, nil, revoker)
	payload := []byte(`{"action":"revoked","sender":{"id":11,"login":"octocat"}}`)
	result, err := service.Handle(
		context.Background(), signedWebhookRequest("webhook-secret", "delivery-4", "github_app_authorization", payload),
	)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 2 || len(revoker.revoked) != 2 || revoker.revoked[0] != "workspace-1:user-1" {
		t.Fatalf("Handle() = %+v, revoked = %v", result, revoker.revoked)
	}
}

func TestGitHubWebhookLateAuthorizationRevokePreservesVerifiedReconnect(t *testing.T) {
	store := &githubWebhookMemoryStore{users: map[int64][]*UserConnection{11: {{
		WorkspaceID: "workspace-1", UserID: "user-1", GitHubUserID: 11,
		CredentialGeneration: 2,
	}}}}
	reconciler := &webhookPersonalReconciler{}
	service := NewGitHubWebhookService(
		"webhook-secret", store, nil, nil,
		GitHubWebhookReconciliation{Personal: reconciler},
	)
	payload := []byte(`{"action":"revoked","sender":{"id":11}}`)
	result, err := service.Handle(context.Background(), signedWebhookRequest(
		"webhook-secret", "delivery-late-revoke", "github_app_authorization", payload,
	))
	if err != nil || result.Affected != 0 || reconciler.calls != 1 {
		t.Fatalf("Handle()=%+v err=%v reconciler calls=%d", result, err, reconciler.calls)
	}
}

func TestGitHubWebhookRetriesFailedDelivery(t *testing.T) {
	installationID := int64(42)
	connection := &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID, Status: ConnectionStatusActive, CredentialGeneration: 2,
	}
	store := &githubWebhookMemoryStore{
		workspaces: map[int64][]*WorkspaceConnection{42: {connection}},
	}
	repos := &webhookRepoUpdater{err: errors.New("temporary database failure")}
	service := NewGitHubWebhookService("webhook-secret", store, repos, nil)
	payload := []byte(`{
		"action":"added","installation":{"id":42},
		"repositories_added":[{"id":7,"full_name":"acme/repo"}],"repositories_removed":[]
	}`)
	request := signedWebhookRequest("webhook-secret", "delivery-retry", "installation_repositories", payload)

	if _, err := service.Handle(context.Background(), request); err == nil {
		t.Fatal("first Handle() unexpectedly succeeded")
	}
	if got := store.deliveries[request.DeliveryID].Status; got != WebhookDeliveryStatusFailed {
		t.Fatalf("failed delivery status = %q", got)
	}
	result, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("retry Handle() error = %v", err)
	}
	if result.Duplicate || result.Affected != 1 || len(repos.updates) != 1 ||
		store.deliveries[request.DeliveryID].Status != WebhookDeliveryStatusProcessed {
		t.Fatalf("retry result=%+v updates=%d delivery=%+v", result, len(repos.updates), store.deliveries[request.DeliveryID])
	}
}

func TestGitHubWebhookReclaimsStaleReceivedDelivery(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	store := &githubWebhookMemoryStore{deliveries: map[string]*WebhookDelivery{
		"delivery-stale": {
			DeliveryID: "delivery-stale", Event: "installation", Status: WebhookDeliveryStatusReceived,
			ReceivedAt: now.Add(-webhookDeliveryStaleAfter - time.Second),
		},
	}}
	service := NewGitHubWebhookService("webhook-secret", store, nil, nil)
	service.now = func() time.Time { return now }
	payload := []byte(`{"action":"created","installation":{"id":999}}`)
	result, err := service.Handle(
		context.Background(), signedWebhookRequest("webhook-secret", "delivery-stale", "installation", payload),
	)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Duplicate || result.Status != WebhookDeliveryStatusIgnored {
		t.Fatalf("Handle() = %+v", result)
	}
}

func signedWebhookRequest(secret, deliveryID, event string, payload []byte) GitHubWebhookRequest {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return GitHubWebhookRequest{
		DeliveryID: deliveryID,
		Event:      event,
		Signature:  "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		Payload:    payload,
	}
}
