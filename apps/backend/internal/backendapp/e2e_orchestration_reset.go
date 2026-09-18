package backendapp

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type e2eOrchestrationOwner struct{ id, role string }

// Call only after native task deletion and resource cleanup have finished for
// the fixture workspace. Shared providers and other workspaces remain intact.
func resetOrchestrationForE2E(ctx context.Context, database *sqlx.DB, workspace string) error {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	owners, err := orchestrationOwnersForE2E(ctx, tx, workspace)
	if err != nil {
		return err
	}
	for _, owner := range owners {
		if err = deleteOrchestrationOwnerForE2E(ctx, tx, owner); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func orchestrationOwnersForE2E(ctx context.Context, tx *sqlx.Tx, workspace string) ([]e2eOrchestrationOwner, error) {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT a.id,COALESCE(o.role_id,'') FROM agent_profiles a
 LEFT JOIN workspace_orchestrators o ON o.agent_id=a.id WHERE a.workspace_id=?
 AND (o.agent_id IS NOT NULL OR EXISTS (SELECT 1 FROM orchestration_conversations c WHERE c.agent_profile_id=a.id))
 ORDER BY a.id`), workspace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	owners := []e2eOrchestrationOwner{}
	for rows.Next() {
		var owner e2eOrchestrationOwner
		if err = rows.Scan(&owner.id, &owner.role); err != nil {
			return nil, err
		}
		owners = append(owners, owner)
	}
	return owners, rows.Err()
}

func deleteOrchestrationOwnerForE2E(ctx context.Context, tx *sqlx.Tx, owner e2eOrchestrationOwner) error {
	for _, query := range []string{
		`DELETE FROM orchestration_memory WHERE agent_profile_id=?`,
		`DELETE FROM orchestration_instructions WHERE agent_profile_id=?`,
		`DELETE FROM orchestration_conversations WHERE agent_profile_id=?`,
		`DELETE FROM orchestration_legacy_imports WHERE agent_profile_id=?`,
		`DELETE FROM agent_profiles WHERE id=?`,
	} {
		if _, err := tx.ExecContext(ctx, tx.Rebind(query), owner.id); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM orchestration_roles WHERE id=? AND id<>'chief-of-staff'
 AND NOT EXISTS (SELECT 1 FROM workspace_orchestrators WHERE role_id=?)`), owner.role, owner.role)
	return err
}

func resetOfficeRoutingForE2E(ctx context.Context, database *sqlx.DB, workspace string) error {
	_, err := database.ExecContext(ctx, database.Rebind(`UPDATE agent_profiles
 SET settings='{"routing":{"provider_order_source":"inherit","tier_source":"inherit"}}'
 WHERE workspace_id=? AND role<>''
 AND NOT EXISTS (SELECT 1 FROM workspace_orchestrators WHERE agent_id=agent_profiles.id)
 AND NOT EXISTS (SELECT 1 FROM orchestration_conversations WHERE agent_profile_id=agent_profiles.id)`), workspace)
	return err
}
