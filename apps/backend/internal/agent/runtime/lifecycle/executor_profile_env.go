package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// ExecutorProfileReader loads session state, task-cleanup admission, and the
// executor profile selected for a session. Production launch safety requires
// the task repository implementation wired via SetExecutorProfileReader.
type ExecutorProfileReader interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	HasActiveTaskResourceCleanupJob(ctx context.Context, taskID string) (bool, error)
	GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error)
	GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error)
}

// SetExecutorProfileReader wires launch admission and the reader used to expose
// executor-profile env vars to user shell terminals. Production must provide a
// reader; nil is supported by isolated lifecycle tests, and terminals then fall
// back to inheriting only the backend process environment.
func (m *Manager) SetExecutorProfileReader(reader ExecutorProfileReader) {
	m.executorProfileReader = reader
}

// SetPluginExecutorProfileLoader wires the host-owned profile and transient
// secret reader used by the plugin remote runtime.
func (m *Manager) SetPluginExecutorProfileLoader(loader PluginExecutorProfileLoader) {
	m.pluginExecutorProfileLoader = loader
}

// SetPluginRuntimeAPIURL sets the externally reachable Kandev API URL that plugin
// executor environments call back to. Empty leaves plugin launches without one.
func (m *Manager) SetPluginRuntimeAPIURL(url string) {
	m.pluginRuntimeAPIURL = url
}

// ExecutorProfileEnvForSession resolves the executor profile's env vars for a
// terminal, revealing secret-backed entries. It mirrors what the agent
// subprocess receives (the orchestrator merges the same profile env into the
// launch request), so a user shell terminal opened on the workspace sees the
// same tokens the agent and the repository setup script do.
//
// Resolution is best-effort for missing profile records. A secret failure is
// returned so the terminal caller can fail closed instead of starting a shell
// with an incomplete profile environment.
func (m *Manager) ExecutorProfileEnvForSession(ctx context.Context, sessionID, taskEnvironmentID string) (map[string]string, error) {
	if m.executorProfileReader == nil {
		return nil, nil
	}
	profileID := m.terminalExecutorProfileID(ctx, sessionID, taskEnvironmentID)
	if profileID == "" {
		return nil, nil
	}
	profile, err := m.executorProfileReader.GetExecutorProfile(ctx, profileID)
	if err != nil {
		m.logger.Warn("failed to load executor profile for terminal env",
			zap.String("session_id", sessionID),
			zap.String("task_environment_id", taskEnvironmentID),
			zap.String("executor_profile_id", profileID),
			zap.Error(err))
		return nil, nil
	}
	if profile == nil || len(profile.EnvVars) == 0 {
		return nil, nil
	}
	// ExecutorProfile.EnvVars and agent-profile env vars are the same type, so
	// the secret-revealing resolver is shared.
	resolved, err := m.resolveAgentProfileEnvVars(ctx, profile.EnvVars)
	if err != nil {
		m.logger.Warn("failed to resolve executor profile environment for terminal",
			zap.String("session_id", sessionID),
			zap.String("executor_profile_id", profileID),
			zap.Error(err))
		return nil, err
	}
	return resolved, nil
}

// terminalExecutorProfileID picks the executor profile the terminal should
// inherit from. The session's profile wins: buildLaunchAgentRequest resolves the
// agent's env from session.ExecutorProfileID, while the task_environments row
// keeps whatever the *first* session stamped on it — the reuse branch in
// persistTaskEnvironment never refreshes executor_profile_id. Reading the
// environment row alone would hand a terminal stale secrets whenever a later
// session picked a different profile of the same executor type. The environment
// row is only a fallback for sessions that never recorded one.
func (m *Manager) terminalExecutorProfileID(ctx context.Context, sessionID, taskEnvironmentID string) string {
	if sessionID != "" {
		session, err := m.executorProfileReader.GetTaskSession(ctx, sessionID)
		if err != nil {
			m.logger.Warn("failed to load session for terminal env",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else if session != nil && session.ExecutorProfileID != "" {
			return session.ExecutorProfileID
		}
	}
	if taskEnvironmentID == "" {
		return ""
	}
	env, err := m.executorProfileReader.GetTaskEnvironment(ctx, taskEnvironmentID)
	if err != nil {
		m.logger.Warn("failed to load task environment for terminal env",
			zap.String("task_environment_id", taskEnvironmentID),
			zap.Error(err))
		return ""
	}
	if env == nil {
		return ""
	}
	return env.ExecutorProfileID
}

// preparePluginExecutorLaunch attaches the provider profile and points the remote
// environment at the externally reachable Kandev API.
func (m *Manager) preparePluginExecutorLaunch(ctx context.Context, execReq *ExecutorCreateRequest, metadata map[string]interface{}, taskEnvironmentID string) error {
	if m.pluginExecutorProfileLoader == nil {
		return errors.New("plugin executor profile loader is unavailable")
	}
	profileID := strings.TrimSpace(getMetadataString(metadata, MetadataKeyExecutorProfileID))
	profile, err := m.pluginExecutorProfileLoader.ExecutorProviderProfileForLaunch(ctx, profileID, taskEnvironmentID)
	if err != nil {
		return fmt.Errorf("resolve plugin executor profile: %w", err)
	}
	if profile == nil {
		return errors.New("plugin executor profile is unavailable")
	}
	execReq.PluginExecutor = &PluginExecutorLaunch{Profile: *profile}
	if m.pluginRuntimeAPIURL != "" {
		execReq.Env = cloneStringMap(execReq.Env)
		execReq.Env[envKeyKandevAPIURL] = m.pluginRuntimeAPIURL
	}
	return nil
}
