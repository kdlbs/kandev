package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	sprites "github.com/superfly/sprites-go"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/remoteauth"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/scriptengine"
	"github.com/kandev/kandev/internal/secrets"
)

type RemoteAuthAgentLister interface {
	ListEnabled() []agents.Agent
}

// spriteFileUploader implements FileUploader for a Sprites instance.
type spriteFileUploader struct {
	sprite  *sprites.Sprite
	runtime *SpritesExecutor
}

func (u *spriteFileUploader) WriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error {
	if u.runtime == nil {
		return u.sprite.Filesystem().WriteFileContext(ctx, path, data, mode)
	}
	return u.runtime.writeFileWithRetry(ctx, u.sprite, path, data, mode)
}

func (u *spriteFileUploader) RunCommand(ctx context.Context, name string, args ...string) error {
	_, err := u.sprite.CommandContext(ctx, name, args...).Output()
	return err
}

func (u *spriteFileUploader) RunCommandOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return u.sprite.CommandContext(ctx, name, args...).Output()
}

type commandOutputRunner interface {
	RunCommandOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

const (
	spritesAgentctlPort    = 8765
	spritesWorkspacePath   = "/workspace"
	spritesNamePrefix      = "kandev-"
	spriteStepTimeout      = 120 * time.Second
	spriteHealthTimeout    = 15 * time.Second
	spriteDestroyTimeout   = 30 * time.Second
	spriteHealthRetryWait  = 500 * time.Millisecond
	spritesTotalSteps      = 7 // create, upload, credentials, prepare script, health, create instance, network policy
	spriteOutputMaxLines   = 20
	spriteUploadMaxRetries = 3
)

var uploadHTTPStatusRE = regexp.MustCompile(`(?i)\b(?:http|status)\s*:?\s*(\d{3})\b`)

// SpritesProxySession tracks an active port-forwarding session to a sprite.
type SpritesProxySession struct {
	spriteName   string
	localPort    int
	proxySession *sprites.ProxySession
	cancel       context.CancelFunc
}

// SpritesExecutor implements ExecutorBackend for Sprites.dev remote sandboxes.
type SpritesExecutor struct {
	secretStore      secrets.SecretStore
	agentList        RemoteAuthAgentLister
	agentctlResolver *AgentctlResolver
	logger           *logger.Logger
	agentctlPort     int
	mu               sync.RWMutex
	proxies          map[string]*SpritesProxySession
	tokens           map[string]string // cached API tokens per instance
}

// NewSpritesExecutor creates a new Sprites runtime.
func NewSpritesExecutor(
	secretStore secrets.SecretStore,
	agentList RemoteAuthAgentLister,
	resolver *AgentctlResolver,
	agentctlPort int,
	log *logger.Logger,
) *SpritesExecutor {
	return &SpritesExecutor{
		secretStore:      secretStore,
		agentList:        agentList,
		agentctlResolver: resolver,
		logger:           log.WithFields(zap.String("runtime", "sprites")),
		agentctlPort:     agentctlPort,
		proxies:          make(map[string]*SpritesProxySession),
		tokens:           make(map[string]string),
	}
}

func (r *SpritesExecutor) Name() executor.Name {
	return executor.NameSprites
}

func (r *SpritesExecutor) HealthCheck(_ context.Context) error {
	return nil
}

func (r *SpritesExecutor) ResumeRemoteInstance(_ context.Context, req *ExecutorCreateRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	return nil
}

func (r *SpritesExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	token := req.Env["SPRITES_API_TOKEN"]
	if token == "" {
		return nil, fmt.Errorf("SPRITES_API_TOKEN not set in execution environment (configure it in the executor profile)")
	}

	r.mu.Lock()
	r.tokens[req.InstanceID] = token
	r.mu.Unlock()

	spriteName := spritesNamePrefix + req.InstanceID[:12]
	client := sprites.New(token, sprites.WithDisableControl())
	reconnectRequired := req.PreviousExecutionID != ""
	if reconnectRequired {
		suffix := req.PreviousExecutionID
		if len(suffix) > 12 {
			suffix = suffix[:12]
		}
		spriteName = spritesNamePrefix + suffix
	}

	r.logger.Info("creating sprite instance",
		zap.String("instance_id", req.InstanceID),
		zap.String("sprite_name", spriteName),
		zap.Bool("reconnect_required", reconnectRequired))

	report := newStepReporter(req.OnProgress)
	var spriteInfo *sprites.Sprite
	destroyOnFailure := !reconnectRequired

	// Step 0: Create sprite
	stepName := "Creating cloud sandbox"
	if reconnectRequired {
		stepName = "Reconnecting cloud sandbox"
	}
	step := beginStep(stepName)
	report(step, 0)
	var sprite *sprites.Sprite
	var err error
	if reconnectRequired {
		sprite, err = r.reconnectSprite(ctx, client, spriteName)
		if err != nil {
			completeStepError(&step, err.Error())
			report(step, 0)
			return nil, err
		}
	} else {
		sprite, err = r.createSprite(ctx, client, spriteName)
		if err != nil {
			completeStepError(&step, err.Error())
			report(step, 0)
			r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
			return nil, err
		}
	}
	spriteInfo = sprite
	completeStepSuccess(&step)
	report(step, 0)

	// Step 1: Upload agentctl binary
	step = beginStep("Uploading agent controller")
	report(step, 1)
	if reconnectRequired {
		completeStepSkipped(&step)
	} else {
		if err := r.uploadAgentctl(ctx, sprite); err != nil {
			completeStepError(&step, err.Error())
			report(step, 1)
			r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
			return nil, err
		}
		completeStepSuccess(&step)
	}
	report(step, 1)

	// Step 2: Upload remote credentials (SSH keys, gh CLI, agent auth)
	step = beginStep("Uploading credentials")
	report(step, 2)
	if reconnectRequired {
		completeStepSkipped(&step)
	} else {
		if err := r.uploadCredentials(ctx, sprite, req, func(output string) {
			step.Output = output
			report(step, 2)
		}); err != nil {
			r.logger.Warn("failed to upload credentials (non-fatal)", zap.Error(err))
			completeStepSkipped(&step)
		} else {
			completeStepSuccess(&step)
		}
	}
	report(step, 2)

	// Step 3: Run prepare script (with output streaming)
	step = beginStep("Running prepare script")
	report(step, 3)
	if reconnectRequired {
		completeStepSkipped(&step)
		report(step, 3)
	} else {
		err = r.runPrepareScript(ctx, sprite, req, func(output string) {
			step.Output = output
			report(step, 3)
		})
		if err != nil {
			completeStepError(&step, err.Error())
			report(step, 3)
			r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
			return nil, err
		}
		completeStepSuccess(&step)
		report(step, 3)
	}

	// Step 4: Wait for agentctl health
	step = beginStep("Waiting for agent controller")
	report(step, 4)
	if err := r.waitForHealth(ctx, sprite); err != nil {
		completeStepError(&step, err.Error())
		report(step, 4)
		r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
		return nil, err
	}
	completeStepSuccess(&step)
	report(step, 4)

	// Step 5: Create agent instance (control server → per-instance server)
	step = beginStep("Creating agent instance")
	report(step, 5)
	instancePort, err := r.createAgentInstance(ctx, sprite, req)
	if err != nil {
		completeStepError(&step, err.Error())
		report(step, 5)
		r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
		return nil, err
	}
	completeStepSuccess(&step)
	report(step, 5)

	// Step 6: Apply network policy
	step = beginStep("Applying network policy")
	report(step, 6)
	if err := r.applyNetworkPolicy(ctx, client, spriteName, req); err != nil {
		r.logger.Warn("failed to apply network policy from profile", zap.Error(err))
		completeStepSkipped(&step)
	} else {
		completeStepSuccess(&step)
	}
	report(step, 6)

	// Port forwarding to the per-instance server (not the control server)
	localPort, err := r.setupPortForwarding(ctx, sprite, spriteName, req.InstanceID, instancePort)
	if err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID, destroyOnFailure)
		return nil, err
	}

	agentctlClient := agentctl.NewClient("127.0.0.1", localPort, r.logger,
		agentctl.WithExecutionID(req.InstanceID),
		agentctl.WithSessionID(req.SessionID))

	r.logger.Info("sprite instance ready",
		zap.String("instance_id", req.InstanceID),
		zap.String("sprite_name", spriteName),
		zap.Int("local_port", localPort),
		zap.Int("instance_port", instancePort))

	return &ExecutorInstance{
		InstanceID:    req.InstanceID,
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		RuntimeName:   string(r.Name()),
		Client:        agentctlClient,
		WorkspacePath: spritesWorkspacePath,
		Metadata: map[string]interface{}{
			"sprite_name":       spriteName,
			"sprite_state":      strings.TrimSpace(spriteInfo.Status),
			"sprite_created_at": spriteInfo.CreatedAt,
			"local_port":        localPort,
			MetadataKeyIsRemote: true,
		},
	}, nil
}

func (r *SpritesExecutor) reconnectSprite(ctx context.Context, client *sprites.Client, name string) (*sprites.Sprite, error) {
	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()
	sprite, err := client.GetSprite(stepCtx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to reconnect sprite %q: %w", name, err)
	}
	return sprite, nil
}

func (r *SpritesExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, _ bool) error {
	spriteName := getMetadataString(instance.Metadata, "sprite_name")
	if spriteName == "" {
		return nil
	}

	r.mu.Lock()
	if proxy, ok := r.proxies[instance.InstanceID]; ok {
		r.closeProxySession(proxy)
		delete(r.proxies, instance.InstanceID)
	}
	r.mu.Unlock()

	r.mu.RLock()
	token := r.tokens[instance.InstanceID]
	r.mu.RUnlock()
	if token == "" {
		r.logger.Warn("no cached API token for sprite instance, cannot destroy",
			zap.String("instance_id", instance.InstanceID))
		return nil
	}
	client := sprites.New(token)
	sprite := client.Sprite(spriteName)
	r.runTerminalCleanupScript(ctx, sprite, instance)
	if err := sprite.Destroy(); err != nil {
		r.logger.Warn("failed to destroy sprite",
			zap.String("sprite_name", spriteName),
			zap.Error(err))
		return fmt.Errorf("failed to destroy sprite: %w", err)
	}

	r.mu.Lock()
	delete(r.tokens, instance.InstanceID)
	r.mu.Unlock()

	r.logger.Info("sprite destroyed", zap.String("sprite_name", spriteName))
	return nil
}

func (r *SpritesExecutor) runTerminalCleanupScript(ctx context.Context, sprite *sprites.Sprite, instance *ExecutorInstance) {
	if sprite == nil || instance == nil {
		return
	}
	if !shouldRunExecutorCleanup(instance.StopReason) {
		return
	}
	script := strings.TrimSpace(getMetadataString(instance.Metadata, MetadataKeyCleanupScript))
	if script == "" {
		return
	}

	resolver := scriptengine.NewResolver().
		WithProvider(scriptengine.WorkspaceProvider(spritesWorkspacePath)).
		WithProvider(scriptengine.AgentctlProvider(r.agentctlPort, spritesWorkspacePath)).
		WithProvider(scriptengine.GitIdentityProvider(instance.Metadata)).
		WithProvider(scriptengine.RepositoryProvider(
			instance.Metadata,
			nil,
			getGitRemoteURL,
			r.injectTokenIntoURL,
		))
	resolved := resolver.Resolve(script)
	if strings.TrimSpace(resolved) == "" {
		return
	}

	stepCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	out, err := sprite.CommandContext(stepCtx, "sh", "-c", resolved).CombinedOutput()
	if err != nil {
		r.logger.Warn("cleanup script failed in sprite",
			zap.String("instance_id", instance.InstanceID),
			zap.String("reason", instance.StopReason),
			zap.String("output", strings.TrimSpace(lastLines(string(out), spriteOutputMaxLines))),
			zap.Error(err))
		return
	}
	r.logger.Debug("cleanup script completed in sprite",
		zap.String("instance_id", instance.InstanceID),
		zap.String("reason", instance.StopReason))
}

func shouldRunExecutorCleanup(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "task archived", "task deleted", "session archived", "session deleted":
		return true
	default:
		return false
	}
}

func (r *SpritesExecutor) GetRemoteStatus(ctx context.Context, instance *ExecutorInstance) (*RemoteStatus, error) {
	if instance == nil {
		return nil, fmt.Errorf("instance is nil")
	}
	spriteName := strings.TrimSpace(getMetadataString(instance.Metadata, "sprite_name"))
	if spriteName == "" {
		return nil, fmt.Errorf("sprite name not available in metadata")
	}

	r.mu.RLock()
	token := r.tokens[instance.InstanceID]
	r.mu.RUnlock()
	if token == "" {
		return nil, fmt.Errorf("sprites api token is not cached for instance")
	}

	stepCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client := sprites.New(token, sprites.WithDisableControl())
	sprite, err := client.GetSprite(stepCtx, spriteName)
	if err != nil {
		return nil, err
	}

	return &RemoteStatus{
		RuntimeName:   string(r.Name()),
		RemoteName:    spriteName,
		State:         normalizeSpriteState(sprite.Status),
		CreatedAt:     nonZeroTimePtr(sprite.CreatedAt),
		LastCheckedAt: time.Now().UTC(),
	}, nil
}

func normalizeSpriteState(raw string) string {
	state := strings.ToLower(strings.TrimSpace(raw))
	if state == "" {
		return "unknown"
	}
	return state
}

func nonZeroTimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (r *SpritesExecutor) RecoverInstances(_ context.Context) ([]*ExecutorInstance, error) {
	return nil, nil
}

func (r *SpritesExecutor) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}

// createAgentInstance creates a per-instance server on the agentctl control server
// running inside the sprite. Returns the port of the per-instance server.
func (r *SpritesExecutor) createAgentInstance(
	ctx context.Context,
	sprite *sprites.Sprite,
	req *ExecutorCreateRequest,
) (int, error) {
	instanceReq := agentctl.CreateInstanceRequest{
		ID:            req.InstanceID,
		WorkspacePath: spritesWorkspacePath,
		SessionID:     req.SessionID,
	}
	reqJSON, err := json.Marshal(instanceReq)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal instance request: %w", err)
	}

	createCmd := fmt.Sprintf(
		"curl -sf -X POST http://localhost:%d/api/v1/instances -H 'Content-Type: application/json' -d '%s'",
		r.agentctlPort, string(reqJSON))

	stepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	out, err := sprite.CommandContext(stepCtx, "sh", "-c", createCmd).Output()
	if err != nil {
		return 0, fmt.Errorf("failed to create agent instance: %w", err)
	}

	var resp agentctl.CreateInstanceResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, fmt.Errorf("failed to parse instance response: %w (output: %s)", err, string(out))
	}

	r.logger.Debug("agent instance created inside sprite",
		zap.String("instance_id", resp.ID),
		zap.Int("port", resp.Port))

	return resp.Port, nil
}

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
// into environment variables (e.g., gh_cli secret → GITHUB_TOKEN).
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

// --- core operations ---

// createSprite creates a new sprite via the API (explicit POST, not lazy).
func (r *SpritesExecutor) createSprite(ctx context.Context, client *sprites.Client, name string) (*sprites.Sprite, error) {
	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	r.logger.Debug("creating sprite via API", zap.String("sprite", name))
	sprite, err := client.CreateSprite(stepCtx, name, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create sprite: %w", err)
	}
	return sprite, nil
}

func (r *SpritesExecutor) uploadAgentctl(ctx context.Context, sprite *sprites.Sprite) error {
	binaryPath, err := r.agentctlResolver.ResolveLinuxBinary()
	if err != nil {
		return fmt.Errorf("agentctl binary not found: %w", err)
	}

	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to read agentctl binary: %w", err)
	}

	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	r.logger.Debug("uploading agentctl binary", zap.Int("size_bytes", len(data)))

	// Upload via sprites filesystem API (HTTP PUT, much faster than stdin pipe for large binaries).
	if err := r.writeFileWithRetry(stepCtx, sprite, "/usr/local/bin/agentctl", data, 0o755); err != nil {
		return fmt.Errorf("failed to upload agentctl: %w", err)
	}

	// Verify the binary is present and executable.
	if _, err := sprite.CommandContext(stepCtx, "test", "-x", "/usr/local/bin/agentctl").Output(); err != nil {
		return fmt.Errorf("agentctl verification failed: %w", err)
	}
	return nil
}

// runPrepareScript resolves the prepare script with scriptengine and executes it,
// streaming stdout/stderr output through the onOutput callback.
func (r *SpritesExecutor) runPrepareScript(
	ctx context.Context,
	sprite *sprites.Sprite,
	req *ExecutorCreateRequest,
	onOutput func(string),
) error {
	script := r.resolvePrepareScript(req)
	if script == "" {
		r.logger.Debug("no prepare script configured, skipping")
		return nil
	}

	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	r.logger.Debug("running prepare script")
	cmd := sprite.CommandContext(stepCtx, "sh", "-c", script)
	cmd.Env = r.buildSpriteEnv(req.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start prepare script: %w", err)
	}

	// Stream stdout and stderr concurrently (io.MultiReader is sequential,
	// which blocks stderr until stdout EOF — bad for git progress output).
	var outputBuf strings.Builder
	var outputMu sync.Mutex
	emitOutput := func(chunk []byte) {
		outputMu.Lock()
		outputBuf.Write(chunk)
		if onOutput != nil {
			onOutput(lastLines(outputBuf.String(), spriteOutputMaxLines))
		}
		outputMu.Unlock()
	}

	var wg sync.WaitGroup
	readStream := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				emitOutput(buf[:n])
			}
			if readErr != nil {
				break
			}
		}
	}
	wg.Add(2)
	go readStream(stdout)
	go readStream(stderr)

	waitErr := cmd.Wait()
	wg.Wait() // ensure all output is collected

	if waitErr != nil {
		return fmt.Errorf("prepare script failed: %w\n%s", waitErr, lastLines(outputBuf.String(), spriteOutputMaxLines))
	}
	return nil
}

// resolvePrepareScript builds the resolved prepare script using scriptengine.
func (r *SpritesExecutor) resolvePrepareScript(req *ExecutorCreateRequest) string {
	script := getMetadataString(req.Metadata, MetadataKeySetupScript)
	if script == "" {
		script = DefaultPrepareScript("sprites")
	}
	if script == "" {
		return ""
	}

	resolver := scriptengine.NewResolver().
		WithProvider(scriptengine.WorkspaceProvider(spritesWorkspacePath)).
		WithProvider(scriptengine.AgentctlProvider(r.agentctlPort, spritesWorkspacePath)).
		WithProvider(scriptengine.GitIdentityProvider(req.Metadata)).
		WithProvider(scriptengine.RepositoryProvider(
			req.Metadata,
			req.Env,
			getGitRemoteURL,
			r.injectTokenIntoURL,
		))

	return resolver.Resolve(script)
}

func (r *SpritesExecutor) waitForHealth(ctx context.Context, sprite *sprites.Sprite) error {
	deadline := time.Now().Add(spriteHealthTimeout)
	healthURL := fmt.Sprintf("http://localhost:%d/health", r.agentctlPort)

	for time.Now().Before(deadline) {
		checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		out, err := sprite.CommandContext(checkCtx, "curl", "-sf", healthURL).Output()
		cancel()

		if err == nil && len(out) > 0 {
			r.logger.Debug("agentctl is healthy in sprite")
			return nil
		}
		time.Sleep(spriteHealthRetryWait)
	}
	return fmt.Errorf("agentctl did not become healthy within %v", spriteHealthTimeout)
}

func (r *SpritesExecutor) setupPortForwarding(
	_ context.Context,
	sprite *sprites.Sprite,
	spriteName, instanceID string,
	remotePort int,
) (int, error) {
	localPort, err := getFreePort()
	if err != nil {
		return 0, fmt.Errorf("failed to get free port: %w", err)
	}

	r.logger.Debug("setting up port forwarding",
		zap.Int("local_port", localPort),
		zap.Int("remote_port", remotePort))

	proxyCtx, cancel := context.WithCancel(context.Background())
	session, err := sprite.ProxyPort(proxyCtx, localPort, remotePort)
	if err != nil {
		cancel()
		return 0, fmt.Errorf("port forwarding failed: %w", err)
	}

	r.mu.Lock()
	r.proxies[instanceID] = &SpritesProxySession{
		spriteName:   spriteName,
		localPort:    localPort,
		proxySession: session,
		cancel:       cancel,
	}
	r.mu.Unlock()

	return localPort, nil
}

func (r *SpritesExecutor) cleanupOnFailure(_ context.Context, sprite *sprites.Sprite, instanceID string, destroySprite bool) {
	if sprite == nil {
		return
	}
	r.logger.Warn("cleaning up sprite after failure", zap.String("instance_id", instanceID))

	r.mu.Lock()
	if proxy, ok := r.proxies[instanceID]; ok {
		r.closeProxySession(proxy)
		delete(r.proxies, instanceID)
	}
	r.mu.Unlock()

	if !destroySprite {
		r.logger.Info("preserving sprite during cleanup (reconnect flow)",
			zap.String("instance_id", instanceID))
		return
	}

	if err := sprite.Destroy(); err != nil {
		r.logger.Warn("failed to destroy sprite during cleanup", zap.Error(err))
	}
}

func (r *SpritesExecutor) closeProxySession(proxy *SpritesProxySession) {
	if proxy == nil {
		return
	}
	if proxy.cancel != nil {
		proxy.cancel()
	}
	if proxy.proxySession != nil {
		_ = proxy.proxySession.Close()
	}
}

func (r *SpritesExecutor) applyNetworkPolicy(
	ctx context.Context,
	client *sprites.Client,
	spriteName string,
	req *ExecutorCreateRequest,
) error {
	rulesJSON, _ := req.Metadata["sprites_network_policy_rules"].(string)
	if rulesJSON == "" {
		return nil
	}

	var rules []sprites.NetworkPolicyRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return fmt.Errorf("failed to parse network policy rules: %w", err)
	}
	if len(rules) == 0 {
		return nil
	}

	policyCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	r.logger.Info("applying network policy from profile",
		zap.String("sprite_name", spriteName),
		zap.Int("rule_count", len(rules)))

	return client.UpdateNetworkPolicy(policyCtx, spriteName, &sprites.NetworkPolicy{Rules: rules})
}

// --- utility helpers ---

func (r *SpritesExecutor) injectTokenIntoURL(remoteURL string, env map[string]string) string {
	token := env["GITHUB_TOKEN"]
	if token == "" {
		return remoteURL
	}
	if converted := rewriteGitHubSSHToHTTPS(remoteURL); converted != "" {
		remoteURL = converted
	}
	if strings.HasPrefix(remoteURL, "https://") {
		return strings.Replace(remoteURL, "https://", "https://"+token+"@", 1)
	}
	return remoteURL
}

func rewriteGitHubSSHToHTTPS(remoteURL string) string {
	const (
		sshPrefixA = "git@github.com:"
		sshPrefixB = "ssh://git@github.com/"
	)
	switch {
	case strings.HasPrefix(remoteURL, sshPrefixA):
		return "https://github.com/" + strings.TrimPrefix(remoteURL, sshPrefixA)
	case strings.HasPrefix(remoteURL, sshPrefixB):
		return "https://github.com/" + strings.TrimPrefix(remoteURL, sshPrefixB)
	default:
		return ""
	}
}

func (r *SpritesExecutor) writeFileWithRetry(
	ctx context.Context,
	sprite *sprites.Sprite,
	path string,
	data []byte,
	mode os.FileMode,
) error {
	backoff := 700 * time.Millisecond
	var lastErr error
	for attempt := 1; attempt <= spriteUploadMaxRetries+1; attempt++ {
		err := sprite.Filesystem().WriteFileContext(ctx, path, data, mode)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt > spriteUploadMaxRetries || !isTransientUploadError(err) || ctx.Err() != nil {
			break
		}

		jitter := time.Duration(rand.Intn(300)) * time.Millisecond
		wait := backoff + jitter
		r.logger.Warn("retrying sprite file upload after transient error",
			zap.String("path", path),
			zap.Int("attempt", attempt),
			zap.Duration("retry_in", wait),
			zap.Error(err))
		time.Sleep(wait)
		backoff *= 2
	}
	return lastErr
}

func isTransientUploadError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	msg := strings.ToLower(err.Error())
	if status := extractUploadHTTPStatus(msg); status != 0 {
		if status == 408 || status == 429 || status >= 500 {
			return true
		}
	}
	return strings.Contains(msg, "client.timeout exceeded while awaiting headers") ||
		strings.Contains(msg, "request canceled") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "text file busy") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "temporary")
}

func extractUploadHTTPStatus(msg string) int {
	matches := uploadHTTPStatusRE.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return 0
	}
	code, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return code
}

func (r *SpritesExecutor) buildSpriteEnv(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

// --- step reporting helpers ---

// newStepReporter creates a reporting function that calls OnProgress if non-nil.
func newStepReporter(onProgress PrepareProgressCallback) func(PrepareStep, int) {
	return func(step PrepareStep, idx int) {
		if onProgress != nil {
			onProgress(step, idx, spritesTotalSteps)
		}
	}
}

// lastLines returns the last n lines of s.
func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// getFreePort finds an available local port.
func getFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}
