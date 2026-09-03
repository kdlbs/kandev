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
	if existing, err := e.existingWorkspaceInventoryRepairReceipt(
		ctx, repairer, task, session, req, env, repositories, idempotencyKey,
	); err != nil {
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
	running, _ := e.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	preservation := workspaceInventoryPreservation(spec, session, evidence, running)
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
	postEvidence, verifiedAt, attestErr := e.recordWorkspaceInventoryPostRepairAttestation(ctx, repairer, task.ID, idempotencyKey, spec, session, running, after, matched)
	if attestErr != nil {
		return nil, fmt.Errorf("%w: post-repair attestation was not durable", models.ErrWorkspaceInventoryRecoveryConflict)
	}
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
//
// A committed receipt is never handed back as a successful retry unless it
// carries durable positive post-repair attestation: see
// attestedExistingWorkspaceInventoryReceipt.
func (e *Executor) existingWorkspaceInventoryRepairReceipt(
	ctx context.Context,
	repairer workspaceInventoryRepairRepository,
	task *v1.Task,
	session *models.TaskSession,
	req *LaunchAgentRequest,
	env *models.TaskEnvironment,
	repositories []*repoInfo,
	idempotencyKey string,
) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	existing, err := repairer.GetWorkspaceInventoryRepairReceipt(ctx, task.ID, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("load existing workspace inventory repair receipt: %w", err)
	}
	if existing == nil {
		return nil, nil
	}
	retryHash, spec, info, candidate, ok := workspaceInventoryRetryIdentity(existing, task, session, req, env, repositories)
	if !ok || retryHash != existing.RequestHash {
		return nil, models.ErrWorkspaceInventoryRecoveryIdempotencyConflict
	}
	return e.attestedExistingWorkspaceInventoryReceipt(ctx, repairer, existing, spec, session, info, candidate)
}

func (e *Executor) existingAttestedWorkspaceInventoryRepairReceipt(
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
	return e.existingWorkspaceInventoryRepairReceipt(ctx, repairer, task, session, req, env, repositories, idempotencyKey)
}

// attestedExistingWorkspaceInventoryReceipt gates an already-committed
// receipt behind durable positive post-repair attestation before it is ever
// handed back as a successful retry. The repair transaction and its
// post-repair attestation are two separate writes (see
// repairReuseEnvironmentInventory): a crash after the transaction commits,
// or a failure persisting the attestation, can leave a committed receipt
// with no attestation recorded at all. Returning that receipt as success
// would let an unsafe crash/divergence boundary reach launch, so instead this
// safely completes the attestation now — by re-inspecting the exact
// preserved checkout under the identity the caller already re-validated —
// before the receipt can be reused. A receipt that already carries a
// negative/divergent attestation is never retried as success; it remains a
// stable, typed conflict.
func (e *Executor) attestedExistingWorkspaceInventoryReceipt(
	ctx context.Context,
	repairer workspaceInventoryRepairRepository,
	existing *models.WorkspaceInventoryRecoveryReceipt,
	spec RepoSpec,
	session *models.TaskSession,
	info *repoInfo,
	candidate *models.TaskEnvironmentRepo,
) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	if existing.PostRepairVerifiedAt != nil {
		if existing.PostRepairMatched {
			return existing, nil
		}
		return nil, fmt.Errorf("%w: post-repair attestation recorded a divergent checkout", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	if info == nil || candidate == nil {
		return nil, fmt.Errorf("%w: no provable checkout to complete attestation", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	before := preservationEvidenceFromModel(existing.Preservation)
	after, inspectErr := inspectWorkspaceInventoryCandidate(ctx, info, candidate)
	matched := inspectErr == nil && samePreservationEvidence(before, after)
	running, _ := e.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	postEvidence, verifiedAt, attestErr := e.recordWorkspaceInventoryPostRepairAttestation(
		ctx, repairer, existing.TaskID, existing.IdempotencyKey, spec, session, running, after, matched,
	)
	if attestErr != nil {
		return nil, fmt.Errorf("%w: post-repair attestation was not durable", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	completed := *existing
	completed.PostRepairEvidence = postEvidence
	completed.PostRepairMatched = matched
	completed.PostRepairVerifiedAt = &verifiedAt
	if !matched {
		return nil, fmt.Errorf("%w: checkout changed during metadata repair", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	return &completed, nil
}

// preservationEvidenceFromModel converts durable preservation evidence back
// into the comparable in-memory shape samePreservationEvidence expects.
func preservationEvidenceFromModel(p models.WorkspaceInventoryPreservation) *worktree.PreservationEvidence {
	return &worktree.PreservationEvidence{
		ObservedBranch: p.ObservedBranch, RefName: p.RefName, HeadOID: p.HeadOID,
		WorktreeID: p.WorktreeID, PathHash: p.PathHash, StatusHash: p.StatusHash,
		ContentHash: p.ContentHash, DirtyCount: p.DirtyCount, UntrackedCount: p.UntrackedCount,
	}
}

func workspaceInventoryRetryIdentity(
	existing *models.WorkspaceInventoryRecoveryReceipt,
	task *v1.Task,
	session *models.TaskSession,
	req *LaunchAgentRequest,
	env *models.TaskEnvironment,
	repositories []*repoInfo,
) (string, RepoSpec, *repoInfo, *models.TaskEnvironmentRepo, bool) {
	if !workspaceInventoryRetryReceiptMatches(existing, task, session, env) ||
		!workspaceInventoryRetryRequestMatches(req, task) {
		return "", RepoSpec{}, nil, nil, false
	}
	spec, ok := workspaceInventoryRetrySpec(req, existing)
	if !ok {
		return "", RepoSpec{}, nil, nil, false
	}
	info, candidate, ok := workspaceInventoryRetryCandidate(env, repositories, existing, spec)
	if !ok {
		return "", RepoSpec{}, nil, nil, false
	}
	preservation := existing.Preservation
	preservation.ExpectedBranchSlug = launchRepoBranchIdentitySlug(spec)
	repair := &models.WorkspaceInventoryRepair{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, SessionID: session.ID,
		TaskEnvironmentID: env.ID, TaskRepositoryID: info.TaskRepositoryID,
		RepositoryID: info.RepositoryID, EnvironmentRepoID: candidate.ID,
		BranchSlug: launchRepoBranchIdentitySlug(spec), WorktreeID: candidate.WorktreeID,
		WorktreePath: candidate.WorktreePath, WorktreeBranch: candidate.WorktreeBranch,
		Position: info.Position, Preservation: preservation,
	}
	return workspaceInventoryRepairHash(repair), spec, info, candidate, true
}

func workspaceInventoryRetryReceiptMatches(
	existing *models.WorkspaceInventoryRecoveryReceipt,
	task *v1.Task,
	session *models.TaskSession,
	env *models.TaskEnvironment,
) bool {
	return existing != nil && task != nil && session != nil && env != nil &&
		existing.TaskID == task.ID && existing.WorkspaceID == task.WorkspaceID &&
		existing.SessionID == session.ID && session.TaskID == task.ID &&
		existing.TaskEnvironmentID == env.ID && env.TaskID == task.ID &&
		env.Status == models.TaskEnvironmentStatusReady
}

func workspaceInventoryRetryRequestMatches(req *LaunchAgentRequest, task *v1.Task) bool {
	return req != nil && task != nil && req.TaskID == task.ID && req.WorkspaceID == task.WorkspaceID
}

func workspaceInventoryRetrySpec(
	req *LaunchAgentRequest,
	existing *models.WorkspaceInventoryRecoveryReceipt,
) (RepoSpec, bool) {
	specs := req.Repositories
	if len(specs) == 0 {
		if spec, ok := topLevelLaunchRepoSpec(req); ok {
			specs = []RepoSpec{spec}
		}
	}
	var matched []RepoSpec
	for _, spec := range specs {
		if spec.TaskRepositoryID == existing.TaskRepositoryID && spec.RepositoryID == existing.RepositoryID {
			matched = append(matched, spec)
		}
	}
	if len(matched) != 1 {
		return RepoSpec{}, false
	}
	return matched[0], true
}

func workspaceInventoryRetryCandidate(
	env *models.TaskEnvironment,
	repositories []*repoInfo,
	existing *models.WorkspaceInventoryRecoveryReceipt,
	spec RepoSpec,
) (*repoInfo, *models.TaskEnvironmentRepo, bool) {
	info, ok := workspaceInventoryRetryRepoInfo(repositories, spec)
	if !ok {
		return nil, nil, false
	}
	if canonicalInventoryMatches(spec, env.Repos, true) != 1 {
		return nil, nil, false
	}
	candidate, ok := workspaceInventoryRetryRow(env.Repos, existing, spec, info.Position)
	if !ok || validateWorkspaceInventoryRepairCandidate(env, info, candidate) != nil {
		return nil, nil, false
	}
	return info, candidate, true
}

func workspaceInventoryRetryRepoInfo(repositories []*repoInfo, spec RepoSpec) (*repoInfo, bool) {
	var matched []*repoInfo
	for _, info := range repositories {
		if info != nil && info.TaskRepositoryID == spec.TaskRepositoryID && info.RepositoryID == spec.RepositoryID {
			matched = append(matched, info)
		}
	}
	if len(matched) != 1 {
		return nil, false
	}
	return matched[0], true
}

func workspaceInventoryRetryRow(
	rows []*models.TaskEnvironmentRepo,
	existing *models.WorkspaceInventoryRecoveryReceipt,
	spec RepoSpec,
	position int,
) (*models.TaskEnvironmentRepo, bool) {
	var matchedRows []*models.TaskEnvironmentRepo
	for _, row := range rows {
		if workspaceInventoryRetryRowMatches(row, existing, spec, position) {
			matchedRows = append(matchedRows, row)
		}
	}
	if len(matchedRows) != 1 {
		return nil, false
	}
	return matchedRows[0], true
}

func workspaceInventoryRetryRowMatches(
	row *models.TaskEnvironmentRepo,
	existing *models.WorkspaceInventoryRecoveryReceipt,
	spec RepoSpec,
	position int,
) bool {
	return row != nil && row.ID == existing.EnvironmentRepoID && row.RepositoryID == spec.RepositoryID &&
		row.Position == position && row.BranchSlug == launchRepoBranchIdentitySlug(spec) &&
		row.DeletedAt == nil && row.Status != taskEnvironmentRepoStatusFailed &&
		row.Status != taskEnvironmentRepoStatusDeleted
}

// recordWorkspaceInventoryPostRepairAttestation persists non-secret
// before/after checkout evidence onto the committed receipt and returns the
// evidence/timestamp so the caller can also surface it on the in-memory
// receipt it returns. The repair transaction already committed by the time
// this runs, but launch admission still requires this write to succeed: a
// committed row without durable positive attestation is retryable, not safe
// to launch from. A match/mismatch is recorded either way when persistence
// succeeds so an unexpected concurrent write is itself part of the durable
// audit trail rather than a transient in-memory check.
func (e *Executor) recordWorkspaceInventoryPostRepairAttestation(
	ctx context.Context,
	repairer workspaceInventoryRepairRepository,
	taskID string,
	idempotencyKey string,
	spec RepoSpec,
	session *models.TaskSession,
	running *models.ExecutorRunning,
	after *worktree.PreservationEvidence,
	matched bool,
) (*models.WorkspaceInventoryPreservation, time.Time, error) {
	var postEvidence *models.WorkspaceInventoryPreservation
	if after != nil {
		evidence := workspaceInventoryPreservation(spec, session, after, running)
		postEvidence = &evidence
	}
	verifiedAt := time.Now().UTC()
	if err := repairer.RecordWorkspaceInventoryPostRepairAttestation(ctx, taskID, idempotencyKey, postEvidence, matched, verifiedAt); err != nil {
		if e.logger != nil {
			e.logger.Warn("failed to persist workspace inventory post-repair attestation",
				zap.String("task_id", taskID),
				zap.String("idempotency_key", idempotencyKey),
				zap.Bool("matched", matched),
				zap.Error(err))
		}
		return postEvidence, verifiedAt, err
	}
	return postEvidence, verifiedAt, nil
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
	running *models.ExecutorRunning,
) models.WorkspaceInventoryPreservation {
	preservation := models.WorkspaceInventoryPreservation{
		ExpectedBranchSlug: launchRepoBranchIdentitySlug(spec),
		ObservedBranch:     evidence.ObservedBranch, RefName: evidence.RefName,
		HeadOID: evidence.HeadOID, WorktreeID: evidence.WorktreeID,
		PathHash: evidence.PathHash, StatusHash: evidence.StatusHash,
		ContentHash: evidence.ContentHash, DirtyCount: evidence.DirtyCount,
		UntrackedCount: evidence.UntrackedCount, RuntimeState: string(session.State),
	}
	if running != nil {
		preservation.ExecutorID = running.ExecutorID
		preservation.ExecutorStatus = running.Status
		preservation.AgentExecutionID = running.AgentExecutionID
	}
	return preservation
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

// repairPathIsTaskScoped proves canonical task-root ownership of
// worktreePath. It resolves both paths with worktree.CanonicalDirectory
// rather than lexical filepath.Abs/Clean, so a symlink planted in a parent
// directory cannot make a path outside the task workspace lexically compare
// as scoped to it; a path that does not exist, or does not canonically
// resolve under the task root, is never treated as task-scoped.
func repairPathIsTaskScoped(env *models.TaskEnvironment, worktreePath string) bool {
	if env.WorkspacePath == "" || worktreePath == "" {
		return false
	}
	root, err := worktree.CanonicalDirectory(env.WorkspacePath)
	if err != nil {
		return false
	}
	candidate, err := worktree.CanonicalDirectory(worktreePath)
	if err != nil {
		return false
	}
	if env.TaskDirName == "" {
		return root == candidate
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
	value := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%s",
		repair.TaskID, repair.WorkspaceID, repair.SessionID, repair.TaskEnvironmentID,
		repair.TaskRepositoryID, repair.RepositoryID, repair.EnvironmentRepoID,
		repair.BranchSlug, repair.WorktreeID, repair.WorktreePath, repair.WorktreeBranch, repair.Position,
		repair.Preservation.ExpectedBranchSlug, repair.Preservation.ObservedBranch,
		repair.Preservation.RefName, repair.Preservation.HeadOID, repair.Preservation.PathHash,
		repair.Preservation.StatusHash, repair.Preservation.DirtyCount,
		repair.Preservation.UntrackedCount, repair.Preservation.ContentHash)
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
