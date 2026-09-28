package inventoryrepair

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
)

func (in *inspection) inspectWorkspaces(ctx context.Context) error {
	seen := map[string]bool{}
	for _, w := range in.report.Plan.Workspaces {
		env := in.envs[w.EnvironmentID]
		if env == nil || seen[w.EnvironmentID] {
			return errors.New("workspace repair needs a unique selected environment")
		}
		seen[w.EnvironmentID] = true
		if w.Path != filepath.Join(in.report.Plan.TasksRoot, stringValue(env, "task_dir_name")) {
			return errors.New("workspace replacement is not the canonical task root")
		}
		rows, err := in.read(ctx, "sessions:"+w.EnvironmentID, `SELECT * FROM task_sessions WHERE task_environment_id = ? ORDER BY id`, w.EnvironmentID)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(rows))
		for _, s := range rows {
			ids = append(ids, stringValue(s, "id"))
		}
		want := append([]string{}, w.SessionIDs...)
		sort.Strings(want)
		if !reflect.DeepEqual(ids, want) {
			return errors.New("workspace repair must explicitly include every bound session")
		}
	}
	return nil
}

func (in *inspection) inspectJobs(ctx context.Context) error {
	seen := map[string]bool{}
	for _, id := range in.report.Plan.CleanupJobs {
		job := in.jobs[id]
		if job == nil || seen[id] {
			return errors.New("cleanup repair must select a unique job of a selected task")
		}
		seen[id] = true
		if err := in.validateCleanupJob(job); err != nil {
			return fmt.Errorf("cleanup %s: %w", id, err)
		}
		if _, err := in.successorSnapshot(ctx, job, ""); err != nil {
			return err
		}
	}
	return nil
}

func (in *inspection) validateCleanupJob(job row) error {
	switch stringValue(job, "state") {
	case "pending", "retry_wait", "failed":
	default:
		return errors.New("only a non-running incomplete job can be superseded")
	}
	trigger := stringValue(job, "trigger")
	if trigger != "archive" && trigger != "cascade_archive" {
		return errors.New("only archive/cascade_archive snapshots can be repaired")
	}
	task := in.tasks[stringValue(job, "task_id")]
	if task == nil || task["archived_at"] == nil {
		return errors.New("cleanup task is no longer archived")
	}
	worktrees, err := repairableSnapshotWorktrees(job)
	if err != nil {
		return err
	}
	matched := false
	for _, wt := range worktrees {
		id, _ := wt["id"].(string)
		if wt["task_id"] != stringValue(job, "task_id") {
			return errors.New("snapshot contains another task's worktree")
		}
		if slot := in.slots[id]; slot != nil {
			if wt["path"] != stringValue(slot, "worktree_path") || wt["repository_id"] != stringValue(slot, "repository_id") {
				return errors.New("snapshot worktree differs from original inventory")
			}
			matched = true
		}
	}
	if !matched {
		return errors.New("cleanup snapshot contains no selected repair")
	}
	return nil
}

func repairableSnapshotWorktrees(job row) ([]map[string]any, error) {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stringValue(job, "resource_snapshot")), &snapshot); err != nil {
		return nil, err
	}
	if string(snapshot["archive_source_manifest_captured"]) == "true" || (len(snapshot["archive_source_manifest"]) > 0 && string(snapshot["archive_source_manifest"]) != "[]" && string(snapshot["archive_source_manifest"]) != "null") {
		return nil, errors.New("completed source evidence cannot be replaced by inventory repair")
	}
	var worktrees []map[string]any
	if err := json.Unmarshal(snapshot["worktrees"], &worktrees); err != nil {
		return nil, err
	}
	return worktrees, nil
}
