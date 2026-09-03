package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type workspaceInventoryRepairRepository interface {
	RepairWorkspaceInventory(context.Context, *models.WorkspaceInventoryRepair) (*models.WorkspaceInventoryRecoveryReceipt, error)
	GetWorkspaceInventoryRepairReceipt(ctx context.Context, taskID, idempotencyKey string) (*models.WorkspaceInventoryRecoveryReceipt, error)
	RecordWorkspaceInventoryPostRepairAttestation(ctx context.Context, taskID, idempotencyKey string, evidence *models.WorkspaceInventoryPreservation, matched bool, verifiedAt time.Time) error
}

func (e *Executor) repairReuseEnvironmentInventory(
	ctx context.Context,
	task *v1.Task,
	session *models.TaskSession,
	req *LaunchAgentRequest,
	env *models.TaskEnvironment,
	repositories []*repoInfo,
	idempotencyKey string,
) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	repairer, err := e.workspaceInventoryRepairer(task, session, req, env, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing, err := e.existingWorkspaceInventoryRepairReceipt(ctx, repairer, task, session, env, idempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}
	repairSession, err := e.workspaceInventoryRepairSession(ctx, req, env, session, repositories)
	if err != nil {
		return nil, err
	}
	spec, info, candidate, err := selectWorkspaceInventoryRepairTarget(req, env, repairSession, repositories)
	if err != nil {
		return nil, err
	}
	if err := e.rejectCompetingWorkspaceWriters(ctx, task.ID, session.ID); err != nil {
		return nil, err
	}
	if err := validateWorkspaceInventoryRepairCandidate(env, info, candidate); err != nil {
		return nil, err
	}
	evidence, err := inspectWorkspaceInventoryCandidate(ctx, info, candidate)
	if err != nil {
		return nil, fmt.Errorf("%w: checkout proof failed", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	preservation := workspaceInventoryPreservation(spec, session, evidence)
	repair := &models.WorkspaceInventoryRepair{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, SessionID: session.ID,
		TaskEnvironmentID: env.ID, TaskRepositoryID: info.TaskRepositoryID,
		RepositoryID: info.RepositoryID, EnvironmentRepoID: candidate.ID,
		ExpectedEnvironmentUpdatedAt:  env.UpdatedAt,
		ExpectedTaskRepositoryUpdate:  info.TaskRepositoryUpdatedAt,
		ExpectedEnvironmentRepoUpdate: candidate.UpdatedAt,
		BranchSlug:                    launchRepoBranchIdentitySlug(spec), WorktreeID: candidate.WorktreeID,
		WorktreePath: candidate.WorktreePath, WorktreeBranch: candidate.WorktreeBranch,
		Position: info.Position, IdempotencyKey: idempotencyKey,
		Preservation: preservation,
	}
	if repair.EnvironmentRepoID == "" {
		repair.EnvironmentRepoID = uuid.NewString()
	}
	repair.RequestHash = workspaceInventoryRepairHash(repair)
	receipt, err := repairer.RepairWorkspaceInventory(ctx, repair)
	if err != nil {
		return nil, err
	}
	after, inspectErr := inspectWorkspaceInventoryCandidate(ctx, info, candidate)
	matched := inspectErr == nil && samePreservationEvidence(evidence, after)
	postEvidence, verifiedAt := e.recordWorkspaceInventoryPostRepairAttestation(ctx, repairer, task.ID, idempotencyKey, spec, session, after, matched)
	receipt.PostRepairEvidence = postEvidence
	receipt.PostRepairMatched = matched
	receipt.PostRepairVerifiedAt = &verifiedAt
	if !matched {
		return nil, fmt.Errorf("%w: checkout changed during metadata repair", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	return receipt, nil
}

// existingWorkspaceInventoryRepairReceipt returns an already-committed receipt
// for this task-scoped idempotency key when its session/environment identity
// still matches the caller's context. This must run before candidate
// selection: once a prior repair committed, the canonical inventory already
// matches, leaving no provable mismatch for selectWorkspaceInventoryRepairTarget
// to find, which would otherwise misreport a retry as a conflict.
func (e *Executor) existingWorkspaceInventoryRepairReceipt(
	ctx context.Context,
	repairer workspaceInventoryRepairRepository,
	task *v1.Task,
	session *models.TaskSession,
	env *models.TaskEnvironment,
	idempotencyKey string,
) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	existing, err := repairer.GetWorkspaceInventoryRepairReceipt(ctx, task.ID, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("load existing workspace inventory repair receipt: %w", err)
	}
	if existing == nil {
		return nil, nil
	}
	if existing.SessionID != session.ID || existing.TaskEnvironmentID != env.ID {
		return nil, models.ErrWorkspaceInventoryRecoveryIdempotencyConflict
	}
	return existing, nil
}

// recordWorkspaceInventoryPostRepairAttestation persists non-secret
// before/after checkout evidence onto the committed receipt and returns the
// evidence/timestamp so the caller can also surface it on the in-memory
// receipt it returns. The repair transaction already committed by the time
// this runs, so a failure to persist this attestation is logged and never
// turns an otherwise-successful repair into a failure; a match/mismatch is
// recorded either way so an unexpected concurrent write is itself part of
// the durable audit trail rather than a transient in-memory check.
func (e *Executor) recordWorkspaceInventoryPostRepairAttestation(
	ctx context.Context,
	repairer workspaceInventoryRepairRepository,
	taskID string,
	idempotencyKey string,
	spec RepoSpec,
	session *models.TaskSession,
	after *worktree.PreservationEvidence,
	matched bool,
) (*models.WorkspaceInventoryPreservation, time.Time) {
	var postEvidence *models.WorkspaceInventoryPreservation
	if after != nil {
		evidence := workspaceInventoryPreservation(spec, session, after)
		postEvidence = &evidence
	}
	verifiedAt := time.Now().UTC()
	if err := repairer.RecordWorkspaceInventoryPostRepairAttestation(ctx, taskID, idempotencyKey, postEvidence, matched, verifiedAt); err != nil && e.logger != nil {
		e.logger.Warn("failed to persist workspace inventory post-repair attestation",
			zap.String("task_id", taskID),
			zap.String("idempotency_key", idempotencyKey),
			zap.Bool("matched", matched),
			zap.Error(err))
	}
	return postEvidence, verifiedAt
}

func (e *Executor) workspaceInventoryRepairer(
	task *v1.Task,
	session *models.TaskSession,
	req *LaunchAgentRequest,
	env *models.TaskEnvironment,
	idempotencyKey string,
) (workspaceInventoryRepairRepository, error) {
	if idempotencyKey == "" || task == nil || session == nil || env == nil ||
		!req.WorkspaceReuseRequired || !req.UseWorktree {
		return nil, models.ErrWorkspaceInventoryRecoveryInvalid
	}
	repairer, ok := e.repo.(workspaceInventoryRepairRepository)
	if !ok {
		return nil, models.ErrWorkspaceInventoryRecoveryInvalid
	}
	return repairer, nil
}

func validateWorkspaceInventoryRepairCandidate(
	env *models.TaskEnvironment,
	info *repoInfo,
	candidate *models.TaskEnvironmentRepo,
) error {
	if !repairPathIsTaskScoped(env, candidate.WorktreePath) || info.Repository == nil ||
		info.Repository.SourceType == sourceTypeLocal || !isLocalGitRepo(info.RepositoryPath) {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	return nil
}

func inspectWorkspaceInventoryCandidate(
	ctx context.Context,
	info *repoInfo,
	candidate *models.TaskEnvironmentRepo,
) (*worktree.PreservationEvidence, error) {
	return worktree.InspectPreservedCheckout(ctx, worktree.PreservationRequest{
		RepositoryPath: info.RepositoryPath,
		WorktreePath:   candidate.WorktreePath,
		ExpectedBranch: candidate.WorktreeBranch,
		WorktreeID:     candidate.WorktreeID,
	})
}

func workspaceInventoryPreservation(
	spec RepoSpec,
	session *models.TaskSession,
	evidence *worktree.PreservationEvidence,
) models.WorkspaceInventoryPreservation {
	return models.WorkspaceInventoryPreservation{
		ExpectedBranchSlug: launchRepoBranchIdentitySlug(spec),
		ObservedBranch:     evidence.ObservedBranch, RefName: evidence.RefName,
		HeadOID: evidence.HeadOID, WorktreeID: evidence.WorktreeID,
		PathHash: evidence.PathHash, StatusHash: evidence.StatusHash,
		ContentHash: evidence.ContentHash, DirtyCount: evidence.DirtyCount,
		UntrackedCount: evidence.UntrackedCount, RuntimeState: string(session.State),
	}
}

func (e *Executor) workspaceInventoryRepairSession(
	ctx context.Context,
	req *LaunchAgentRequest,
	env *models.TaskEnvironment,
	session *models.TaskSession,
	repositories []*repoInfo,
) (*models.TaskSession, error) {
	if len(env.Repos) != 0 || len(session.Worktrees) != 0 {
		return session, nil
	}
	spec, ok := singleWorkspaceInventoryRepairSpec(req)
	if !ok {
		return nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	position, ok := workspaceInventoryRepairPosition(spec, repositories)
	if !ok {
		return nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	running, err := e.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil || running == nil || running.TaskID != session.TaskID ||
		running.WorktreeID == "" || running.WorktreePath == "" || running.WorktreeBranch == "" {
		return nil, fmt.Errorf("%w: no server-owned checkout identity", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	copySession := *session
	copySession.Worktrees = []*models.TaskEnvironmentRepo{{
		TaskEnvironmentID: env.ID, RepositoryID: spec.RepositoryID,
		WorktreeID: running.WorktreeID, WorktreePath: running.WorktreePath,
		WorktreeBranch: running.WorktreeBranch, Position: position,
	}}
	return &copySession, nil
}

func singleWorkspaceInventoryRepairSpec(req *LaunchAgentRequest) (RepoSpec, bool) {
	if len(req.Repositories) == 1 {
		return req.Repositories[0], true
	}
	if len(req.Repositories) != 0 {
		return RepoSpec{}, false
	}
	return topLevelLaunchRepoSpec(req)
}

func workspaceInventoryRepairPosition(spec RepoSpec, repositories []*repoInfo) (int, bool) {
	for _, info := range repositories {
		if info.TaskRepositoryID == spec.TaskRepositoryID && info.RepositoryID == spec.RepositoryID {
			return info.Position, true
		}
	}
	return 0, false
}

func selectWorkspaceInventoryRepairTarget(
	req *LaunchAgentRequest,
	env *models.TaskEnvironment,
	session *models.TaskSession,
	repositories []*repoInfo,
) (RepoSpec, *repoInfo, *models.TaskEnvironmentRepo, error) {
	specs := req.Repositories
	if len(specs) == 0 {
		if spec, ok := topLevelLaunchRepoSpec(req); ok {
			specs = []RepoSpec{spec}
		}
	}
	var unmatched []RepoSpec
	for _, spec := range specs {
		if canonicalInventoryMatches(spec, env.Repos, req.UseWorktree) != 1 {
			unmatched = append(unmatched, spec)
		}
	}
	if len(unmatched) != 1 {
		return RepoSpec{}, nil, nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	spec := unmatched[0]
	var info *repoInfo
	for _, candidateInfo := range repositories {
		if candidateInfo.TaskRepositoryID == spec.TaskRepositoryID {
			info = candidateInfo
			break
		}
	}
	if info == nil || info.RepositoryID != spec.RepositoryID {
		return RepoSpec{}, nil, nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	candidates := matchingPhysicalCandidates(env.Repos, spec.RepositoryID, info.Position)
	if len(candidates) == 0 {
		candidates = matchingPhysicalCandidates(session.Worktrees, spec.RepositoryID, info.Position)
	}
	if len(candidates) != 1 {
		return RepoSpec{}, nil, nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	return spec, info, candidates[0], nil
}

func matchingPhysicalCandidates(rows []*models.TaskEnvironmentRepo, repositoryID string, position int) []*models.TaskEnvironmentRepo {
	result := make([]*models.TaskEnvironmentRepo, 0, 1)
	for _, row := range rows {
		if row == nil || row.RepositoryID != repositoryID || row.Position != position ||
			row.WorktreeID == "" || row.WorktreePath == "" || row.WorktreeBranch == "" ||
			row.DeletedAt != nil || row.Status == taskEnvironmentRepoStatusFailed ||
			row.Status == taskEnvironmentRepoStatusDeleted {
			continue
		}
		result = append(result, row)
	}
	return result
}

func repairPathIsTaskScoped(env *models.TaskEnvironment, worktreePath string) bool {
	if env.WorkspacePath == "" || worktreePath == "" {
		return false
	}
	root, err := filepath.Abs(env.WorkspacePath)
	if err != nil {
		return false
	}
	candidate, err := filepath.Abs(worktreePath)
	if err != nil {
		return false
	}
	if env.TaskDirName == "" {
		return filepath.Clean(root) == filepath.Clean(candidate)
	}
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != "." && relative != ".." &&
		!filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (e *Executor) rejectCompetingWorkspaceWriters(ctx context.Context, taskID, sessionID string) error {
	sessions, err := e.repo.ListActiveTaskSessionsByTaskID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("%w: cannot prove exclusive task writer", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	for _, active := range sessions {
		if active != nil && active.ID != sessionID {
			return fmt.Errorf("%w: another task session is active", models.ErrWorkspaceInventoryRecoveryConflict)
		}
	}
	return nil
}

// workspaceInventoryLaunchIdempotencyKey derives a stable, server-owned
// idempotency key for the automatic repair attempted during fresh/additional
// -session launch (LaunchPreparedSession). Unlike resume's explicit
// RecoverSession action, a fresh launch has no caller-supplied key, so this
// binds every retry of the same session to the same key without letting any
// caller-supplied value become part of repair identity.
func workspaceInventoryLaunchIdempotencyKey(sessionID string) string {
	return "launch-session-inventory-repair:" + sessionID
}

func workspaceInventoryRepairHash(repair *models.WorkspaceInventoryRepair) string {
	value := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s",
		repair.TaskID, repair.WorkspaceID, repair.SessionID, repair.TaskEnvironmentID,
		repair.TaskRepositoryID, repair.RepositoryID, repair.EnvironmentRepoID,
		repair.BranchSlug, repair.WorktreeID, repair.Preservation.HeadOID,
		repair.Preservation.StatusHash+repair.Preservation.ContentHash)
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func samePreservationEvidence(before, after *worktree.PreservationEvidence) bool {
	return before != nil && after != nil &&
		before.ObservedBranch == after.ObservedBranch && before.RefName == after.RefName &&
		before.HeadOID == after.HeadOID && before.WorktreeID == after.WorktreeID &&
		before.PathHash == after.PathHash && before.StatusHash == after.StatusHash &&
		before.ContentHash == after.ContentHash && before.DirtyCount == after.DirtyCount &&
		before.UntrackedCount == after.UntrackedCount
}
