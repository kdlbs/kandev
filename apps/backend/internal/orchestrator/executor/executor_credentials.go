package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	// envGitHubToken is the environment variable name for GitHub authentication tokens.
	envGitHubToken = "GITHUB_TOKEN"
	// envGHToken is the gh CLI compatible environment variable name.
	envGHToken = "GH_TOKEN"
)

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
	authSecretsJSON, _ := metadata["remote_auth_secrets"].(string)
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

	credsJSON, _ := metadata["remote_credentials"].(string)
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
func detectGHToken() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "auth", "token")
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
