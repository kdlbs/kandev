package github

import "context"

// Mock authorization uses the real persisted flow boundaries without provider requests.
func (s *Service) mockAppRuntime(registration *AppRegistration) *githubAppRuntime {
	flows := NewOAuthFlowManager(s.store)
	remote := mockAppAuthorization{host: registration.GitHubHost}
	base := registration.PublicBaseURL + "/api/v1/github/app/registrations/" + registration.ID
	personal := NewPersonalAuthService(PersonalAuthConfig{
		RegistrationID: registration.ID, GitHubHost: registration.GitHubHost,
		AppCredentialGeneration: registration.CredentialGeneration,
		ClientID:                registration.ClientID, CallbackURL: base + "/personal/callback",
	}, flows, NewStorePersonalConnectionRepository(s.store, s.connectionSecrets), remote)
	personal.SetWorkspaceMutationLock(s.workspaceConnectionMutationLock)
	return &githubAppRuntime{
		registrationID: registration.ID, source: DeploymentAppSourceManaged,
		generation: registration.CredentialGeneration, appID: registration.AppID,
		appInstallationAuth: NewAppInstallationService(AppInstallationConfig{
			RegistrationID: registration.ID, Slug: registration.Slug, GitHubHost: registration.GitHubHost,
			AppCredentialGeneration: registration.CredentialGeneration, CallbackURL: base + "/install/callback",
		}, flows, &serviceAppConnectionStore{service: s}, remote, remote),
		personalAuth: personal,
	}
}

type mockAppAuthorization struct{ host string }

func (m mockAppAuthorization) ExchangeUserCode(context.Context, string, string, string) (GitHubOAuthTokens, error) {
	return GitHubOAuthTokens{AccessToken: "mock-oauth:" + m.host}, nil
}

func (m mockAppAuthorization) RefreshUserToken(context.Context, string) (GitHubOAuthTokens, error) {
	return GitHubOAuthTokens{AccessToken: "mock-oauth:" + m.host}, nil
}

func (mockAppAuthorization) GetOAuthUser(context.Context, string) (GitHubOAuthUser, error) {
	return GitHubOAuthUser{ID: 702, Login: "enterprise-human"}, nil
}

func (mockAppAuthorization) UserCanAccessInstallation(context.Context, string, int64) (bool, error) {
	return true, nil
}

func (mockAppAuthorization) GetInstallation(_ context.Context, id int64) (AppInstallation, error) {
	return AppInstallation{ID: id, AccountID: 702, AccountLogin: "acme", AccountType: "Organization",
		Permissions: InstallationPermissions{"contents": PermissionRead, "pull_requests": PermissionWrite, "issues": PermissionWrite, "metadata": PermissionRead}}, nil
}
