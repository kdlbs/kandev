package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// EnvironmentDestroyer tears down the runtime resources recorded on a TaskEnvironment.
// Implemented by an adapter over the lifecycle.Manager and worktree.Manager.
// Worktree destruction preserves the underlying branch — user data that hasn't been
// pushed is never deleted by reset.
type EnvironmentDestroyer interface {
	DestroyContainer(ctx context.Context, containerID string) error
	DestroySandbox(ctx context.Context, sandboxID, executionID string) error
	DestroyWorktree(ctx context.Context, worktreeID string) error
	// PushEnvironmentBranch best-effort pushes the current branch of the environment's
	// workspace to its upstream. Returns an error if the push fails; callers can decide
	// whether to abort the reset on failure.
	PushEnvironmentBranch(ctx context.Context, env *models.TaskEnvironment) error
	// GetContainerLiveStatus returns a real-time snapshot of a Docker container,
	// or nil when the executor type doesn't have a container layer.
	GetContainerLiveStatus(ctx context.Context, containerID string) (*ContainerLiveStatus, error)
}

// ContainerLiveStatus mirrors lifecycle.ContainerLiveStatus for the task service
// layer so the EnvironmentDestroyer interface doesn't import lifecycle types.
type ContainerLiveStatus struct {
	ContainerID string `json:"container_id"`
	State       string `json:"state"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	ExitCode    int    `json:"exit_code,omitempty"`
	Health      string `json:"health,omitempty"`
	Missing     bool   `json:"missing,omitempty"`
}

// SSHLiveStatus surfaces the SSH-runtime fields needed by the Executor
// Settings popover for an SSH-backed task: where the agent is running
// (host/user/workdir), the local port forward we set up for the agent
// stream, and the remote agentctl process info. Sourced from the latest
// running ExecutorRunning row for the task's most recent session — the
// SSH executor writes these metadata keys in CreateInstance and we just
// project them back out here.
type SSHLiveStatus struct {
	Host               string `json:"host,omitempty"`
	Port               int    `json:"port,omitempty"`
	User               string `json:"user,omitempty"`
	RemoteTaskDir      string `json:"remote_task_dir,omitempty"`
	RemoteAgentctlPID  int    `json:"remote_agentctl_pid,omitempty"`
	RemoteAgentctlPort int    `json:"remote_agentctl_port,omitempty"`
	LocalForwardPort   int    `json:"local_forward_port,omitempty"`
	Fingerprint        string `json:"fingerprint,omitempty"`
}

// ResetOptions controls destructive behavior of ResetTaskEnvironment.
type ResetOptions struct {
	// PushBranch runs `git push` on the environment's current branch before teardown.
	// If the push fails, ResetTaskEnvironment aborts and the environment stays intact.
	PushBranch bool
}

// SessionRunningChecker reports whether a session has a live executor running.
// Used by ResetTaskEnvironment to block resets when the user still has an agent
// attached to the environment.
type SessionRunningChecker interface {
	IsAnySessionRunningForTask(ctx context.Context, taskID string) (bool, error)
}

// ErrSessionRunning is returned by ResetTaskEnvironment when at least one session
// on the task is still actively running and the caller must stop it first.
var ErrSessionRunning = errors.New("active session is running on this task; stop it before resetting the environment")

// ErrNoEnvironment is returned when the task has no TaskEnvironment to reset.
var ErrNoEnvironment = errors.New("no environment exists for this task")

// SetEnvironmentDestroyer wires the runtime-resource destroyer used by ResetTaskEnvironment.
func (s *Service) SetEnvironmentDestroyer(d EnvironmentDestroyer) {
	s.envDestroyer = d
}

// SetSessionRunningChecker wires a custom running-session guard. When unset, the
// service falls back to a default implementation over the existing session repo.
func (s *Service) SetSessionRunningChecker(c SessionRunningChecker) {
	s.sessionRunningChecker = c
}

// GetTaskEnvironmentLiveStatus returns a real-time snapshot of the task's
// container, plus the recorded TaskEnvironment row. Returns (nil, nil) when
// the task has no environment yet. Callers (e.g. Executor Settings popover)
// poll this endpoint to surface live state changes.
func (s *Service) GetTaskEnvironmentLiveStatus(ctx context.Context, taskID string) (*ContainerLiveStatus, error) {
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil || env == nil {
		return nil, err
	}
	if env.ContainerID == "" || s.envDestroyer == nil {
		return nil, nil
	}
	return s.envDestroyer.GetContainerLiveStatus(ctx, env.ContainerID)
}

// GetSSHLiveStatus returns the SSH-specific runtime info for the task's
// most recent session, or (nil, nil) when the task has no environment yet
// or the environment isn't SSH-backed. The SSH executor writes these
// fields into ExecutorRunning.Metadata in CreateInstance; here we just
// look up the latest session's row and project the keys back out. We
// don't probe the remote here — that's GetRemoteStatus's job — because
// the popover polls this endpoint and a per-poll SSH dial would be both
// slow and a lot of TCP traffic.
func (s *Service) GetSSHLiveStatus(ctx context.Context, taskID string) (*SSHLiveStatus, error) {
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil || env == nil {
		return nil, err
	}
	if env.ExecutorType != string(models.ExecutorTypeSSH) {
		return nil, nil
	}
	running := s.latestRunningForTask(ctx, taskID)
	if running == nil || running.Metadata == nil {
		return nil, nil
	}
	return buildSSHLiveStatus(running.Metadata), nil
}

// latestRunningForTask picks the newest ExecutorRunning row across the
// task's sessions. Sessions are returned newest-first by the repository
// today; if that ever changes the popover would silently surface stale
// data, so this helper is defensive: it walks every session and keeps
// the row with the latest StartedAt.
func (s *Service) latestRunningForTask(ctx context.Context, taskID string) *models.ExecutorRunning {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil
	}
	var latest *models.ExecutorRunning
	for _, sess := range sessions {
		if sess == nil {
			continue
		}
		running, err := s.executors.GetExecutorRunningBySessionID(ctx, sess.ID)
		if err != nil || running == nil {
			continue
		}
		if latest == nil || running.CreatedAt.After(latest.CreatedAt) {
			latest = running
		}
	}
	return latest
}

// buildSSHLiveStatus projects the SSH metadata keys onto the live-status
// shape. Pure function so the projection contract is unit-testable.
func buildSSHLiveStatus(md map[string]interface{}) *SSHLiveStatus {
	status := &SSHLiveStatus{
		Host:          mdString(md, "ssh_host"),
		User:          mdString(md, "ssh_user"),
		RemoteTaskDir: mdString(md, "ssh_remote_task_dir"),
		Fingerprint:   mdString(md, "ssh_host_fingerprint"),
	}
	status.Port = mdInt(md, "ssh_port")
	status.RemoteAgentctlPID = mdInt(md, "ssh_remote_agentctl_pid")
	status.RemoteAgentctlPort = mdInt(md, "ssh_remote_agentctl_port")
	status.LocalForwardPort = mdInt(md, "ssh_local_forward_port")
	return status
}

func mdString(md map[string]interface{}, key string) string {
	if v, ok := md[key].(string); ok {
		return v
	}
	return ""
}

func mdInt(md map[string]interface{}, key string) int {
	switch v := md[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		// SSH executor stores numeric metadata as strings (strconv.Itoa) so
		// every JSON round-trip preserves the value verbatim. Tolerate both.
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

// checkAnySessionRunning delegates to the configured checker or falls back to
// the default (session has an ExecutorRunning row).
func (s *Service) checkAnySessionRunning(ctx context.Context, taskID string) (bool, error) {
	if s.sessionRunningChecker != nil {
		return s.sessionRunningChecker.IsAnySessionRunningForTask(ctx, taskID)
	}
	return s.isAnySessionRunning(ctx, taskID)
}

// isAnySessionRunning is the default SessionRunningChecker: a session is running
// if it has an ExecutorRunning row and is actively starting or processing a turn.
// Idle sessions can keep an executor row so terminals and resumed prompts have
// a live workspace, but Reset Environment must still be able to tear that
// workspace down once the agent is waiting for input.
func (s *Service) isAnySessionRunning(ctx context.Context, taskID string) (bool, error) {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, sess := range sessions {
		if sess == nil {
			continue
		}
		running, err := s.executors.GetExecutorRunningBySessionID(ctx, sess.ID)
		if err != nil {
			if errors.Is(err, models.ErrExecutorRunningNotFound) {
				continue
			}
			return false, err
		}
		if running != nil && sessionBlocksEnvironmentReset(sess.State) {
			return true, nil
		}
	}
	return false, nil
}

func sessionBlocksEnvironmentReset(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateStarting || state == models.TaskSessionStateRunning
}

// GetTaskEnvironmentByTaskID returns the active task environment for a task.
// Returns nil if no environment exists yet.
func (s *Service) GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error) {
	return s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
}

// ResetTaskEnvironment tears down the task's current environment (container/sandbox/worktree)
// and deletes the TaskEnvironment row, so the next session launch starts fresh.
//
// Guards:
//   - No environment → ErrNoEnvironment
//   - Any session on the task is actively running → ErrSessionRunning
//
// Cleanup is best-effort per resource. If any resource fails to destroy, the
// TaskEnvironment row is preserved so the user can retry. Success deletes the row.
//
// If opts.PushBranch is set, the branch is pushed before teardown; a failed push
// aborts the reset and leaves the environment intact so the user can investigate.
func (s *Service) ResetTaskEnvironment(ctx context.Context, taskID string, opts ResetOptions) error {
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("lookup environment: %w", err)
	}
	if env == nil {
		return ErrNoEnvironment
	}

	// Fail closed: if the running-session check itself errors (DB hiccup,
	// locked table) we cannot prove the task is idle and must abort rather
	// than risk destroying a container or sandbox while an agent is still
	// writing to it.
	running, err := s.checkAnySessionRunning(ctx, taskID)
	if err != nil {
		return fmt.Errorf("check running sessions before reset: %w", err)
	}
	if running {
		return ErrSessionRunning
	}

	s.logger.Info("resetting task environment",
		zap.String("task_id", taskID),
		zap.String("env_id", env.ID),
		zap.String("executor_type", env.ExecutorType),
		zap.String("container_id", env.ContainerID),
		zap.String("sandbox_id", env.SandboxID),
		zap.String("worktree_id", env.WorktreeID),
		zap.Bool("push_branch", opts.PushBranch))

	if opts.PushBranch {
		if s.envDestroyer == nil {
			return fmt.Errorf("environment destroyer not configured; cannot push branch")
		}
		if err := s.envDestroyer.PushEnvironmentBranch(ctx, env); err != nil {
			return fmt.Errorf("push branch before reset: %w", err)
		}
	}

	if err := s.teardownEnvironmentResources(ctx, env); err != nil {
		return err
	}

	if err := s.taskEnvironments.DeleteTaskEnvironment(ctx, env.ID); err != nil {
		return fmt.Errorf("delete task environment row: %w", err)
	}
	s.logger.Info("task environment reset complete",
		zap.String("task_id", taskID),
		zap.String("env_id", env.ID))
	return nil
}

// teardownEnvironmentResources destroys runtime resources (container/sandbox/worktree)
// recorded on a TaskEnvironment. Best-effort per resource: every resource is
// attempted even when an earlier one fails, and all failures are joined into a
// single error so a stuck container can't permanently orphan the worktree.
// On any error, the caller should preserve the row so the user can retry.
func (s *Service) teardownEnvironmentResources(ctx context.Context, env *models.TaskEnvironment) error {
	if env.ContainerID == "" && env.SandboxID == "" && env.WorktreeID == "" {
		return nil
	}
	if s.envDestroyer == nil {
		return fmt.Errorf("environment destroyer not configured; cannot tear down resources")
	}

	var errs []error
	if env.ContainerID != "" {
		if err := s.envDestroyer.DestroyContainer(ctx, env.ContainerID); err != nil {
			errs = append(errs, fmt.Errorf("destroy container %s: %w", env.ContainerID, err))
		}
	}
	if env.SandboxID != "" {
		if err := s.envDestroyer.DestroySandbox(ctx, env.SandboxID, ""); err != nil {
			errs = append(errs, fmt.Errorf("destroy sandbox %s: %w", env.SandboxID, err))
		}
	}
	if env.WorktreeID != "" {
		if err := s.envDestroyer.DestroyWorktree(ctx, env.WorktreeID); err != nil {
			errs = append(errs, fmt.Errorf("destroy worktree %s: %w", env.WorktreeID, err))
		}
	}
	return errors.Join(errs...)
}
