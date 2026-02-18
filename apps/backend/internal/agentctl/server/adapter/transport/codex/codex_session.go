package codex

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/codex"
	"go.uber.org/zap"
)

// NewSession creates a new Codex thread (session).
// Note: The mcpServers parameter is ignored because Codex reads MCP configuration from
// ~/.codex/config.toml at startup time, not through the protocol. MCP servers are written
// to the config file by PrepareEnvironment() before the Codex process starts.
func (a *Adapter) NewSession(ctx context.Context, _ []types.McpServer) (string, error) {
	// Check client under lock, but don't hold lock during Call() to avoid deadlock
	// with handleNotification which also needs the lock
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return "", fmt.Errorf("adapter not initialized")
	}

	// Determine approval policy - default to "untrusted" if not specified.
	// "untrusted" forces Codex to request approval for all commands/writes.
	// Other options: "on-failure", "on-request", "never"
	approvalPolicy := a.cfg.ApprovalPolicy
	if approvalPolicy == "" {
		approvalPolicy = "untrusted"
	}

	a.logger.Info("starting codex thread with approval policy",
		zap.String("approval_policy", approvalPolicy),
		zap.String("work_dir", a.cfg.WorkDir))

	resp, err := client.Call(ctx, codex.MethodThreadStart, &codex.ThreadStartParams{
		Cwd:            a.cfg.WorkDir,
		ApprovalPolicy: approvalPolicy, // "untrusted", "on-failure", "on-request", "never"
		SandboxPolicy: &codex.SandboxPolicy{
			Type:          "workspace-write",       // Sandbox to workspace only (kebab-case per Codex docs)
			WritableRoots: []string{a.cfg.WorkDir}, // Allow writing to workspace
			NetworkAccess: true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to start thread: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("thread start error: %s", resp.Error.Message)
	}

	var result codex.ThreadStartResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse thread start result: %w", err)
	}

	a.mu.Lock()
	a.threadID = result.Thread.ID
	a.mu.Unlock()

	a.logger.Info("created new thread", zap.String("thread_id", a.threadID))

	return a.threadID, nil
}

// LoadSession resumes an existing Codex thread.
// It passes the same approval policy and sandbox settings as NewSession to ensure
// permission requirements are preserved across resume (see openai/codex#5322).
func (a *Adapter) LoadSession(ctx context.Context, sessionID string) error {
	// Check client under lock, but don't hold lock during Call() to avoid deadlock
	// with handleNotification which also needs the lock
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Determine approval policy - same logic as NewSession
	// "untrusted" forces Codex to request approval for all commands/writes.
	approvalPolicy := a.cfg.ApprovalPolicy
	if approvalPolicy == "" {
		approvalPolicy = "untrusted"
	}

	a.logger.Info("resuming codex thread with approval policy",
		zap.String("thread_id", sessionID),
		zap.String("approval_policy", approvalPolicy),
		zap.String("work_dir", a.cfg.WorkDir))

	resp, err := client.Call(ctx, codex.MethodThreadResume, &codex.ThreadResumeParams{
		ThreadID:       sessionID,
		Cwd:            a.cfg.WorkDir,
		ApprovalPolicy: approvalPolicy,
		SandboxPolicy: &codex.SandboxPolicy{
			Type:          "workspace-write", // kebab-case per Codex docs
			WritableRoots: []string{a.cfg.WorkDir},
			NetworkAccess: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to resume thread: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("thread resume error: %s", resp.Error.Message)
	}

	a.mu.Lock()
	a.threadID = sessionID
	a.mu.Unlock()

	a.logger.Info("resumed thread", zap.String("thread_id", a.threadID))

	return nil
}
