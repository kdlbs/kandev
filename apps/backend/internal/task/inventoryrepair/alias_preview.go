package inventoryrepair

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

func inspectAliases(ctx context.Context, p Plan, db queryer) (*inspection, error) {
	in := &inspection{report: &Report{Plan: p}, db: db, rows: map[string]string{}, git: map[string]GitState{}, slots: map[string]row{}, envs: map[string]row{}, tasks: map[string]row{}, jobs: map[string]row{}}
	for _, r := range p.Repairs {
		if err := in.inspectAlias(ctx, r); err != nil {
			return nil, fmt.Errorf("worktree %s: %w", r.WorktreeID, err)
		}
	}
	if err := in.inspectSharedAliases(ctx); err != nil {
		return nil, err
	}
	if err := in.inspectAliasDependencies(ctx); err != nil {
		return nil, err
	}
	if err := in.inspectWorkspaces(ctx); err != nil {
		return nil, err
	}
	if err := in.validateAliasWorkspacePaths(ctx); err != nil {
		return nil, err
	}
	if err := in.inspectAliasForeignKeys(ctx); err != nil {
		return nil, err
	}
	if p.ExpectedRows != nil && !reflect.DeepEqual(p.ExpectedRows, in.rows) {
		return nil, errors.New("repair database observations changed; prepare and review a new plan")
	}
	if p.ExpectedGit != nil && !reflect.DeepEqual(p.ExpectedGit, in.git) {
		return nil, errors.New("repair Git/content observations changed; prepare and review a new plan")
	}
	in.report.Plan.ExpectedRows = in.rows
	in.report.Plan.ExpectedGit = in.git
	return in, nil
}

func (in *inspection) inspectAlias(ctx context.Context, r Repair) error {
	slot, claims, _, err := in.aliasClaims(ctx, r)
	if err != nil {
		return err
	}
	in.slots[r.WorktreeID] = slot
	env, err := in.inspectEnvironment(ctx, r.EnvironmentID)
	if err != nil {
		return err
	}
	if r.SourceRootTaskID != stringValue(env, "task_id") {
		return errors.New("canonical checkout must belong to its environment owner")
	}
	if err = in.inspectRepository(ctx, r, slot, env); err != nil {
		return err
	}
	if err = in.inspectRoots(ctx, r, env, in.tasks[r.SourceRootTaskID]); err != nil {
		return err
	}
	state, err := inspectGit(ctx, r.RepositoryPath, r.Path)
	if err != nil {
		return err
	}
	if state.Head != r.HeadOID || state.Branch != r.Branch {
		return errors.New("canonical checkout HEAD differs from explicit selection")
	}
	in.git[r.WorktreeID] = state
	for _, claim := range claims {
		if stringValue(claim, "id") == stringValue(slot, "id") {
			continue
		}
		if err = in.validateAliasPath(r, claim); err != nil {
			return err
		}
	}
	return in.observeAliasPaths(ctx, r)
}

func (in *inspection) validateAliasPath(r Repair, claim row) error {
	if stringValue(claim, "repository_id") != r.RepositoryID || stringValue(claim, "worktree_branch") != r.Branch {
		return errors.New("alias repository or branch differs from canonical checkout")
	}
	if stringValue(claim, "task_environment_id") != r.EnvironmentID {
		for _, e := range in.report.Plan.RedundantEnvironments {
			if e.EnvironmentID == stringValue(claim, "task_environment_id") && e.CanonicalEnvironmentID == r.EnvironmentID && stringValue(claim, "worktree_path") == r.Path {
				return nil
			}
		}
		return errors.New("cross-environment claim needs complete shared-environment proof")
	}
	root, err := taskRoot(in.report.Plan.TasksRoot, r.Path)
	if err != nil {
		return err
	}
	if stringValue(claim, "worktree_path") != root {
		return errors.New("alias is not the canonical checkout's enclosing task root")
	}
	if _, err = os.Lstat(filepath.Join(root, ".git")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("task-root alias has Git metadata or cannot be inspected")
	}
	return nil
}

func (in *inspection) observeAliasPaths(ctx context.Context, r Repair) error {
	selected := map[string]bool{}
	for _, a := range in.report.Plan.Aliases {
		selected[a.CanonicalRowID] = true
		for _, id := range a.AliasRowIDs {
			selected[id] = true
		}
	}

	root, err := taskRoot(in.report.Plan.TasksRoot, r.Path)
	if err != nil {
		return err
	}
	rows, err := in.read(ctx, "alias-paths:"+r.WorktreeID, `SELECT * FROM task_environment_repos WHERE (worktree_path=? OR worktree_path=?) AND status='active' AND deleted_at IS NULL ORDER BY id`, r.Path, root)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if !selected[stringValue(row, "id")] {
			return errors.New("checkout or root has an unselected physical claim")
		}
	}
	return nil
}

func (in *inspection) aliasClaims(ctx context.Context, r Repair) (row, []row, map[string]bool, error) {
	var selection AliasSelection
	for _, a := range in.report.Plan.Aliases {
		if a.WorktreeID == r.WorktreeID {
			selection = a
		}
	}
	claims, err := in.read(ctx, "alias-claims:"+r.WorktreeID, `SELECT * FROM task_environment_repos WHERE worktree_id=? ORDER BY id`, r.WorktreeID)
	if err != nil {
		return nil, nil, nil, err
	}
	selected := map[string]bool{selection.CanonicalRowID: true}
	for _, id := range selection.AliasRowIDs {
		selected[id] = true
	}
	if len(claims) != len(selected) {
		return nil, nil, nil, errors.New("selection must include every competing claim")
	}
	var slot row
	for _, claim := range claims {
		if !selected[stringValue(claim, "id")] || stringValue(claim, "status") != aliasStatusActive || claim["deleted_at"] != nil {
			return nil, nil, nil, errors.New("unsupported or omitted competing claim")
		}
		if stringValue(claim, "id") == selection.CanonicalRowID {
			slot = claim
		}
	}
	if !canonicalAliasSlot(slot, r) {
		return nil, nil, nil, errors.New("canonical slot differs from explicit checkout identity")
	}
	return slot, claims, selected, nil
}

func canonicalAliasSlot(slot row, r Repair) bool {
	return slot != nil && stringValue(slot, "task_environment_id") == r.EnvironmentID && stringValue(slot, "worktree_path") == r.Path && stringValue(slot, "repository_id") == r.RepositoryID && stringValue(slot, "worktree_branch") == r.Branch
}
