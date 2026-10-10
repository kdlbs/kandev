package inventoryrepair

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/mattn/go-sqlite3"
)

var columnPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// The final aliases retain timestamp text for exact rollback instead of driver normalization.
const cleanupJobProjection = `*, CAST(created_at AS TEXT) AS created_at,
CAST(updated_at AS TEXT) AS updated_at, CAST(completed_at AS TEXT) AS completed_at,
CAST(next_attempt_at AS TEXT) AS next_attempt_at`

func (in *inspection) cleanupMutations(ctx context.Context, job row, at time.Time) ([]mutation, error) {
	snapshot, err := in.successorSnapshot(ctx, job, at.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	predecessor := maps.Clone(job)
	timestamp := at.Format(sqlite3.SQLiteTimestampFormats[0])
	predecessor["state"] = "cancelled"
	predecessor["completed_at"] = timestamp
	predecessor["updated_at"] = timestamp
	successor := maps.Clone(job)
	id := "inventory-repair-" + in.report.Plan.OperationID + "-" + stringValue(job, "id")
	successor["id"] = id
	successor["operation_id"] = id
	successor["state"] = "pending"
	successor["attempts"] = 0
	successor["next_attempt_at"] = nil
	successor["last_error"] = ""
	successor["completed_at"] = nil
	successor["created_at"] = timestamp
	successor["updated_at"] = timestamp
	successor["resource_snapshot"] = snapshot
	existing, err := queryRows(ctx, in.db, `SELECT id FROM task_resource_cleanup_jobs WHERE id = ? OR operation_id = ?`, id, id)
	if err != nil {
		return nil, err
	}
	if len(existing) != 0 {
		return nil, errors.New("cleanup successor already exists")
	}
	return []mutation{{"task_resource_cleanup_jobs", stringValue(job, "id"), job, predecessor}, {"task_resource_cleanup_jobs", id, nil, successor}}, nil
}

func (in *inspection) successorSnapshot(ctx context.Context, job row, at string) (string, error) {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stringValue(job, "resource_snapshot")), &snapshot); err != nil {
		return "", err
	}
	var worktrees []map[string]any
	if err := json.Unmarshal(snapshot["worktrees"], &worktrees); err != nil {
		return "", err
	}
	heads, err := snapshotStrings(snapshot, "worktree_head_oids")
	if err != nil {
		return "", err
	}
	dirs, err := snapshotStrings(snapshot, "worktree_task_dir_names")
	if err != nil {
		return "", err
	}
	absent := []string{}
	for _, wt := range worktrees {
		id, _ := wt["id"].(string)
		if r, ok := in.selectedRepair(id); ok {
			wt["repository_id"] = r.RepositoryID
			wt["repository_path"] = r.RepositoryPath
			wt["path"] = r.Path
			wt["branch"] = r.Branch
			heads[id] = r.HeadOID
			dirs[id] = stringValue(in.envs[r.EnvironmentID], "task_dir_name")
			continue
		}
		if err := in.verifyAbsentSnapshotWorktree(ctx, wt); err != nil {
			return "", err
		}
		absent = append(absent, id)
	}
	snapshot["worktrees"], err = json.Marshal(worktrees)
	if err != nil {
		return "", err
	}
	snapshot["worktree_head_oids"], err = json.Marshal(heads)
	if err != nil {
		return "", err
	}
	snapshot["worktree_task_dir_names"], err = json.Marshal(dirs)
	if err != nil {
		return "", err
	}
	snapshot["inventory_repair"], err = json.Marshal(map[string]any{"operation_id": in.report.Plan.OperationID, "predecessor_job_id": stringValue(job, "id"), "observed_at": at, "absent_worktree_ids": absent})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(snapshot)
	return string(data), err
}

func snapshotStrings(snapshot map[string]json.RawMessage, key string) (map[string]string, error) {
	values := map[string]string{}
	if len(snapshot[key]) == 0 {
		return values, nil
	}
	if err := json.Unmarshal(snapshot[key], &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = map[string]string{}
	}
	return values, nil
}

func (in *inspection) selectedRepair(id string) (Repair, bool) {
	for _, r := range in.report.Plan.Repairs {
		if r.WorktreeID == id {
			return r, true
		}
	}
	return Repair{}, false
}

func (in *inspection) verifyAbsentSnapshotWorktree(ctx context.Context, wt row) error {
	path := stringValue(wt, "path")
	root, err := taskRoot(in.report.Plan.TasksRoot, path)
	if err != nil {
		return err
	}
	if _, err = readMarker(in.report.Plan.TasksRoot, root); err != nil {
		return err
	}
	if filepath.Clean(path) != path {
		return errors.New("noncanonical absent snapshot path")
	}
	if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return errors.New("unselected snapshot worktree is present or inaccessible")
	}
	rows, err := in.read(ctx, "absent-worktree:"+stringValue(wt, "id"), `SELECT id FROM task_environment_repos WHERE worktree_id = ? AND deleted_at IS NULL AND status = 'active'`, stringValue(wt, "id"))
	if err != nil {
		return err
	}
	if len(rows) != 0 {
		return errors.New("absent snapshot worktree still has active inventory")
	}
	if stringValue(wt, "repository_id") == "" || stringValue(wt, "repository_path") == "" {
		return fmt.Errorf("absent snapshot worktree %s lacks historical repository identity", stringValue(wt, "id"))
	}
	return nil
}
