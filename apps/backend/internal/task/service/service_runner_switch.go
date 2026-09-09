package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ErrRunnerSwitchMalformed reports that id or executor_profile_id was blank:
// absent, empty, or whitespace-only. Checked before any lookup.
var ErrRunnerSwitchMalformed = errors.New("id and executor_profile_id are required")

// ErrExecutorProfileInvalid reports that the named executor profile does not
// exist, or its executor is soft-deleted or not active.
var ErrExecutorProfileInvalid = errors.New("executor profile is invalid or not available")

// runnerCloneURLResolutionTimeout bounds the local-checkout `git remote
// get-url origin` fallback the compatibility gate runs before opening the
// switch transaction, so a stalled subprocess cannot hold up every
// session-creation path serialized against the same task row lock.
const runnerCloneURLResolutionTimeout = 5 * time.Second

// RunnerMutabilityView is the derived runner-mutability verdict for one
// task. Mirrors models.RunnerMutabilityVerdict without exposing repository
// batching details to callers.
type RunnerMutabilityView struct {
	Editable bool
	Reason   string
}

// WorkspaceGroupMembershipReader is the narrow slice of the office
// workspace-group repository the runner-mutability evaluator and switch
// action need: whether a task currently holds an active (non-released)
// group membership. Existence-only, so it never needs office's
// WorkspaceGroup model. Satisfied structurally by *officesqlite.Repository.
type WorkspaceGroupMembershipReader interface {
	HasWorkspaceGroupForTask(ctx context.Context, taskID string) (bool, error)
	GetActiveWorkspaceGroupTaskIDs(ctx context.Context, taskIDs []string) (map[string]bool, error)
}

// SetWorkspaceGroupMembershipReader wires the office-owned workspace-group
// membership reads the runner-mutability gate needs (condition 9) but the
// task repository cannot reach directly. Optional: when unset, every task is
// treated as not a group member, matching a workspace with no Office groups.
func (s *Service) SetWorkspaceGroupMembershipReader(r WorkspaceGroupMembershipReader) {
	s.wsGroupMembership = r
}

// ExecutorCapabilityProber answers whether an executor type's runtime
// requires a git clone URL to materialize a workspace. Satisfied directly by
// *lifecycle.Manager.
type ExecutorCapabilityProber interface {
	RequiresCloneURL(executorType string) bool
}

// SetExecutorCapabilityProber wires the executor-type capability probe the
// runner switch's compatibility gate uses. Optional: when unset, the gate
// treats every executor type as not requiring a clone URL, so the switch
// never blocks on it.
func (s *Service) SetExecutorCapabilityProber(p ExecutorCapabilityProber) {
	s.executorCapabilityProber = p
}

// BuildRunnerMutabilityViews computes the runner-mutability verdict for a
// batch of tasks in a bounded number of queries, mirroring
// BuildDependencyViews: one batched read per signal, never a per-task
// fan-out.
//
// A failing signal read degrades every task in the batch to
// evaluation_unavailable: every signal query here covers every task in the
// batch it was given, so a query-level failure has no narrower scope to
// degrade to.
func (s *Service) BuildRunnerMutabilityViews(ctx context.Context, tasks []*models.Task) map[string]RunnerMutabilityView {
	out := make(map[string]RunnerMutabilityView, len(tasks))
	if len(tasks) == 0 {
		return out
	}
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t != nil {
			ids = append(ids, t.ID)
		}
	}

	unavailable := func() map[string]RunnerMutabilityView {
		for _, id := range ids {
			out[id] = RunnerMutabilityView{Reason: models.RunnerReasonEvaluationUnavailable}
		}
		return out
	}

	// A caller that constructs a Service with only the repositories its own
	// path needs (every isolated-service test does this) leaves some of
	// these unwired. That is a normal configuration, not a bug, so it
	// degrades the same way a failed read does rather than panicking.
	if s.taskRepos == nil || s.sessions == nil || s.taskEnvironments == nil ||
		s.executors == nil || s.workspaceFolders == nil {
		return unavailable()
	}

	batch, err := s.loadRunnerMutabilitySignalBatch(ctx, ids)
	if err != nil {
		s.logger.Warn("failed to load signals for runner mutability", zap.Error(err))
		return unavailable()
	}

	for _, t := range tasks {
		if t == nil {
			continue
		}
		out[t.ID] = batch.verdictFor(t)
	}
	return out
}

// runnerMutabilitySignalBatch holds every batched signal read
// BuildRunnerMutabilityViews needs, keyed by task ID, so the caller can
// collapse five separate read-or-degrade steps into one error check.
type runnerMutabilitySignalBatch struct {
	repoLinks     map[string][]*models.TaskRepository
	sessionCounts map[string]int
	envExists     map[string]bool
	execExists    map[string]bool
	folders       map[string][]*models.TaskWorkspaceFolder
	groupMembers  map[string]bool
}

// loadRunnerMutabilitySignalBatch runs the five (or six, with an office
// reader wired) batched signal reads and stops at the first failure — every
// query here covers the whole of ids, so partial success has no meaningful
// partial result to keep.
func (s *Service) loadRunnerMutabilitySignalBatch(ctx context.Context, ids []string) (runnerMutabilitySignalBatch, error) {
	var batch runnerMutabilitySignalBatch
	var err error

	batch.repoLinks, err = s.taskRepos.ListTaskRepositoriesByTaskIDs(ctx, ids)
	if err != nil {
		return runnerMutabilitySignalBatch{}, err
	}
	batch.sessionCounts, err = s.sessions.GetSessionCountsByTaskIDs(ctx, ids)
	if err != nil {
		return runnerMutabilitySignalBatch{}, err
	}
	batch.envExists, err = s.taskEnvironments.GetTaskEnvironmentExistenceByTaskIDs(ctx, ids)
	if err != nil {
		return runnerMutabilitySignalBatch{}, err
	}
	batch.execExists, err = s.executors.GetExecutorRunningExistenceByTaskIDs(ctx, ids)
	if err != nil {
		return runnerMutabilitySignalBatch{}, err
	}
	batch.folders, err = s.workspaceFolders.ListTaskWorkspaceFoldersByTaskIDs(ctx, ids)
	if err != nil {
		return runnerMutabilitySignalBatch{}, err
	}
	batch.groupMembers = map[string]bool{}
	if s.wsGroupMembership != nil {
		batch.groupMembers, err = s.wsGroupMembership.GetActiveWorkspaceGroupTaskIDs(ctx, ids)
		if err != nil {
			return runnerMutabilitySignalBatch{}, err
		}
	}
	return batch, nil
}

// verdictFor evaluates one task's mutability verdict against the batch's
// already-loaded signals.
func (b runnerMutabilitySignalBatch) verdictFor(t *models.Task) RunnerMutabilityView {
	signals := models.RunnerSignalsFromTask(t)
	signals.RepositoryCount = len(b.repoLinks[t.ID])
	signals.HasSession = b.sessionCounts[t.ID] > 0
	signals.HasEnvironment = b.envExists[t.ID]
	signals.HasExecutorRunning = b.execExists[t.ID]
	signals.HasWorkspaceFolder = len(b.folders[t.ID]) > 0
	signals.HasActiveGroupMembership = b.groupMembers[t.ID]
	verdict := models.EvaluateRunnerMutability(signals)
	return RunnerMutabilityView{Editable: verdict.Editable, Reason: verdict.Reason}
}

// runnerMutabilityEventView is the batch-of-one precedent
// dependencyEventFields sets: compute the verdict for a single just-touched
// task so publishTaskEventNow can stamp it unconditionally on every event.
func (s *Service) runnerMutabilityEventView(ctx context.Context, task *models.Task) RunnerMutabilityView {
	views := s.BuildRunnerMutabilityViews(ctx, []*models.Task{task})
	return views[task.ID]
}

// SwitchTaskRunner authorizes the caller, resolves the compatibility gate
// outside any transaction, and applies the switch through the task
// repository's single locked transaction. Outcome precedence, checked in
// order: malformed request, not found, not authorized, target invalid,
// mutability conflict, compatibility conflict, then assignment.
func (s *Service) SwitchTaskRunner(ctx context.Context, taskID, executorProfileID string) (*models.Task, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(executorProfileID) == "" {
		return nil, ErrRunnerSwitchMalformed
	}

	// Not-found is checked before authorization, and independently of it:
	// authorizeTaskScope only performs its own existence check for a scoped
	// (identity-bearing) caller, so an unscoped internal caller would
	// otherwise skip the not-found stage entirely.
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}

	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}

	executor, err := s.resolveExecutorForProfile(ctx, executorProfileID)
	if err != nil {
		return nil, err
	}

	compat, err := s.resolveRunnerCompatibility(ctx, taskID, executor)
	if err != nil {
		return nil, err
	}

	req := models.RunnerSwitchRequest{
		TaskID:                      taskID,
		ExecutorProfileID:           executorProfileID,
		CompatibilityApplicable:     compat.applicable,
		CompatibilityChecked:        compat.checked,
		CompatibilityCloneURLFound:  compat.cloneURLFound,
		ResolvedRepositoryID:        compat.repositoryID,
		ResolvedRepositoryUpdatedAt: compat.repositoryUpdatedAt,
		GroupMembershipChecker:      s.runnerGroupMembershipChecker,
	}

	result, err := s.tasks.SwitchTaskRunner(ctx, req)
	if err != nil {
		return nil, err
	}

	if result.Changed {
		previous, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)
		s.logger.Info("task runner switched",
			zap.String("task_id", taskID),
			zap.String("previous_executor_profile_id", previous),
			zap.String("executor_profile_id", executorProfileID))
		s.PublishTaskUpdated(ctx, result.Task)
	}
	return result.Task, nil
}

func (s *Service) runnerGroupMembershipChecker(ctx context.Context, taskID string) (bool, error) {
	if s.wsGroupMembership == nil {
		return false, nil
	}
	return s.wsGroupMembership.HasWorkspaceGroupForTask(ctx, taskID)
}

// resolveExecutorForProfile resolves and validates the target executor: it
// is invalid when its profile does not exist or its executor is
// soft-deleted or not active. Both produce the same invalid outcome.
func (s *Service) resolveExecutorForProfile(ctx context.Context, executorProfileID string) (*models.Executor, error) {
	profile, err := s.executors.GetExecutorProfile(ctx, executorProfileID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrExecutorProfileNotFound) {
			return nil, ErrExecutorProfileInvalid
		}
		return nil, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}
	executor, err := s.executors.GetExecutor(ctx, profile.ExecutorID)
	if err != nil {
		if errors.Is(err, models.ErrExecutorNotFound) {
			return nil, ErrExecutorProfileInvalid
		}
		return nil, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}
	if executor.Status != models.ExecutorStatusActive {
		return nil, ErrExecutorProfileInvalid
	}
	return executor, nil
}

// runnerCompatibilityResolution carries the compatibility gate's
// pre-transaction resolution into the request the repository layer
// re-confirms inside the locked transaction.
type runnerCompatibilityResolution struct {
	// applicable is true whenever the target executor type requires a clone
	// URL, independent of whether checked below is also true. The repository
	// layer uses this to tell "the gate never applied to this executor" from
	// "the gate applied but the repository shape did not allow resolution",
	// which must be re-validated rather than silently skipped.
	applicable          bool
	checked             bool
	cloneURLFound       bool
	repositoryID        string
	repositoryUpdatedAt time.Time
}

// resolveRunnerCompatibility resolves the compatibility gate's verdict: it
// runs entirely outside any transaction, so a subprocess call here never
// holds the task row lock. When the target executor does not require a
// clone URL, resolution is skipped and inapplicable — the mutability gate
// reports the correct code for that shape at its own ordered stage. When the
// task does not have exactly one repository attachment right now, resolution
// is skipped but stays applicable, so the repository layer re-checks the
// shape from inside the transaction rather than trusting a stale skip. A
// candidate lookup that errors or times out aborts the whole switch as
// evaluation_unavailable immediately, rather than being carried forward as a
// "no URL found" verdict.
func (s *Service) resolveRunnerCompatibility(ctx context.Context, taskID string, executor *models.Executor) (runnerCompatibilityResolution, error) {
	if s.executorCapabilityProber == nil || !s.executorCapabilityProber.RequiresCloneURL(string(executor.Type)) {
		return runnerCompatibilityResolution{}, nil
	}

	links, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return runnerCompatibilityResolution{}, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}
	if len(links) != 1 {
		return runnerCompatibilityResolution{applicable: true}, nil
	}
	link := links[0]
	repo, err := s.repoEntities.GetRepository(ctx, link.RepositoryID)
	if err != nil {
		return runnerCompatibilityResolution{}, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}

	found, err := runnerRepositoryHasCloneURL(ctx, repo)
	if err != nil {
		return runnerCompatibilityResolution{}, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}
	return runnerCompatibilityResolution{
		applicable:          true,
		checked:             true,
		cloneURLFound:       found,
		repositoryID:        link.RepositoryID,
		repositoryUpdatedAt: link.UpdatedAt,
	}, nil
}

// runnerRepositoryHasCloneURL resolves candidates in the launch path's order
// — a recorded remote URL, then a provider-derived URL, then the local
// checkout's origin remote — but, unlike that path's own helper, distinguishes
// an exhausted candidate list (false, nil) from a failed or timed-out lookup
// (false, non-nil error). Git reporting a negative answer (e.g. "no such
// remote 'origin'") is an exhausted candidate; failing to launch git at all,
// or exceeding the bounded timeout, is a failed lookup.
func runnerRepositoryHasCloneURL(ctx context.Context, repo *models.Repository) (bool, error) {
	if strings.TrimSpace(repo.RemoteURL) != "" {
		return true, nil
	}
	if repo.ProviderOwner != "" && repo.ProviderName != "" &&
		(!strings.EqualFold(repo.Provider, "gitlab") || strings.TrimSpace(repo.ProviderHost) != "") {
		cloneURL, err := repoclone.CloneURLWithHost(
			repo.Provider, repo.ProviderHost, repo.ProviderOwner, repo.ProviderName, repoclone.ProtocolHTTPS,
		)
		if err == nil && strings.TrimSpace(cloneURL) != "" {
			return true, nil
		}
	}
	if repo.LocalPath == "" {
		return false, nil
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, runnerCloneURLResolutionTimeout)
	defer cancel()
	cmd := subproc.NewGitCommand(timeoutCtx, "-C", repo.LocalPath, "remote", "get-url", "origin")
	out, err := subproc.RunGitOutputClass(timeoutCtx, subproc.GitLifecycle, cmd)
	if err != nil {
		if timeoutCtx.Err() != nil {
			return false, timeoutCtx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}
