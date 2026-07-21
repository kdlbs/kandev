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

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/githubauth"
)

const (
	// envGitHubToken is the environment variable name for GitHub authentication tokens.
	envGitHubToken = "GITHUB_TOKEN"
	// envGHToken is the gh CLI compatible environment variable name.
	envGHToken          = "GH_TOKEN"
	envGitLabToken      = "GITLAB_TOKEN"
	envGitLabHost       = "GITLAB_HOST"
	envKandevGitLabHost = "KANDEV_GITLAB_HOST"

	gitHubCredentialHelper = "!agentctl git-credential"
	defaultGitHubHost      = "github.com"
	gitLabCredentialHelper = `!f() { echo "username=oauth2"; echo "password=$GITLAB_TOKEN"; }; f`
)

var ErrGitHubCredentialBrokerURL = errors.New("invalid GitHub credential broker URL")

type GitHubCredentialLeaseRequest struct {
	WorkspaceID  string
	TaskID       string
	SessionID    string
	RepositoryID string
	Owner        string
	Repo         string
	Host         string
}

type GitHubCredentialLease struct {
	Token string
}

type githubCredentialScope struct {
	Lease        string `json:"lease"`
	TaskID       string `json:"task_id"`
	SessionID    string `json:"session_id"`
	RepositoryID string `json:"repository_id"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Host         string `json:"host"`
}

type GitHubCredentialLeaseIssuer interface {
	IssueGitHubCredentialLease(context.Context, GitHubCredentialLeaseRequest) (GitHubCredentialLease, error)
}

// SetGitHubCredentialBroker configures renewable workspace automation credentials.
// brokerURL is the full credential-resolution endpoint URL.
func (e *Executor) SetGitHubCredentialBroker(issuer GitHubCredentialLeaseIssuer, brokerURL string) {
	e.githubCredentialIssuer = issuer
	e.githubCredentialBrokerURL = strings.TrimSpace(brokerURL)
}

func (e *Executor) configureGitHubCredentialBroker(
	ctx context.Context,
	req *LaunchAgentRequest,
	info *repoInfo,
) error {
	return e.configureGitHubCredentialBrokerForRepositories(ctx, req, []*repoInfo{info})
}

func (e *Executor) configureGitHubCredentialBrokerForRepositories(
	ctx context.Context,
	req *LaunchAgentRequest,
	infos []*repoInfo,
) error {
	if e.githubCredentialIssuer == nil || len(infos) == 0 {
		return nil
	}
	if req.Env == nil {
		req.Env = make(map[string]string)
	}
	if req.Env[envGitHubToken] != "" || req.Env[envGHToken] != "" {
		return nil
	}
	scopes := make([]githubCredentialScope, 0, len(infos))
	for _, info := range infos {
		scope, err := e.issueGitHubCredentialScope(ctx, req, info)
		if err != nil {
			return err
		}
		if scope != nil {
			scopes = append(scopes, *scope)
		}
	}
	if len(scopes) == 0 {
		return nil
	}
	encodedScopes, err := json.Marshal(scopes)
	if err != nil {
		return fmt.Errorf("encode GitHub credential scopes: %w", err)
	}
	primary := scopes[0]
	req.Env[githubauth.CredentialBrokerURLEnv] = e.githubCredentialBrokerURL
	req.Env[githubauth.CredentialLeaseEnv] = primary.Lease
	req.Env[githubauth.CredentialTaskIDEnv] = primary.TaskID
	req.Env[githubauth.CredentialSessionIDEnv] = primary.SessionID
	req.Env[githubauth.CredentialRepositoryEnv] = primary.RepositoryID
	req.Env[githubauth.CredentialOwnerEnv] = primary.Owner
	req.Env[githubauth.CredentialRepoEnv] = primary.Repo
	req.Env[githubauth.CredentialHostEnv] = primary.Host
	req.Env[githubauth.CredentialScopesEnv] = string(encodedScopes)
	req.Env["GIT_TERMINAL_PROMPT"] = "0"
	appendGitConfig(req.Env, "credential.https://github.com.helper", gitHubCredentialHelper)
	appendGitConfig(req.Env, "credential.useHttpPath", "true")
	return nil
}

func (e *Executor) issueGitHubCredentialScope(
	ctx context.Context,
	req *LaunchAgentRequest,
	info *repoInfo,
) (*githubCredentialScope, error) {
	if info == nil || info.Repository == nil {
		return nil, nil
	}
	repository := info.Repository
	if repository.Provider != "" && !strings.EqualFold(repository.Provider, "github") {
		return nil, nil
	}
	owner := strings.TrimSpace(repository.ProviderOwner)
	repo := strings.TrimSpace(repository.ProviderName)
	if owner == "" || repo == "" || info.RepositoryID == "" {
		return nil, nil
	}
	if err := validateGitHubCredentialBrokerURL(e.githubCredentialBrokerURL, req.ExecutorType); err != nil {
		return nil, err
	}
	lease, err := e.githubCredentialIssuer.IssueGitHubCredentialLease(ctx, GitHubCredentialLeaseRequest{
		WorkspaceID: req.WorkspaceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RepositoryID: info.RepositoryID, Owner: owner, Repo: repo, Host: defaultGitHubHost,
	})
	if err != nil {
		return nil, fmt.Errorf("issue GitHub credential lease: %w", err)
	}
	if strings.TrimSpace(lease.Token) == "" {
		return nil, fmt.Errorf("issue GitHub credential lease: empty lease")
	}
	return &githubCredentialScope{
		Lease: lease.Token, TaskID: req.TaskID, SessionID: req.SessionID,
		RepositoryID: info.RepositoryID, Owner: owner, Repo: repo, Host: defaultGitHubHost,
	}, nil
}

func validateGitHubCredentialBrokerURL(raw, executorType string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%w: absolute endpoint is required", ErrGitHubCredentialBrokerURL)
	}
	if parsed.Scheme == "https" {
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
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" ||
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

		value, err := e.secretStore.Reveal(ctx, secretID)
		if err != nil {
			e.logger.Debug("failed to resolve auth secret",
				zap.String("method_id", methodID),
				zap.String("secret_id", secretID),
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
