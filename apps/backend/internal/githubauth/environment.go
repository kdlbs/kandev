package githubauth

const (
	CredentialBrokerURLEnv  = "KANDEV_GITHUB_CREDENTIAL_BROKER_URL"
	CredentialHelperPathEnv = "KANDEV_GITHUB_CREDENTIAL_HELPER_PATH"
	CredentialCLIShimDirEnv = "KANDEV_GITHUB_CLI_SHIM_DIR"
	CredentialCLIBashEnvEnv = "KANDEV_GITHUB_CLI_BASH_ENV"
	CredentialParentBashEnv = "KANDEV_GITHUB_PARENT_BASH_ENV"
	CredentialLeaseEnv      = "KANDEV_GITHUB_CREDENTIAL_LEASE"
	CredentialTaskIDEnv     = "KANDEV_GITHUB_CREDENTIAL_TASK_ID"
	CredentialSessionIDEnv  = "KANDEV_GITHUB_CREDENTIAL_SESSION_ID"
	CredentialRepositoryEnv = "KANDEV_GITHUB_CREDENTIAL_REPOSITORY_ID"
	CredentialOwnerEnv      = "KANDEV_GITHUB_CREDENTIAL_OWNER"
	CredentialRepoEnv       = "KANDEV_GITHUB_CREDENTIAL_REPO"
	CredentialHostEnv       = "KANDEV_GITHUB_CREDENTIAL_HOST"
	CredentialScopesEnv     = "KANDEV_GITHUB_CREDENTIAL_SCOPES"

	ManagedGitCredentialHelper    = `!"${KANDEV_GITHUB_CREDENTIAL_HELPER_PATH}" git-credential`
	LegacyShimGitCredentialHelper = `!"${KANDEV_GITHUB_CLI_SHIM_DIR}/agentctl" git-credential`
	LegacyGitCredentialHelper     = "!agentctl git-credential"
	CLIBashEnvFilename            = "bash-env.sh"
)
