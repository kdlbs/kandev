package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// serviceAppConnectionStore adapts the workspace connection store for App
// installation, webhook, and personal-auth flows.
type serviceAppConnectionStore struct {
	service *Service
}

func (r *serviceAppConnectionStore) GetWorkspaceConnection(ctx context.Context, workspaceID string) (*WorkspaceConnection, error) {
	return r.service.store.GetWorkspaceConnection(ctx, workspaceID)
}

func (r *serviceAppConnectionStore) UpsertWorkspaceConnection(ctx context.Context, connection *WorkspaceConnection) error {
	lock := r.service.workspaceConnectionMutationLock(connection.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()
	existing, err := r.service.store.GetWorkspaceConnection(ctx, connection.WorkspaceID)
	if err != nil {
		return err
	}
	connection.CredentialGeneration = nextCredentialGeneration(existing)
	return r.upsertWorkspaceConnectionLocked(ctx, existing, connection)
}

func (r *serviceAppConnectionStore) ReplaceWorkspaceConnection(ctx context.Context, connection *WorkspaceConnection, expected WorkspaceConnectionExpectation) error {
	lock := r.service.workspaceConnectionMutationLock(connection.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()
	existing, err := r.service.store.GetWorkspaceConnection(ctx, connection.WorkspaceID)
	if err != nil {
		return err
	}
	if !matchesWorkspaceConnectionExpectation(existing, expected) || connection.CredentialGeneration != expected.CredentialGeneration+1 {
		return ErrOAuthFlowStale
	}
	return r.upsertWorkspaceConnectionLocked(ctx, existing, connection)
}

func (r *serviceAppConnectionStore) upsertWorkspaceConnectionLocked(ctx context.Context, existing, connection *WorkspaceConnection) error {
	var err error
	if existing != nil && !existing.CreatedAt.IsZero() {
		connection.CreatedAt = existing.CreatedAt
	}
	var previousPAT string
	var hadPreviousPAT bool
	if existing != nil && existing.Source == ConnectionSourcePAT {
		previousPAT, hadPreviousPAT, err = revealOptionalSecret(ctx, r.service.connectionSecrets, WorkspacePATSecretKey(connection.WorkspaceID))
		if err != nil {
			return err
		}
	}
	if err := r.service.applyAutomationTransition(ctx, existing, connection, func() error { return r.service.store.UpsertWorkspaceConnection(ctx, connection) }); err != nil {
		return fmt.Errorf("replace workspace GitHub connection: %w", err)
	}
	if existing != nil && existing.Source == ConnectionSourcePAT && hadPreviousPAT {
		if err := r.service.connectionSecrets.Delete(ctx, WorkspacePATSecretKey(connection.WorkspaceID)); err != nil {
			return errors.Join(err, restoreWorkspaceConnection(ctx, r.service.store, existing, connection.WorkspaceID), restoreOptionalSecret(ctx, r.service.connectionSecrets, WorkspacePATSecretKey(connection.WorkspaceID), workspacePATSecretName, previousPAT, hadPreviousPAT))
		}
	}
	if existing != nil && existing.InstallationID != nil {
		r.service.InvalidateAppInstallationCredentials(existing.AppRegistrationID, *existing.InstallationID)
	}
	r.service.invalidateWorkspaceCredential(connection.WorkspaceID)
	return nil
}

func restoreOptionalSecret(ctx context.Context, secrets ConnectionSecretStore, key, name, value string, existed bool) error {
	if existed {
		return secrets.Set(ctx, key, name, value)
	}
	return deleteOptionalSecret(ctx, secrets, key)
}

func (r *serviceAppConnectionStore) ListWorkspaceConnectionsByInstallation(ctx context.Context, installationID int64) ([]*WorkspaceConnection, error) {
	return r.service.store.ListWorkspaceConnectionsByInstallation(ctx, installationID)
}
func (r *serviceAppConnectionStore) ListWorkspaceConnectionsByAppInstallation(ctx context.Context, registrationID string, installationID int64) ([]*WorkspaceConnection, error) {
	return r.service.store.ListWorkspaceConnectionsByAppInstallation(ctx, registrationID, installationID)
}

func (r *serviceAppConnectionStore) TransitionWorkspaceInstallationConnection(ctx context.Context, expected, next *WorkspaceConnection) (bool, error) {
	lock := r.service.workspaceConnectionMutationLock(expected.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()
	updated, err := r.service.store.TransitionWorkspaceInstallationConnection(ctx, expected, next)
	if err != nil || !updated {
		return updated, err
	}
	if expected.InstallationID != nil {
		r.service.InvalidateAppInstallationCredentials(expected.AppRegistrationID, *expected.InstallationID)
	}
	r.service.invalidateWorkspaceCredential(expected.WorkspaceID)
	return true, nil
}

func (r *serviceAppConnectionStore) ListUserConnectionsByGitHubUser(ctx context.Context, githubUserID int64) ([]*UserConnection, error) {
	return r.service.store.ListUserConnectionsByGitHubUser(ctx, githubUserID)
}
func (r *serviceAppConnectionStore) ListUserConnectionsByAppGitHubUser(ctx context.Context, registrationID string, githubUserID int64) ([]*UserConnection, error) {
	return r.service.store.ListUserConnectionsByAppGitHubUser(ctx, registrationID, githubUserID)
}
func (r *serviceAppConnectionStore) ClaimWebhookDelivery(ctx context.Context, delivery *WebhookDelivery, staleBefore time.Time) (WebhookDeliveryClaim, error) {
	return r.service.store.ClaimWebhookDelivery(ctx, delivery, staleBefore)
}
func (r *serviceAppConnectionStore) CompleteWebhookDelivery(ctx context.Context, deliveryID string, status WebhookDeliveryStatus, result string, processedAt time.Time) error {
	return r.service.store.CompleteWebhookDelivery(ctx, deliveryID, status, result, processedAt)
}
func (r *serviceAppConnectionStore) CompleteAppRegistrationWebhookDelivery(ctx context.Context, registrationID, deliveryID string, status WebhookDeliveryStatus, result string, processedAt time.Time) error {
	return r.service.store.CompleteAppRegistrationWebhookDelivery(ctx, registrationID, deliveryID, status, result, processedAt)
}

func restoreWorkspaceConnection(ctx context.Context, store *Store, connection *WorkspaceConnection, workspaceID string) error {
	if connection == nil {
		return store.DeleteWorkspaceConnection(ctx, workspaceID)
	}
	return store.UpsertWorkspaceConnection(ctx, connection)
}

type installationRepositorySettingsUpdater struct{ service *Service }

func (u *installationRepositorySettingsUpdater) ApplyInstallationRepositories(ctx context.Context, change InstallationRepositoriesChange) (bool, error) {
	if u == nil || u.service == nil {
		return false, errors.New("GitHub installation repository updater is not configured")
	}
	lock := u.service.workspaceConnectionMutationLock(change.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()
	connection, err := u.service.store.GetWorkspaceConnection(ctx, change.WorkspaceID)
	if err != nil {
		return false, err
	}
	expectedInstallationID := change.InstallationID
	if !matchesWorkspaceConnectionExpectation(connection, WorkspaceConnectionExpectation{Source: change.ConnectionSource, CredentialGeneration: change.CredentialGeneration, InstallationID: &expectedInstallationID, AppRegistrationID: change.AppRegistrationID}) || connection.Status != ConnectionStatusActive {
		return false, nil
	}
	u.service.InvalidateAppInstallationCredentials(change.AppRegistrationID, change.InstallationID)
	u.service.invalidateWorkspaceCredential(change.WorkspaceID)
	if len(change.Removed) == 0 {
		return true, nil
	}
	settings, err := u.service.store.GetWorkspaceSettings(ctx, change.WorkspaceID)
	if err != nil || settings == nil || settings.RepoScopeMode != RepoScopeModeRepos {
		return err == nil, err
	}
	removed := make(map[string]struct{}, len(change.Removed))
	for _, repository := range change.Removed {
		removed[strings.ToLower(repository.FullName)] = struct{}{}
	}
	filtered := settings.RepoScopeRepos[:0]
	for _, repository := range settings.RepoScopeRepos {
		if _, ok := removed[strings.ToLower(repository.Owner+"/"+repository.Name)]; !ok {
			filtered = append(filtered, repository)
		}
	}
	if len(filtered) == len(settings.RepoScopeRepos) {
		return true, nil
	}
	settings.RepoScopeRepos = filtered
	if err := u.service.store.UpsertWorkspaceSettings(ctx, settings); err != nil {
		return false, err
	}
	return true, nil
}
