package inventoryrepair

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/kandev/kandev/internal/common/proclive"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func Preview(ctx context.Context, p Plan) (*Report, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	db, err := openDatabase(p, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	in, err := inspect(ctx, p, db)
	if err != nil {
		return nil, err
	}
	return in.report, nil
}

func inspect(ctx context.Context, p Plan, db queryer) (*inspection, error) {
	in := &inspection{report: &Report{Plan: p}, db: db, rows: map[string]string{}, git: map[string]GitState{}, slots: map[string]row{}, envs: map[string]row{}, tasks: map[string]row{}, jobs: map[string]row{}}
	for _, r := range p.Repairs {
		if err := in.inspectRepair(ctx, r); err != nil {
			return nil, fmt.Errorf("worktree %s: %w", r.WorktreeID, err)
		}
	}
	if err := in.inspectWorkspaces(ctx); err != nil {
		return nil, err
	}
	if err := in.inspectJobs(ctx); err != nil {
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

func (in *inspection) inspectRepair(ctx context.Context, r Repair) error {
	slot, err := in.one(ctx, "worktree:"+r.WorktreeID, `SELECT * FROM task_environment_repos WHERE worktree_id = ?`, r.WorktreeID)
	if err != nil {
		return err
	}
	in.slots[r.WorktreeID] = slot
	if stringValue(slot, "task_environment_id") != r.EnvironmentID || stringValue(slot, "worktree_path") != r.SourcePath || stringValue(slot, "status") != "active" || slot["deleted_at"] != nil {
		return errors.New("worktree slot does not match the selected active path/environment")
	}
	env, err := in.inspectEnvironment(ctx, r.EnvironmentID)
	if err != nil {
		return err
	}
	if err := in.inspectRepository(ctx, r, slot, env); err != nil {
		return err
	}
	task := in.tasks[stringValue(env, "task_id")]
	if err := in.verifySlotCollision(ctx, r, slot); err != nil {
		return err
	}
	if err := in.inspectRoots(ctx, r, env, task); err != nil {
		return err
	}
	state, err := inspectGit(ctx, r.RepositoryPath, r.SourcePath)
	if err != nil {
		return err
	}
	if state.Head != r.HeadOID || state.Branch != r.Branch {
		return errors.New("observed HEAD or attached branch differs from the explicit repair")
	}
	in.git[r.WorktreeID] = state
	in.report.Changes = append(in.report.Changes, Change{WorktreeID: r.WorktreeID, OldRepositoryID: stringValue(slot, "repository_id"), OldBranch: stringValue(slot, "worktree_branch"), OldPath: r.SourcePath, Repair: r})
	return nil
}

func (in *inspection) verifySlotCollision(ctx context.Context, r Repair, slot row) error {
	rows, err := in.read(ctx, "slot-key:"+r.WorktreeID, `SELECT id FROM task_environment_repos WHERE task_environment_id = ? AND repository_id = ? AND branch_slug = ? AND id <> ? ORDER BY id`, r.EnvironmentID, r.RepositoryID, stringValue(slot, "branch_slug"), stringValue(slot, "id"))
	if err != nil {
		return err
	}
	if len(rows) != 0 {
		return errors.New("replacement repository collides with another inventory slot")
	}
	owners, err := in.read(ctx, "path-owner:"+r.WorktreeID, `SELECT id FROM task_environment_repos WHERE worktree_path = ? AND deleted_at IS NULL AND status = 'active' ORDER BY id`, r.SourcePath)
	if err != nil {
		return err
	}
	if len(owners) != 1 || stringValue(owners[0], "id") != stringValue(slot, "id") {
		return errors.New("checkout path has competing inventory owners")
	}
	return nil
}

func (in *inspection) inspectEnvironment(ctx context.Context, id string) (row, error) {
	if env := in.envs[id]; env != nil {
		return env, nil
	}
	env, err := in.one(ctx, "environment:"+id, `SELECT * FROM task_environments WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	if stringValue(env, "executor_type") != "worktree" {
		return nil, errors.New("only host Worktree environments are repairable")
	}
	if stringValue(env, "task_dir_name") == "" {
		return nil, errors.New("environment has no canonical task root")
	}
	task, err := in.one(ctx, "task:"+stringValue(env, "task_id"), `SELECT * FROM tasks WHERE id = ?`, stringValue(env, "task_id"))
	if err != nil {
		return nil, err
	}
	in.envs[id] = env
	in.tasks[stringValue(env, "task_id")] = task
	if err := in.inspectConsumers(ctx, id, stringValue(env, "task_id")); err != nil {
		return nil, err
	}
	return env, nil
}

func (in *inspection) inspectConsumers(ctx context.Context, id, taskID string) error {
	sessions, err := in.read(ctx, "sessions:"+id, `SELECT * FROM task_sessions WHERE task_environment_id = ? ORDER BY id`, id)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		switch stringValue(s, "state") {
		case "COMPLETED", "CANCELLED", "FAILED":
		default:
			in.report.Blockers = append(in.report.Blockers, "nonterminal session "+stringValue(s, "id"))
		}
	}
	runtimes, err := in.read(ctx, "runtimes:"+id, `SELECT er.* FROM executors_running er JOIN task_sessions s ON s.id = er.session_id WHERE s.task_environment_id = ? ORDER BY er.id`, id)
	if err != nil {
		return err
	}
	for _, r := range runtimes {
		switch stringValue(r, "status") {
		case "stopped", "failed", "exited":
		default:
			in.report.Blockers = append(in.report.Blockers, "runtime "+stringValue(r, "id"))
		}
		in.inspectRuntimePIDs(r)
	}
	claims, err := in.read(ctx, "claims:"+id, `SELECT * FROM task_environment_recovery_claims WHERE task_environment_id = ?`, id)
	if err != nil {
		return err
	}
	if len(claims) > 0 {
		in.report.Blockers = append(in.report.Blockers, "environment recovery claim "+id)
	}
	jobs, err := in.read(ctx, "jobs:"+taskID, `SELECT * FROM task_resource_cleanup_jobs WHERE task_id = ? ORDER BY id`, taskID)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		in.jobs[stringValue(job, "id")] = job
		if stringValue(job, "state") == "running" {
			in.report.Blockers = append(in.report.Blockers, "running cleanup "+stringValue(job, "id"))
		}
	}
	return nil
}

func (in *inspection) inspectRuntimePIDs(r row) {
	for _, key := range []string{"pid", "local_pid"} {
		pid, _ := r[key].(int64)
		if pid <= 0 {
			continue
		}
		alive, known := proclive.Alive(pid)
		if alive || !known {
			in.report.Blockers = append(in.report.Blockers, fmt.Sprintf("runtime %s has live or unknown %s %d", stringValue(r, "id"), key, pid))
		}
	}
}

func taskRoot(base, path string) (string, error) {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("checkout is outside the managed tasks root")
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 {
		return "", errors.New("checkout cannot be the task root")
	}
	return filepath.Join(base, parts[0]), nil
}

func readMarker(base, root string) (storageworkspaces.OwnershipMarker, error) {
	handle, err := storageworkspaces.OpenDirectoryNoFollow(base, root)
	if err != nil {
		return storageworkspaces.OwnershipMarker{}, err
	}
	defer func() { _ = handle.Close() }()
	marker, found, err := storageworkspaces.ReadOwnershipMarker(root)
	if err != nil {
		return marker, err
	}
	if !found {
		return marker, errors.New("root ownership marker is absent")
	}
	return marker, handle.VerifyPath(root)
}

func (in *inspection) inspectRoots(ctx context.Context, r Repair, env, task row) error {
	p := in.report.Plan
	sourceRoot, err := taskRoot(p.TasksRoot, r.SourcePath)
	if err != nil {
		return err
	}
	marker, err := readMarker(p.TasksRoot, sourceRoot)
	if err != nil {
		return err
	}
	if marker.TaskID != r.SourceRootTaskID || marker.TaskDirName != filepath.Base(sourceRoot) || marker.WorkspaceID != stringValue(task, "workspace_id") {
		return errors.New("source root marker does not match explicit task/workspace identity")
	}
	canonical := filepath.Join(p.TasksRoot, stringValue(env, "task_dir_name"))
	in.rows["marker:"+sourceRoot] = digest(marker)
	destRoot, err := taskRoot(p.TasksRoot, r.Path)
	if err != nil {
		return err
	}
	if destRoot != canonical {
		return errors.New("replacement checkout must be in the canonical environment root")
	}
	destMarker, err := readMarker(p.TasksRoot, destRoot)
	if err != nil {
		return err
	}
	if destMarker.TaskID != stringValue(env, "task_id") || destMarker.WorkspaceID != marker.WorkspaceID || destMarker.TaskDirName != filepath.Base(canonical) {
		return errors.New("destination root marker does not match environment ownership")
	}
	if r.SourcePath == r.Path {
		in.rows["marker:"+destRoot] = digest(destMarker)
		return nil
	}
	in.rows["marker:"+destRoot] = digest(destMarker)
	if _, err := os.Lstat(r.Path); !errors.Is(err, os.ErrNotExist) {
		return errors.New("relocation destination is present or cannot be inspected")
	}
	parent, err := storageworkspaces.OpenDirectoryNoFollow(p.TasksRoot, filepath.Dir(r.Path))
	if err != nil {
		return err
	}
	defer func() { _ = parent.Close() }()
	return in.verifySharedSource(ctx, r, env, task)
}

func (in *inspection) verifySharedSource(ctx context.Context, r Repair, env, task row) error {
	if r.SourceRootTaskID == stringValue(env, "task_id") {
		return nil
	}
	source, err := in.one(ctx, "root-task:"+r.SourceRootTaskID, `SELECT * FROM tasks WHERE id = ?`, r.SourceRootTaskID)
	if err != nil {
		return err
	}
	if stringValue(source, "parent_id") != stringValue(env, "task_id") || stringValue(source, "workspace_id") != stringValue(task, "workspace_id") {
		return errors.New("relocation source has no explicit shared parent relationship")
	}
	rows, err := in.read(ctx, "source-binding:"+r.WorktreeID, `SELECT id FROM task_sessions WHERE task_id = ? AND task_environment_id = ? AND repository_id = ? ORDER BY id`, r.SourceRootTaskID, r.EnvironmentID, r.RepositoryID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return errors.New("source task has no repository session bound to the selected environment")
	}
	return nil
}

func (in *inspection) inspectRepository(ctx context.Context, r Repair, slot, env row) error {
	repo, err := in.one(ctx, "repository:"+r.RepositoryID, `SELECT * FROM repositories WHERE id = ?`, r.RepositoryID)
	if err != nil {
		return err
	}
	task := in.tasks[stringValue(env, "task_id")]
	if stringValue(repo, "workspace_id") != stringValue(task, "workspace_id") || stringValue(repo, "local_path") != r.RepositoryPath || repo["deleted_at"] != nil {
		return errors.New("repository is unavailable or does not match workspace/local path")
	}
	if old := stringValue(slot, "repository_id"); old != "" && old != r.RepositoryID {
		return errors.New("repair cannot replace a nonempty repository identity")
	}
	return nil
}
