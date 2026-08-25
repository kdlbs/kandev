package sqlite

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// principalContextConstraintName is the UNIQUE(workspace_id,
// plugin_installation_id, logical_key) constraint declared inline on
// workspace_agent_principals. SQLite auto-names inline table constraints
// "sqlite_autoindex_<table>_<n>" and never exposes that name in errors, so
// the SQLite branch matches the failing column list instead; Postgres names
// the constraint "workspace_agent_principals_workspace_id_plugin_install
// ation_id_logical_key_key", which is unambiguous for the typed branch.
const principalContextConstraintName = "workspace_agent_principals_workspace_id_plugin_installation_id_logical_key_key"

const (
	principalTaskBindingIndexName    = "uniq_active_workspace_agent_principal_task"
	principalSessionBindingIndexName = "uniq_active_workspace_agent_principal_session"
)

// sqlitePrincipalContextViolationMessage is the substring go-sqlite3 puts in
// a UNIQUE-constraint error for the inline principal-context constraint. The
// column triple appears in exactly one UNIQUE constraint on the table, so
// matching it attributes the violation correctly - a bare "UNIQUE constraint
// failed" match would also fire on the table's primary key. Mirrors
// sqliteUsageEventIDViolationMessage (usage_events_unique.go).
const sqlitePrincipalContextViolationMessage = "UNIQUE constraint failed: workspace_agent_principals.workspace_id, workspace_agent_principals.plugin_installation_id, workspace_agent_principals.logical_key"

const (
	sqlitePrincipalTaskBindingViolationMessage    = "UNIQUE constraint failed: workspace_agent_principals.workspace_id, workspace_agent_principals.backing_task_id"
	sqlitePrincipalSessionBindingViolationMessage = "UNIQUE constraint failed: workspace_agent_principals.workspace_id, workspace_agent_principals.backing_session_id"
)

const principalGrantScopeIndexName = "uniq_active_principal_coordinator_grants_scope"

// sqlitePrincipalGrantScopeViolationMessage attributes a violation of
// uniq_active_principal_coordinator_grants_scope by its column list. The
// partial unique index covers exactly these three columns on
// task_coordinator_grants, distinct from the legacy task-bound index
// (coordinator_task_id, scope_kind, scope_id).
const sqlitePrincipalGrantScopeViolationMessage = "UNIQUE constraint failed: task_coordinator_grants.principal_id, task_coordinator_grants.scope_kind, task_coordinator_grants.scope_id"

// sqliteTaskGrantScopeViolationMessage is the SQLite message for the
// task-bound partial index, which fires when principal_id is empty
// and the same (coordinator_task_id, scope_kind, scope_id) combination
// already has an active grant.
const sqliteTaskGrantScopeViolationMessage = "UNIQUE constraint failed: task_coordinator_grants.coordinator_task_id, task_coordinator_grants.scope_kind, task_coordinator_grants.scope_id"

// taskGrantScopeIndexName is the name of the task-bound partial unique
// index on (coordinator_task_id, scope_kind, scope_id) WHERE principal_id IS NULL AND revoked_at IS NULL.
const taskGrantScopeIndexName = "uniq_active_task_coordinator_grants_scope"

// isPrincipalGrantScopeUniqueViolation reports whether err is a violation of

// isPrincipalConflictViolation reports whether err would make either the
// durable context or its active task/session binding ambiguous.
func isPrincipalConflictViolation(err error) bool {
	return isUniqueViolation(err, principalContextConstraintName, sqlitePrincipalContextViolationMessage) ||
		isUniqueViolation(err, principalTaskBindingIndexName, sqlitePrincipalTaskBindingViolationMessage) ||
		isUniqueViolation(err, principalSessionBindingIndexName, sqlitePrincipalSessionBindingViolationMessage)
}

// isPrincipalGrantScopeUniqueViolation reports whether err is a violation of
// uniq_active_principal_coordinator_grants_scope specifically — i.e. the
// principal already holds an active grant for the same scope.
func isPrincipalGrantScopeUniqueViolation(err error) bool {
	return isUniqueViolation(err, principalGrantScopeIndexName, sqlitePrincipalGrantScopeViolationMessage)
}

// isTaskGrantScopeUniqueViolation reports whether err is a violation of
// uniq_active_task_coordinator_grants_scope — i.e. the same
// (coordinator_task_id, scope_kind, scope_id) combination already has an
// active grant with no principal_id (the HTTP API path).
func isTaskGrantScopeUniqueViolation(err error) bool {
	return isUniqueViolation(err, taskGrantScopeIndexName, sqliteTaskGrantScopeViolationMessage)
}

// isCoordinatorGrantScopeUniqueViolation checks both the principal-bound and
// task-bound unique indexes, covering both the plugin (principal_id non-empty)
// and HTTP API (principal_id empty) code paths.
func isCoordinatorGrantScopeUniqueViolation(err error) bool {
	return isPrincipalGrantScopeUniqueViolation(err) || isTaskGrantScopeUniqueViolation(err)
}

func isUniqueViolation(err error, pgConstraintName, sqliteMessage string) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == pgConstraintName
	}
	return strings.Contains(err.Error(), sqliteMessage)
}
