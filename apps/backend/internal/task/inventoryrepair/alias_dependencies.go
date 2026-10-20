package inventoryrepair

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

func (in *inspection) inspectAliasTask(ctx context.Context, id string) error {
	task, err := in.one(ctx, "task:"+id, `SELECT * FROM tasks WHERE id=?`, id)
	if err != nil {
		return err
	}
	in.tasks[id] = task
	sessions, err := in.read(ctx, "task-sessions:"+id, `SELECT * FROM task_sessions WHERE task_id=? ORDER BY id`, id)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		switch stringValue(s, "state") {
		case "CREATED", sessionWaitingState, "COMPLETED", "CANCELLED", "FAILED":
		default:
			in.report.Blockers = append(in.report.Blockers, "active or unknown session "+stringValue(s, "id"))
		}
	}
	jobs, err := in.read(ctx, "jobs:"+id, `SELECT `+cleanupJobProjection+` FROM task_resource_cleanup_jobs WHERE task_id=? ORDER BY id`, id)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		switch stringValue(job, "state") {
		case "succeeded", aliasStatusCancelled:
		default:
			in.report.Blockers = append(in.report.Blockers, "non-inert cleanup "+stringValue(job, "id"))
		}
	}
	runtimes, err := in.read(ctx, "task-runtimes:"+id, `SELECT er.* FROM executors_running er JOIN task_sessions s ON s.id=er.session_id WHERE s.task_id=? ORDER BY er.id`, id)
	if err != nil {
		return err
	}
	for _, runtime := range runtimes {
		switch stringValue(runtime, "status") {
		case "stopped", aliasStatusFailed, "exited":
		default:
			in.report.Blockers = append(in.report.Blockers, "runtime "+stringValue(runtime, "id"))
		}
		in.inspectRuntimePIDs(runtime)
	}
	return nil
}

func (in *inspection) inspectAliasDependencies(ctx context.Context) error {
	paths := map[string]bool{}
	sessionCount := 0
	taskIDs := map[string]bool{}
	for id := range in.tasks {
		taskIDs[id] = true
	}
	for id := range in.envs {
		if err := addConsumerPath(paths, stringValue(in.envs[id], "workspace_path")); err != nil {
			return err
		}
		sessions, err := in.read(ctx, "sessions:"+id, `SELECT * FROM task_sessions WHERE task_environment_id=? ORDER BY id`, id)
		if err != nil {
			return err
		}
		sessionCount += len(sessions)
		if sessionCount > 256 || len(in.envs) > 16 {
			return errors.New("selection exceeds 256 sessions or 16 environments")
		}
		for _, session := range sessions {
			if err := addConsumerPath(paths, stringValue(session, "workspace_path")); err != nil {
				return err
			}
			taskIDs[stringValue(session, "task_id")] = true
		}
	}
	consumerPaths := make([]string, 0, len(paths))
	for path := range paths {
		consumerPaths = append(consumerPaths, path)
	}
	slices.Sort(consumerPaths)
	if expected := in.report.Plan.ExpectedConsumerPaths; expected != nil && !slices.Equal(expected, consumerPaths) {
		return errors.New("workspace consumer paths changed since preview")
	}
	in.report.Plan.ExpectedConsumerPaths = consumerPaths
	for id := range taskIDs {
		if err := in.inspectAliasTask(ctx, id); err != nil {
			return err
		}
	}
	return in.inspectAliasSchema(ctx)
}

func (in *inspection) inspectAliasSchema(ctx context.Context) error {
	schema, err := in.read(ctx, "alias-schema", `SELECT type,name,tbl_name,sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name`)
	if err != nil {
		return err
	}
	for _, table := range schema {
		if stringValue(table, "type") != "table" {
			continue
		}
		if err = in.inspectAliasTableReferences(ctx, stringValue(table, "name")); err != nil {
			return err
		}
	}
	return nil
}

func (in *inspection) inspectAliasTableReferences(ctx context.Context, table string) error {
	if !columnPattern.MatchString(table) {
		return errors.New("unsupported table identifier in repair schema")
	}
	columns, err := queryRows(ctx, in.db, `PRAGMA table_info("`+table+`")`)
	if err != nil {
		return err
	}
	refs := map[string]bool{}
	for _, c := range columns {
		name := stringValue(c, "name")
		if name == "environment_id" || strings.HasSuffix(name, "_environment_id") {
			refs[name] = true
		}
	}
	keys, err := queryRows(ctx, in.db, `PRAGMA foreign_key_list("`+table+`")`)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if stringValue(key, "table") == aliasEnvironmentTable {
			refs[stringValue(key, "from")] = true
		}
	}
	for column := range refs {
		if !columnPattern.MatchString(column) {
			return errors.New("unsupported reference column")
		}
		for id := range in.envs {
			if err = in.inspectAliasReference(ctx, table, column, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (in *inspection) inspectAliasReference(ctx context.Context, table, column, id string) error {
	rows, err := in.read(ctx, "references:"+table+":"+column+":"+id, `SELECT * FROM "`+table+`" WHERE "`+column+`"=? ORDER BY 1`, id)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	if column == environmentIDColumn {
		switch table {
		case aliasSlotTable, aliasSessionTable, snapshotTable:
			return nil
		}
	}
	if table == "task_workspace_groups" && column == "materialized_environment_id" {
		for _, e := range in.report.Plan.RedundantEnvironments {
			if e.EnvironmentID == id {
				return errors.New("redundant environment remains a group materialization")
			}
		}
		return nil
	}
	return fmt.Errorf("unsupported dependent records in %s.%s for environment %s", table, column, id)
}

func addConsumerPath(paths map[string]bool, path string) error {
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("consumer workspace path is not canonical and absolute")
	}
	paths[path] = true
	return nil
}
