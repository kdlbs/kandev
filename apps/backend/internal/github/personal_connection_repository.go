package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const (
	personalAccessTokenSecretName  = "GitHub personal access token"
	personalRefreshTokenSecretName = "GitHub personal refresh token"
)

// StorePersonalConnectionRepository provides the compensation boundary needed
// to keep encrypted OAuth tokens and their metadata consistent even though the
// two stores do not share a database transaction.
type StorePersonalConnectionRepository struct {
	store   *Store
	secrets ConnectionSecretStore

	mu sync.Mutex
}

func NewStorePersonalConnectionRepository(
	store *Store,
	secrets ConnectionSecretStore,
) *StorePersonalConnectionRepository {
	return &StorePersonalConnectionRepository{store: store, secrets: secrets}
}

func (r *StorePersonalConnectionRepository) GetWorkspaceConnection(
	ctx context.Context,
	workspaceID string,
) (*WorkspaceConnection, error) {
	return r.store.GetWorkspaceConnection(ctx, workspaceID)
}

func (r *StorePersonalConnectionRepository) GetUserConnection(
	ctx context.Context,
	workspaceID, userID string,
) (*UserConnection, error) {
	return r.store.GetUserConnection(ctx, workspaceID, userID)
}

func (r *StorePersonalConnectionRepository) GetPersonalConnectionGeneration(
	ctx context.Context,
	workspaceID, userID string,
) (int64, error) {
	return r.store.GetUserConnectionGeneration(ctx, workspaceID, userID)
}

func (r *StorePersonalConnectionRepository) GetPersonalTokens(
	ctx context.Context,
	workspaceID, userID string,
) (GitHubOAuthTokens, error) {
	connection, err := r.store.GetUserConnection(ctx, workspaceID, userID)
	if err != nil {
		return GitHubOAuthTokens{}, err
	}
	if connection == nil {
		return GitHubOAuthTokens{}, ErrGitHubPersonalRequired
	}
	return r.GetPersonalTokensForConnection(ctx, connection)
}

func (r *StorePersonalConnectionRepository) GetPersonalTokensForConnection(ctx context.Context, connection *UserConnection) (GitHubOAuthTokens, error) {
	if connection == nil {
		return GitHubOAuthTokens{}, ErrGitHubPersonalRequired
	}
	current, err := r.store.GetUserConnection(ctx, connection.WorkspaceID, connection.UserID)
	if err != nil {
		return GitHubOAuthTokens{}, err
	}
	if !sameUserConnectionVersion(current, connection) {
		return GitHubOAuthTokens{}, ErrPersonalConnectionStale
	}
	accessKey, refreshKey, err := r.personalSecretKeys(ctx, connection)
	if err != nil {
		return GitHubOAuthTokens{}, err
	}
	access, err := r.secrets.Reveal(ctx, accessKey)
	if err != nil {
		return GitHubOAuthTokens{}, err
	}
	refresh, _, err := revealOptionalSecret(ctx, r.secrets, refreshKey)
	if err != nil {
		return GitHubOAuthTokens{}, err
	}
	return GitHubOAuthTokens{
		AccessToken:      access,
		RefreshToken:     refresh,
		AccessExpiresAt:  connection.AccessExpiresAt,
		RefreshExpiresAt: connection.RefreshExpiresAt,
	}, nil
}

func (r *StorePersonalConnectionRepository) ReplacePersonalConnection(
	ctx context.Context,
	connection *UserConnection,
	tokens GitHubOAuthTokens,
	expectedGeneration int64,
) error {
	if r == nil || r.store == nil || r.secrets == nil || connection == nil {
		return errors.New("personal GitHub connection repository is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.replacePersonalConnectionLocked(ctx, connection, tokens, expectedGeneration)
}

func (r *StorePersonalConnectionRepository) replacePersonalConnectionLocked(
	ctx context.Context,
	connection *UserConnection,
	tokens GitHubOAuthTokens,
	expectedGeneration int64,
) error {
	if strings.TrimSpace(tokens.AccessToken) == "" {
		return ErrPersonalTokenInvalid
	}
	oldConnection, err := r.store.GetUserConnection(ctx, connection.WorkspaceID, connection.UserID)
	if err != nil {
		return err
	}
	currentGeneration, err := r.store.GetUserConnectionGeneration(ctx, connection.WorkspaceID, connection.UserID)
	if err != nil {
		return err
	}
	if currentGeneration != expectedGeneration ||
		connection.CredentialGeneration != expectedGeneration+1 {
		return ErrPersonalConnectionStale
	}
	accessKey, refreshKey, err := r.personalSecretKeys(ctx, connection)
	if err != nil {
		return err
	}
	oldAccess, hadAccess, err := revealOptionalSecret(ctx, r.secrets, accessKey)
	if err != nil {
		return err
	}
	oldRefresh, hadRefresh, err := revealOptionalSecret(ctx, r.secrets, refreshKey)
	if err != nil {
		return err
	}
	if err := r.secrets.Set(ctx, accessKey, personalAccessTokenSecretName, tokens.AccessToken); err != nil {
		return err
	}
	if err := r.storeRefreshSecret(ctx, refreshKey, tokens.RefreshToken); err != nil {
		return errors.Join(err, r.restoreSecret(ctx, accessKey, personalAccessTokenSecretName, oldAccess, hadAccess))
	}
	if err := r.store.UpsertUserConnection(ctx, connection); err != nil {
		return errors.Join(
			err,
			r.restoreSecret(ctx, accessKey, personalAccessTokenSecretName, oldAccess, hadAccess),
			r.restoreSecret(ctx, refreshKey, personalRefreshTokenSecretName, oldRefresh, hadRefresh),
			r.restoreUserConnection(ctx, oldConnection, connection.WorkspaceID, connection.UserID),
		)
	}
	return nil
}

func (r *StorePersonalConnectionRepository) ReplacePersonalConnectionForFlow(
	ctx context.Context,
	connection *UserConnection,
	tokens GitHubOAuthTokens,
	expectedGeneration int64,
	expectedWorkspace WorkspaceConnectionExpectation,
) error {
	if r == nil || r.store == nil || r.secrets == nil || connection == nil {
		return errors.New("personal GitHub connection repository is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	workspace, err := r.store.GetWorkspaceConnection(ctx, connection.WorkspaceID)
	if err != nil {
		return err
	}
	if !matchesWorkspaceConnectionExpectation(workspace, expectedWorkspace) {
		return ErrOAuthFlowStale
	}
	return r.replacePersonalConnectionLocked(ctx, connection, tokens, expectedGeneration)
}

func (r *StorePersonalConnectionRepository) MarkPersonalConnectionInvalid(
	ctx context.Context,
	workspaceID, userID string,
	expectedGeneration int64,
	reason error,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	connection, err := r.store.GetUserConnection(ctx, workspaceID, userID)
	if err != nil || connection == nil {
		return err
	}
	if connection.CredentialGeneration != expectedGeneration || connection.Status != ConnectionStatusActive {
		return ErrPersonalConnectionStale
	}
	connection.Status = ConnectionStatusInvalid
	connection.CredentialGeneration++
	connection.LastError = truncateConnectionError(reason)
	return r.store.UpsertUserConnection(ctx, connection)
}

func (r *StorePersonalConnectionRepository) RevokePersonalConnection(
	ctx context.Context,
	workspaceID, userID string,
) error {
	if r == nil || r.store == nil || r.secrets == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	connection, err := r.store.GetUserConnection(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	return r.revokePersonalConnectionLocked(ctx, connection, workspaceID, userID)
}

func (r *StorePersonalConnectionRepository) RevokePersonalConnectionIfUnchanged(
	ctx context.Context,
	expected *UserConnection,
) (bool, error) {
	if r == nil || r.store == nil || r.secrets == nil || expected == nil {
		return false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, err := r.store.GetUserConnection(ctx, expected.WorkspaceID, expected.UserID)
	if err != nil || !sameUserConnectionVersion(current, expected) {
		return false, err
	}
	if err := r.revokePersonalConnectionLocked(
		ctx, current, expected.WorkspaceID, expected.UserID,
	); err != nil {
		return false, err
	}
	return true, nil
}

func (r *StorePersonalConnectionRepository) revokePersonalConnectionLocked(
	ctx context.Context,
	connection *UserConnection,
	workspaceID, userID string,
) error {
	accessKey, refreshKey := UserAccessTokenSecretKey(workspaceID, userID), UserRefreshTokenSecretKey(workspaceID, userID)
	if connection != nil {
		var err error
		accessKey, refreshKey, err = r.personalSecretKeys(ctx, connection)
		if err != nil {
			return err
		}
	}
	oldAccess, hadAccess, err := revealOptionalSecret(ctx, r.secrets, accessKey)
	if err != nil {
		return err
	}
	oldRefresh, hadRefresh, err := revealOptionalSecret(ctx, r.secrets, refreshKey)
	if err != nil {
		return err
	}
	if err := deleteOptionalSecret(ctx, r.secrets, accessKey); err != nil {
		return err
	}
	if err := deleteOptionalSecret(ctx, r.secrets, refreshKey); err != nil {
		return errors.Join(err, r.restoreSecret(ctx, accessKey, personalAccessTokenSecretName, oldAccess, hadAccess))
	}
	if err := r.store.DeleteUserConnection(ctx, workspaceID, userID); err != nil {
		return errors.Join(
			err,
			r.restoreSecret(ctx, accessKey, personalAccessTokenSecretName, oldAccess, hadAccess),
			r.restoreSecret(ctx, refreshKey, personalRefreshTokenSecretName, oldRefresh, hadRefresh),
			r.restoreUserConnection(ctx, connection, workspaceID, userID),
		)
	}
	return nil
}

type personalConnectionSnapshot struct {
	accessKey, refreshKey string
	connection            *UserConnection
	access, refresh       string
	hadAccess, hadRefresh bool
}

// RevokeWorkspacePersonalConnections removes every App user identity in a
// workspace. Secret deletion is compensated before metadata is committed so
// callers never observe a partially revoked set of users.
func (r *StorePersonalConnectionRepository) RevokeWorkspacePersonalConnections(
	ctx context.Context,
	workspaceID string,
) error {
	return r.TransitionWorkspacePersonalConnections(ctx, workspaceID, func() error { return nil })
}

// TransitionWorkspacePersonalConnections revokes a workspace's personal App
// identities and runs the workspace mutation under the same compensation
// boundary. A failed mutation restores both metadata and encrypted tokens.
func (r *StorePersonalConnectionRepository) TransitionWorkspacePersonalConnections(
	ctx context.Context,
	workspaceID string,
	mutation func() error,
) error {
	if r == nil || r.store == nil || r.secrets == nil {
		return mutation()
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	connections, err := r.store.ListUserConnectionsByWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	if len(connections) == 0 {
		return mutation()
	}
	snapshots, err := r.loadWorkspacePersonalSnapshots(ctx, connections)
	if err != nil {
		return err
	}
	if err := r.deleteWorkspacePersonalSecrets(ctx, snapshots); err != nil {
		return errors.Join(err, r.restoreWorkspacePersonalSnapshots(ctx, snapshots))
	}
	if err := r.store.DeleteUserConnectionsByWorkspace(ctx, workspaceID); err != nil {
		return errors.Join(err, r.restoreWorkspacePersonalSnapshots(ctx, snapshots))
	}
	if err := mutation(); err != nil {
		return errors.Join(err, r.restoreWorkspacePersonalSnapshots(ctx, snapshots))
	}
	return nil
}

func (r *StorePersonalConnectionRepository) loadWorkspacePersonalSnapshots(
	ctx context.Context,
	connections []*UserConnection,
) ([]personalConnectionSnapshot, error) {
	snapshots := make([]personalConnectionSnapshot, 0, len(connections))
	for _, connection := range connections {
		accessKey, refreshKey, err := r.personalSecretKeys(ctx, connection)
		if err != nil {
			return nil, err
		}
		access, hadAccess, err := revealOptionalSecret(
			ctx, r.secrets, accessKey,
		)
		if err != nil {
			return nil, err
		}
		refresh, hadRefresh, err := revealOptionalSecret(
			ctx, r.secrets, refreshKey,
		)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, personalConnectionSnapshot{
			connection: connection, access: access, refresh: refresh,
			accessKey: accessKey, refreshKey: refreshKey,
			hadAccess: hadAccess, hadRefresh: hadRefresh,
		})
	}
	return snapshots, nil
}

func (r *StorePersonalConnectionRepository) deleteWorkspacePersonalSecrets(
	ctx context.Context,
	snapshots []personalConnectionSnapshot,
) error {
	for _, snapshot := range snapshots {
		if err := deleteOptionalSecret(ctx, r.secrets, snapshot.accessKey); err != nil {
			return err
		}
		if err := deleteOptionalSecret(ctx, r.secrets, snapshot.refreshKey); err != nil {
			return err
		}
	}
	return nil
}

func (r *StorePersonalConnectionRepository) restoreWorkspacePersonalSnapshots(
	ctx context.Context,
	snapshots []personalConnectionSnapshot,
) error {
	var restoreErr error
	for _, snapshot := range snapshots {
		restoreErr = errors.Join(restoreErr,
			r.restorePersonalConnectionMetadata(ctx, snapshot.connection),
			r.restoreSecret(ctx, snapshot.accessKey,
				personalAccessTokenSecretName, snapshot.access, snapshot.hadAccess),
			r.restoreSecret(ctx, snapshot.refreshKey,
				personalRefreshTokenSecretName, snapshot.refresh, snapshot.hadRefresh),
		)
	}
	return restoreErr
}

func (r *StorePersonalConnectionRepository) restorePersonalConnectionMetadata(
	ctx context.Context,
	connection *UserConnection,
) error {
	currentGeneration, err := r.store.GetUserConnectionGeneration(
		ctx, connection.WorkspaceID, connection.UserID,
	)
	if err != nil {
		return err
	}
	restored := *connection
	if currentGeneration > restored.CredentialGeneration {
		restored.CredentialGeneration = currentGeneration
	}
	return r.store.UpsertUserConnection(ctx, &restored)
}

func (r *StorePersonalConnectionRepository) restoreSecret(
	ctx context.Context,
	key, name, value string,
	existed bool,
) error {
	if existed {
		return r.secrets.Set(ctx, key, name, value)
	}
	return deleteOptionalSecret(ctx, r.secrets, key)
}

func (r *StorePersonalConnectionRepository) restoreUserConnection(
	ctx context.Context,
	connection *UserConnection,
	workspaceID, userID string,
) error {
	if connection == nil {
		return r.store.DeleteUserConnection(ctx, workspaceID, userID)
	}
	return r.store.UpsertUserConnection(ctx, connection)
}

func truncateConnectionError(reason error) string {
	if reason == nil {
		return ""
	}
	message := reason.Error()
	if len(message) > 512 {
		return message[:512]
	}
	return message
}

type personalAuthCredentialProvider struct {
	service *PersonalAuthService
}

func (p *personalAuthCredentialProvider) ResolveUser(
	ctx context.Context,
	connection *UserConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	if p == nil || p.service == nil || connection == nil {
		return nil, ErrGitHubPersonalRequired
	}
	token, current, err := p.service.ResolvePersonalToken(ctx, connection.WorkspaceID, connection.UserID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.GitHubUserID != connection.GitHubUserID {
		return nil, fmt.Errorf("%w: personal connection changed during resolution", ErrGitHubConnectionInvalid)
	}
	tracker := NewRateTracker(nil, nil)
	host, err := connectionHost(p.service.config.GitHubHost)
	if err != nil {
		return nil, err
	}
	client := NewTokenClientForHost(token, TokenPrincipal{Kind: TokenCredentialUser, PrincipalID: fmt.Sprintf("user:%d", current.GitHubUserID), Login: current.Login}, host).WithRateTracker(tracker)
	return &ResolvedCredential{
		Client:     client,
		GitHubHost: host.String(),
		Principal: AuthPrincipal{
			GitHubHost: host.String(),
			Kind:       AuthPrincipalHuman,
			Source:     ConnectionSourceGitHubAppUser,
			Login:      current.Login,
		},
		CredentialGeneration: current.CredentialGeneration,
		ExpiresAt:            current.AccessExpiresAt,
		RateTracker:          tracker,
		credential:           token,
	}, nil
}

func (r *StorePersonalConnectionRepository) personalSecretKeys(ctx context.Context, connection *UserConnection) (string, string, error) {
	registration, err := r.store.GetAppRegistration(ctx, connection.AppRegistrationID)
	if err != nil {
		return "", "", err
	}
	if registration == nil {
		return "", "", ErrGitHubPersonalRequired
	}
	host, err := connectionHost(registration.GitHubHost)
	if err != nil {
		return "", "", err
	}
	return UserAccessTokenSecretKeyForHost(connection.WorkspaceID, connection.UserID, host.String()), UserRefreshTokenSecretKeyForHost(connection.WorkspaceID, connection.UserID, host.String()), nil
}

func (r *StorePersonalConnectionRepository) storeRefreshSecret(ctx context.Context, key, value string) error {
	if value == "" {
		return deleteOptionalSecret(ctx, r.secrets, key)
	}
	return r.secrets.Set(ctx, key, personalRefreshTokenSecretName, value)
}
