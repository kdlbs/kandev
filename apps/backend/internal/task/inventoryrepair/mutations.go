package inventoryrepair

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"
)

type mutation struct {
	Table  string `json:"table"`
	ID     string `json:"id"`
	Before row    `json:"before"`
	After  row    `json:"after"`
}

func (in *inspection) mutations(ctx context.Context, at time.Time) ([]mutation, error) {
	var result []mutation
	for _, r := range in.report.Plan.Repairs {
		before := in.slots[r.WorktreeID]
		after := maps.Clone(before)
		after["repository_id"] = r.RepositoryID
		after["worktree_branch"] = r.Branch
		after["worktree_path"] = r.Path
		result = append(result, mutation{"task_environment_repos", stringValue(before, "id"), before, after})
	}
	for _, w := range in.report.Plan.Workspaces {
		before := in.envs[w.EnvironmentID]
		after := maps.Clone(before)
		after["workspace_path"] = w.Path
		result = append(result, mutation{"task_environments", w.EnvironmentID, before, after})
		sessions, err := queryRows(ctx, in.db, `SELECT * FROM task_sessions WHERE task_environment_id = ? ORDER BY id`, w.EnvironmentID)
		if err != nil {
			return nil, err
		}
		for _, s := range sessions {
			a := maps.Clone(s)
			a["workspace_path"] = w.Path
			result = append(result, mutation{"task_sessions", stringValue(s, "id"), s, a})
		}
	}
	for _, id := range in.report.Plan.CleanupJobs {
		changes, err := in.cleanupMutations(ctx, in.jobs[id], at)
		if err != nil {
			return nil, err
		}
		result = append(result, changes...)
	}
	return result, nil
}

func mutationTable(table string) (string, error) {
	switch table {
	case "task_environment_repos", "task_environments", "task_sessions", "task_resource_cleanup_jobs":
		return table, nil
	default:
		return "", errors.New("unsupported repair table")
	}
}

func mutationRow(ctx context.Context, db queryer, m mutation) (row, error) {
	table, err := mutationTable(m.Table)
	if err != nil {
		return nil, err
	}
	columns := "*"
	if table == "task_resource_cleanup_jobs" {
		columns = cleanupJobProjection
	}
	rows, err := queryRows(ctx, db, `SELECT `+columns+` FROM `+table+` WHERE id = ?`, m.ID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 {
		return nil, errors.New("ambiguous mutation row")
	}
	return rows[0], nil
}

func rowsMatch(ctx context.Context, db queryer, changes []mutation, after bool) error {
	for _, m := range changes {
		actual, err := mutationRow(ctx, db, m)
		if err != nil {
			return err
		}
		want := m.Before
		if after {
			want = m.After
		}
		if digest(actual) != digest(want) {
			return fmt.Errorf("%s %s changed since repair observation", m.Table, m.ID)
		}
	}
	return nil
}

func publish(ctx context.Context, tx *sql.Tx, changes []mutation, forward bool) error {
	if err := rowsMatch(ctx, tx, changes, !forward); err != nil {
		return err
	}
	for _, m := range changes {
		if !forward {
			m.Before, m.After = m.After, m.Before
		}
		if err := writeMutation(ctx, tx, m); err != nil {
			return err
		}
	}
	return rowsMatch(ctx, tx, changes, forward)
}

func writeMutation(ctx context.Context, tx *sql.Tx, m mutation) error {
	table, err := mutationTable(m.Table)
	if err != nil {
		return err
	}
	if m.After == nil {
		_, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE id = ?`, m.ID)
		return err
	}
	keys := make([]string, 0, len(m.After))
	for k, v := range m.After {
		if !columnPattern.MatchString(k) {
			return errors.New("invalid repair column")
		}
		if m.Before != nil && digest(v) == digest(m.Before[k]) {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	args := make([]any, 0, len(keys)+1)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		args = append(args, m.After[k])
		parts = append(parts, `"`+k+`" = ?`)
	}
	query := `UPDATE ` + table + ` SET ` + strings.Join(parts, ",") + ` WHERE id = ?`
	if m.Before == nil {
		query = `INSERT INTO ` + table + ` ("` + strings.Join(keys, `","`) + `") VALUES (` + strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",") + `)`
	} else {
		args = append(args, m.ID)
	}
	_, err = tx.ExecContext(ctx, query, args...)
	return err
}
