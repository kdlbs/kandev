package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
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
	key := delivery.AppRegistrationID + ":" + delivery.DeliveryID
	existing, exists := s.deliveries[key]
	if exists && existing.Status != WebhookDeliveryStatusFailed &&
		(existing.Status != WebhookDeliveryStatusReceived || existing.ReceivedAt.After(staleBefore)) {
		return WebhookDeliveryClaim{Status: existing.Status}, nil
	}
	copy := *delivery
	s.deliveries[key] = &copy
	return WebhookDeliveryClaim{Acquired: true, Status: WebhookDeliveryStatusReceived}, nil
}

func (s *githubWebhookMemoryStore) CompleteWebhookDelivery(
	_ context.Context, deliveryID string, status WebhookDeliveryStatus, result string, processedAt time.Time,
) error {
	var delivery *WebhookDelivery
	for _, candidate := range s.deliveries {
		if candidate.DeliveryID == deliveryID {
			delivery = candidate
			break
		}
	}
	if delivery == nil {
		return errors.New("delivery not found")
	}
	delivery.Status = status
	delivery.Result = result
	delivery.ProcessedAt = &processedAt
	return nil
}

func (s *githubWebhookMemoryStore) CompleteAppRegistrationWebhookDelivery(
	_ context.Context,
	registrationID, deliveryID string,
	status WebhookDeliveryStatus,
	result string,
	processedAt time.Time,
) error {
	delivery := s.deliveries[registrationID+":"+deliveryID]
	if delivery == nil {
		return errors.New("delivery not found")
	}
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

func (s *githubWebhookMemoryStore) ListWorkspaceConnectionsByAppInstallation(
	_ context.Context, registrationID string, installationID int64,
) ([]*WorkspaceConnection, error) {
	connections := s.workspaces[installationID]
	matched := make([]*WorkspaceConnection, 0, len(connections))
	for _, connection := range connections {
		if connection != nil && connection.AppRegistrationID == registrationID {
			matched = append(matched, connection)
		}
	}
	return matched, nil
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
	for _, current := range s.workspaces[*expected.InstallationID] {
		if current.WorkspaceID != expected.WorkspaceID {
			continue
		}
		if current.Source != expected.Source || current.InstallationID == nil ||
			*current.InstallationID != *expected.InstallationID ||
			current.AppRegistrationID != expected.AppRegistrationID ||
			current.CredentialGeneration != expected.CredentialGeneration || current.Status != expected.Status {
			return false, nil
		}
		*current = *next
		s.updates++
		return true, nil
	}
	return false, nil
}

func (s *githubWebhookMemoryStore) ListUserConnectionsByGitHubUser(
	_ context.Context, githubUserID int64,
) ([]*UserConnection, error) {
	return s.users[githubUserID], nil
}

func (s *githubWebhookMemoryStore) ListUserConnectionsByAppGitHubUser(
	_ context.Context, registrationID string, githubUserID int64,
) ([]*UserConnection, error) {
	connections := s.users[githubUserID]
	matched := make([]*UserConnection, 0, len(connections))
	for _, connection := range connections {
		if connection != nil && connection.AppRegistrationID == registrationID {
			matched = append(matched, connection)
		}
	}
	return matched, nil
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

func newRegistrationWebhookService(
	registrationID, secret string,
	store githubWebhookStore,
	repositories installationRepositoryUpdater,
	personal personalConnectionRevoker,
	reconciliation ...GitHubWebhookReconciliation,
) *GitHubWebhookService {
	return NewAppRegistrationWebhookService(
		registrationID, secret, store, repositories, personal, reconciliation...,
	)
}

func TestGitHubWebhookDeliveryIdentityIncludesRegistration(t *testing.T) {
	store := &githubWebhookMemoryStore{}
	payload := []byte(`{}`)
	request := signedWebhookRequest("shared-secret", "delivery-1", "ping", payload)

	for _, registrationID := range []string{"registration-a", "registration-b"} {
		service := newRegistrationWebhookService(registrationID, "shared-secret", store, nil, nil)
		result, err := service.Handle(context.Background(), request)
		if err != nil || result.Duplicate {
			t.Fatalf("registration %s Handle() = %+v, err %v", registrationID, result, err)
		}
	}
	if len(store.deliveries) != 2 {
		t.Fatalf("deliveries = %#v, want one per registration", store.deliveries)
	}
}

func TestGitHubWebhookOnlyMutatesMatchingRegistrationInstallation(t *testing.T) {
	installationID := int64(42)
	work := &WorkspaceConnection{
		WorkspaceID: "work", AppRegistrationID: "registration-work",
		Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID,
		Status: ConnectionStatusActive, CredentialGeneration: 1,
	}
	personal := &WorkspaceConnection{
		WorkspaceID: "personal", AppRegistrationID: "registration-personal",
		Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID,
		Status: ConnectionStatusActive, CredentialGeneration: 1,
	}
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{
		installationID: {work, personal},
	}}
	service := newRegistrationWebhookService("registration-work", "work-secret", store, nil, nil)
	request := signedWebhookRequest(
		"work-secret", "delivery-1", "installation",
		[]byte(`{"action":"suspend","installation":{"id":42}}`),
	)

	result, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 1 || work.Status != ConnectionStatusSuspended ||
		personal.Status != ConnectionStatusActive {
		t.Fatalf("result=%+v work=%+v personal=%+v", result, work, personal)
	}
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
	service := newRegistrationWebhookService("registration-b", "registration-b-secret", store, nil, nil)
	wrongRouteRequest := signedWebhookRequest(
		"registration-a-secret", "delivery-1", "installation", []byte(`{"action":`),
	)
	_, err := service.Handle(context.Background(), wrongRouteRequest)
	if !errors.Is(err, ErrInvalidWebhookSignature) || len(store.deliveries) != 0 {
		t.Fatalf("Handle() error = %v, deliveries = %v", err, store.deliveries)
	}
}

func TestGitHubWebhookAuthorizationRevocationOnlyMatchesRegistration(t *testing.T) {
	store := &githubWebhookMemoryStore{users: map[int64][]*UserConnection{11: {
		{WorkspaceID: "work", UserID: "user-1", AppRegistrationID: "registration-work", GitHubUserID: 11},
		{WorkspaceID: "personal", UserID: "user-1", AppRegistrationID: "registration-personal", GitHubUserID: 11},
	}}}
	revoker := &webhookPersonalRevoker{}
	service := newRegistrationWebhookService("registration-work", "work-secret", store, nil, revoker)
	request := signedWebhookRequest(
		"work-secret", "delivery-authorization", "github_app_authorization",
		[]byte(`{"action":"revoked","sender":{"id":11}}`),
	)

	result, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 1 || len(revoker.revoked) != 1 || revoker.revoked[0] != "work:user-1" {
		t.Fatalf("result=%+v revoked=%v", result, revoker.revoked)
	}
}

func TestAppRegistrationWebhookHealthUpdateIsRegistrationLocal(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, registration := range []*AppRegistration{
		newAppRegistration("registration-work", 101, "Work", now),
		newAppRegistration("registration-personal", 202, "Personal", now),
	} {
		if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
			t.Fatal(err)
		}
	}

	updated, err := store.updateAppRegistrationWebhookHealth(
		ctx, "registration-work", 1, DeploymentAppWebhookVerified, now, "",
	)
	if err != nil || !updated {
		t.Fatalf("update work health = %v, err %v", updated, err)
	}
	work, err := store.GetAppRegistration(ctx, "registration-work")
	if err != nil || work.WebhookStatus != DeploymentAppWebhookVerified || work.LastWebhookAt == nil {
		t.Fatalf("work registration = %+v, err %v", work, err)
	}
	personal, err := store.GetAppRegistration(ctx, "registration-personal")
	if err != nil || personal.WebhookStatus != DeploymentAppWebhookUnverified || personal.LastWebhookAt != nil {
		t.Fatalf("personal registration = %+v, err %v", personal, err)
	}

	updated, err = store.updateAppRegistrationWebhookHealth(
		ctx, "registration-work", 0, DeploymentAppWebhookFailing, now, "stale",
	)
	if err != nil || updated {
		t.Fatalf("stale generation update = %v, err %v", updated, err)
	}
}

func TestGitHubWebhookConcurrentDeliveryIsProcessedOncePerRegistration(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedConnectionWorkspaces(t, store, "workspace-1")
	if err := store.UpsertDeploymentAppRegistration(
		ctx, newAppRegistration("registration-work", 101, "Work", now),
	); err != nil {
		t.Fatal(err)
	}
	installationID := int64(42)
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID: "workspace-1", AppRegistrationID: "registration-work",
		Source: ConnectionSourceGitHubAppInstallation, GitHubHost: "github.com",
		InstallationID: &installationID, InstallationAccountLogin: "acme",
		InstallationAccountType: "Organization", Status: ConnectionStatusActive,
		CredentialGeneration: 1,
	}); err != nil {
		t.Fatal(err)
	}
	service := NewAppRegistrationWebhookService(
		"registration-work", "work-secret", store, nil, nil,
	)
	request := signedWebhookRequest(
		"work-secret", "delivery-concurrent", "installation",
		[]byte(`{"action":"suspend","installation":{"id":42}}`),
	)

	type outcome struct {
		result GitHubWebhookResult
		err    error
	}
	ready := sync.WaitGroup{}
	ready.Add(2)
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			result, err := service.Handle(ctx, request)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	ready.Wait()
	close(start)

	affected, duplicates := 0, 0
	for range 2 {
		outcome := <-outcomes
		if outcome.err != nil {
			t.Fatalf("Handle() error = %v", outcome.err)
		}
		affected += outcome.result.Affected
		if outcome.result.Duplicate {
			duplicates++
		}
	}
	if affected != 1 || duplicates != 1 {
		t.Fatalf("affected=%d duplicates=%d", affected, duplicates)
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
		AppRegistrationID: "registration-work", InstallationID: &installationID,
		Status: ConnectionStatusActive, CredentialGeneration: 3,
	}}}}
	repos := &webhookRepoUpdater{}
	service := newRegistrationWebhookService("registration-work", "webhook-secret", store, repos, nil)
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
		repos.updates[0].AppRegistrationID != "registration-work" ||
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
	if got := store.deliveries[":"+request.DeliveryID].Status; got != WebhookDeliveryStatusFailed {
		t.Fatalf("failed delivery status = %q", got)
	}
	result, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("retry Handle() error = %v", err)
	}
	if result.Duplicate || result.Affected != 1 || len(repos.updates) != 1 ||
		store.deliveries[":"+request.DeliveryID].Status != WebhookDeliveryStatusProcessed {
		t.Fatalf(
			"retry result=%+v updates=%d delivery=%+v",
			result, len(repos.updates), store.deliveries[":"+request.DeliveryID],
		)
	}
}

func TestGitHubWebhookReclaimsStaleReceivedDelivery(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	store := &githubWebhookMemoryStore{deliveries: map[string]*WebhookDelivery{
		":delivery-stale": {
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
