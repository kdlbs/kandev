package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/subproc"
)

const (
	// envGitHubToken is the environment variable name for GitHub authentication tokens.
	envGitHubToken = "GITHUB_TOKEN"
	// envGHToken is the gh CLI compatible environment variable name.
	envGHToken             = "GH_TOKEN"
	envGitLabToken         = "GITLAB_TOKEN"
	envGitLabHost          = "GITLAB_HOST"
	envKandevGitLabHost    = "KANDEV_GITLAB_HOST"
	gitLabCredentialHelper = `!f() { echo "username=oauth2"; echo "password=$GITLAB_TOKEN"; }; f`
)

// injectGitLabWorkspaceCredentials overrides any inherited/profile GitLab
// credential with the current workspace's configured connection. Empty
// values are deliberate: standalone executions otherwise inherit the backend
// process environment and could receive another workspace's fallback token.
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
	} else {
		env["GIT_CONFIG_COUNT"] = strconv.Itoa(len(entries))
	}
}

func appendGitLabCredentialHelper(env map[string]string, host string) {
	origin, err := url.Parse(strings.TrimSpace(host))
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil || (origin.Path != "" && origin.Path != "/") {
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

// injectGitHubToken injects GITHUB_TOKEN and GH_TOKEN into the request env for remote executors.
// It looks for a GITHUB_TOKEN secret in the secrets store and injects it if found.
// Both env vars are set for compatibility: GITHUB_TOKEN for git, GH_TOKEN for gh CLI.
func (e *Executor) injectGitHubToken(ctx context.Context, req *LaunchAgentRequest) {
	if req.Env == nil {
		req.Env = make(map[string]string)
	}

	// Skip if already set (user configured explicitly in profile)
	if req.Env[envGitHubToken] != "" || req.Env[envGHToken] != "" {
		return
	}

	if e.secretStore == nil {
		return
	}

	// Look for a GITHUB_TOKEN secret in the secrets store
	items, err := e.secretStore.List(ctx)
	if err != nil {
		e.logger.Debug("failed to list secrets for GitHub token injection", zap.Error(err))
		return
	}

	var tokenSecretID string
	for _, item := range items {
		if !item.HasValue {
			continue
		}
		if item.Name == envGitHubToken || item.Name == "github_token" {
			tokenSecretID = item.ID
			break
		}
	}
	if tokenSecretID == "" {
		return
	}

	token, err := e.secretStore.Reveal(ctx, tokenSecretID)
	if err != nil || token == "" {
		e.logger.Debug("failed to reveal GitHub token secret", zap.Error(err))
		return
	}

	// Inject both env vars for compatibility
	req.Env[envGitHubToken] = token
	req.Env[envGHToken] = token
	e.logger.Debug("injected GitHub token for remote executor")
}

// injectGitHubTokenFromCLI is the final fallback that extracts a token from
// the local gh CLI if no other credential source provided a GitHub token.
// This enables "just works" behavior when the user has gh authenticated locally.
func (e *Executor) injectGitHubTokenFromCLI(_ context.Context, req *LaunchAgentRequest) {
	if req.Env == nil {
		req.Env = make(map[string]string)
	}

	// Skip if already set by any previous method
	if req.Env[envGitHubToken] != "" || req.Env[envGHToken] != "" {
		return
	}

	// Try to extract token from local gh CLI
	token, err := detectGHToken()
	if err != nil {
		e.logger.Debug("no GitHub token from local gh CLI", zap.Error(err))
		return
	}

	req.Env[envGitHubToken] = token
	req.Env[envGHToken] = token
	e.logger.Debug("injected GitHub token from local gh CLI")
}

// resolveRemoteCredentials handles profile-level remote auth credentials.
// It resolves remote_auth_secrets (secret-based env vars like gh_cli_env)
// and remote_credentials (including gh_cli_token which extracts from local gh CLI).
func (e *Executor) resolveRemoteCredentials(ctx context.Context, req *LaunchAgentRequest, metadata map[string]interface{}) {
	if req.Env == nil {
		req.Env = make(map[string]string)
	}

	// 1. Resolve remote_auth_secrets (e.g., gh_cli_env method with a secret ID)
	e.resolveAuthSecrets(ctx, req, metadata)

	// 2. Handle gh_cli_token from remote_credentials (extract from local gh CLI)
	e.resolveGHCLIToken(req, metadata)
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

// resolveGHCLIToken handles gh_cli_token from remote_credentials metadata.
// If selected, it extracts the token from the local gh CLI via `gh auth token`.
func (e *Executor) resolveGHCLIToken(req *LaunchAgentRequest, metadata map[string]interface{}) {
	// Skip if either GITHUB_TOKEN or GH_TOKEN is already set (respect profile env vars priority)
	if req.Env[envGitHubToken] != "" || req.Env[envGHToken] != "" {
		return
	}

	credsJSON, _ := metadata[profileKeyRemoteCredentials].(string)
	if credsJSON == "" {
		return
	}

	var credentialIDs []string
	if err := json.Unmarshal([]byte(credsJSON), &credentialIDs); err != nil {
		e.logger.Debug("failed to parse remote_credentials", zap.Error(err))
		return
	}

	// Check if gh_cli_token is selected
	hasGHToken := false
	for _, id := range credentialIDs {
		if id == "gh_cli_token" {
			hasGHToken = true
			break
		}
	}
	if !hasGHToken {
		return
	}

	// Extract token from local gh CLI
	token, err := detectGHToken()
	if err != nil {
		e.logger.Debug("failed to detect gh token from local CLI", zap.Error(err))
		return
	}

	req.Env[envGitHubToken] = token
	req.Env[envGHToken] = token
	e.logger.Debug("resolved GITHUB_TOKEN from local gh CLI")
}

// detectGHToken runs `gh auth token` to extract the GitHub token from the local gh CLI.
//
// Throttle Acquire (30s budget) runs first; the 5s exec ctx is constructed
// AFTER the slot is held so its deadline starts from when gh actually
// begins running. Building it earlier would let throttle queue time eat
// into the exec budget and silently skip GITHUB_TOKEN injection.
func detectGHToken() (string, error) {
	acquireCtx, cancelAcquire := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelAcquire()
	release, err := subproc.GH().Acquire(acquireCtx)
	if err != nil {
		return "", fmt.Errorf("gh throttle acquire: %w", err)
	}
	defer release()
	execCtx, cancelExec := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelExec()

	cmd := exec.CommandContext(execCtx, "gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("gh auth token returned empty")
	}
	return token, nil
}
