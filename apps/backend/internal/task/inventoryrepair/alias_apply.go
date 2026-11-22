package inventoryrepair

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"strings"
)

type aliasMutationBuilder struct {
	ctx     context.Context
	db      queryer
	changes []mutation
	indices map[string]int
}

func (b *aliasMutationBuilder) add(table, id string, values row, remove bool) error {
	key := table + ":" + id
	index, exists := b.indices[key]
	var m mutation
	if exists {
		m = b.changes[index]
	} else {
		m = mutation{Table: table, ID: id, RawTimestamps: true}
		original, err := mutationRow(b.ctx, b.db, m)
		if err != nil {
			return err
		}
		if original == nil {
			return errors.New("selected mutation row disappeared")
		}
		m.Before = original
		m.After = maps.Clone(original)
	}
	if remove {
		m.After = nil
	} else {
		for k, v := range values {
			m.After[k] = v
		}
	}
	if exists {
		b.changes[index] = m
	} else {
		b.indices[key] = len(b.changes)
		b.changes = append(b.changes, m)
	}
	return nil
}

func (in *inspection) aliasMutations(ctx context.Context) ([]mutation, error) {
	b := &aliasMutationBuilder{ctx: ctx, db: in.db, indices: map[string]int{}}
	for _, a := range in.report.Plan.Aliases {
		for _, id := range a.AliasRowIDs {
			if err := b.add(aliasSlotTable, id, nil, true); err != nil {
				return nil, err
			}
		}
	}
	for _, w := range in.report.Plan.Workspaces {
		if !in.retiresEnvironment(w.EnvironmentID) {
			if err := b.add(aliasEnvironmentTable, w.EnvironmentID, row{"workspace_path": w.Path}, false); err != nil {
				return nil, err
			}
		}
		for _, id := range w.SessionIDs {
			if err := b.add(aliasSessionTable, id, row{"workspace_path": w.Path}, false); err != nil {
				return nil, err
			}
		}
	}
	if err := in.aliasBindingMutations(b); err != nil {
		return nil, err
	}
	return b.changes, nil
}

func (in *inspection) retiresEnvironment(id string) bool {
	for _, e := range in.report.Plan.RedundantEnvironments {
		if e.EnvironmentID == id {
			return true
		}
	}
	return false
}

// SQLite's time conversion normalizes text. Journal removals retain the original
// timestamp bytes so reinsertion does not rewrite historical observations.
func rawTimestampProjection(ctx context.Context, db queryer, table string) (string, error) {
	columns, err := queryRows(ctx, db, `PRAGMA table_info("`+table+`")`)
	if err != nil {
		return "", err
	}
	projection := make([]string, 0, len(columns))
	for _, column := range columns {
		name := stringValue(column, "name")
		if !columnPattern.MatchString(name) {
			return "", errors.New("unsupported journal column")
		}
		quoted := `"` + name + `"`
		typ := strings.ToUpper(stringValue(column, "type"))
		if strings.Contains(typ, "TIME") || typ == "DATE" {
			quoted = `CAST(` + quoted + ` AS TEXT) AS ` + quoted
		}
		projection = append(projection, quoted)
	}
	return strings.Join(projection, ","), nil
}

func (in *inspection) validateAliasWorkspacePaths(ctx context.Context) error {
	for _, e := range in.report.Plan.RedundantEnvironments {
		root := filepath.Join(in.report.Plan.TasksRoot, stringValue(in.envs[e.CanonicalEnvironmentID], "task_dir_name"))
		sessions, err := queryRows(ctx, in.db, `SELECT * FROM task_sessions WHERE task_environment_id=? ORDER BY id`, e.EnvironmentID)
		if err != nil {
			return err
		}
		for _, s := range sessions {
			if stringValue(s, "workspace_path") == root {
				continue
			}
			found := false
			for _, w := range in.report.Plan.Workspaces {
				if w.EnvironmentID == e.EnvironmentID && w.Path == root {
					found = true
				}
			}
			if !found {
				return errors.New("session path correction requires an explicit workspace selection")
			}
		}
	}
	return nil
}

func (in *inspection) aliasBindingMutations(b *aliasMutationBuilder) error {
	for _, e := range in.report.Plan.RedundantEnvironments {
		for _, id := range e.SessionIDs {
			if err := b.add(aliasSessionTable, id, row{environmentIDColumn: e.CanonicalEnvironmentID}, false); err != nil {
				return err
			}
		}
		for _, id := range e.SnapshotIDs {
			if err := b.add(snapshotTable, id, row{environmentIDColumn: e.CanonicalEnvironmentID}, false); err != nil {
				return err
			}
		}
	}
	for _, e := range in.report.Plan.RedundantEnvironments {
		if err := b.add(aliasEnvironmentTable, e.EnvironmentID, nil, true); err != nil {
			return err
		}
	}
	return nil
}
