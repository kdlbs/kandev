package sqlite

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const principalContextConstraintName = "workspace_agent_principals_workspace_id_plugin_installation_id_logical_key_key"

const (
	principalTaskBindingIndexName    = "uniq_active_workspace_agent_principal_task"
	principalSessionBindingIndexName = "uniq_active_workspace_agent_principal_session"
)

const sqlitePrincipalContextViolationMessage = "UNIQUE constraint failed: workspace_agent_principals.workspace_id, workspace_agent_principals.plugin_installation_id, workspace_agent_principals.logical_key"

const (
	sqlitePrincipalTaskBindingViolationMessage    = "UNIQUE constraint failed: workspace_agent_principals.workspace_id, workspace_agent_principals.backing_task_id"
	sqlitePrincipalSessionBindingViolationMessage = "UNIQUE constraint failed: workspace_agent_principals.workspace_id, workspace_agent_principals.backing_session_id"
)

const principalGrantScopeIndexName = "uniq_active_principal_coordinator_grants_scope"

const sqlitePrincipalGrantScopeViolationMessage = "UNIQUE constraint failed: task_coordinator_grants.principal_id, task_coordinator_grants.scope_kind, task_coordinator_grants.scope_id"

func isPrincipalConflictViolation(err error) bool {
	return isCoordinatorAuthorityUniqueViolation(err, principalContextConstraintName, sqlitePrincipalContextViolationMessage) ||
		isCoordinatorAuthorityUniqueViolation(err, principalTaskBindingIndexName, sqlitePrincipalTaskBindingViolationMessage) ||
		isCoordinatorAuthorityUniqueViolation(err, principalSessionBindingIndexName, sqlitePrincipalSessionBindingViolationMessage)
}

func isPrincipalGrantScopeUniqueViolation(err error) bool {
	return isCoordinatorAuthorityUniqueViolation(err, principalGrantScopeIndexName, sqlitePrincipalGrantScopeViolationMessage)
}

func isCoordinatorAuthorityUniqueViolation(err error, pgConstraintName, sqliteMessage string) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == pgConstraintName
	}
	return strings.Contains(err.Error(), sqliteMessage)
}
