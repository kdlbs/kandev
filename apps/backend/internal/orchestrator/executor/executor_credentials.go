package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/gitconfigenv"
	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	// envGitHubToken is the environment variable name for GitHub authentication tokens.
	envGitHubToken = "GITHUB_TOKEN"
	// envGHToken is the gh CLI compatible environment variable name.
	envGHToken          = "GH_TOKEN"
	envGitLabToken      = "GITLAB_TOKEN"
	envGitLabHost       = "GITLAB_HOST"
	envKandevGitLabHost = "KANDEV_GITLAB_HOST"

	gitCredentialHelper            = githubauth.ManagedGitCredentialHelper
	gitHubCredentialHelper         = gitCredentialHelper
	legacyShimGitCredentialHelper  = githubauth.LegacyShimGitCredentialHelper
	legacyGitHubCredentialHelper   = githubauth.LegacyGitCredentialHelper
	defaultGitHubHost              = "github.com"
	gitHubProviderID               = "github"
	gitLabProviderID               = "gitlab"
	httpsScheme                    = "https"
	gitLabCredentialHelper         = `!f() { echo "username=oauth2"; echo "password=$GITLAB_TOKEN"; }; f`
	taskGitCredentialsModeManaged  = "managed"
	taskGitCredentialsModeExecutor = "executor"
)

var ErrGitHubCredentialBrokerURL = errors.New("invalid Git credential broker URL")

type githubCredentialScope struct {
	Lease        string `json:"lease"`
	TaskID       string `json:"task_id"`
	SessionID    string `json:"session_id"`
	RepositoryID string `json:"repository_id"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Host         string `json:"host"`
	Path         string `json:"path"`
}

// GitCredentialLeaseIssuer creates opaque helper leases. *gitcredentials.Broker
// satisfies it directly; credentials never enter an executor payload.
type GitCredentialLeaseIssuer interface {
	Issue(context.Context, gitcredentials.Scope) (gitcredentials.Lease, error)
}

// GitHubCredentialLeaseIssuer is retained as a source-compatible name for
// callers which configure the same generic HTTPS helper broker.
type GitHubCredentialLeaseIssuer = GitCredentialLeaseIssuer

// TaskGitCredentialPolicy is the non-secret routing contract resolved for a workspace.
type TaskGitCredentialPolicy struct {
	Mode            string
	WorkspaceMethod string
	WorkspaceActor  string
}

// TaskGitCredentialPolicyResolver resolves the workspace policy without exposing credentials.
type TaskGitCredentialPolicyResolver interface {
	ResolveTaskGitCredentialPolicy(context.Context, string) (TaskGitCredentialPolicy, error)
}

// SetGitHubCredentialBroker configures renewable workspace automation credentials.
// brokerURL is the full credential-resolution endpoint URL.
func (e *Executor) SetGitHubCredentialBroker(issuer GitCredentialLeaseIssuer, brokerURL string) {
	e.gitCredentialIssuer = issuer
	e.gitCredentialBrokerURL = strings.TrimSpace(brokerURL)
}

// SetAgentctlBinaryPath configures the launcher-owned helper executable used
// by host-side Local and Worktree preparation before agentctl startup.
func (e *Executor) SetAgentctlBinaryPath(path string) {
	e.agentctlBinaryPath = strings.TrimSpace(path)
}

// SetTaskGitCredentialPolicyResolver configures workspace-specific task Git routing.
func (e *Executor) SetTaskGitCredentialPolicyResolver(resolver TaskGitCredentialPolicyResolver) {
	e.githubCredentialPolicyResolver = resolver
}

func (e *Executor) applyGitCredentialSnapshot(
	ctx context.Context,
	req *LaunchAgentRequest,
	session *models.TaskSession,
) error {
	if req == nil || session == nil {
		return nil
	}
	policy := TaskGitCredentialPolicy{Mode: taskGitCredentialsModeManaged}
	if e.githubCredentialPolicyResolver != nil {
		resolved, err := e.githubCredentialPolicyResolver.ResolveTaskGitCredentialPolicy(ctx, req.WorkspaceID)
		if err != nil {
			if req.Env[githubauth.CredentialBrokerURLEnv] == "" {
				return fmt.Errorf("resolve task Git credential policy: %w", err)
			}
			// A plugin-only repository does not require a GitHub workspace
			// policy. Its opaque lease has already been scoped and issued.
			// Keep the non-secret snapshot rather than making that launch
			// depend on an unrelated GitHub configuration.
		} else {
			policy = resolved
		}
	}

	snapshot := models.GitCredentialSnapshot{
		Version:      1,
		Policy:       policy.Mode,
		ExecutorType: req.ExecutorType,
		CapturedAt:   time.Now().UTC(),
	}
	switch {
	case req.Env[envGitHubToken] != "" || req.Env[envGHToken] != "":
		snapshot.Source = "executor_profile"
		snapshot.Transport = "profile_token"
	case policy.Mode == taskGitCredentialsModeExecutor:
		snapshot.Source = "executor"
		snapshot.Transport = "executor_selected"
	default:
		if req.Env[githubauth.CredentialBrokerURLEnv] == "" {
			return nil
		}
		snapshot.Source = "workspace"
		snapshot.WorkspaceMethod = policy.WorkspaceMethod
		snapshot.Actor = policy.WorkspaceActor
		if snapshot.Actor == "" {
			snapshot.Actor = "runtime_selected"
		}
		snapshot.Transport = "managed_https"
	}
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata[models.SessionMetaKeyGitCredentialSnapshot] = snapshot
	return nil
}

func (e *Executor) configureGitHubCredentialBroker(
	ctx context.Context,
	req *LaunchAgentRequest,
	info *repoInfo,
) error {
	return e.configureGitCredentialBrokerForRepositories(ctx, req, []*repoInfo{info})
}

func (e *Executor) configureGitHubCredentialBrokerForRepositories(
	ctx context.Context,
	req *LaunchAgentRequest,
	infos []*repoInfo,
) error {
	return e.configureGitCredentialBrokerForRepositories(ctx, req, infos)
}

func (e *Executor) configureGitCredentialBrokerForRepositories(
	ctx context.Context,
	req *LaunchAgentRequest,
	infos []*repoInfo,
) error {
	if e.gitCredentialIssuer == nil || len(infos) == 0 {
		return nil
	}
	githubManaged, err := e.resolveManagedGitHubCredentials(ctx, req, infos)
	if err != nil {
		return err
	}
	if req.Env == nil {
		req.Env = make(map[string]string)
	}
	scopes, helpers, err := e.issueGitCredentialScopes(ctx, req, infos, githubManaged)
	if err != nil {
		return err
	}
	if len(scopes) == 0 {
		return nil
	}
	return e.configureGitCredentialEnvironment(req, scopes, helpers)
}

func (e *Executor) resolveManagedGitHubCredentials(
	ctx context.Context,
	req *LaunchAgentRequest,
	infos []*repoInfo,
) (bool, error) {
	if e.githubCredentialPolicyResolver == nil || !includesGitHubRepository(infos) {
		return true, nil
	}
	policy, err := e.githubCredentialPolicyResolver.ResolveTaskGitCredentialPolicy(ctx, req.WorkspaceID)
	if err != nil {
		return false, fmt.Errorf("resolve task Git credential policy: %w", err)
	}
	if policy.Mode != taskGitCredentialsModeExecutor {
		return true, nil
	}
	if err := removeManagedGitCredentials(req); err != nil {
		return false, err
	}
	return false, nil
}

func (e *Executor) issueGitCredentialScopes(
	ctx context.Context,
	req *LaunchAgentRequest,
	infos []*repoInfo,
	githubManaged bool,
) ([]githubCredentialScope, map[string]struct{}, error) {
	scopes := make([]githubCredentialScope, 0, len(infos)*2)
	helpers := make(map[string]struct{}, len(infos))
	for _, info := range infos {
		scope, err := e.issueGitCredentialScope(ctx, req, info, githubManaged)
		if err != nil {
			return nil, nil, err
		}
		if scope != nil {
			scopes = append(scopes, *scope)
			helpers[scope.Host] = struct{}{}
		}
		contributionScope, err := e.issueGitHubContributionCredentialScope(ctx, req, info)
		if err != nil {
			return nil, nil, err
		}
		if contributionScope != nil {
			scopes = append(scopes, *contributionScope)
			helpers[contributionScope.Host] = struct{}{}
		}
	}
	return scopes, helpers, nil
}

func (e *Executor) configureGitCredentialEnvironment(
	req *LaunchAgentRequest,
	scopes []githubCredentialScope,
	helpers map[string]struct{},
) error {
	encodedScopes, err := json.Marshal(scopes)
	if err != nil {
		return fmt.Errorf("encode GitHub credential scopes: %w", err)
	}
	primary := scopes[0]
	req.Env[githubauth.CredentialBrokerURLEnv] = e.gitCredentialBrokerURL
	req.Env[githubauth.CredentialLeaseEnv] = primary.Lease
	req.Env[githubauth.CredentialTaskIDEnv] = primary.TaskID
	req.Env[githubauth.CredentialSessionIDEnv] = primary.SessionID
	req.Env[githubauth.CredentialRepositoryEnv] = primary.RepositoryID
	req.Env[githubauth.CredentialOwnerEnv] = primary.Owner
	req.Env[githubauth.CredentialRepoEnv] = primary.Repo
	req.Env[githubauth.CredentialHostEnv] = primary.Host
	req.Env[githubauth.CredentialScopesEnv] = string(encodedScopes)
	req.Env["GIT_TERMINAL_PROMPT"] = "0"
	switch models.ExecutorType(req.ExecutorType) {
	case "", models.ExecutorTypeLocal, models.ExecutorTypeWorktree:
		if e.agentctlBinaryPath != "" {
			req.Env[githubauth.CredentialHelperPathEnv] = e.agentctlBinaryPath
		}
	}
	for host := range helpers {
		appendGitConfig(req.Env, "credential.https://"+host+".helper", "")
		appendGitConfig(req.Env, "credential.https://"+host+".helper", gitCredentialHelper)
	}
	appendGitConfig(req.Env, "credential.useHttpPath", "true")
	return nil
}

func includesGitHubRepository(infos []*repoInfo) bool {
	for _, info := range infos {
		if info != nil && info.Repository != nil && strings.EqualFold(info.Repository.Provider, gitHubProviderID) {
			return true
		}
	}
	return false
}

func removeManagedGitCredentials(req *LaunchAgentRequest) error {
	if req == nil || req.Env == nil {
		return nil
	}
	for _, key := range []string{
		githubauth.CredentialBrokerURLEnv,
		githubauth.CredentialHelperPathEnv,
		githubauth.CredentialLeaseEnv,
		githubauth.CredentialTaskIDEnv,
		githubauth.CredentialSessionIDEnv,
		githubauth.CredentialRepositoryEnv,
		githubauth.CredentialOwnerEnv,
		githubauth.CredentialRepoEnv,
		githubauth.CredentialHostEnv,
		githubauth.CredentialScopesEnv,
	} {
		delete(req.Env, key)
	}
	entries, err := gitconfigenv.Filter(req.Env, func(index int, entries []gitconfigenv.Entry) bool {
		entry := entries[index]
		if isHTTPSCredentialHelperKey(entry.Key) && isManagedGitHubCredentialHelper(entry.Value) {
			return false
		}
		return !isHTTPSCredentialHelperKey(entry.Key) || entry.Value != "" ||
			index+1 >= len(entries) || entries[index+1].Key != entry.Key ||
			!isManagedGitHubCredentialHelper(entries[index+1].Value)
	})
	if err != nil {
		return fmt.Errorf("remove managed GitHub credential helper: %w", err)
	}
	req.Env = entries
	return nil
}

func isHTTPSCredentialHelperKey(key string) bool {
	normalized := strings.ToLower(key)
	return strings.HasPrefix(normalized, "credential.https://") && strings.HasSuffix(normalized, ".helper")
}

func isManagedGitHubCredentialHelper(value string) bool {
	return value == gitCredentialHelper || value == legacyShimGitCredentialHelper ||
		value == legacyGitHubCredentialHelper
}

func (e *Executor) issueGitCredentialScope(
	ctx context.Context,
	req *LaunchAgentRequest,
	info *repoInfo,
	githubManaged bool,
) (*githubCredentialScope, error) {
	if info == nil || info.Repository == nil {
		return nil, nil
	}
	repository := info.Repository
	providerID := managedGitCredentialProvider(repository, githubManaged, req.Env)
	if providerID == "" {
		return nil, nil
	}
	host, path, owner, repo, err := gitCredentialCloneIdentity(repository, info.RepositoryID)
	if err != nil {
		return nil, err
	}
	if err := validateGitHubCredentialBrokerURL(e.gitCredentialBrokerURL, req.ExecutorType); err != nil {
		return nil, err
	}
	lease, err := e.gitCredentialIssuer.Issue(ctx, gitcredentials.Scope{
		ProviderID: providerID, WorkspaceID: req.WorkspaceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RepositoryID: info.RepositoryID, Host: host, Path: path,
	})
	if err != nil {
		return nil, fmt.Errorf("issue Git credential lease: %w", err)
	}
	if strings.TrimSpace(lease.Token) == "" {
		return nil, fmt.Errorf("issue Git credential lease: empty lease")
	}
	return &githubCredentialScope{
		Lease: lease.Token, TaskID: req.TaskID, SessionID: req.SessionID,
		RepositoryID: info.RepositoryID, Owner: owner, Repo: repo, Host: host, Path: path,
	}, nil
}

func (e *Executor) issueGitHubContributionCredentialScope(
	ctx context.Context,
	req *LaunchAgentRequest,
	info *repoInfo,
) (*githubCredentialScope, error) {
	if info == nil || info.Repository == nil || info.RemoteContribution == nil {
		return nil, nil
	}
	binding := info.RemoteContribution
	if err := binding.Validate(); err != nil {
		return nil, fmt.Errorf("validate remote contribution credential scope: %w", err)
	}
	if binding.Provider != models.RemoteContributionProviderGitHub {
		return nil, nil
	}
	if !strings.EqualFold(binding.SourceRepository.Host, defaultGitHubHost) {
		return nil, fmt.Errorf("remote contribution source is not a GitHub repository")
	}
	parts := strings.Split(binding.SourceRepository.Path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("remote contribution GitHub source identity is invalid")
	}
	targetOwner := strings.TrimSpace(info.Repository.ProviderOwner)
	targetRepo := strings.TrimSpace(info.Repository.ProviderName)
	if strings.EqualFold(parts[0], targetOwner) && strings.EqualFold(parts[1], targetRepo) {
		return nil, nil
	}
	if !binding.CollaborationAllowed {
		return nil, fmt.Errorf("remote contribution does not permit collaboration")
	}
	if err := validateGitHubCredentialBrokerURL(e.gitCredentialBrokerURL, req.ExecutorType); err != nil {
		return nil, err
	}
	path := "/" + parts[0] + "/" + parts[1] + ".git"
	lease, err := e.gitCredentialIssuer.Issue(ctx, gitcredentials.Scope{
		ProviderID: gitHubProviderID, WorkspaceID: req.WorkspaceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RepositoryID: info.RepositoryID, Host: defaultGitHubHost, Path: path,
	})
	if err != nil {
		return nil, fmt.Errorf("issue Git credential lease: %w", err)
	}
	if strings.TrimSpace(lease.Token) == "" {
		return nil, fmt.Errorf("issue Git credential lease: empty lease")
	}
	return &githubCredentialScope{
		Lease: lease.Token, TaskID: req.TaskID, SessionID: req.SessionID,
		RepositoryID: info.RepositoryID, Owner: parts[0], Repo: parts[1], Host: defaultGitHubHost, Path: path,
	}, nil
}

func managedGitCredentialProvider(repository *models.Repository, githubManaged bool, env map[string]string) string {
	if repository.SourceType == sourceTypeLocal && strings.TrimSpace(repository.Provider) == "" {
		return ""
	}
	providerID := strings.ToLower(strings.TrimSpace(repository.Provider))
	if providerID == "" {
		providerID = gitHubProviderID
	}
	// Native GitLab and Azure DevOps credentials keep their existing executor
	// paths. The generic broker is reserved for managed GitHub credentials and
	// manifest-owned plugin providers.
	if providerID == gitLabProviderID || providerID == providerAzureDevOps {
		return ""
	}
	if providerID == gitHubProviderID && (!githubManaged || env[envGitHubToken] != "" || env[envGHToken] != "") {
		return ""
	}
	return providerID
}

func gitCredentialCloneIdentity(repository *models.Repository, repositoryID string) (string, string, string, string, error) {
	cloneURL := repositoryCloneURL(repository)
	if cloneURL == "" {
		return "", "", "", "", fmt.Errorf("repository %q has no HTTPS clone URL for managed credentials", repositoryID)
	}
	parsed, err := url.Parse(cloneURL)
	if err != nil || parsed.Scheme != httpsScheme || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.Path == "" {
		return "", "", "", "", fmt.Errorf("repository %q must use an HTTPS clone URL for managed credentials", repositoryID)
	}
	providerHost := strings.TrimSpace(repository.ProviderHost)
	providerID := strings.ToLower(strings.TrimSpace(repository.Provider))
	if providerHost == "" && (providerID == "" || providerID == gitHubProviderID) {
		providerHost = "https://" + defaultGitHubHost
	}
	if err := repoclone.ValidateHTTPSCloneOrigin(cloneURL, providerHost); err != nil {
		return "", "", "", "", fmt.Errorf("repository %q provider origin: %w", repositoryID, err)
	}
	owner, repo, err := credentialHelperPath(parsed.Path)
	if err != nil || repositoryID == "" {
		return "", "", "", "", fmt.Errorf("repository %q has an invalid HTTPS clone path", repositoryID)
	}
	return parsed.Host, parsed.Path, owner, repo, nil
}

func credentialHelperPath(path string) (string, string, error) {
	trimmed := strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	owner, repo, found := strings.Cut(trimmed, "/")
	if !found || owner == "" || repo == "" {
		return "", "", errors.New("repository path needs owner and name")
	}
	return owner, repo, nil
}

func validateGitHubCredentialBrokerURL(raw, executorType string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%w: absolute endpoint is required", ErrGitHubCredentialBrokerURL)
	}
	if parsed.Scheme == httpsScheme {
		return nil
	}
	if parsed.Scheme != "http" || executorNeedsResolvedCredentials(executorType) || !isLoopbackHost(parsed.Hostname()) {
		return fmt.Errorf("%w: HTTPS is required for non-local executors", ErrGitHubCredentialBrokerURL)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func appendGitConfig(env map[string]string, key, value string) {
	count, _ := strconv.Atoi(env["GIT_CONFIG_COUNT"])
	env[fmt.Sprintf("GIT_CONFIG_KEY_%d", count)] = key
	env[fmt.Sprintf("GIT_CONFIG_VALUE_%d", count)] = value
	env["GIT_CONFIG_COUNT"] = strconv.Itoa(count + 1)
}

// injectGitLabWorkspaceCredentials overrides inherited GitLab credentials
// with the current workspace's configured connection.
func (e *Executor) injectGitLabWorkspaceCredentials(ctx context.Context, req *LaunchAgentRequest) {
	if req.Env == nil {
		req.Env = make(map[string]string)
	}
	req.Env[envGitLabToken] = ""
	req.Env[envGitLabHost] = ""
	req.Env[envKandevGitLabHost] = ""
	removeGitLabCredentialHelpers(req.Env)
	if e.gitlabCredentials == nil || req.WorkspaceID == "" {
		return
	}
	host, token, err := e.gitlabCredentials.ResolveGitLabExecutionCredentials(ctx, req.WorkspaceID)
	if err != nil || strings.TrimSpace(host) == "" {
		e.logger.Debug("GitLab execution credentials unavailable for workspace")
		return
	}
	req.Env[envGitLabToken] = strings.TrimSpace(token)
	req.Env[envGitLabHost] = strings.TrimSpace(host)
	req.Env[envKandevGitLabHost] = strings.TrimSpace(host)
	appendGitLabCredentialHelper(req.Env, host)
}

func removeGitLabCredentialHelpers(env map[string]string) {
	count, _ := strconv.Atoi(env["GIT_CONFIG_COUNT"])
	type entry struct{ key, value string }
	entries := make([]entry, 0, count)
	for i := 0; i < count; i++ {
		keyName := fmt.Sprintf("GIT_CONFIG_KEY_%d", i)
		valueName := fmt.Sprintf("GIT_CONFIG_VALUE_%d", i)
		key, keyOK := env[keyName]
		value, valueOK := env[valueName]
		delete(env, keyName)
		delete(env, valueName)
		if keyOK && valueOK && (!strings.HasPrefix(strings.ToLower(key), "credential.http") || value != gitLabCredentialHelper) {
			entries = append(entries, entry{key: key, value: value})
		}
	}
	for i, item := range entries {
		env[fmt.Sprintf("GIT_CONFIG_KEY_%d", i)] = item.key
		env[fmt.Sprintf("GIT_CONFIG_VALUE_%d", i)] = item.value
	}
	if len(entries) == 0 {
		delete(env, "GIT_CONFIG_COUNT")
		return
	}
	env["GIT_CONFIG_COUNT"] = strconv.Itoa(len(entries))
}

func appendGitLabCredentialHelper(env map[string]string, host string) {
	origin, err := url.Parse(strings.TrimSpace(host))
	if err != nil || (origin.Scheme != "http" && origin.Scheme != httpsScheme) || origin.Host == "" ||
		origin.User != nil || (origin.Path != "" && origin.Path != "/") {
		return
	}
	origin.Path = ""
	origin.RawPath = ""
	origin.RawQuery = ""
	origin.Fragment = ""

	count, _ := strconv.Atoi(env["GIT_CONFIG_COUNT"])
	for i := 0; i < count; i++ {
		key := env[fmt.Sprintf("GIT_CONFIG_KEY_%d", i)]
		if strings.EqualFold(key, "credential."+origin.String()+".helper") {
			env[fmt.Sprintf("GIT_CONFIG_VALUE_%d", i)] = gitLabCredentialHelper
			return
		}
	}
	env[fmt.Sprintf("GIT_CONFIG_KEY_%d", count)] = "credential." + origin.String() + ".helper"
	env[fmt.Sprintf("GIT_CONFIG_VALUE_%d", count)] = gitLabCredentialHelper
	env["GIT_CONFIG_COUNT"] = strconv.Itoa(count + 1)
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// resolveRemoteCredentials handles explicit profile-level remote auth secrets.
func (e *Executor) resolveRemoteCredentials(ctx context.Context, req *LaunchAgentRequest, metadata map[string]interface{}) {
	if req.Env == nil {
		req.Env = make(map[string]string)
	}

	e.resolveAuthSecrets(ctx, req, metadata)
}

// resolveAuthSecrets reads remote_auth_secrets from metadata and resolves secret values
// into environment variables (e.g., gh_cli_env -> GITHUB_TOKEN).
func (e *Executor) resolveAuthSecrets(ctx context.Context, req *LaunchAgentRequest, metadata map[string]interface{}) {
	authSecretsJSON, _ := metadata[profileKeyRemoteAuthSecrets].(string)
	if authSecretsJSON == "" {
		return
	}

	var authSecrets map[string]string
	if err := json.Unmarshal([]byte(authSecretsJSON), &authSecrets); err != nil {
		e.logger.Debug("failed to parse remote_auth_secrets", zap.Error(err))
		return
	}

	for methodID, secretID := range authSecrets {
		if secretID == "" {
			continue
		}
		// Map method IDs to env var names
		envVar := methodIDToEnvVar(methodID)
		if envVar == "" {
			continue
		}
		// Skip if already set
		if req.Env[envVar] != "" {
			continue
		}
		if e.secretStore == nil {
			continue
		}

		value, err := e.revealGlobalSecret(ctx, secretID)
		if err != nil {
			e.logger.Debug("failed to resolve auth secret",
				zap.String("method_id", methodID),
				zap.Error(err))
			continue
		}
		req.Env[envVar] = value
		// Also set GH_TOKEN for gh CLI compatibility
		if envVar == envGitHubToken {
			req.Env[envGHToken] = value
		}
		e.logger.Debug("resolved remote auth secret", zap.String("env_var", envVar))
	}
}

// methodIDToEnvVar maps remote auth method IDs to environment variable names.
func methodIDToEnvVar(methodID string) string {
	switch methodID {
	case "gh_cli_env":
		return envGitHubToken
	default:
		// For agent-specific methods like "agent:claude_code:env:ANTHROPIC_API_KEY"
		if strings.HasPrefix(methodID, "agent:") && strings.Contains(methodID, ":env:") {
			parts := strings.Split(methodID, ":env:")
			if len(parts) == 2 {
				return parts[1]
			}
		}
		return ""
	}
}
