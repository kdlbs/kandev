package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	sprites "github.com/superfly/sprites-go"
	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/scriptengine"
)

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
	cmd := sprite.CommandContext(stepCtx, "bash", "-c", script)
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
	readStream := func(rd io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := rd.Read(buf)
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

	installScripts := r.collectAgentInstallScripts(req)

	resolver := scriptengine.NewResolver().
		WithProvider(scriptengine.WorkspaceProvider(spritesWorkspacePath)).
		WithProvider(scriptengine.AgentctlProvider(r.agentctlPort, spritesWorkspacePath)).
		WithProvider(scriptengine.GitIdentityProvider(req.Metadata)).
		WithProvider(scriptengine.GitHubAuthProvider(req.Env)).
		WithProvider(scriptengine.AgentInstallProvider(installScripts)).
		WithProvider(scriptengine.RepositoryProvider(
			req.Metadata,
			req.Env,
			getGitRemoteURL,
			r.injectTokenIntoURL,
		))

	return resolver.Resolve(script)
}

// collectAgentInstallScripts extracts agent IDs from the executor profile metadata
// and the current task's agent config, then collects their install scripts.
func (r *SpritesExecutor) collectAgentInstallScripts(req *ExecutorCreateRequest) []string {
	agentIDs := map[string]bool{}

	// Always include the current task's agent.
	if req.AgentConfig != nil {
		agentIDs[req.AgentConfig.ID()] = true
	}

	// Extract agent IDs from remote_credentials (e.g., "agent:claude-code:files:0").
	if credsJSON, _ := req.Metadata["remote_credentials"].(string); credsJSON != "" {
		var methodIDs []string
		if json.Unmarshal([]byte(credsJSON), &methodIDs) == nil {
			for _, id := range methodIDs {
				if agentID := extractAgentID(id); agentID != "" {
					agentIDs[agentID] = true
				}
			}
		}
	}

	// Extract agent IDs from remote_auth_secrets (e.g., "agent:codex:env:OPENAI_API_KEY").
	if secretsJSON, _ := req.Metadata["remote_auth_secrets"].(string); secretsJSON != "" {
		var secretsMap map[string]string
		if json.Unmarshal([]byte(secretsJSON), &secretsMap) == nil {
			for methodID := range secretsMap {
				if agentID := extractAgentID(methodID); agentID != "" {
					agentIDs[agentID] = true
				}
			}
		}
	}

	// Look up install scripts from the agent list.
	var scripts []string
	if r.agentList != nil {
		for _, ag := range r.agentList.ListEnabled() {
			if agentIDs[ag.ID()] {
				scripts = append(scripts, ag.InstallScript())
			}
		}
	}
	return scripts
}

// extractAgentID extracts the agent ID from method IDs like "agent:claude-code:env:TOKEN".
func extractAgentID(methodID string) string {
	parts := strings.SplitN(methodID, ":", 3)
	if len(parts) >= 2 && parts[0] == "agent" {
		return parts[1]
	}
	return ""
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
		TaskID:        req.TaskID,
		Protocol:      req.Protocol,
		AgentType:     agentTypeFromReq(req),
		McpServers:    req.McpServers,
		McpMode:       req.McpMode,
	}
	reqJSON, err := json.Marshal(instanceReq)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal instance request: %w", err)
	}

	stepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := sprite.CommandContext(stepCtx, "sh", "-c",
		fmt.Sprintf("curl -sf -X POST http://localhost:%d/api/v1/instances -H 'Content-Type: application/json' -d @-",
			r.agentctlPort))
	cmd.Stdin = bytes.NewReader(reqJSON)
	out, err := cmd.Output()
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

// getExistingInstancePort queries agentctl inside the sprite to check if a
// previously created instance is still running. Returns its port if found.
func (r *SpritesExecutor) getExistingInstancePort(
	ctx context.Context,
	sprite *sprites.Sprite,
	instanceID string,
) (int, error) {
	if instanceID == "" {
		return 0, fmt.Errorf("no instance ID")
	}

	checkCmd := fmt.Sprintf(
		"curl -sf http://localhost:%d/api/v1/instances/%s",
		r.agentctlPort, instanceID)

	stepCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	out, err := sprite.CommandContext(stepCtx, "sh", "-c", checkCmd).Output()
	if err != nil {
		return 0, fmt.Errorf("instance %s not found: %w", instanceID, err)
	}

	var resp struct {
		Port int `json:"port"`
	}
	if err := json.Unmarshal(out, &resp); err != nil || resp.Port == 0 {
		return 0, fmt.Errorf("failed to parse instance response: %v", err)
	}

	r.logger.Info("reusing existing agent instance",
		zap.String("instance_id", instanceID),
		zap.Int("port", resp.Port))
	return resp.Port, nil
}

// isAgentSubprocessRunning checks whether the agent subprocess (e.g., Claude Code)
// is still alive inside an existing agentctl instance by querying the status endpoint.
func (r *SpritesExecutor) isAgentSubprocessRunning(
	ctx context.Context,
	sprite *sprites.Sprite,
	instancePort int,
) bool {
	statusCmd := fmt.Sprintf("curl -sf http://localhost:%d/api/v1/status", instancePort)
	stepCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := sprite.CommandContext(stepCtx, "sh", "-c", statusCmd).Output()
	if err != nil {
		return false
	}

	var status struct {
		AgentStatus string `json:"agent_status"`
	}
	if json.Unmarshal(out, &status) != nil {
		return false
	}

	alive := status.AgentStatus == "running" || status.AgentStatus == "ready"
	r.logger.Debug("checked agent subprocess status",
		zap.Int("instance_port", instancePort),
		zap.String("agent_status", status.AgentStatus),
		zap.Bool("alive", alive))
	return alive
}

func agentTypeFromReq(req *ExecutorCreateRequest) string {
	if req.AgentConfig != nil {
		return req.AgentConfig.ID()
	}
	return ""
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
