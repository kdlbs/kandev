package github

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStorePersonalConnectionRepositoryRoundTripAndRevoke(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	seedPersonalAppConnection(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repo := NewStorePersonalConnectionRepository(store, secrets)
	now := time.Now().UTC()
	refreshExpiry := now.Add(30 * 24 * time.Hour)
	connection := &UserConnection{
		WorkspaceID: "workspace-1", UserID: "user-1", AppRegistrationID: "registration-personal-test",
		GitHubUserID: 42, Login: "octocat",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(8 * time.Hour),
		RefreshExpiresAt: &refreshExpiry, CredentialGeneration: 1,
	}
	tokens := GitHubOAuthTokens{
		AccessToken: "access", RefreshToken: "refresh",
		AccessExpiresAt: connection.AccessExpiresAt, RefreshExpiresAt: &refreshExpiry,
	}
	if err := repo.ReplacePersonalConnection(context.Background(), connection, tokens, 0); err != nil {
		t.Fatalf("ReplacePersonalConnection: %v", err)
	}
	got, err := repo.GetPersonalTokens(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatalf("GetPersonalTokens: %v", err)
	}
	if got.AccessToken != "access" || got.RefreshToken != "refresh" {
		t.Fatalf("tokens = %+v", got)
	}
	if err := repo.RevokePersonalConnection(context.Background(), "workspace-1", "user-1"); err != nil {
		t.Fatalf("RevokePersonalConnection: %v", err)
	}
	stored, err := store.GetUserConnection(context.Background(), "workspace-1", "user-1")
	if err != nil || stored != nil {
		t.Fatalf("stored connection after revoke = %+v, err %v", stored, err)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("personal secrets remain after revoke: %#v", secrets.values)
	}
}

func TestStorePersonalConnectionRepositoryCompensatesPartialSecretRotation(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	seedPersonalAppConnection(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repo := NewStorePersonalConnectionRepository(store, secrets)
	now := time.Now().UTC()
	oldConnection := &UserConnection{
		WorkspaceID: "workspace-1", UserID: "user-1", AppRegistrationID: "registration-personal-test",
		GitHubUserID: 42, Login: "old",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}
	if err := repo.ReplacePersonalConnection(context.Background(), oldConnection, GitHubOAuthTokens{
		AccessToken: "old-access", RefreshToken: "old-refresh", AccessExpiresAt: oldConnection.AccessExpiresAt,
	}, 0); err != nil {
		t.Fatal(err)
	}
	secrets.setErr = errors.New("refresh write failed")
	secrets.setErrKey = UserRefreshTokenSecretKey("workspace-1", "user-1")
	replacement := *oldConnection
	replacement.Login = "new"
	replacement.CredentialGeneration = 2
	if err := repo.ReplacePersonalConnection(context.Background(), &replacement, GitHubOAuthTokens{
		AccessToken: "new-access", RefreshToken: "new-refresh", AccessExpiresAt: now.Add(2 * time.Hour),
	}, 1); err == nil {
		t.Fatal("expected rotation failure")
	}
	if got := secrets.values[UserAccessTokenSecretKey("workspace-1", "user-1")]; got != "old-access" {
		t.Fatalf("access token after compensation = %q", got)
	}
	stored, err := store.GetUserConnection(context.Background(), "workspace-1", "user-1")
	if err != nil || stored.Login != "old" || stored.CredentialGeneration != 1 {
		t.Fatalf("metadata after compensation = %+v, err %v", stored, err)
	}
}

func TestStorePersonalConnectionRepositoryRejectsStaleReplacementAfterRevoke(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	seedPersonalAppConnection(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repo := NewStorePersonalConnectionRepository(store, secrets)
	now := time.Now().UTC()
	connection := &UserConnection{
		WorkspaceID: "workspace-1", UserID: "user-1", AppRegistrationID: "registration-personal-test",
		GitHubUserID: 42, Login: "octocat",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Minute), CredentialGeneration: 1,
	}
	oldTokens := GitHubOAuthTokens{
		AccessToken: "old-access", RefreshToken: "old-refresh", AccessExpiresAt: connection.AccessExpiresAt,
	}
	if err := repo.ReplacePersonalConnection(context.Background(), connection, oldTokens, 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.RevokePersonalConnection(context.Background(), "workspace-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	replacement := *connection
	replacement.CredentialGeneration = 2
	err := repo.ReplacePersonalConnection(context.Background(), &replacement, GitHubOAuthTokens{
		AccessToken: "new-access", RefreshToken: "new-refresh", AccessExpiresAt: now.Add(time.Hour),
	}, 1)
	if !errors.Is(err, ErrPersonalConnectionStale) {
		t.Fatalf("ReplacePersonalConnection() error = %v, want stale connection", err)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("stale replacement restored secrets: %#v", secrets.values)
	}
}

func TestStorePersonalConnectionRepositoryLateRevokePreservesReplacement(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	seedPersonalAppConnection(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repo := NewStorePersonalConnectionRepository(store, secrets)
	now := time.Now().UTC()
	first := &UserConnection{
		WorkspaceID: "workspace-1", UserID: "user-1", AppRegistrationID: "registration-personal-test",
		GitHubUserID: 42, Login: "octocat",
		Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}
	if err := repo.ReplacePersonalConnection(context.Background(), first, GitHubOAuthTokens{
		AccessToken: "old", RefreshToken: "old-refresh", AccessExpiresAt: first.AccessExpiresAt,
	}, 0); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.GetUserConnection(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	replacement := *snapshot
	replacement.CredentialGeneration++
	replacement.AccessExpiresAt = now.Add(2 * time.Hour)
	if err := repo.ReplacePersonalConnection(context.Background(), &replacement, GitHubOAuthTokens{
		AccessToken: "new", RefreshToken: "new-refresh", AccessExpiresAt: replacement.AccessExpiresAt,
	}, snapshot.CredentialGeneration); err != nil {
		t.Fatal(err)
	}
	revoked, err := repo.RevokePersonalConnectionIfUnchanged(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := repo.GetPersonalTokens(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if revoked || tokens.AccessToken != "new" {
		t.Fatalf("revoked = %v, tokens = %+v", revoked, tokens)
	}
}

func TestStorePersonalConnectionRepositoryBulkRevokeCompensatesSecretDeleteFailure(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	seedPersonalAppConnection(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repo := NewStorePersonalConnectionRepository(store, secrets)
	now := time.Now().UTC()
	for index, userID := range []string{"user-1", "user-2"} {
		connection := &UserConnection{
			WorkspaceID: "workspace-1", UserID: userID, AppRegistrationID: "registration-personal-test",
			GitHubUserID: int64(index + 1), Login: userID,
			Status: ConnectionStatusActive, AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
		}
		if err := repo.ReplacePersonalConnection(context.Background(), connection, GitHubOAuthTokens{
			AccessToken: "access-" + userID, RefreshToken: "refresh-" + userID,
			AccessExpiresAt: connection.AccessExpiresAt,
		}, 0); err != nil {
			t.Fatal(err)
		}
	}
	secrets.deleteErr = errors.New("delete failed")
	secrets.deleteErrKey = UserRefreshTokenSecretKey("workspace-1", "user-2")

	if err := repo.RevokeWorkspacePersonalConnections(context.Background(), "workspace-1"); err == nil {
		t.Fatal("expected bulk revoke failure")
	}
	for _, userID := range []string{"user-1", "user-2"} {
		connection, err := store.GetUserConnection(context.Background(), "workspace-1", userID)
		if err != nil || connection == nil {
			t.Fatalf("connection %s after compensation = %+v, err %v", userID, connection, err)
		}
		if got := secrets.values[UserAccessTokenSecretKey("workspace-1", userID)]; got != "access-"+userID {
			t.Fatalf("access secret for %s after compensation = %q", userID, got)
		}
		if got := secrets.values[UserRefreshTokenSecretKey("workspace-1", userID)]; got != "refresh-"+userID {
			t.Fatalf("refresh secret for %s after compensation = %q", userID, got)
		}
	}
}

func TestStorePersonalConnectionRepositoryFailedTransitionAllowsLaterRotation(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	seedPersonalAppConnection(t, store, "workspace-1")
	secrets := newFakeConnectionSecrets()
	repo := NewStorePersonalConnectionRepository(store, secrets)
	ctx := context.Background()
	now := time.Now().UTC()
	connection := &UserConnection{
		WorkspaceID: "workspace-1", UserID: "user-1", AppRegistrationID: "registration-personal-test",
		GitHubUserID: 42, Login: "octocat", Status: ConnectionStatusActive,
		AccessExpiresAt: now.Add(time.Hour), CredentialGeneration: 1,
	}
	if err := repo.ReplacePersonalConnection(ctx, connection, GitHubOAuthTokens{
		AccessToken: "old-access", RefreshToken: "old-refresh", AccessExpiresAt: connection.AccessExpiresAt,
	}, 0); err != nil {
		t.Fatal(err)
	}

	transitionErr := errors.New("workspace credential transition failed")
	if err := repo.TransitionWorkspacePersonalConnections(ctx, "workspace-1", func() error {
		return transitionErr
	}); !errors.Is(err, transitionErr) {
		t.Fatalf("TransitionWorkspacePersonalConnections() error = %v, want %v", err, transitionErr)
	}
	restored, err := repo.GetUserConnection(ctx, "workspace-1", "user-1")
	if err != nil || restored == nil {
		t.Fatalf("restored connection = %+v, err %v", restored, err)
	}
	currentGeneration, err := repo.GetPersonalConnectionGeneration(ctx, "workspace-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if restored.CredentialGeneration != currentGeneration {
		t.Fatalf("restored generation = %d, current generation = %d", restored.CredentialGeneration, currentGeneration)
	}

	replacement := *restored
	replacement.CredentialGeneration++
	replacement.AccessExpiresAt = now.Add(2 * time.Hour)
	if err := repo.ReplacePersonalConnection(ctx, &replacement, GitHubOAuthTokens{
		AccessToken: "new-access", RefreshToken: "new-refresh", AccessExpiresAt: replacement.AccessExpiresAt,
	}, restored.CredentialGeneration); err != nil {
		t.Fatalf("ReplacePersonalConnection() after compensated transition: %v", err)
	}
	tokens, err := repo.GetPersonalTokens(ctx, "workspace-1", "user-1")
	if err != nil || tokens.AccessToken != "new-access" || tokens.RefreshToken != "new-refresh" {
		t.Fatalf("rotated tokens = %+v, err %v", tokens, err)
	}
}

func seedPersonalAppConnection(t *testing.T, store *Store, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	registration := newAppRegistration(
		"registration-personal-test", 987, "Personal test", time.Now().UTC(),
	)
	if err := store.UpsertDeploymentAppRegistration(ctx, registration); err != nil {
		t.Fatalf("seed App registration: %v", err)
	}
	installationID := int64(42)
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID: workspaceID, Source: ConnectionSourceGitHubAppInstallation,
		GitHubHost: "github.com", InstallationID: &installationID,
		InstallationAccountLogin: "acme", InstallationAccountType: "Organization",
		AppRegistrationID: registration.ID, Status: ConnectionStatusActive,
	}); err != nil {
		t.Fatalf("seed workspace App connection: %v", err)
	}
}
