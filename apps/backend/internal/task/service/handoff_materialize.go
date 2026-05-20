package service

import (
	"context"
	"encoding/json"
	"errors"

	"go.uber.org/zap"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

// GetSharedGroupEnvironment returns the materialized environment ID of
// the task's active workspace group, or "" when the task is not in a
// group or the group has not yet been materialized. The orchestrator
// uses this to propagate an existing group's environment to a new
// session launching with workspace_mode=shared_group (and inherit_parent
// when the parent path is exhausted).
func (s *HandoffService) GetSharedGroupEnvironment(ctx context.Context, taskID string) string {
	if s.wsGroups == nil || taskID == "" {
		return ""
	}
	g, err := s.wsGroups.GetWorkspaceGroupForTask(ctx, taskID)
	if err != nil || g == nil {
		return ""
	}
	return g.MaterializedEnvironmentID
}

// MarkOwnerSessionMaterialized records the materialized workspace on a
// task's workspace group when the group has not yet been materialized
// AND the launching task actually produced worktree/environment state
// on disk. Idempotent — safe to call after every session-prepare event.
//
// First-to-launch wins. For inherit_parent groups this is almost always
// the OwnerTaskID (the parent), because child sessions don't have their
// own worktrees — they inherit the parent's. For shared_group, where
// OwnerTaskID is whoever created the group and may not be the first to
// launch, this lets any member's session produce the recorded
// materialization. The MaterializedPath != "" check is the
// idempotency guard.
//
// This is the single place where owned_by_kandev gets flipped to true
// and cleanup_policy switches from never_delete to
// delete_when_last_member_archived_or_deleted. The MarkWorkspaceMaterialized
// repo method does both in one transaction.
func (s *HandoffService) MarkOwnerSessionMaterialized(ctx context.Context, taskID string) {
	if s.wsGroups == nil || s.sessions == nil || taskID == "" {
		return
	}
	group, err := s.wsGroups.GetWorkspaceGroupForTask(ctx, taskID)
	if err != nil || group == nil {
		return
	}
	if group.MaterializedPath != "" || group.MaterializedEnvironmentID != "" {
		// Already materialized. Idempotent re-invocations are common
		// since PrepareTaskSession runs every time a task focuses.
		return
	}
	mw, ok := s.materializationFromSession(ctx, taskID, group)
	if !ok {
		return
	}
	if err := s.wsGroups.MarkWorkspaceMaterialized(ctx, group.ID, mw); err != nil {
		s.logf().Error("mark workspace materialized",
			zap.String("group_id", group.ID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

// materializationFromSession resolves a task's primary session and
// builds the MaterializedWorkspace value passed to
// MarkWorkspaceMaterialized. Returns ok=false when the task has no
// session yet, no worktrees, or the data is incomplete — the caller
// retries on the next launch.
func (s *HandoffService) materializationFromSession(ctx context.Context, taskID string, group *orchmodels.WorkspaceGroup) (orchmodels.MaterializedWorkspace, bool) {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil || len(sessions) == 0 {
		return orchmodels.MaterializedWorkspace{}, false
	}
	primary := pickPrimarySession(sessions)
	if primary == nil {
		return orchmodels.MaterializedWorkspace{}, false
	}
	worktrees, err := s.sessions.ListTaskSessionWorktrees(ctx, primary.ID)
	if err != nil {
		return orchmodels.MaterializedWorkspace{}, false
	}
	kind, path, restoreCfg := buildMaterialization(group, worktrees, primary.TaskEnvironmentID)
	if path == "" && primary.TaskEnvironmentID == "" {
		// Plain folders / remote envs without a path AND no environment
		// id mean the launch hasn't actually placed anything on disk;
		// don't flip ownership yet.
		return orchmodels.MaterializedWorkspace{}, false
	}
	return orchmodels.MaterializedWorkspace{
		Path:          path,
		EnvironmentID: primary.TaskEnvironmentID,
		Kind:          kind,
		OwnedByKandev: true,
		RestoreConfig: restoreCfg,
	}, true
}

func pickPrimarySession(sessions []*models.TaskSession) *models.TaskSession {
	for _, s := range sessions {
		if s.IsPrimary {
			return s
		}
	}
	if len(sessions) > 0 {
		return sessions[0]
	}
	return nil
}

// buildMaterialization picks the right MaterializedKind + path +
// restore_config_json for the worktree set the session produced. The
// kind is single_repo for a single worktree, multi_repo for >1
// worktrees, and falls back to whatever the group declared when there
// are zero worktrees.
func buildMaterialization(group *orchmodels.WorkspaceGroup, worktrees []*models.TaskSessionWorktree, envID string) (kind, path, restoreCfg string) {
	switch len(worktrees) {
	case 0:
		// No worktrees but the session may still own a TaskEnvironment
		// (remote_environment / plain_folder). Keep the declared kind;
		// path stays empty, restore config records the env id only.
		return group.MaterializedKind, "", encodeRestoreConfig(restoreConfig{
			Kind:      group.MaterializedKind,
			RemoteEnv: envID,
		})
	case 1:
		wt := worktrees[0]
		return orchmodels.WorkspaceGroupKindSingleRepo, wt.WorktreePath, encodeRestoreConfig(restoreConfig{
			Kind:          orchmodels.WorkspaceGroupKindSingleRepo,
			RepositoryIDs: []string{wt.RepositoryID},
			WorktreeIDs:   map[string]string{wt.RepositoryID: wt.WorktreeID},
			Branches:      map[string]string{wt.RepositoryID: wt.WorktreeBranch},
			Path:          wt.WorktreePath,
		})
	default:
		repoIDs := make([]string, 0, len(worktrees))
		wtIDs := make(map[string]string, len(worktrees))
		branches := make(map[string]string, len(worktrees))
		for _, wt := range worktrees {
			repoIDs = append(repoIDs, wt.RepositoryID)
			wtIDs[wt.RepositoryID] = wt.WorktreeID
			branches[wt.RepositoryID] = wt.WorktreeBranch
		}
		// Multi-repo task root is the parent of any single worktree path.
		// Worktree paths share a parent because TaskWorktreePath builds
		// them as {tasksBase}/{taskDirName}/{repoName}.
		root := parentDir(worktrees[0].WorktreePath)
		return orchmodels.WorkspaceGroupKindMultiRepo, root, encodeRestoreConfig(restoreConfig{
			Kind:          orchmodels.WorkspaceGroupKindMultiRepo,
			RepositoryIDs: repoIDs,
			WorktreeIDs:   wtIDs,
			Branches:      branches,
			Path:          root,
		})
	}
}

// restoreConfig is the canonical JSON shape persisted into
// task_workspace_groups.restore_config_json. Read by the unarchive
// disk-recreation path (handoff_restore.go) to recreate a workspace
// from scratch.
type restoreConfig struct {
	Kind          string            `json:"kind"`
	RepositoryIDs []string          `json:"repository_ids,omitempty"`
	GitHubURLs    []string          `json:"github_urls,omitempty"`
	LocalPaths    []string          `json:"local_paths,omitempty"`
	Branches      map[string]string `json:"branches,omitempty"` // repo_id → branch
	WorktreeIDs   map[string]string `json:"worktree_ids,omitempty"`
	Path          string            `json:"path,omitempty"`
	RemoteEnv     string            `json:"remote_env,omitempty"`
}

func encodeRestoreConfig(rc restoreConfig) string {
	data, err := json.Marshal(rc)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodeRestoreConfig(raw string) (restoreConfig, error) {
	if raw == "" {
		return restoreConfig{}, errors.New("restore_config_json is empty")
	}
	var rc restoreConfig
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		return restoreConfig{}, err
	}
	return rc, nil
}

func parentDir(path string) string {
	if path == "" {
		return ""
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[:i]
		}
	}
	return ""
}
