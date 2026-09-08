package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// SSH executor metadata keys read off executor_running rows when a terminal
// task cleanup decides whether it may reclaim the remote task directory. The
// canonical definitions live in internal/agent/runtime/lifecycle; they are
// re-declared here as literals because the task service does not import the
// lifecycle package, the same way buildSSHLiveStatus already reads them.
const (
	sshMetaHost            = "ssh_host"
	sshMetaPort            = "ssh_port"
	sshMetaUser            = "ssh_user"
	sshMetaHostFingerprint = "ssh_host_fingerprint"
	sshMetaIdentitySource  = "ssh_identity_source"
	sshMetaIdentityFile    = "ssh_identity_file"
	sshMetaProxyJump       = "ssh_proxy_jump"
	sshMetaShell           = "ssh_shell"
	sshMetaWorkdirRoot     = "ssh_workdir_root"
	sshMetaRemoteTaskDir   = "ssh_remote_task_dir"
	sshMetaReclaimTaskDir  = "ssh_reclaim_task_dir"

	// sshReclaimEnabledValue is the only value that opts a profile in. It is
	// compared, never displayed, and is written by the executor-profile
	// projection as an authoritative key.
	sshReclaimEnabledValue = "true"
)

// Skip reasons decided by this phase. The probe-decided reasons
// (dirty_worktree, unpushed_commits, stash_present, not_a_checkout,
// probe_failed) are produced by the reclaimer and passed through verbatim.
//
// Every skip is a *success* for the cleanup job: the directory is preserved on
// purpose, and retrying would not change the verdict.
const (
	sshReclaimSkipDisabled = "disabled"
	sshReclaimSkipShared   = "shared"
)

// sshReclaimTarget is the durable snapshot record of one remote task directory
// that a terminal cleanup may reclaim. It is captured before the task rows are
// mutated so the job stays self-contained across a backend restart, exactly
// like the session, worktree and environment records beside it.
type sshReclaimTarget struct {
	Host            string `json:"host"`
	Port            int    `json:"port,omitempty"`
	User            string `json:"user,omitempty"`
	HostFingerprint string `json:"host_fingerprint,omitempty"`
	IdentitySource  string `json:"identity_source,omitempty"`
	IdentityFile    string `json:"identity_file,omitempty"`
	ProxyJump       string `json:"proxy_jump,omitempty"`
	Shell           string `json:"shell,omitempty"`
	WorkdirRoot     string `json:"workdir_root"`
	TaskDir         string `json:"task_dir"`
	// Enabled records the profile's opt-in as it stood when the directory was
	// launched. The profile is the sole source of truth; a task can never set
	// it, because the executor projection treats the key as authoritative.
	Enabled bool `json:"enabled,omitempty"`
	// Outcome, SkipReason, and Detail are filled after the cleanup attempt so a
	// successful safety skip remains discoverable in the durable job snapshot.
	Outcome    string `json:"outcome,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// sameRemoteDir reports whether two targets name the same directory on the
// same host. Used to detect a directory another task is still using.
func (t sshReclaimTarget) sameRemoteDir(other sshReclaimTarget) bool {
	sameHost := strings.EqualFold(t.Host, other.Host) ||
		(t.HostFingerprint != "" && t.HostFingerprint == other.HostFingerprint)
	return sameHost &&
		t.Port == other.Port &&
		t.User == other.User &&
		t.TaskDir == other.TaskDir
}

// usable reports whether the target names a directory at all. WorkdirRoot is
// allowed to be empty: a profile that leaves the workspace root blank launches
// into the SSH executor's default, and the reclaimer applies the same default
// (and the same tilde expansion) rather than the service duplicating it.
func (t sshReclaimTarget) usable() bool {
	return t.Host != "" && t.TaskDir != ""
}

// SSHTaskDirReclaimRequest is the task service's own description of one
// reclamation attempt. It deliberately mirrors the SSH connection tuple rather
// than importing the lifecycle types, keeping the dependency one-directional.
type SSHTaskDirReclaimRequest struct {
	Host            string
	Port            int
	User            string
	HostFingerprint string
	IdentitySource  string
	IdentityFile    string
	ProxyJump       string
	Shell           string
	WorkdirRoot     string
	TaskDir         string
}

// SSHTaskDirReclaimResult reports what an attempt did. Removed and SkipReason
// are mutually exclusive: a non-removal always carries a reason.
type SSHTaskDirReclaimResult struct {
	Removed    bool
	SkipReason string
	Detail     string
}

// SSHTaskDirReclaimer removes a remote task directory after establishing that
// nothing in it is at risk. Implemented by an adapter over the SSH executor's
// reclaimer; nil when no SSH executor is wired, which disables the phase.
type SSHTaskDirReclaimer interface {
	ReclaimTaskDir(ctx context.Context, req SSHTaskDirReclaimRequest) (SSHTaskDirReclaimResult, error)
}

// SetSSHTaskDirReclaimer wires the remote task-directory reclaimer used by the
// durable task-resource cleanup job.
func (s *Service) SetSSHTaskDirReclaimer(r SSHTaskDirReclaimer) {
	s.sshTaskDirReclaimer = r
}

// gatherSSHReclaimTargets captures the remote task directories of a task's
// launched sessions, de-duplicated by (host, port, user, task dir) so a
// multi-session task yields one target and a task spanning two hosts yields
// two. Called while the executor_running rows still exist.
func (s *Service) gatherSSHReclaimTargets(ctx context.Context, taskID string) ([]sshReclaimTarget, error) {
	if s.executors == nil {
		return nil, nil
	}
	rows, err := s.executors.ListExecutorsRunningByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	targets := make([]sshReclaimTarget, 0, len(rows))
	for _, running := range rows {
		if running == nil {
			continue
		}
		target := sshReclaimTargetFromMetadata(running.Metadata)
		if !target.usable() {
			continue
		}
		duplicate := false
		for index := range targets {
			if targets[index].sameRemoteDir(target) {
				targets[index].Enabled = targets[index].Enabled && target.Enabled
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return nil, nil
	}
	return targets, nil
}

func containsRemoteDir(targets []sshReclaimTarget, candidate sshReclaimTarget) bool {
	for _, existing := range targets {
		if existing.sameRemoteDir(candidate) {
			return true
		}
	}
	return false
}

// sshReclaimTargetFromMetadata projects an executor_running metadata map onto a
// reclamation target. Pure function so the projection contract is unit-testable.
func sshReclaimTargetFromMetadata(md map[string]interface{}) sshReclaimTarget {
	return sshReclaimTarget{
		Host:            strings.TrimSpace(mdString(md, sshMetaHost)),
		Port:            mdInt(md, sshMetaPort),
		User:            strings.TrimSpace(mdString(md, sshMetaUser)),
		HostFingerprint: strings.TrimSpace(mdString(md, sshMetaHostFingerprint)),
		IdentitySource:  strings.TrimSpace(mdString(md, sshMetaIdentitySource)),
		IdentityFile:    strings.TrimSpace(mdString(md, sshMetaIdentityFile)),
		ProxyJump:       strings.TrimSpace(mdString(md, sshMetaProxyJump)),
		Shell:           strings.TrimSpace(mdString(md, sshMetaShell)),
		WorkdirRoot:     strings.TrimSpace(mdString(md, sshMetaWorkdirRoot)),
		TaskDir:         strings.TrimSpace(mdString(md, sshMetaRemoteTaskDir)),
		Enabled:         strings.TrimSpace(mdString(md, sshMetaReclaimTaskDir)) == sshReclaimEnabledValue,
	}
}

// Reclamation outcomes recorded on the structured log line so an operator can
// tell a deliberate preservation from a broken probe.
const (
	sshReclaimOutcomeRemoved = "removed"
	sshReclaimOutcomeSkipped = "skipped"
	sshReclaimOutcomeFailed  = "failed"
)

// reclaimSSHTaskDirs is the reclamation phase of the durable task-resource
// cleanup job. It runs after every runtime stop has succeeded and after
// performTaskCleanup, so the profile's cleanup_script has already run over the
// live checkout and local cleanup can never be blocked by a remote failure.
//
// It returns errors, not a boolean: a failed removal folds into the job's error
// set so the existing backoff, last_error and failed-state handling applies. A
// skip returns no error, because a directory preserved on purpose is a correct
// outcome and retrying would not change the verdict.
func (s *Service) reclaimSSHTaskDirs(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
	snapshot *taskResourceCleanupSnapshot,
) []error {
	if s.sshTaskDirReclaimer == nil || snapshot == nil || len(snapshot.SSHTaskDirs) == 0 {
		return nil
	}
	// Never begin an irreversible remote removal on a context that is already
	// going away. Backend shutdown cancels the cleanup worker mid-job; the job
	// row survives and the phase runs on the next attempt with a live context.
	// The guard lives here rather than at the call site so a later reordering
	// of executeTaskResourceCleanupJob cannot quietly remove it.
	if cause := context.Cause(ctx); cause != nil {
		return nil
	}
	owned, ownershipDetail := s.taskOwnsItsWorkspace(ctx, job.TaskID, snapshot.TaskEnvironment)
	claims, claimErr := s.remoteTaskDirsClaimedByOtherTasks(ctx, job.TaskID)
	var errs []error
	for index := range snapshot.SSHTaskDirs {
		target := &snapshot.SSHTaskDirs[index]
		switch {
		case !target.Enabled:
			s.recordSSHReclaimOutcome(target, sshReclaimOutcomeSkipped, sshReclaimSkipDisabled, "")
			s.logSSHReclaim(job.TaskID, *target, sshReclaimOutcomeSkipped, sshReclaimSkipDisabled, "")
		case !owned:
			s.recordSSHReclaimOutcome(target, sshReclaimOutcomeSkipped, sshReclaimSkipShared, ownershipDetail)
			s.logSSHReclaim(job.TaskID, *target, sshReclaimOutcomeSkipped, sshReclaimSkipShared, ownershipDetail)
		case claimErr != nil:
			detail := "directory ownership lookup failed: " + claimErr.Error()
			s.recordSSHReclaimOutcome(target, sshReclaimOutcomeSkipped, sshReclaimSkipShared, detail)
			s.logSSHReclaim(job.TaskID, *target, sshReclaimOutcomeSkipped, sshReclaimSkipShared, detail)
		case containsRemoteDir(claims, *target):
			detail := "another task still holds this remote directory"
			s.recordSSHReclaimOutcome(target, sshReclaimOutcomeSkipped, sshReclaimSkipShared, detail)
			s.logSSHReclaim(job.TaskID, *target, sshReclaimOutcomeSkipped, sshReclaimSkipShared, detail)
		default:
			if err := s.reclaimOneSSHTaskDir(ctx, job.TaskID, target); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

func (s *Service) reclaimOneSSHTaskDir(ctx context.Context, taskID string, target *sshReclaimTarget) error {
	result, err := s.sshTaskDirReclaimer.ReclaimTaskDir(ctx, SSHTaskDirReclaimRequest{
		Host:            target.Host,
		Port:            target.Port,
		User:            target.User,
		HostFingerprint: target.HostFingerprint,
		IdentitySource:  target.IdentitySource,
		IdentityFile:    target.IdentityFile,
		ProxyJump:       target.ProxyJump,
		Shell:           target.Shell,
		WorkdirRoot:     target.WorkdirRoot,
		TaskDir:         target.TaskDir,
	})
	if err != nil {
		s.recordSSHReclaimOutcome(target, sshReclaimOutcomeFailed, "", err.Error())
		s.logSSHReclaim(taskID, *target, sshReclaimOutcomeFailed, "", err.Error())
		return fmt.Errorf("reclaim remote task dir %s on %s: %w", target.TaskDir, target.Host, err)
	}
	if result.Removed {
		s.recordSSHReclaimOutcome(target, sshReclaimOutcomeRemoved, "", result.Detail)
		s.logSSHReclaim(taskID, *target, sshReclaimOutcomeRemoved, "", result.Detail)
		return nil
	}
	s.recordSSHReclaimOutcome(target, sshReclaimOutcomeSkipped, result.SkipReason, result.Detail)
	s.logSSHReclaim(taskID, *target, sshReclaimOutcomeSkipped, result.SkipReason, result.Detail)
	return nil
}

func (s *Service) recordSSHReclaimOutcome(target *sshReclaimTarget, outcome, reason, detail string) {
	target.Outcome = outcome
	target.SkipReason = reason
	target.Detail = detail
}

// taskOwnsItsWorkspace reports whether the cleaned task owns the workspace the
// snapshot recorded, rather than borrowing one. An inherit_parent subtask binds
// its session to the parent's environment and therefore has no environment row
// of its own, so it fails the first check and its parent's directory survives.
//
// Any inconclusive answer, including a repository error, resolves to not owned.
func (s *Service) taskOwnsItsWorkspace(
	ctx context.Context,
	taskID string,
	env *models.TaskEnvironment,
) (bool, string) {
	if env == nil || strings.TrimSpace(env.ID) == "" {
		return false, "task has no environment of its own"
	}
	if strings.TrimSpace(env.TaskID) != taskID {
		return false, "environment is owned by another task"
	}
	borrowed, err := s.hasActiveOtherTaskSessionsForEnvironment(ctx, taskID, env)
	if err != nil {
		return false, "environment ownership check failed: " + err.Error()
	}
	if borrowed {
		return false, "another task still has active sessions on this environment"
	}
	return true, ""
}

// remoteTaskDirsClaimedByOtherTasks lists the remote directories still recorded
// against some other task's launched session. It is the cross-task analogue of
// CountActiveWorktreeReferences and catches sharing the environment row cannot
// see. An error is returned rather than an empty list so the caller skips.
func (s *Service) remoteTaskDirsClaimedByOtherTasks(
	ctx context.Context,
	taskID string,
) ([]sshReclaimTarget, error) {
	if s.executors == nil {
		return nil, errors.New("executor repository is unavailable")
	}
	rows, err := s.executors.ListExecutorsRunning(ctx)
	if err != nil {
		return nil, err
	}
	claims := make([]sshReclaimTarget, 0, len(rows))
	for _, running := range rows {
		if running == nil || running.TaskID == "" || running.TaskID == taskID {
			continue
		}
		target := sshReclaimTargetFromMetadata(running.Metadata)
		if target.TaskDir == "" {
			continue
		}
		claims = append(claims, target)
	}
	return claims, nil
}

func (s *Service) logSSHReclaim(taskID string, target sshReclaimTarget, outcome, reason, detail string) {
	fields := []zap.Field{
		zap.String("task_id", taskID),
		zap.String("host", target.Host),
		zap.String("remote_task_dir", target.TaskDir),
		zap.String("outcome", outcome),
	}
	if reason != "" {
		fields = append(fields, zap.String("reason", reason))
	}
	if detail != "" {
		fields = append(fields, zap.String("detail", detail))
	}
	if outcome == sshReclaimOutcomeFailed {
		s.logger.Warn("remote task directory reclamation failed", fields...)
		return
	}
	s.logger.Info("remote task directory reclamation", fields...)
}
