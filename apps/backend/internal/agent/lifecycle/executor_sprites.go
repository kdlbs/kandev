package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/kandev/kandev/internal/scriptengine"
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
	spritesTotalSteps     = 6 // create, upload, prepare script, health, create instance, network policy
	spriteOutputMaxLines  = 20
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

	r.mu.Lock()
	r.tokens[req.InstanceID] = token
	r.mu.Unlock()

	spriteName := spritesNamePrefix + req.InstanceID[:12]
	client := sprites.New(token, sprites.WithDisableControl())

	r.logger.Info("creating sprite instance",
		zap.String("instance_id", req.InstanceID),
		zap.String("sprite_name", spriteName))

	report := newStepReporter(req.OnProgress)

	// Step 0: Create sprite
	step := beginStep("Creating cloud sandbox")
	report(step, 0)
	sprite, err := r.createSprite(ctx, client, spriteName)
	if err != nil {
		completeStepError(&step, err.Error())
		report(step, 0)
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	completeStepSuccess(&step)
	report(step, 0)

	// Step 1: Upload agentctl binary
	step = beginStep("Uploading agent controller")
	report(step, 1)
	if err := r.uploadAgentctl(ctx, sprite); err != nil {
		completeStepError(&step, err.Error())
		report(step, 1)
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	completeStepSuccess(&step)
	report(step, 1)

	// Step 2: Run prepare script (with output streaming)
	step = beginStep("Running prepare script")
	report(step, 2)
	err = r.runPrepareScript(ctx, sprite, req, func(output string) {
		step.Output = output
		report(step, 2)
	})
	if err != nil {
		completeStepError(&step, err.Error())
		report(step, 2)
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	completeStepSuccess(&step)
	report(step, 2)

	// Step 3: Wait for agentctl health
	step = beginStep("Waiting for agent controller")
	report(step, 3)
	if err := r.waitForHealth(ctx, sprite); err != nil {
		completeStepError(&step, err.Error())
		report(step, 3)
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	completeStepSuccess(&step)
	report(step, 3)

	// Step 4: Create agent instance (control server → per-instance server)
	step = beginStep("Creating agent instance")
	report(step, 4)
	instancePort, err := r.createAgentInstance(ctx, sprite, req)
	if err != nil {
		completeStepError(&step, err.Error())
		report(step, 4)
		r.cleanupOnFailure(ctx, sprite, req.InstanceID)
		return nil, err
	}
	completeStepSuccess(&step)
	report(step, 4)

	// Step 5: Apply network policy
	step = beginStep("Applying network policy")
	report(step, 5)
	if err := r.applyNetworkPolicy(ctx, client, spriteName, req); err != nil {
		r.logger.Warn("failed to apply network policy from profile", zap.Error(err))
		completeStepSkipped(&step)
	} else {
		completeStepSuccess(&step)
	}
	report(step, 5)

	// Port forwarding to the per-instance server (not the control server)
	localPort, err := r.setupPortForwarding(ctx, sprite, spriteName, req.InstanceID, instancePort)
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
			"local_port":        localPort,
			MetadataKeyIsRemote: true,
		},
	}, nil
}

func (r *SpritesExecutor) StopInstance(_ context.Context, instance *ExecutorInstance, _ bool) error {
	spriteName := getMetadataString(instance.Metadata, "sprite_name")
	if spriteName == "" {
		return nil
	}

	r.mu.Lock()
	if proxy, ok := r.proxies[instance.InstanceID]; ok {
		if proxy.proxySession != nil {
			_ = proxy.proxySession.Close()
		}
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
	if err := sprite.Filesystem().WriteFileContext(stepCtx, "/usr/local/bin/agentctl", data, 0o755); err != nil {
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
		script = scriptengine.DefaultPrepareScript("sprites")
	}
	if script == "" {
		return ""
	}

	resolver := scriptengine.NewResolver().
		WithProvider(scriptengine.WorkspaceProvider(spritesWorkspacePath)).
		WithProvider(scriptengine.AgentctlProvider(r.agentctlPort, spritesWorkspacePath)).
		WithProvider(scriptengine.RepositoryProvider(
			req.Metadata,
			req.Env,
			r.getGitRemoteURL,
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
	ctx context.Context,
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

	session, err := sprite.ProxyPort(ctx, localPort, remotePort)
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
	if sprite == nil {
		return
	}
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

func (r *SpritesExecutor) getGitRemoteURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func (r *SpritesExecutor) injectTokenIntoURL(remoteURL string, env map[string]string) string {
	token := env["GITHUB_TOKEN"]
	if token == "" {
		return remoteURL
	}
	if strings.HasPrefix(remoteURL, "https://") {
		return strings.Replace(remoteURL, "https://", "https://"+token+"@", 1)
	}
	return remoteURL
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
