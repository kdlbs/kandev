package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/remoteauth"
)

// uploadCredentials reads the remote_credentials metadata and uploads the selected
// credential files to the sprite. Also handles gh_cli_token auto-detect,
// secret-based auth via remote_auth_secrets, and agent auth setup scripts.
func (r *SpritesExecutor) uploadCredentials(
	ctx context.Context,
	sprite *sprites.Sprite,
	req *ExecutorCreateRequest,
	onOutput func(string),
) error {
	catalog := r.buildRemoteAuthCatalog()

	// Handle secret-based auth (e.g., GITHUB_TOKEN from a stored secret)
	r.resolveAuthSecrets(ctx, req, catalog)

	// Run auth setup scripts for env-type methods (e.g., Claude Code credential files)
	r.runAuthSetupScripts(ctx, sprite, req, catalog, onOutput)

	credsJSON, _ := req.Metadata["remote_credentials"].(string)
	if credsJSON == "" {
		return nil
	}

	var selectedMethodIDs []string
	if err := json.Unmarshal([]byte(credsJSON), &selectedMethodIDs); err != nil {
		return fmt.Errorf("failed to parse remote_credentials: %w", err)
	}

	// Handle gh_cli_token: detect locally and inject as env var
	selectedMethodIDs = r.resolveGHToken(selectedMethodIDs, req)

	if len(selectedMethodIDs) == 0 {
		return nil
	}

	fileMethods := make([]remoteauth.Method, 0, len(selectedMethodIDs))
	for _, methodID := range selectedMethodIDs {
		method, ok := catalog.FindMethod(methodID)
		if !ok {
			r.logger.Warn("unknown remote auth method, skipping", zap.String("method_id", methodID))
			continue
		}
		if method.Type != "files" {
			continue
		}
		fileMethods = append(fileMethods, method)
	}
	if len(fileMethods) == 0 {
		return nil
	}

	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	uploader := &spriteFileUploader{sprite: sprite, runtime: r}
	targetHomeDir, err := r.resolveRemoteAuthHomeDir(stepCtx, req, uploader)
	if err != nil {
		return err
	}
	return UploadCredentialFiles(stepCtx, uploader, fileMethods, targetHomeDir, r.logger)
}

// runAuthSetupScripts executes setup scripts for env-type auth methods whose env var
// is present in req.Env. This handles both secret-store-resolved and directly-injected env vars.
// The optional onOutput callback streams output to the caller (e.g., progress UI).
func (r *SpritesExecutor) runAuthSetupScripts(
	ctx context.Context,
	sprite *sprites.Sprite,
	req *ExecutorCreateRequest,
	catalog remoteauth.Catalog,
	onOutput func(string),
) {
	for _, spec := range catalog.Specs {
		for _, method := range spec.Methods {
			if method.Type != "env" || method.SetupScript == "" || method.EnvVar == "" {
				continue
			}
			if req.Env[method.EnvVar] == "" {
				continue
			}

			if onOutput != nil {
				onOutput(fmt.Sprintf("Setting up %s credentials...", spec.DisplayName))
			}

			stepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			cmd := sprite.CommandContext(stepCtx, "sh", "-c", method.SetupScript)
			cmd.Env = r.buildSpriteEnv(req.Env)
			out, err := cmd.CombinedOutput()
			cancel()

			if err != nil {
				r.logger.Warn("auth setup script failed",
					zap.String("method_id", method.MethodID),
					zap.String("output", strings.TrimSpace(string(out))),
					zap.Error(err))
				if onOutput != nil {
					onOutput(fmt.Sprintf("Warning: %s credential setup failed", spec.DisplayName))
				}
			} else {
				r.logger.Debug("auth setup script completed",
					zap.String("method_id", method.MethodID))
			}
		}
	}
}

// resolveGHToken handles the gh_cli_token credential: detects the token locally
// and injects it as GITHUB_TOKEN in the request env. Returns filtered credential IDs.
func (r *SpritesExecutor) resolveGHToken(credentialIDs []string, req *ExecutorCreateRequest) []string {
	if !containsID(credentialIDs, "gh_cli_token") {
		return credentialIDs
	}
	token, err := DetectGHToken()
	if err != nil {
		r.logger.Warn("failed to detect gh token", zap.Error(err))
	} else {
		if req.Env == nil {
			req.Env = make(map[string]string)
		}
		req.Env["GITHUB_TOKEN"] = token
		r.logger.Debug("set GITHUB_TOKEN from local gh auth token")
	}
	return removeID(credentialIDs, "gh_cli_token")
}

// resolveAuthSecrets reads remote_auth_secrets from metadata and resolves secret values
// into environment variables (e.g., gh_cli secret -> GITHUB_TOKEN).
func (r *SpritesExecutor) resolveAuthSecrets(
	ctx context.Context,
	req *ExecutorCreateRequest,
	catalog remoteauth.Catalog,
) {
	authSecretsJSON, _ := req.Metadata["remote_auth_secrets"].(string)
	if authSecretsJSON == "" {
		return
	}
	var authSecrets map[string]string
	if err := json.Unmarshal([]byte(authSecretsJSON), &authSecrets); err != nil {
		r.logger.Warn("failed to parse remote_auth_secrets", zap.Error(err))
		return
	}
	for methodID, secretID := range authSecrets {
		if secretID == "" {
			continue
		}
		method, ok := catalog.FindMethod(methodID)
		if !ok || method.Type != "env" || method.EnvVar == "" {
			continue
		}
		value, err := r.secretStore.Reveal(ctx, secretID)
		if err != nil {
			r.logger.Warn("failed to resolve auth secret",
				zap.String("method_id", methodID),
				zap.String("secret_id", secretID),
				zap.Error(err))
			continue
		}
		if req.Env == nil {
			req.Env = make(map[string]string)
		}
		req.Env[method.EnvVar] = value
		r.logger.Debug("set env from auth secret", zap.String("key", method.EnvVar), zap.String("method_id", methodID))
	}
}

func (r *SpritesExecutor) buildRemoteAuthCatalog() remoteauth.Catalog {
	if r.agentList == nil {
		return remoteauth.BuildCatalog(nil)
	}
	return remoteauth.BuildCatalog(r.agentList.ListEnabled())
}

func (r *SpritesExecutor) resolveRemoteAuthHomeDir(
	ctx context.Context,
	req *ExecutorCreateRequest,
	cmdRunner commandOutputRunner,
) (string, error) {
	if req != nil && req.Metadata != nil {
		if override, ok := req.Metadata[MetadataKeyRemoteAuthHome].(string); ok {
			trimmed := strings.TrimSpace(override)
			if trimmed != "" {
				r.logger.Debug("using remote auth home override", zap.String("home_dir", trimmed))
				return trimmed, nil
			}
		}
	}

	if cmdRunner == nil {
		return "", fmt.Errorf("failed to resolve remote user home for credential upload: command runner unavailable")
	}

	out, err := cmdRunner.RunCommandOutput(ctx, "sh", "-lc", "printf %s \"$HOME\"")
	if err != nil {
		return "", fmt.Errorf("failed to resolve remote user home for credential upload: %w", err)
	}
	home := strings.TrimSpace(string(out))
	if home == "" {
		return "", fmt.Errorf("failed to resolve remote user home for credential upload: empty HOME")
	}
	r.logger.Debug("resolved remote auth home", zap.String("home_dir", home))
	return home, nil
}

func containsID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func removeID(ids []string, target string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != target {
			result = append(result, id)
		}
	}
	return result
}
