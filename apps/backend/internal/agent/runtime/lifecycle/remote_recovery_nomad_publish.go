package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

const (
	nomadRecoveryClassStarting       = "starting"
	nomadRecoveryClassOperatorAction = "operator_action"

	// NomadRecoveryCodeStarting and NomadRecoveryCodeOperatorAction are the
	// stable codes a guarded workspace read reports for a Created-session Nomad
	// recovery. Callers receive the code, never free text.
	NomadRecoveryCodeStarting       = "nomad_recovery_starting"
	NomadRecoveryCodeOperatorAction = "nomad_recovery_operator_action"

	// nomadRecoveryReconcileTimeout bounds the repository reconcile call. The
	// Created-Nomad attempt context is this plus two remoteRecoveryAttemptTimeout
	// windows, so a slow reconcile delays the shared remote status tick by up to
	// that long.
	nomadRecoveryReconcileTimeout = 2 * time.Minute
)

// NomadEnvironmentReadyPublication identifies the recovered operation whose
// environment is published ready. ExpectedRevision is the inventory revision
// read immediately before publishing.
type NomadEnvironmentReadyPublication struct {
	SessionID           string
	EnvironmentID       string
	ExecutionID         string
	OwnershipGeneration int64
	ExpectedRevision    uint64
	Repos               []*models.TaskEnvironmentRepo
}

// NomadEnvironmentReadyPublisher writes the creating-to-ready environment
// transition for a recovered Nomad operation. It reports false without writing
// when the environment is already published for the same operation.
type NomadEnvironmentReadyPublisher interface {
	PublishRecoveredNomadEnvironmentReady(ctx context.Context, params NomadEnvironmentReadyPublication) (bool, error)
}

// SetNomadEnvironmentReadyPublisher wires the durable environment publisher.
func (m *Manager) SetNomadEnvironmentReadyPublisher(publisher NomadEnvironmentReadyPublisher) {
	m.nomadReadyPublisher = publisher
}

// NomadRecoveryGuardError is returned for a workspace read refused by a
// Created-session Nomad recovery guard. It satisfies
// errors.Is(err, ErrSessionRecoveryGuarded) and carries a stable Code.
type NomadRecoveryGuardError struct {
	Code string
}

func (e *NomadRecoveryGuardError) Error() string {
	return "workspace recovery guarded: " + e.Code
}

// Is reports the recovery-guard sentinel so existing callers keep matching.
func (e *NomadRecoveryGuardError) Is(target error) bool {
	return target == ErrSessionRecoveryGuarded
}

// recoveryClassError pins a recovery attempt failure to an outcome class.
type recoveryClassError struct {
	class string
	err   error
}

func (e *recoveryClassError) Error() string { return e.err.Error() }
func (e *recoveryClassError) Unwrap() error { return e.err }

func operatorActionRecoveryError(err error) error {
	return &recoveryClassError{class: nomadRecoveryClassOperatorAction, err: err}
}

func (m *Manager) remoteAttemptTimeout() time.Duration {
	if m.remoteAttemptBudget > 0 {
		return m.remoteAttemptBudget
	}
	return remoteRecoveryAttemptTimeout
}

func (m *Manager) nomadReconcileTimeout() time.Duration {
	if m.nomadRecoveryReconcileBudget > 0 {
		return m.nomadRecoveryReconcileBudget
	}
	return nomadRecoveryReconcileTimeout
}

// isCreatedNomadRecoveryEntry reports whether the entry is a Nomad recovery for
// a Created (or not yet pinned) session.
func isCreatedNomadRecoveryEntry(entry *remoteRecoveryRetry) bool {
	return entry != nil && entry.record != nil && entry.record.Runtime == agentruntime.RuntimeNomad &&
		(entry.snapshot == nil || entry.snapshot.SessionState == models.TaskSessionStateCreated)
}

// createdNomadRecoveryClass returns the recorded outcome class of the last
// attempt, or "" before any attempt.
func (m *Manager) createdNomadRecoveryClass(entry *remoteRecoveryRetry) string {
	if entry == nil {
		return ""
	}
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	return entry.nomadClass
}

func (m *Manager) recordCreatedNomadRecoveryClass(entry *remoteRecoveryRetry, class string) {
	if entry == nil {
		return
	}
	m.remoteRecoveryMu.Lock()
	entry.nomadClass = class
	m.remoteRecoveryMu.Unlock()
}

func nomadRecoveryGuardCode(class string) string {
	if class == nomadRecoveryClassOperatorAction {
		return NomadRecoveryCodeOperatorAction
	}
	return NomadRecoveryCodeStarting
}

// classifyCreatedNomadRecoveryError maps an attempt failure to its outcome
// class. Fence and identity losses are operator_action; read, transport and
// attempt-window failures are starting. ctxErr is the attempt context's error.
func (m *Manager) classifyCreatedNomadRecoveryError(entry *remoteRecoveryRetry, err, ctxErr error) string {
	var classed *recoveryClassError
	var materializeErr *agentctl.WorkspaceMaterializationError
	switch {
	case errors.As(err, &classed):
		return classed.class
	case errors.Is(err, models.ErrExecutionRotated), errors.Is(err, models.ErrWorkspaceReuseUnsafe):
		return nomadRecoveryClassOperatorAction
	case errors.As(err, &materializeErr):
		if materializeErr.StatusCode == http.StatusConflict || materializeErr.StatusCode == http.StatusUnprocessableEntity {
			return nomadRecoveryClassOperatorAction
		}
	case errors.Is(err, errRemoteRecoveryFenced):
		if ctxErr == nil && m.createdNomadRecoveryEntryLost(entry) {
			return nomadRecoveryClassOperatorAction
		}
	}
	return nomadRecoveryClassStarting
}

// createdNomadRecoveryEntryLost reports a removed or replaced entry or a
// requested stop.
func (m *Manager) createdNomadRecoveryEntryLost(entry *remoteRecoveryRetry) bool {
	if entry == nil || entry.record == nil {
		return true
	}
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	return m.remoteRecoveryPending[entry.record.SessionID] != entry || entry.stopRequested
}

// createdNomadReadyInventory re-reads the durable inventory and requires phase
// ready. A pre-ready phase ends the attempt as starting; a post-ready phase is
// rejected by the monotonic re-read as an operator_action loss.
func (m *Manager) createdNomadReadyInventory(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	initial nomad.EnvironmentState,
) (nomad.EnvironmentState, error) {
	record, err := m.currentNomadPreACPRecoveryRecord(ctx, entry, initial)
	if err != nil {
		return nomad.EnvironmentState{}, err
	}
	state, ok := createdNomadRecoveryState(record)
	if !ok {
		return nomad.EnvironmentState{}, operatorActionRecoveryError(errors.New("pre-ACP Nomad inventory state is not recoverable"))
	}
	if state.Phase != nomadPhaseReady {
		return nomad.EnvironmentState{}, fmt.Errorf("pre-ACP Nomad inventory is in phase %q: %w", state.Phase, models.ErrNomadRecoveryNotReady)
	}
	return state, nil
}

// publishCreatedNomadRecoveredEnvironment publishes the environment ready under
// the in-flight guarded owner predicate. The caller keeps the recovery guard.
func (m *Manager) publishCreatedNomadRecoveredEnvironment(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	info *WorkspaceInfo,
	ready nomad.EnvironmentState,
) (bool, error) {
	publisher := m.nomadReadyPublisher
	if publisher == nil {
		return false, errors.New("nomad environment publisher is unavailable")
	}
	if err := m.createdNomadPublishAllowed(entry, info, ready); err != nil {
		return false, err
	}
	publishCtx, cancel := m.remoteAttemptContext(context.WithoutCancel(ctx))
	defer cancel()
	return publisher.PublishRecoveredNomadEnvironmentReady(publishCtx, NomadEnvironmentReadyPublication{
		SessionID: entry.record.SessionID, EnvironmentID: ready.EnvironmentID, ExecutionID: ready.ExecutionID,
		OwnershipGeneration: ready.Generation, ExpectedRevision: ready.Revision,
		Repos: createdNomadEnvironmentRepos(info),
	})
}

// createdNomadPublishAllowed re-proves the in-flight owner and compares the
// pinned entry to the freshly read inventory by operation identity plus
// monotonic revision and phase, so a first attempt that advanced the inventory
// is retried to success.
func (m *Manager) createdNomadPublishAllowed(entry *remoteRecoveryRetry, info *WorkspaceInfo, fresh nomad.EnvironmentState) error {
	entryState, err := m.createdNomadInFlightOwner(info.TaskID, info, entry)
	if err != nil {
		if m.createdNomadRecoveryEntryLost(entry) {
			return fmt.Errorf("%w: %w", errRemoteRecoveryFenced, err)
		}
		if createdNomadRecoveryOwnerMatches(info.TaskID, info, entry) && !m.createdNomadRecoveryEntryActive(entry) {
			// Only the attempt window ended; the next tick retries from the pinned entry.
			return err
		}
		return operatorActionRecoveryError(err)
	}
	if !sameNomadPreACPOperation(entryState, fresh) || fresh.Revision < entryState.Revision ||
		nomadRecoveryPhaseRank(fresh.Phase) < nomadRecoveryPhaseRank(entryState.Phase) {
		return operatorActionRecoveryError(errors.New("pre-ACP Nomad inventory operation or phase changed before publication"))
	}
	return nil
}

// requireCreatedNomadEnvironmentReady re-reads the environment and requires the
// published ready state for this operation.
func (m *Manager) requireCreatedNomadEnvironmentReady(ctx context.Context, info *WorkspaceInfo, state nomad.EnvironmentState) error {
	if m.executorProfileReader == nil {
		return errors.New("nomad environment reader is unavailable")
	}
	env, err := m.executorProfileReader.GetTaskEnvironment(ctx, info.TaskEnvironmentID)
	if err != nil {
		return fmt.Errorf("re-read published Nomad environment: %w", err)
	}
	if env == nil || env.ID != state.EnvironmentID || env.OwnershipGeneration != state.Generation ||
		env.ExecutorType != string(models.ExecutorTypeNomad) {
		return operatorActionRecoveryError(errors.New("published Nomad environment identity changed"))
	}
	if env.Status != models.TaskEnvironmentStatusReady {
		return fmt.Errorf("published Nomad environment is %q, want ready", env.Status)
	}
	return nil
}

// createdNomadEnvironmentRepos projects the canonical workspace repositories
// into environment rows exactly as the original clone-based launch did. The
// branch slug copies the orchestrator's launchRepoBranchIdentitySlug.
func createdNomadEnvironmentRepos(info *WorkspaceInfo) []*models.TaskEnvironmentRepo {
	repos := make([]*models.TaskEnvironmentRepo, 0, len(info.WorkspaceRepositories))
	for position, spec := range info.WorkspaceRepositories {
		if spec.RepositoryID == "" {
			continue
		}
		repos = append(repos, &models.TaskEnvironmentRepo{
			RepositoryID:          spec.RepositoryID,
			BranchSlug:            NomadRecoveryRepoBranchSlug(spec.BranchIdentitySlug, spec.BranchSlug),
			WorkspaceRelativePath: spec.WorkspaceRelativePath,
			Position:              position,
		})
	}
	return repos
}

// NomadRecoveryRepoBranchSlug is the recovery projection of a repository branch
// identity; it must equal the launch path's slug so first-prompt row matching
// never duplicates rows.
func NomadRecoveryRepoBranchSlug(identity, branch string) string {
	if identity != "" {
		return worktree.SanitizeBranchSlug(identity)
	}
	return worktree.SanitizeBranchSlug(branch)
}

// reconcileCreatedNomadRepositories reruns repository materialization against
// the attached instance. A valid existing checkout is reused untouched; any
// other existing destination is a collision the caller classes operator_action.
func (m *Manager) reconcileCreatedNomadRepositories(ctx context.Context, execution *AgentExecution, info *WorkspaceInfo) error {
	repositories, err := createdNomadReconcileProjection(info)
	if err != nil {
		return operatorActionRecoveryError(err)
	}
	if len(repositories) == 0 {
		return nil
	}
	client, release := execution.AcquireAgentCtlClient()
	if client == nil {
		return errors.New("pre-ACP Nomad execution has no agentctl client for repository reconcile")
	}
	defer release()
	reconcileCtx, cancel := context.WithTimeout(ctx, m.nomadReconcileTimeout())
	defer cancel()
	if err := materializeWorkspaceRepositories(reconcileCtx, client, repositories); err != nil {
		return fmt.Errorf("reconcile pre-ACP Nomad repositories: %w", err)
	}
	return nil
}

// createdNomadReconcileProjection rebuilds the launch repository projection.
// A repository whose original materialization options recovery cannot
// reproduce, or whose clone locator is unknown, fails closed.
func createdNomadReconcileProjection(info *WorkspaceInfo) ([]WorkspaceRepositoryMaterialization, error) {
	specs := make([]RepoLaunchSpec, 0, len(info.WorkspaceRepositories))
	for _, repository := range info.WorkspaceRepositories {
		if local, err := createdNomadLocalWorktreeVerified(info.WorkspacePath, repository); err != nil {
			return nil, err
		} else if local {
			continue
		}
		if reason := createdNomadUnreproducibleReason(repository); reason != "" {
			return nil, fmt.Errorf("repository %q cannot be reconciled during recovery: %s", repository.RepositoryID, reason)
		}
		baseBranch := repository.BaseBranch
		if baseBranch == "" {
			baseBranch = repository.DefaultBranch
		}
		specs = append(specs, RepoLaunchSpec{
			RepositoryID: repository.RepositoryID, RepositoryURL: repository.RepositoryURL, RepoName: repository.RepoName,
			BaseBranch: baseBranch, CheckoutBranch: repository.CheckoutBranch,
		})
	}
	if len(specs) == 0 {
		return nil, nil
	}
	return remoteWorkspaceProjectionFromLaunch(&LaunchRequest{Repositories: specs}, true)
}

func createdNomadUnreproducibleReason(repository WorkspaceRepositorySpec) string {
	switch {
	case repository.RepositoryURL == "":
		return "clone locator is unknown"
	case repository.PRNumber > 0 || repository.QualifiedPRBase != nil:
		return "pull request checkout options are not recoverable"
	case repository.RemoteContribution != nil || repository.ContributionDestination != nil:
		return "remote contribution options are not recoverable"
	case repository.CheckoutOptions != nil && (repository.CheckoutOptions.DownloadMode != models.DownloadStandard ||
		len(repository.CheckoutOptions.SparseDirectories) != 0):
		return "sparse or on-demand checkout options are not recoverable"
	default:
		return ""
	}
}

// createdNomadLocalWorktreeVerified reports whether the repository is a local
// pool worktree already on disk. Such a worktree has a .git file, which agentctl
// materialization does not accept, so it is verified against its recorded
// repository and the generation branch instead and left out of the projection.
func createdNomadLocalWorktreeVerified(workspacePath string, repository WorkspaceRepositorySpec) (bool, error) {
	if repository.RepositoryPath == "" || workspacePath == "" {
		return false, nil
	}
	baseBranch := repository.BaseBranch
	if baseBranch == "" {
		baseBranch = repository.DefaultBranch
	}
	destination, err := remoteWorkspaceRepositoryDestination(RepoLaunchSpec{
		RepoName: repository.RepoName, BaseBranch: baseBranch, CheckoutBranch: repository.CheckoutBranch,
	})
	if err != nil || !localPoolWorktreePresent(workspacePath, destination) {
		return false, nil
	}
	if err := verifyLocalPoolWorktree(context.Background(), repository.RepositoryPath,
		filepath.Join(workspacePath, destination), localPoolWorktreeBranchForWorkspace(workspacePath)); err != nil {
		return false, fmt.Errorf("repository %q local worktree cannot be reconciled during recovery: %w", repository.RepositoryID, err)
	}
	return true, nil
}
