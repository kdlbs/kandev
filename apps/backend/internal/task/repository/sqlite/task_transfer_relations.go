package sqlite

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

// transferRelationInventoryCache memoizes inspectTaskTransferRelations. The
// schema it inspects (information_schema.columns on Postgres, PRAGMA
// table_info on SQLite, plus sqlite_master/information_schema.tables table
// sweeps) is fixed once Repository.initSchema completes during construction,
// so recomputing it on every transfer attempt is pure per-call overhead. The
// cache is filled lazily on first use and held for the lifetime of the
// Repository instance.
type transferRelationInventoryCache struct {
	mu          sync.RWMutex
	ready       bool
	projections []transferWorkspaceProjection
	counts      []transferWorkspaceProjection
	inventory   transferRelationInventory
}

func (r *Repository) lockTaskTransferRelations(
	ctx context.Context,
	tx *sqlx.Tx,
	projections []transferWorkspaceProjection,
	counts []transferWorkspaceProjection,
	inventory transferRelationInventory,
) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		_, err := tx.ExecContext(ctx, `UPDATE task_transfer_serialization SET version = version WHERE id = 1`)
		return err
	}
	tableSet := map[string]struct{}{
		"workspaces": {}, "workflows": {}, "workflow_steps": {}, "task_transfer_operations": {},
		"task_transfer_audit": {}, "task_transfer_serialization": {},
	}
	if inventory.labels {
		tableSet["office_labels"] = struct{}{}
	}
	if inventory.treeHolds {
		tableSet["office_task_tree_holds"] = struct{}{}
	}
	if inventory.agentProfiles {
		tableSet["agent_profiles"] = struct{}{}
	}
	for _, relation := range append(append([]transferWorkspaceProjection{}, projections...), counts...) {
		tableSet[relation.table] = struct{}{}
	}
	tables := make([]string, 0, len(tableSet))
	for table := range tableSet {
		if !validTransferRelationName(table) {
			return fmt.Errorf("invalid task transfer relation name")
		}
		tables = append(tables, `"`+strings.ReplaceAll(table, `"`, `""`)+`"`)
	}
	sort.Strings(tables)
	_, err := tx.ExecContext(ctx, `LOCK TABLE `+strings.Join(tables, ", ")+` IN SHARE ROW EXCLUSIVE MODE`)
	return err
}

func validTransferRelationName(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func (r *Repository) inspectTaskTransferRelations(
	ctx context.Context,
) ([]transferWorkspaceProjection, []transferWorkspaceProjection, transferRelationInventory, error) {
	cache := &r.transferRelationCache
	cache.mu.RLock()
	if cache.ready {
		projections, counts, inventory := cache.projections, cache.counts, cache.inventory
		cache.mu.RUnlock()
		return projections, counts, inventory, nil
	}
	cache.mu.RUnlock()

	projections, err := r.presentTransferTables(ctx, transferWorkspaceProjections, true)
	if err != nil {
		return nil, nil, transferRelationInventory{}, err
	}
	counts, err := r.presentTransferTables(ctx, transferPreservationTables, false)
	if err != nil {
		return nil, nil, transferRelationInventory{}, err
	}
	projections, counts, err = r.addDiscoveredTaskTransferRelations(ctx, projections, counts)
	if err != nil {
		return nil, nil, transferRelationInventory{}, err
	}
	inventory, err := r.inspectTransferRelationInventory(ctx)
	if err != nil {
		return nil, nil, transferRelationInventory{}, err
	}

	cache.mu.Lock()
	if !cache.ready {
		cache.projections, cache.counts, cache.inventory, cache.ready = projections, counts, inventory, true
	}
	cache.mu.Unlock()
	return projections, counts, inventory, nil
}

func (r *Repository) inspectTransferRelationInventory(ctx context.Context) (transferRelationInventory, error) {
	has := func(table string, columns ...string) (bool, error) {
		return r.relationHasColumns(ctx, table, columns...)
	}
	taskLabels, err := has("office_task_labels", "task_id", "label_id")
	if err != nil {
		return transferRelationInventory{}, err
	}
	labels, err := has("office_labels", "id", "workspace_id")
	if err != nil {
		return transferRelationInventory{}, err
	}
	groups, err := has("task_workspace_groups", "owner_task_id")
	if err != nil {
		return transferRelationInventory{}, err
	}
	members, err := has("task_workspace_group_members", "task_id")
	if err != nil {
		return transferRelationInventory{}, err
	}
	pendingMoves, err := has("pending_moves", "task_id", "workflow_id", "workflow_step_id")
	if err != nil {
		return transferRelationInventory{}, err
	}
	participants, err := has("workflow_step_participants", "task_id", "step_id", "id")
	if err != nil {
		return transferRelationInventory{}, err
	}
	decisions, err := has("workflow_step_decisions", "task_id", "step_id", "id")
	if err != nil {
		return transferRelationInventory{}, err
	}
	holds, err := has("office_task_tree_holds", "root_task_id", "workspace_id")
	if err != nil {
		return transferRelationInventory{}, err
	}
	holdMembers, err := has("office_task_tree_hold_members", "hold_id", "task_id")
	if err != nil {
		return transferRelationInventory{}, err
	}
	agentProfiles, err := has("agent_profiles", "id", "workspace_id", "role", "deleted_at", "enabled", "status")
	return transferRelationInventory{
		labels: taskLabels && labels, groups: groups && members, pendingMoves: pendingMoves,
		participants: participants, decisions: decisions, treeHolds: holds && holdMembers,
		agentProfiles: agentProfiles,
	}, err
}

func (r *Repository) addDiscoveredTaskTransferRelations(
	ctx context.Context,
	projections []transferWorkspaceProjection,
	counts []transferWorkspaceProjection,
) ([]transferWorkspaceProjection, []transferWorkspaceProjection, error) {
	tables, err := r.transferRelationTableNames(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, table := range tables {
		if table == "task_step_transitions" || table == "task_transfer_operations" || table == "task_transfer_audit" {
			continue
		}
		hasTaskID, relationErr := r.relationHasColumns(ctx, table, "task_id")
		if relationErr != nil {
			return nil, nil, relationErr
		}
		if !hasTaskID {
			continue
		}
		counts = appendTransferRelationIfMissing(counts, transferWorkspaceProjection{
			table: table, taskColumn: "task_id", identityColumn: "task_id",
		})
		hasWorkspaceID, relationErr := r.relationHasColumns(ctx, table, "workspace_id")
		if relationErr != nil {
			return nil, nil, relationErr
		}
		if hasWorkspaceID {
			projections = appendTransferRelationIfMissing(projections, transferWorkspaceProjection{
				table: table, taskColumn: "task_id",
			})
		}
	}
	sort.Slice(projections, func(i, j int) bool { return projections[i].table < projections[j].table })
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].table == counts[j].table {
			return counts[i].receiptKey < counts[j].receiptKey
		}
		return counts[i].table < counts[j].table
	})
	return projections, counts, nil
}

func (r *Repository) transferRelationTableNames(ctx context.Context) ([]string, error) {
	var tables []string
	if dialect.IsPostgres(r.db.DriverName()) {
		err := r.ro.SelectContext(ctx, &tables, `
			SELECT table_name FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' ORDER BY table_name`)
		return tables, err
	}
	err := r.ro.SelectContext(ctx, &tables, `
		SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	return tables, err
}

func appendTransferRelationIfMissing(
	relations []transferWorkspaceProjection,
	candidate transferWorkspaceProjection,
) []transferWorkspaceProjection {
	for _, existing := range relations {
		if existing.table == candidate.table && existing.taskColumn == candidate.taskColumn {
			return relations
		}
	}
	return append(relations, candidate)
}

func (r *Repository) presentTransferTables(
	ctx context.Context,
	candidates []transferWorkspaceProjection,
	requireWorkspace bool,
) ([]transferWorkspaceProjection, error) {
	present := make([]transferWorkspaceProjection, 0, len(candidates))
	for _, candidate := range candidates {
		columns := []string{candidate.taskColumn}
		if candidate.identityColumn != "" && candidate.identityColumn != candidate.taskColumn {
			columns = append(columns, candidate.identityColumn)
		}
		if requireWorkspace {
			columns = append(columns, "workspace_id")
		}
		hasColumns, err := r.relationHasColumns(ctx, candidate.table, columns...)
		if err != nil {
			return nil, err
		}
		if hasColumns {
			present = append(present, candidate)
		}
	}
	sort.Slice(present, func(i, j int) bool { return present[i].table < present[j].table })
	return present, nil
}

func (r *Repository) relationHasColumns(ctx context.Context, table string, columns ...string) (bool, error) {
	if dialect.IsPostgres(r.db.DriverName()) {
		var found []string
		err := r.ro.SelectContext(ctx, &found, r.ro.Rebind(`
			SELECT column_name FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ?`), table)
		return containsTransferColumns(found, columns), err
	}
	query := fmt.Sprintf(`PRAGMA table_info(%q)`, table)
	rows, err := r.ro.QueryxContext(ctx, query)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	found := make([]string, 0, len(columns))
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		found = append(found, name)
	}
	return containsTransferColumns(found, columns), rows.Err()
}

func containsTransferColumns(found, required []string) bool {
	set := make(map[string]struct{}, len(found))
	for _, column := range found {
		set[column] = struct{}{}
	}
	for _, column := range required {
		if _, ok := set[column]; !ok {
			return false
		}
	}
	return len(found) > 0
}
