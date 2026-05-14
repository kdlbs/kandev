package executor

import (
	"context"
	"maps"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// GetExecutionBySession returns the execution state for a specific session.
// "Has been launched" is determined by whether an executors_running row exists
// (the lifecycle manager creates it in lockstep with executionStore.Add); the
// removed task_sessions.agent_execution_id column was a denormalized copy that
// drifted from the in-memory store and produced the divergence bug.
func (e *Executor) GetExecutionBySession(sessionID string) (*TaskExecution, bool) {
	ctx := context.Background()
	const startupGracePeriod = 30 * time.Second

	// Load from database
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, false
	}
	hasRunning, hasErr := e.repo.HasExecutorRunningRow(ctx, sessionID)
	if hasErr != nil || !hasRunning {
		return nil, false
	}

	// Verify the agent is actually running
	if !e.agentManager.IsAgentRunningForSession(ctx, sessionID) {
		if (session.State == models.TaskSessionStateStarting || session.State == models.TaskSessionStateRunning) &&
			time.Since(session.UpdatedAt) < startupGracePeriod {
			return FromTaskSession(session), true
		}
		return nil, false
	}

	return FromTaskSession(session), true
}

// ListExecutions returns all active executions
func (e *Executor) ListExecutions() []*TaskExecution {
	ctx := context.Background()
	sessions, err := e.repo.ListActiveTaskSessions(ctx)
	if err != nil {
		return nil
	}

	result := make([]*TaskExecution, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, FromTaskSession(session))
	}
	return result
}

// ActiveCount returns the number of active executions
func (e *Executor) ActiveCount() int {
	ctx := context.Background()
	sessions, err := e.repo.ListActiveTaskSessions(ctx)
	if err != nil {
		return 0
	}
	return len(sessions)
}

// CanExecute returns true if there's capacity for another execution.
// Currently always returns true as there is no concurrent execution limit.
func (e *Executor) CanExecute() bool {
	return true
}

// MarkCompletedBySession marks an execution as completed by session ID
func (e *Executor) MarkCompletedBySession(ctx context.Context, sessionID string, state v1.TaskSessionState) {
	e.logger.Info("execution completed",
		zap.String("session_id", sessionID),
		zap.String("state", string(state)))

	// Update database
	dbState := models.TaskSessionState(state)
	if err := e.repo.UpdateTaskSessionState(ctx, sessionID, dbState, ""); err != nil {
		e.logger.Error("failed to update agent session status in database",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

func (e *Executor) defaultExecutorID(ctx context.Context, workspaceID string) string {
	if workspaceID == "" {
		return ""
	}
	workspace, err := e.repo.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil || workspace.DefaultExecutorID == nil {
		return ""
	}
	return strings.TrimSpace(*workspace.DefaultExecutorID)
}

// executorConfig holds resolved executor configuration.
type executorConfig struct {
	ExecutorID    string
	ExecutorType  string
	ExecutorCfg   map[string]string // The executor record's Config map (docker_host, etc.)
	Metadata      map[string]interface{}
	SetupScript   string            // Setup script from profile
	CleanupScript string            // Cleanup script from profile (terminal teardown)
	ProfileEnv    map[string]string // Resolved env vars from profile (secrets decrypted)
	Resumable     bool              // Whether the executor supports session resume
	RuntimeName   string            // Runtime name from the executor type (e.g. "local_pc")
}

// resolveExecutorConfig resolves executor configuration from an executor ID.
// If executorID is empty, it falls back to the workspace default.
// Returns the resolved config with executor ID, type, and metadata.
func (e *Executor) resolveExecutorConfig(ctx context.Context, executorID, workspaceID string, existingMetadata map[string]interface{}) executorConfig {
	resolved := executorID
	if resolved == "" {
		resolved = e.defaultExecutorID(ctx, workspaceID)
	}

	metadata := existingMetadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	// When no executor ID is resolved, check if the metadata carries an
	// executor profile. The profile references a specific executor, so we
	// can derive the full config from it (critical for review-watch tasks
	// where the executor comes solely from the profile).
	if resolved == "" {
		profileID, _ := metadata["executor_profile_id"].(string)
		if profileID == "" {
			return executorConfig{Metadata: metadata}
		}
		cfg := executorConfig{Metadata: metadata}
		e.applyProfile(ctx, profileID, &cfg, metadata)
		return cfg
	}

	metadata["executor_id"] = resolved

	executor, err := e.repo.GetExecutor(ctx, resolved)
	if err != nil || executor == nil {
		return executorConfig{
			ExecutorID: resolved,
			Metadata:   metadata,
		}
	}

	cfg := executorConfig{
		ExecutorID:   resolved,
		ExecutorType: string(executor.Type),
		ExecutorCfg:  executor.Config,
		Metadata:     metadata,
		Resumable:    executor.Resumable,
		RuntimeName:  string(executor.Type),
	}

	// Load profile by ID if specified in metadata, otherwise skip
	profileID, _ := metadata["executor_profile_id"].(string)
	if profileID != "" {
		e.applyProfile(ctx, profileID, &cfg, metadata)
	}

	return cfg
}

// applyProfile loads an executor profile and applies its settings to the config.
func (e *Executor) applyProfile(ctx context.Context, profileID string, cfg *executorConfig, metadata map[string]interface{}) {
	profile, err := e.repo.GetExecutorProfile(ctx, profileID)
	if err != nil {
		e.logger.Warn("failed to load executor profile",
			zap.String("profile_id", profileID),
			zap.Error(err))
		return
	}
	if profile == nil {
		return
	}

	// The profile is tied to a specific executor. If it differs from the
	// currently resolved executor (e.g. the workspace default), override the
	// config so the correct executor type is used. This is critical for
	// worktree executors selected via review watches or executor profiles.
	if profile.ExecutorID != "" && profile.ExecutorID != cfg.ExecutorID {
		exec, execErr := e.repo.GetExecutor(ctx, profile.ExecutorID)
		if execErr == nil && exec != nil {
			cfg.ExecutorID = profile.ExecutorID
			cfg.ExecutorType = string(exec.Type)
			cfg.ExecutorCfg = exec.Config
			cfg.Resumable = exec.Resumable
			cfg.RuntimeName = string(exec.Type)
			metadata["executor_id"] = profile.ExecutorID
		}
	}

	cfg.SetupScript = profile.PrepareScript
	cfg.CleanupScript = profile.CleanupScript
	cfg.ProfileEnv = e.resolveProfileEnvVars(ctx, profile.EnvVars)
	// Persist secret store IDs in metadata so runtimes can resolve tokens after restart
	// (e.g., SpritesExecutor needs SPRITES_API_TOKEN to poll remote status).
	for _, ev := range profile.EnvVars {
		if ev.SecretID != "" {
			metadata["env_secret_id_"+ev.Key] = ev.SecretID
		}
	}
	if profile.CleanupScript != "" {
		metadata[lifecycle.MetadataKeyCleanupScript] = profile.CleanupScript
	}
	if policyJSON := strings.TrimSpace(profile.McpPolicy); policyJSON != "" {
		metadata["executor_mcp_policy"] = policyJSON
	}
	if rulesJSON := profile.Config["sprites_network_policy_rules"]; rulesJSON != "" {
		metadata["sprites_network_policy_rules"] = rulesJSON
	}
	if credJSON := profile.Config["remote_credentials"]; credJSON != "" {
		metadata["remote_credentials"] = credJSON
	}
	if authSecretsJSON := profile.Config["remote_auth_secrets"]; authSecretsJSON != "" {
		metadata["remote_auth_secrets"] = authSecretsJSON
	}
	if remoteAuthHome := profile.Config["remote_auth_target_home"]; remoteAuthHome != "" {
		metadata["remote_auth_target_home"] = remoteAuthHome
	}
	if gitUserName := profile.Config["git_user_name"]; gitUserName != "" {
		metadata["git_user_name"] = gitUserName
	}
	if gitUserEmail := profile.Config["git_user_email"]; gitUserEmail != "" {
		metadata["git_user_email"] = gitUserEmail
	}
	if imageTag := profile.Config["image_tag"]; imageTag != "" {
		metadata["image_tag_override"] = imageTag
	}
}

// resolveProfileEnvVars resolves profile env vars, dereferencing secret IDs to their values.
func (e *Executor) resolveProfileEnvVars(ctx context.Context, envVars []models.ProfileEnvVar) map[string]string {
	if len(envVars) == 0 {
		return nil
	}
	resolved := make(map[string]string, len(envVars))
	for _, ev := range envVars {
		if ev.SecretID != "" && e.secretStore != nil {
			value, err := e.secretStore.Reveal(ctx, ev.SecretID)
			if err != nil {
				e.logger.Warn("failed to resolve secret for profile env var",
					zap.String("key", ev.Key),
					zap.String("secret_id", ev.SecretID),
					zap.Error(err))
				continue
			}
			resolved[ev.Key] = value
		} else if ev.Value != "" {
			resolved[ev.Key] = ev.Value
		}
	}
	return resolved
}

func cloneMetadata(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(src))
	maps.Copy(out, src)
	return out
}
