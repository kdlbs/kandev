package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	sprites "github.com/superfly/sprites-go"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

const (
	spritesAgentctlPort   = 8765
	spritesWorkspacePath  = "/workspace"
	spritesNamePrefix     = "kandev-"
	spriteStepTimeout     = 120 * time.Second
	spriteHealthTimeout   = 15 * time.Second
	spriteDestroyTimeout  = 30 * time.Second
	spriteHealthRetryWait = 500 * time.Millisecond
)

// SpritesProxySession tracks an active port-forwarding session to a sprite.
type SpritesProxySession struct {
	spriteName   string
	localPort    int
	proxySession *sprites.ProxySession
}

// SpritesExecutor implements ExecutorBackend for Sprites.dev remote sandboxes.
type SpritesExecutor struct {
	secretStore      secrets.SecretStore
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
	resolver *AgentctlResolver,
	agentctlPort int,
	log *logger.Logger,
) *SpritesExecutor {
	return &SpritesExecutor{
		secretStore:      secretStore,
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

func (r *SpritesExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	token := req.Env["SPRITES_API_TOKEN"]
	if token == "" {
		return nil, fmt.Errorf("SPRITES_API_TOKEN not set in execution environment (configure it in the executor profile)")
	}

	// Cache the token for later use (e.g., StopInstance)
	r.mu.Lock()
	r.tokens[req.InstanceID] = token
	r.mu.Unlock()

	spriteName := spritesNamePrefix + req.InstanceID[:12]
	client := sprites.New(token)
	sprite := client.Sprite(spriteName)

	r.logger.Info("creating sprite instance",
		zap.String("instance_id", req.InstanceID),
		zap.String("sprite_name", spriteName))

	// Sequence: create → install deps → upload agentctl → clone repo → setup → start agentctl → proxy
	if err := r.initializeSprite(ctx, sprite, spriteName); err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	if err := r.installDependencies(ctx, sprite); err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	if err := r.uploadAgentctl(ctx, sprite); err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	if err := r.cloneRepository(ctx, sprite, req); err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	if err := r.runSetupScript(ctx, sprite, req); err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	if err := r.startAgentctlProcess(ctx, sprite, req); err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	if err := r.waitForHealth(ctx, sprite); err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}

	localPort, err := r.setupPortForwarding(ctx, sprite, spriteName, req.InstanceID)
	if err != nil {
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}

	agentctlClient := agentctl.NewClient("127.0.0.1", localPort, r.logger,
		agentctl.WithExecutionID(req.InstanceID),
		agentctl.WithSessionID(req.SessionID))

	r.logger.Info("sprite instance ready",
		zap.String("instance_id", req.InstanceID),
		zap.String("sprite_name", spriteName),
		zap.Int("local_port", localPort))

	return &ExecutorInstance{
		InstanceID:  req.InstanceID,
		TaskID:      req.TaskID,
		SessionID:   req.SessionID,
		RuntimeName: string(r.Name()),
		Client:      agentctlClient,
		WorkspacePath: spritesWorkspacePath,
		Metadata: map[string]interface{}{
			"sprite_name": spriteName,
			"local_port":  localPort,
			MetadataKeyIsRemote: true,
		},
	}, nil
}

func (r *SpritesExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, _ bool) error {
	spriteName := getMetadataString(instance.Metadata, "sprite_name")
	if spriteName == "" {
		return nil
	}

	// Close port forwarding
	r.mu.Lock()
	if proxy, ok := r.proxies[instance.InstanceID]; ok {
		if proxy.proxySession != nil {
			_ = proxy.proxySession.Close()
		}
		delete(r.proxies, instance.InstanceID)
	}
	r.mu.Unlock()

	// Get cached token for this instance
	r.mu.RLock()
	token := r.tokens[instance.InstanceID]
	r.mu.RUnlock()
	if token == "" {
		r.logger.Warn("no cached API token for sprite instance, cannot destroy", zap.String("instance_id", instance.InstanceID))
		return nil
	}
	client := sprites.New(token)
	sprite := client.Sprite(spriteName)
	if err := sprite.Destroy(); err != nil {
		r.logger.Warn("failed to destroy sprite",
			zap.String("sprite_name", spriteName),
			zap.Error(err))
		return fmt.Errorf("failed to destroy sprite: %w", err)
	}

	// Clean up cached token
	r.mu.Lock()
	delete(r.tokens, instance.InstanceID)
	r.mu.Unlock()

	r.logger.Info("sprite destroyed", zap.String("sprite_name", spriteName))
	return nil
}

func (r *SpritesExecutor) RecoverInstances(_ context.Context) ([]*ExecutorInstance, error) {
	// Sprites tunnels don't survive backend restarts
	return nil, nil
}

func (r *SpritesExecutor) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}

// --- helper methods ---

func (r *SpritesExecutor) initializeSprite(ctx context.Context, sprite *sprites.Sprite, name string) error {
	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	r.logger.Debug("initializing sprite (lazy create on first command)", zap.String("sprite", name))
	out, err := sprite.CommandContext(stepCtx, "echo", "kandev-ready").Output()
	if err != nil {
		return fmt.Errorf("failed to create sprite: %w", err)
	}
	if !strings.Contains(string(out), "kandev-ready") {
		return fmt.Errorf("unexpected sprite output: %s", string(out))
	}
	return nil
}

func (r *SpritesExecutor) installDependencies(ctx context.Context, sprite *sprites.Sprite) error {
	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	r.logger.Debug("installing system dependencies in sprite")

	// Check if node is already available
	if out, err := sprite.CommandContext(stepCtx, "node", "--version").Output(); err == nil {
		r.logger.Debug("node already installed", zap.String("version", strings.TrimSpace(string(out))))
		return nil
	}

	// Install git, curl, ca-certificates
	script := "apt-get update -qq && apt-get install -y -qq git curl ca-certificates > /dev/null 2>&1"
	if _, err := sprite.CommandContext(stepCtx, "sh", "-c", script).Output(); err != nil {
		return fmt.Errorf("failed to install system deps: %w", err)
	}

	// Install Node.js 22.x
	nodeScript := "curl -fsSL https://deb.nodesource.com/setup_22.x | bash - > /dev/null 2>&1 && " +
		"apt-get install -y -qq nodejs > /dev/null 2>&1"
	if _, err := sprite.CommandContext(stepCtx, "sh", "-c", nodeScript).Output(); err != nil {
		return fmt.Errorf("failed to install Node.js: %w", err)
	}
	return nil
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

	// Upload via stdin pipe
	cmd := sprite.CommandContext(stepCtx, "sh", "-c",
		"cat > /usr/local/bin/agentctl && chmod +x /usr/local/bin/agentctl")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start upload: %w", err)
	}

	if _, err := stdin.Write(data); err != nil {
		return fmt.Errorf("failed to write binary data: %w", err)
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("failed to close stdin: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("upload command failed: %w", err)
	}

	// Verify
	if _, err := sprite.CommandContext(stepCtx, "agentctl", "--version").Output(); err != nil {
		return fmt.Errorf("agentctl verification failed: %w", err)
	}
	return nil
}

func (r *SpritesExecutor) cloneRepository(ctx context.Context, sprite *sprites.Sprite, req *ExecutorCreateRequest) error {
	repoPath := getMetadataString(req.Metadata, MetadataKeyRepositoryPath)
	if repoPath == "" {
		repoPath = req.WorkspacePath
	}
	if repoPath == "" {
		// No repo to clone — just create the workspace directory
		stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
		defer cancel()
		_, err := sprite.CommandContext(stepCtx, "mkdir", "-p", spritesWorkspacePath).Output()
		return err
	}

	// Get remote URL from local repo
	remoteURL, err := r.getGitRemoteURL(repoPath)
	if err != nil || remoteURL == "" {
		r.logger.Warn("no git remote found, creating empty workspace", zap.Error(err))
		stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
		defer cancel()
		_, mkErr := sprite.CommandContext(stepCtx, "mkdir", "-p", spritesWorkspacePath).Output()
		return mkErr
	}

	branch := r.resolveCloneBranch(repoPath, req)
	cloneURL := r.injectTokenIntoURL(remoteURL, req.Env)

	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	r.logger.Debug("cloning repository",
		zap.String("branch", branch),
		zap.String("remote", remoteURL))

	args := []string{"clone", "--depth=1"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, cloneURL, spritesWorkspacePath)

	cmd := sprite.CommandContext(stepCtx, "git", args...)
	cmd.Env = r.buildGitEnv(req.Env)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %w\n%s", err, string(out))
	}
	return nil
}

func (r *SpritesExecutor) runSetupScript(ctx context.Context, sprite *sprites.Sprite, req *ExecutorCreateRequest) error {
	script := getMetadataString(req.Metadata, MetadataKeySetupScript)
	if script == "" {
		return nil
	}

	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	r.logger.Debug("running setup script")
	cmd := sprite.CommandContext(stepCtx, "sh", "-c", script)
	cmd.Dir = spritesWorkspacePath
	cmd.Env = r.buildSpriteEnv(req.Env)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setup script failed: %w\n%s", err, string(out))
	}
	return nil
}

func (r *SpritesExecutor) startAgentctlProcess(ctx context.Context, sprite *sprites.Sprite, req *ExecutorCreateRequest) error {
	r.logger.Debug("starting agentctl process in sprite")

	// Use background context — agentctl should outlive this call
	cmd := sprite.CommandContext(context.Background(),
		"agentctl",
		"--port", fmt.Sprintf("%d", r.agentctlPort),
		"--workspace", spritesWorkspacePath)
	cmd.Env = r.buildSpriteEnv(req.Env)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start agentctl: %w", err)
	}

	_ = ctx // kept for signature consistency
	return nil
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

func (r *SpritesExecutor) setupPortForwarding(ctx context.Context, sprite *sprites.Sprite, spriteName, instanceID string) (int, error) {
	localPort, err := getFreePort()
	if err != nil {
		return 0, fmt.Errorf("failed to get free port: %w", err)
	}

	r.logger.Debug("setting up port forwarding",
		zap.Int("local_port", localPort),
		zap.Int("remote_port", r.agentctlPort))

	session, err := sprite.ProxyPort(ctx, localPort, r.agentctlPort)
	if err != nil {
		return 0, fmt.Errorf("port forwarding failed: %w", err)
	}

	r.mu.Lock()
	r.proxies[instanceID] = &SpritesProxySession{
		spriteName:   spriteName,
		localPort:    localPort,
		proxySession: session,
	}
	r.mu.Unlock()

	return localPort, nil
}

func (r *SpritesExecutor) cleanupOnFailure(_ context.Context, sprite *sprites.Sprite, instanceID string) {
	r.logger.Warn("cleaning up sprite after failure", zap.String("instance_id", instanceID))

	r.mu.Lock()
	if proxy, ok := r.proxies[instanceID]; ok {
		if proxy.proxySession != nil {
			_ = proxy.proxySession.Close()
		}
		delete(r.proxies, instanceID)
	}
	r.mu.Unlock()

	if err := sprite.Destroy(); err != nil {
		r.logger.Warn("failed to destroy sprite during cleanup", zap.Error(err))
	}
}

// --- utility helpers ---

func (r *SpritesExecutor) getGitRemoteURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func (r *SpritesExecutor) resolveCloneBranch(repoPath string, req *ExecutorCreateRequest) string {
	if branch := getMetadataString(req.Metadata, MetadataKeyBaseBranch); branch != "" {
		return branch
	}
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func (r *SpritesExecutor) injectTokenIntoURL(remoteURL string, env map[string]string) string {
	token := env["GITHUB_TOKEN"]
	if token == "" {
		return remoteURL
	}
	// Convert https://github.com/owner/repo.git → https://token@github.com/owner/repo.git
	if strings.HasPrefix(remoteURL, "https://") {
		return strings.Replace(remoteURL, "https://", "https://"+token+"@", 1)
	}
	return remoteURL
}

func (r *SpritesExecutor) buildGitEnv(env map[string]string) []string {
	var result []string
	if token := env["GITHUB_TOKEN"]; token != "" {
		result = append(result, "GITHUB_TOKEN="+token)
	}
	result = append(result, "GIT_TERMINAL_PROMPT=0")
	return result
}

func (r *SpritesExecutor) buildSpriteEnv(env map[string]string) []string {
	var result []string
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
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
