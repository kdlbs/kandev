package githubauth

import "strings"

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

// CanonicalCredentialPath is the single form every side of the credential
// broker compares scope paths in: no trailing slashes and no ".git" suffix.
// Scope paths are derived from clone URLs, which carry the suffix, while the
// `gh` CLI shim asks for a bare "/<owner>/<repo>" — without one canonical form
// the two spellings of the same repository never match. Callers pass an already
// unescaped path; this only normalizes spelling, it does not validate.
func CanonicalCredentialPath(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return strings.TrimSpace(raw)
	}
	if withoutSuffix := strings.TrimSuffix(trimmed, ".git"); withoutSuffix != "" {
		trimmed = withoutSuffix
	}
	return trimmed
}
