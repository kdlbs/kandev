package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/sftp"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/remoteauth"
	"github.com/kandev/kandev/internal/common/logger"
)

// uploadCredentials reads the remote_credentials metadata, runs env-credential
// setup scripts whose env vars are present, and uploads the selected file-type
// credentials to the remote host via SFTP. Mirrors SpritesExecutor.uploadCredentials
// — the credential pipeline is the same shape, the seams (file upload and shell
// runner) are different.
//
// All work is best-effort: a failed setup script logs a warning and falls
// through, a missing local credential file is skipped with a warning. The only
// hard failure is a non-resolvable remote $HOME, because every file path is
// rooted there.
func (r *SSHExecutor) uploadCredentials(
	ctx context.Context,
	client *ssh.Client,
	req *ExecutorCreateRequest,
) error {
	catalog := r.buildRemoteAuthCatalog()
	r.resolveAuthSecrets(ctx, req, catalog)
	r.runAuthSetupScripts(ctx, client, req, catalog)

	credsJSON := getMetadataString(req.Metadata, "remote_credentials")
	if credsJSON == "" {
		return nil
	}

	var selectedMethodIDs []string
	if err := json.Unmarshal([]byte(credsJSON), &selectedMethodIDs); err != nil {
		return fmt.Errorf("failed to parse remote_credentials: %w", err)
	}

	selectedMethodIDs = r.resolveGHToken(selectedMethodIDs, req)

	fileMethods := selectFileMethods(catalog, selectedMethodIDs, r.logger)
	if len(fileMethods) == 0 {
		return nil
	}

	uploader := &sshFileUploader{client: client}
	homeDir, err := r.resolveRemoteAuthHomeDir(ctx, client, req)
	if err != nil {
		return err
	}
	return UploadCredentialFiles(ctx, uploader, fileMethods, homeDir, r.logger)
}

// runAuthSetupScripts executes setup scripts for env-type auth methods whose
// env var is present in req.Env. Mirrors the Sprites flavor; the only
// difference is the script runs via SSH instead of sprite.CommandContext.
func (r *SSHExecutor) runAuthSetupScripts(
	ctx context.Context,
	client *ssh.Client,
	req *ExecutorCreateRequest,
	catalog remoteauth.Catalog,
) {
	for _, spec := range catalog.Specs {
		for _, method := range spec.Methods {
			if method.Type != authMethodTypeEnv || method.SetupScript == "" || method.EnvVar == "" {
				continue
			}
			if req.Env[method.EnvVar] == "" {
				continue
			}
			r.runOneAuthSetupScript(ctx, client, req, spec.DisplayName, method)
		}
	}
}

// runOneAuthSetupScript executes a single env-credential setup script over
// SSH. The agent's env vars are piped to the remote shell via stdin and
// sourced under `set -a` so they're exported into the script's environment
// without ever appearing on the remote shell's argv — the alternative
// (`KEY=val ... bash -lc '...'`) leaks secrets via `ps aux` /
// `/proc/PID/cmdline` for the brief window the setup script runs.
// Best-effort: failures log a warning and return.
func (r *SSHExecutor) runOneAuthSetupScript(
	ctx context.Context,
	client *ssh.Client,
	req *ExecutorCreateRequest,
	displayName string,
	method remoteauth.Method,
) {
	shell := sshShellFromMetadata(req.Metadata)
	envScript := buildSSHEnvInitScript(req.Env)
	// `. /dev/stdin` sources the env lines fed via session.Stdin; `set -a`
	// makes those assignments automatically exported so the user's setup
	// script sees them in env without a per-key `export`. The script body
	// itself runs in the same shell after stdin EOF, which means scripts
	// that need their own stdin are unsupported here — none of the
	// env-type SetupScripts in the catalog (gh_cli_env etc.) consume
	// stdin, so this is fine in practice.
	wrapped := WrapLoginShell(shell, "set -a; . /dev/stdin; set +a\n"+method.SetupScript)
	out, stderr, err := runSSHCommandStdin(ctx, client, wrapped, strings.NewReader(envScript))
	if err != nil {
		r.logger.Warn("auth setup script failed",
			zap.String("display_name", displayName),
			zap.String("method_id", method.MethodID),
			zap.String("stdout", strings.TrimSpace(out)),
			zap.String("stderr", strings.TrimSpace(stderr)),
			zap.Error(err))
		return
	}
	r.logger.Debug("auth setup script completed",
		zap.String("display_name", displayName),
		zap.String("method_id", method.MethodID))
}

// resolveGHToken handles the gh_cli_token credential: detects the token on
// the kandev host and injects it as GITHUB_TOKEN in req.Env. Returns the
// filtered method ID list (gh_cli_token removed). Same shape as Sprites.
func (r *SSHExecutor) resolveGHToken(ids []string, req *ExecutorCreateRequest) []string {
	if !containsID(ids, "gh_cli_token") {
		return ids
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
	return removeID(ids, "gh_cli_token")
}

// resolveAuthSecrets reads remote_auth_secrets from metadata and resolves
// secret values into env vars (e.g. gh_cli secret → GITHUB_TOKEN). Skips
// any methods that aren't env-type.
func (r *SSHExecutor) resolveAuthSecrets(
	ctx context.Context,
	req *ExecutorCreateRequest,
	catalog remoteauth.Catalog,
) {
	authSecretsJSON := getMetadataString(req.Metadata, "remote_auth_secrets")
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
		if !ok || method.Type != authMethodTypeEnv || method.EnvVar == "" {
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

func (r *SSHExecutor) buildRemoteAuthCatalog() remoteauth.Catalog {
	if r.agentList == nil {
		return remoteauth.BuildCatalog(nil)
	}
	return remoteauth.BuildCatalog(r.agentList.ListEnabled())
}

// resolveRemoteAuthHomeDir returns the remote $HOME for credential placement.
// Prefers an explicit override on req.Metadata (lets profiles override per
// deployment), otherwise probes the remote with `printf %s "$HOME"`. We
// can't use the static workdir_root because credentials need to live under
// the user's true home (e.g. ~/.claude/credentials.json) — not under the
// task workdir.
func (r *SSHExecutor) resolveRemoteAuthHomeDir(
	ctx context.Context,
	client *ssh.Client,
	req *ExecutorCreateRequest,
) (string, error) {
	if override := strings.TrimSpace(getMetadataString(req.Metadata, MetadataKeyRemoteAuthHome)); override != "" {
		r.logger.Debug("using remote auth home override", zap.String("home_dir", override))
		return override, nil
	}
	shell := sshShellFromMetadata(req.Metadata)
	out, _, err := runSSHCommand(ctx, client, WrapLoginShell(shell, `printf %s "$HOME"`))
	if err != nil {
		return "", fmt.Errorf("ssh: resolve remote $HOME for credentials: %w", err)
	}
	home := strings.TrimSpace(out)
	if home == "" {
		return "", fmt.Errorf("ssh: remote $HOME resolved to empty string")
	}
	r.logger.Debug("resolved remote auth home", zap.String("home_dir", home))
	return home, nil
}

// selectFileMethods filters a catalog down to file-type methods the user
// actually selected. Unknown method IDs log a warning and are skipped.
func selectFileMethods(
	catalog remoteauth.Catalog,
	selectedIDs []string,
	log *logger.Logger,
) []remoteauth.Method {
	out := make([]remoteauth.Method, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		method, ok := catalog.FindMethod(id)
		if !ok {
			log.Warn("unknown remote auth method, skipping", zap.String("method_id", id))
			continue
		}
		if method.Type != authMethodTypeFiles {
			continue
		}
		out = append(out, method)
	}
	return out
}

// buildSSHEnvInitScript returns a multi-line shell snippet of
// `KEY='value'` assignments, one per line. Designed to be piped to a
// remote shell via stdin and sourced under `set -a` — assignments stay
// out of the shell's argv (and therefore out of `ps aux`) but still get
// exported into the script's environment. Empty input returns "".
func buildSSHEnvInitScript(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	var b strings.Builder
	for k, v := range env {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(shellQuote(v))
		b.WriteString("\n")
	}
	return b.String()
}

// sshFileUploader implements FileUploader by writing files via SFTP. Each
// WriteFile opens a fresh SFTP session to keep failure scopes small — the
// credential upload runs at launch time and only writes a handful of small
// files.
type sshFileUploader struct {
	client *ssh.Client
}

func (u *sshFileUploader) WriteFile(_ context.Context, path string, data []byte, mode os.FileMode) error {
	c, err := sftp.NewClient(u.client)
	if err != nil {
		return fmt.Errorf("sftp: new client: %w", err)
	}
	defer func() { _ = c.Close() }()

	// Ensure the parent directory exists. SFTP doesn't have a `mkdir -p`
	// equivalent, so walk the path and create each segment.
	if err := sshMkdirAll(c, parentDir(path)); err != nil {
		return fmt.Errorf("sftp: mkdir for %s: %w", path, err)
	}

	f, err := c.Create(path)
	if err != nil {
		return fmt.Errorf("sftp: create %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("sftp: write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("sftp: close %s: %w", path, err)
	}
	if err := c.Chmod(path, mode); err != nil {
		return fmt.Errorf("sftp: chmod %s: %w", path, err)
	}
	return nil
}

// sshMkdirAll mimics `mkdir -p` over SFTP. Walks every prefix of dir and
// creates segments that don't exist; treats "already exists" as success.
func sshMkdirAll(c *sftp.Client, dir string) error {
	if dir == "" || dir == "/" {
		return nil
	}
	if _, err := c.Stat(dir); err == nil {
		return nil
	}
	if err := sshMkdirAll(c, parentDir(dir)); err != nil {
		return err
	}
	// MkdirAll on sftp.Client doesn't exist; Mkdir errors with "already
	// exists" if a sibling already created it. Treat that as success.
	if err := c.Mkdir(dir); err != nil {
		if _, statErr := c.Stat(dir); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

// parentDir returns the parent directory of path, with no trailing slash.
// Returns "" for paths with no parent (bare names or root).
func parentDir(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return ""
	}
	return path[:idx]
}
