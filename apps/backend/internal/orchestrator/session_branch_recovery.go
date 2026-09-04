package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// BranchRecoveryError identifies a worktree branch that cannot be restored.
// It wraps the original error so callers can still use errors.Is/errors.As.
type BranchRecoveryError struct {
	Cause          error
	SessionID      string
	RepositoryID   string
	OriginalBranch string
	BaseBranch     string
}

func (e *BranchRecoveryError) Error() string {
	if e == nil {
		return "the worktree branch is no longer available"
	}
	branch := strings.TrimSpace(e.OriginalBranch)
	if branch == "" {
		return "the worktree branch is no longer available; continue on a new branch to keep the conversation history"
	}
	return fmt.Sprintf(
		"the worktree branch %q is no longer available; continue on a new branch to keep the conversation history, but code changes from the lost branch cannot be recovered",
		branch,
	)
}

func (e *BranchRecoveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Details returns the stable WebSocket recovery contract for this error.
func (e *BranchRecoveryError) Details() map[string]interface{} {
	if e == nil {
		return nil
	}
	return map[string]interface{}{
		"kind":            "branch_unrecoverable",
		"recovery_action": "resume_new_branch",
		"original_branch": e.OriginalBranch,
		"base_branch":     e.BaseBranch,
		"repository_id":   e.RepositoryID,
		"session_id":      e.SessionID,
	}
}

type branchRecoveryRepoSnapshot struct {
	RowID          string
	RepositoryID   string
	BranchSlug     string
	WorktreeID     string
	WorktreeBranch string
	BaseBranch     string
	Position       int
}

type branchRecoverySnapshot struct {
	TaskID        string
	SessionID     string
	EnvironmentID string
	Repositories  []branchRecoveryRepoSnapshot
}

const (
	branchRecoveryWarningKeyPrefix       = "branch_recreated_warning:"
	branchRecoveryWarningClaimStaleAfter = 5 * time.Minute
)

type branchRecoveryWarningClaim struct {
	ClaimedAt time.Time `json:"claimed_at"`
}

type branchRecoveryStore interface {
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
	ListTaskEnvironmentRepos(ctx context.Context, envID string) ([]*models.TaskEnvironmentRepo, error)
}

func (s *Service) captureBranchRecoverySnapshot(
	ctx context.Context,
	taskID, sessionID string,
) (*branchRecoverySnapshot, error) {
	store, env, err := s.loadBranchRecoverySnapshotContext(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	snapshot := &branchRecoverySnapshot{
		TaskID:    taskID,
		SessionID: sessionID,
	}
	if env == nil {
		return snapshot, nil
	}
	rows, err := s.branchRecoveryEnvironmentRepos(ctx, store, env)
	if err != nil {
		return nil, err
	}
	baseBranches, err := s.branchRecoveryBaseBranches(ctx, store, taskID)
	if err != nil {
		return nil, err
	}
	snapshot.EnvironmentID = env.ID
	snapshot.Repositories = branchRecoveryRepoSnapshots(rows, baseBranches)
	return snapshot, nil
}

func (s *Service) loadBranchRecoverySnapshotContext(
	ctx context.Context,
	taskID, sessionID string,
) (branchRecoveryStore, *models.TaskEnvironment, error) {
	if s.repo == nil {
		return nil, nil, fmt.Errorf("repository is not configured")
	}
	store, ok := s.repo.(branchRecoveryStore)
	if !ok {
		return nil, nil, fmt.Errorf("repository does not support branch recovery snapshots")
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("load session for branch recovery: %w", err)
	}
	if session == nil {
		return nil, nil, fmt.Errorf("session %q was not found", sessionID)
	}
	env, err := s.repo.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, nil, fmt.Errorf("load task environment for branch recovery: %w", err)
	}
	if env == nil && session.TaskEnvironmentID != "" {
		env, err = s.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
		if err != nil {
			return nil, nil, fmt.Errorf("load session environment for branch recovery: %w", err)
		}
	}
	return store, env, nil
}

func (s *Service) branchRecoveryEnvironmentRepos(
	ctx context.Context,
	store branchRecoveryStore,
	env *models.TaskEnvironment,
) ([]*models.TaskEnvironmentRepo, error) {
	if len(env.Repos) > 0 {
		return env.Repos, nil
	}
	rows, err := store.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil {
		return nil, fmt.Errorf("load environment repositories for branch recovery: %w", err)
	}
	return rows, nil
}

func (s *Service) branchRecoveryBaseBranches(
	ctx context.Context,
	store branchRecoveryStore,
	taskID string,
) (map[string]string, error) {
	taskRepos, err := store.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("load task repositories for branch recovery: %w", err)
	}
	baseBranches := make(map[string]string, len(taskRepos))
	for _, taskRepo := range taskRepos {
		if taskRepo != nil && taskRepo.RepositoryID != "" {
			baseBranches[taskRepo.RepositoryID] = taskRepo.BaseBranch
		}
	}
	return baseBranches, nil
}

func branchRecoveryRepoSnapshots(
	rows []*models.TaskEnvironmentRepo,
	baseBranches map[string]string,
) []branchRecoveryRepoSnapshot {
	snapshots := make([]branchRecoveryRepoSnapshot, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.WorktreeID == "" || row.WorktreeBranch == "" {
			continue
		}
		snapshots = append(snapshots, branchRecoveryRepoSnapshot{
			RowID:          row.ID,
			RepositoryID:   row.RepositoryID,
			BranchSlug:     row.BranchSlug,
			WorktreeID:     row.WorktreeID,
			WorktreeBranch: row.WorktreeBranch,
			BaseBranch:     baseBranches[row.RepositoryID],
			Position:       row.Position,
		})
	}
	return snapshots
}

func (s *Service) branchRecoveryError(ctx context.Context, taskID, sessionID string, cause error) error {
	if cause == nil || !errors.Is(cause, worktree.ErrBranchUnrecoverable) {
		return cause
	}
	var existing *BranchRecoveryError
	if errors.As(cause, &existing) {
		return cause
	}

	originalBranch := ""
	var branchErr *worktree.BranchUnrecoverableError
	if errors.As(cause, &branchErr) {
		originalBranch = branchErr.BranchName()
	}
	snapshot, snapshotErr := s.captureBranchRecoverySnapshot(ctx, taskID, sessionID)
	if snapshotErr != nil {
		snapshot = nil
	}
	repositoryID, baseBranch := branchRecoveryContext(snapshot, originalBranch)
	return &BranchRecoveryError{
		Cause:          cause,
		SessionID:      sessionID,
		RepositoryID:   repositoryID,
		OriginalBranch: originalBranch,
		BaseBranch:     baseBranch,
	}
}

func branchRecoveryContext(snapshot *branchRecoverySnapshot, originalBranch string) (string, string) {
	if snapshot == nil {
		return "", ""
	}
	for _, repo := range snapshot.Repositories {
		if repo.WorktreeBranch == originalBranch {
			return repo.RepositoryID, repo.BaseBranch
		}
	}
	if len(snapshot.Repositories) == 1 {
		return snapshot.Repositories[0].RepositoryID, snapshot.Repositories[0].BaseBranch
	}
	return "", ""
}

func (s *Service) persistBranchRecoveryWarnings(
	ctx context.Context,
	taskID, sessionID string,
	before *branchRecoverySnapshot,
) {
	if before == nil || len(before.Repositories) == 0 || s.messageCreator == nil {
		return
	}
	after, err := s.captureBranchRecoverySnapshot(ctx, taskID, sessionID)
	if err != nil {
		s.logger.Warn("failed to inspect branch recovery result", zapFieldsBranchRecovery(taskID, sessionID, err)...)
		return
	}
	for _, previous := range before.Repositories {
		current, found := findBranchRecoveryRepo(after, previous)
		if !found || current.WorktreeBranch == "" || current.WorktreeBranch == previous.WorktreeBranch || previous.WorktreeBranch == "" {
			continue
		}
		s.persistBranchRecoveryWarning(ctx, taskID, sessionID, previous, current)
	}
}

func findBranchRecoveryRepo(snapshot *branchRecoverySnapshot, previous branchRecoveryRepoSnapshot) (branchRecoveryRepoSnapshot, bool) {
	if snapshot == nil {
		return branchRecoveryRepoSnapshot{}, false
	}
	for _, current := range snapshot.Repositories {
		if previous.RowID != "" && current.RowID == previous.RowID {
			return current, true
		}
		if current.RepositoryID == previous.RepositoryID && current.BranchSlug == previous.BranchSlug && current.Position == previous.Position {
			return current, true
		}
	}
	return branchRecoveryRepoSnapshot{}, false
}

func (s *Service) persistBranchRecoveryWarning(
	ctx context.Context,
	taskID, sessionID string,
	previous, current branchRecoveryRepoSnapshot,
) {
	decisionID := branchRecoveryDecisionID(taskID, sessionID, previous.RepositoryID, previous.WorktreeBranch, current.WorktreeBranch, current.BaseBranch)
	key := branchRecoveryWarningKeyPrefix + decisionID
	release, claimed := s.claimBranchRecoveryWarning(ctx, sessionID, key)
	if !claimed {
		return
	}
	metadata := map[string]interface{}{
		"variant":         "warning",
		"kind":            "branch_recreated",
		"original_branch": previous.WorktreeBranch,
		"new_branch":      current.WorktreeBranch,
		"base_branch":     current.BaseBranch,
		"session_id":      sessionID,
		"repository_id":   current.RepositoryID,
		"decision_id":     decisionID,
	}
	err := s.messageCreator.CreateSessionMessage(
		context.WithoutCancel(ctx),
		taskID,
		"branch_recreated",
		sessionID,
		string(v1.MessageTypeStatus),
		s.getActiveTurnID(sessionID),
		metadata,
		false,
	)
	if err != nil {
		release()
		s.logger.Warn("failed to persist branch recovery warning", zapFieldsBranchRecovery(taskID, sessionID, err)...)
	}
}

func (s *Service) claimBranchRecoveryWarning(ctx context.Context, sessionID, key string) (func(), bool) {
	if s.repo == nil {
		return func() {}, false
	}
	claimCtx := context.WithoutCancel(ctx)
	if claimer, ok := s.repo.(failedSessionMetadataClaimer); ok {
		return s.claimBranchRecoveryWarningWithState(claimCtx, sessionID, key, claimer)
	}
	return s.claimBranchRecoveryWarningWithoutState(claimCtx, sessionID, key)
}

func (s *Service) claimBranchRecoveryWarningWithState(
	ctx context.Context,
	sessionID, key string,
	claimer failedSessionMetadataClaimer,
) (func(), bool) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return func() {}, false
	}
	claim := branchRecoveryWarningClaim{ClaimedAt: time.Now().UTC()}
	claimed, err := claimer.SetSessionMetadataKeyIfAbsentIfState(ctx, sessionID, key, claim, session.State)
	if err != nil {
		return func() {}, false
	}
	if !claimed {
		session, claimed = s.reclaimBranchRecoveryWarningClaim(ctx, sessionID, key, session.State)
		if claimed {
			claimed, err = claimer.SetSessionMetadataKeyIfAbsentIfState(ctx, sessionID, key, claim, session.State)
		}
	}
	if err != nil || !claimed {
		return func() {}, false
	}
	return func() {
		s.releaseBranchRecoveryWarningClaim(ctx, sessionID, key, session.State)
	}, true
}

func (s *Service) claimBranchRecoveryWarningWithoutState(ctx context.Context, sessionID, key string) (func(), bool) {
	claimed, err := s.repo.SetSessionMetadataKeyIfAbsent(
		ctx, sessionID, key, branchRecoveryWarningClaim{ClaimedAt: time.Now().UTC()},
	)
	return func() {}, err == nil && claimed
}

func (s *Service) reclaimBranchRecoveryWarningClaim(
	ctx context.Context,
	sessionID, key string,
	expectedState models.TaskSessionState,
) (*models.TaskSession, bool) {
	if !s.reclaimStaleBranchRecoveryWarningClaim(ctx, sessionID, key, expectedState) {
		return nil, false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	return session, err == nil && session != nil
}

func (s *Service) releaseBranchRecoveryWarningClaim(ctx context.Context, sessionID, key string, expectedState models.TaskSessionState) {
	releaser, ok := s.repo.(failedSessionMetadataClaimReleaser)
	if !ok {
		return
	}
	_, _ = releaser.RemoveSessionMetadataKeyIfState(ctx, sessionID, key, expectedState)
}

// reclaimStaleBranchRecoveryWarningClaim releases a timestamped claim left by
// a crashed process. A matching persisted warning wins over reclamation so a
// crash after message insertion cannot cause a duplicate warning.
func (s *Service) reclaimStaleBranchRecoveryWarningClaim(
	ctx context.Context,
	sessionID, key string,
	expectedState models.TaskSessionState,
) bool {
	releaser, ok := s.repo.(failedSessionMetadataClaimReleaser)
	if !ok {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.State != expectedState || session.Metadata == nil {
		return false
	}
	claim, ok := branchRecoveryWarningClaimFromMetadata(session.Metadata[key])
	if !ok || time.Since(claim.ClaimedAt) < branchRecoveryWarningClaimStaleAfter {
		return false
	}
	decisionID := strings.TrimPrefix(key, branchRecoveryWarningKeyPrefix)
	if decisionID == "" || decisionID == key {
		return false
	}
	messages, err := s.repo.ListMessages(ctx, sessionID)
	if err != nil || branchRecoveryWarningMessageExists(messages, decisionID) {
		return false
	}
	removed, err := releaser.RemoveSessionMetadataKeyIfState(ctx, sessionID, key, session.State)
	return err == nil && removed
}

func branchRecoveryWarningClaimFromMetadata(value interface{}) (branchRecoveryWarningClaim, bool) {
	data, err := json.Marshal(value)
	if err != nil {
		return branchRecoveryWarningClaim{}, false
	}
	var claim branchRecoveryWarningClaim
	if err := json.Unmarshal(data, &claim); err != nil || claim.ClaimedAt.IsZero() {
		return branchRecoveryWarningClaim{}, false
	}
	return claim, true
}

func branchRecoveryWarningMessageExists(messages []*models.Message, decisionID string) bool {
	for _, message := range messages {
		if message == nil || message.Metadata == nil {
			continue
		}
		if message.Metadata["kind"] != "branch_recreated" || message.Metadata["decision_id"] != decisionID {
			continue
		}
		return true
	}
	return false
}

func branchRecoveryDecisionID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("branch-recreated-%x", sum[:12])
}

func zapFieldsBranchRecovery(taskID, sessionID string, err error) []zap.Field {
	return []zap.Field{
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.Error(err),
	}
}
