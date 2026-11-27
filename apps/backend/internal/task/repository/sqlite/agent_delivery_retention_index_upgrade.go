package sqlite

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db"
)

func (r *Repository) upgradeAgentDeliveryEligibleIndex() error {
	ctx := r.migrationContext()
	definition, err := db.IndexDefinitionContext(ctx, r.db, agentDeliveryEligibleIndexName)
	if err != nil {
		return fmt.Errorf("read agent delivery eligible index definition: %w", err)
	}
	if definition == "" || strings.Contains(strings.ToLower(definition), "projected_at is not null") {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent delivery eligible index upgrade: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.failAgentDeliveryRetention("before_eligible_index_upgrade"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_agent_delivery_inbox_retention_eligible`); err != nil {
		return fmt.Errorf("drop old agent delivery eligible index: %w", err)
	}
	if err := r.failAgentDeliveryRetention("after_eligible_index_drop"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, agentDeliveryEligibleIndexQuery); err != nil {
		return fmt.Errorf("create projected agent delivery eligible index: %w", err)
	}
	if err := r.failAgentDeliveryRetention("after_eligible_index_create"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent delivery eligible index upgrade: %w", err)
	}
	return nil
}

func (r *Repository) upgradeAgentDeliveryOwnershipIndex() error {
	ctx := r.migrationContext()
	definition, err := db.IndexDefinitionContext(ctx, r.db, agentDeliveryOwnershipIndexName)
	if err != nil {
		return fmt.Errorf("read agent delivery ownership index definition: %w", err)
	}
	if definition == "" || (strings.Contains(strings.Join(strings.Fields(definition), ""), "(session_id,incarnation_id)") && strings.Contains(definition, "session_id <>")) {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent delivery ownership index upgrade: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.failAgentDeliveryRetention("before_ownership_index_upgrade"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_agent_delivery_effects_retirement`); err != nil {
		return fmt.Errorf("drop old agent delivery ownership index: %w", err)
	}
	if err := r.failAgentDeliveryRetention("after_ownership_index_drop"); err != nil {
		return err
	}
	for _, query := range []string{agentDeliveryOwnershipIndexQuery, agentDeliveryOwnedOrderedIndexQuery} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("create agent delivery ownership index: %w", err)
		}
	}
	if err := r.failAgentDeliveryRetention("after_ownership_index_create"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent delivery ownership index upgrade: %w", err)
	}
	return nil
}

func (r *Repository) upgradeAgentDeliveryUnprojectedIndex() error {
	ctx := r.migrationContext()
	definition, err := db.IndexDefinitionContext(ctx, r.db, agentDeliveryUnprojectedIndexName)
	if err != nil {
		return fmt.Errorf("read agent delivery unprojected index definition: %w", err)
	}
	if strings.Contains(strings.ToLower(definition), "projected_at is null") {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent delivery unprojected index upgrade: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.failAgentDeliveryRetention("before_unprojected_index_upgrade"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_agent_delivery_inbox_unprojected`); err != nil {
		return fmt.Errorf("drop old agent delivery unprojected index: %w", err)
	}
	if err := r.failAgentDeliveryRetention("after_unprojected_index_drop"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, agentDeliveryUnprojectedIndexQuery); err != nil {
		return fmt.Errorf("create agent delivery unprojected index: %w", err)
	}
	if err := r.failAgentDeliveryRetention("after_unprojected_index_create"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent delivery unprojected index upgrade: %w", err)
	}
	return nil
}
