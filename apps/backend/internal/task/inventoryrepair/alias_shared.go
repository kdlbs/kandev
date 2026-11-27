package inventoryrepair

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
)

func (in *inspection) inspectSharedAliases(ctx context.Context) error {
	for _, selection := range in.report.Plan.RedundantEnvironments {
		if err := in.inspectSharedAlias(ctx, selection); err != nil {
			return err
		}
	}
	return nil
}

func (in *inspection) inspectSharedAlias(ctx context.Context, selection EnvironmentRebind) error {
	canonical := in.envs[selection.CanonicalEnvironmentID]
	if canonical == nil {
		return errors.New("redundant environment has no selected canonical owner")
	}
	env, err := in.inspectEnvironment(ctx, selection.EnvironmentID)
	if err != nil {
		return err
	}
	memberIDs, err := in.sharedAliasMembers(ctx, selection, canonical, env)
	if err != nil {
		return err
	}
	if err = in.inspectRedundantSlots(ctx, selection); err != nil {
		return err
	}
	sessions, err := in.read(ctx, "sessions:"+selection.EnvironmentID, `SELECT * FROM task_sessions WHERE task_environment_id=? ORDER BY id`, selection.EnvironmentID)
	if err != nil {
		return err
	}
	if !sameRowIDs(sessions, selection.SessionIDs) {
		return errors.New("rebind must explicitly include every redundant session")
	}
	for _, s := range sessions {
		if !memberIDs[stringValue(s, "task_id")] {
			return errors.New("redundant environment has a foreign session")
		}
		if err = in.inspectAliasTask(ctx, stringValue(s, "task_id")); err != nil {
			return err
		}
	}
	return in.inspectAliasSnapshots(ctx, selection)
}

func (in *inspection) validateSharedPolicy(task row, selection EnvironmentRebind) error {
	var metadata struct {
		Workspace struct {
			Mode    string `json:"mode"`
			GroupID string `json:"group_id"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal([]byte(stringValue(task, "metadata")), &metadata); err != nil {
		return err
	}
	policy := metadata.Workspace
	if (policy.Mode != "inherit_parent" && policy.Mode != "shared_group") || policy.GroupID != selection.GroupID {
		return errors.New("saved task policy does not identify the selected shared group")
	}
	return nil
}

func sameRowIDs(rows []row, ids []string) bool {
	got := make([]string, 0, len(rows))
	for _, r := range rows {
		got = append(got, stringValue(r, "id"))
	}
	want := slices.Clone(ids)
	slices.Sort(want)
	slices.Sort(got)
	return slices.Equal(got, want)
}

func (in *inspection) inspectRedundantSlots(ctx context.Context, selection EnvironmentRebind) error {
	slots, err := in.read(ctx, "redundant-slots:"+selection.EnvironmentID, `SELECT * FROM task_environment_repos WHERE task_environment_id=? ORDER BY id`, selection.EnvironmentID)
	if err != nil {
		return err
	}
	if len(slots) == 0 {
		return errors.New("redundant environment has no physical inventory proof")
	}
	for _, slot := range slots {
		canonical := in.slots[stringValue(slot, "worktree_id")]
		if canonical == nil || stringValue(canonical, "task_environment_id") != selection.CanonicalEnvironmentID || stringValue(slot, "status") != aliasStatusActive || slot["deleted_at"] != nil {
			return errors.New("redundant environment has an independent or inactive slot")
		}
		for _, key := range []string{"repository_id", "worktree_path", "worktree_branch"} {
			if stringValue(slot, key) != stringValue(canonical, key) {
				return errors.New("redundant slot has no identical canonical counterpart")
			}
		}
	}
	return nil
}

func (in *inspection) inspectAliasSnapshots(ctx context.Context, selection EnvironmentRebind) error {
	snapshots, err := in.read(ctx, "snapshots:"+selection.EnvironmentID, `SELECT * FROM task_session_git_snapshots WHERE task_environment_id=? ORDER BY id`, selection.EnvironmentID)
	if err != nil {
		return err
	}
	if !sameRowIDs(snapshots, selection.SnapshotIDs) {
		return errors.New("rebind must explicitly include every Git snapshot")
	}
	for _, snapshot := range snapshots {
		if err = in.proveAliasSnapshot(ctx, selection, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (in *inspection) proveAliasSnapshot(ctx context.Context, selection EnvironmentRebind, snapshot row) error {
	if id := stringValue(snapshot, "session_id"); id != "" && !slices.Contains(selection.SessionIDs, id) {
		return errors.New("snapshot has foreign session provenance")
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(stringValue(snapshot, "metadata")), &metadata); err != nil {
		return err
	}
	matches := 0
	for _, r := range in.report.Plan.Repairs {
		if r.EnvironmentID != selection.CanonicalEnvironmentID || r.Branch != stringValue(snapshot, "branch") {
			continue
		}
		repo, err := in.one(ctx, "repository:"+r.RepositoryID, `SELECT * FROM repositories WHERE id=?`, r.RepositoryID)
		if err != nil {
			return err
		}
		if !aliasSnapshotScope(metadata, repo, r) {
			continue
		}
		matches++
	}
	if matches != 1 {
		return fmt.Errorf("snapshot %s has ambiguous repository scope", stringValue(snapshot, "id"))
	}
	return nil
}

func (in *inspection) sharedAliasMembers(ctx context.Context, selection EnvironmentRebind, canonical, env row) (map[string]bool, error) {
	group, err := in.one(ctx, "group:"+selection.GroupID, `SELECT * FROM task_workspace_groups WHERE id=?`, selection.GroupID)
	if err != nil {
		return nil, err
	}
	task := in.tasks[stringValue(env, "task_id")]
	if stringValue(group, "owner_task_id") != stringValue(canonical, "task_id") || stringValue(group, "materialized_environment_id") != selection.CanonicalEnvironmentID || stringValue(group, "workspace_id") != stringValue(task, "workspace_id") {
		return nil, errors.New("shared group does not prove canonical environment ownership")
	}
	if stringValue(task, "workspace_id") != stringValue(in.tasks[stringValue(canonical, "task_id")], "workspace_id") {
		return nil, errors.New("redundant owner belongs to another workspace")
	}
	if err = in.validateSharedPolicy(task, selection); err != nil {
		return nil, err
	}
	members, err := in.read(ctx, "group-members:"+selection.GroupID, `SELECT * FROM task_workspace_group_members WHERE workspace_group_id=? AND released_at IS NULL ORDER BY task_id`, selection.GroupID)
	if err != nil {
		return nil, err
	}
	memberIDs := map[string]bool{}
	for _, m := range members {
		memberIDs[stringValue(m, "task_id")] = true
	}
	if !memberIDs[stringValue(env, "task_id")] || !memberIDs[stringValue(canonical, "task_id")] {
		return nil, errors.New("both owners require active shared membership")
	}
	return memberIDs, nil
}

func aliasSnapshotScope(metadata map[string]any, repo row, r Repair) bool {
	name, _ := metadata["repository_name"].(string)
	if name != "" && name != stringValue(repo, "name") && name != filepath.Base(r.Path) {
		return false
	}
	if id, ok := metadata["repository_id"].(string); ok && id != r.RepositoryID {
		return false
	}
	if path, ok := metadata["worktree_path"].(string); ok && path != r.Path {
		return false
	}
	return true
}
