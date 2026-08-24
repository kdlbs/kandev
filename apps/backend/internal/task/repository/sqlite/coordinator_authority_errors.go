package sqlite

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const principalContextConstraintName = "workspace_agent_principals_workspace_id_plugin_installation_id_logical_key_key"

const sqlitePrincipalContextViolationMessage = "UNIQUE constraint failed: workspace_agent_principals.workspace_id, workspace_agent_principals.plugin_installation_id, workspace_agent_principals.logical_key"

const principalGrantScopeIndexName = "uniq_active_principal_coordinator_grants_scope"

const sqlitePrincipalGrantScopeViolationMessage = "UNIQUE constraint failed: task_coordinator_grants.principal_id, task_coordinator_grants.scope_kind, task_coordinator_grants.scope_id"

func isPrincipalContextUniqueViolation(err error) bool {
	return isCoordinatorAuthorityUniqueViolation(err, principalContextConstraintName, sqlitePrincipalContextViolationMessage)
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
