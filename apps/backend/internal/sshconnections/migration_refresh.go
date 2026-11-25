package sshconnections

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// refreshSSHMigration retries local resolution for selectors which were
// unavailable at upgrade. It only fills facts absent from the imported route.
func (s *Service) refreshSSHMigration(ctx context.Context, executorID string) error {
	shouldRetry, err := retryableMigrationIn(ctx, s.store.db, executorID)
	if err != nil || !shouldRetry {
		return err
	}
	return s.store.write(ctx, func(tx *sqlx.Tx) error {
		migration, err := migrationIn(ctx, tx, executorID)
		if err != nil {
			return err
		}
		if !retryableMigration(migration) {
			return nil
		}
		return refreshMigrationIn(ctx, tx, executorID, migration)
	})
}

func retryableMigrationIn(ctx context.Context, db *sqlx.DB, executorID string) (bool, error) {
	migration, err := migrationIn(ctx, db, executorID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil && retryableMigration(migration), err
}

func retryableMigration(migration sshMigration) bool {
	return !migration.Verified && (migration.Reason == migrationAlias || migration.Reason == migrationIdentity)
}

func refreshMigrationIn(ctx context.Context, tx *sqlx.Tx, executorID string, migration sshMigration) error {
	executor, err := migrationExecutorIn(ctx, tx, executorID)
	if err != nil {
		return err
	}
	currentID, stamp := executor.connectionID, executor.stamp
	if currentID != migration.ConnectionID || stamp != migration.RouteStamp {
		return nil
	}
	route, err := routeIn(ctx, tx, currentID)
	if err != nil {
		return err
	}
	if route.Stamp != stamp || !route.Destination.ImportedPending || route.Destination.VerifiedRoute != "" {
		return nil
	}
	plan := resolveMigration(migration)
	if plan.reason != "" {
		return nil
	}
	if !migrationRouteUnchanged(ctx, tx, route, plan) {
		return nil
	}
	if err = writeRefreshedMigrationHops(ctx, tx, route, plan); err != nil {
		return err
	}
	route, err = routeIn(ctx, tx, currentID)
	if err != nil {
		return err
	}
	if err = updateMigratedExecutorIn(ctx, tx, executorID, executor.raw, executor.fields, route); err != nil {
		return err
	}
	return commitMigrationIn(ctx, tx, executorID, migration, plan, route, "executor.migration.resolve")
}

type migrationExecutorConfig struct {
	raw          string
	fields       map[string]json.RawMessage
	connectionID string
	stamp        string
}

func migrationExecutorIn(ctx context.Context, tx *sqlx.Tx, executorID string) (migrationExecutorConfig, error) {
	var executor migrationExecutorConfig
	query := tx.Rebind(`SELECT config FROM executors WHERE id=? AND type=? AND deleted_at IS NULL`)
	if err := tx.GetContext(ctx, &executor.raw, query, executorID, executorKindSSH); err != nil {
		return executor, err
	}
	if err := json.Unmarshal([]byte(executor.raw), &executor.fields); err != nil {
		return executor, ErrInvalidRoute
	}
	_ = json.Unmarshal(executor.fields[ConnectionConfigKey], &executor.connectionID)
	_ = json.Unmarshal(executor.fields["ssh_connection_stamp"], &executor.stamp)
	return executor, nil
}

func migrationRouteUnchanged(ctx context.Context, tx *sqlx.Tx, old Route, plan migrationPlan) bool {
	return migrationHopUnchanged(ctx, tx, old.Destination, plan.destination) &&
		(old.Bastion == nil || plan.bastion != nil && migrationHopUnchanged(ctx, tx, *old.Bastion, *plan.bastion))
}

func migrationHopUnchanged(ctx context.Context, tx *sqlx.Tx, old Connection, planned migrationHop) bool {
	next := planned.connection
	if old.Host != "" && old.Host != next.Host || old.User != "" && old.User != next.User ||
		old.Port != 0 && old.Port != next.Port || old.Fingerprint != "" && old.Fingerprint != next.Fingerprint {
		return false
	}
	if old.IdentityID == "" {
		return true
	}
	identity, err := identityIn(ctx, tx, old.IdentityID)
	return err == nil && identity.Kind == planned.identity.Kind && identity.Reference == planned.identity.Reference
}

func writeRefreshedMigrationHops(ctx context.Context, tx *sqlx.Tx, old Route, plan migrationPlan) error {
	destination := plan.destination
	destination.connection.ImportedPending = true
	if plan.bastion != nil {
		var bastion Connection
		var err error
		if old.Bastion != nil {
			bastion, err = saveMigrationHop(ctx, tx, *old.Bastion, *plan.bastion)
		} else {
			bastion, err = putMigrationHop(ctx, tx, uuid.NewString(), *plan.bastion)
		}
		if err != nil {
			return err
		}
		destination.connection.BastionID = bastion.ID
	}
	_, err := saveMigrationHop(ctx, tx, old.Destination, destination)
	return err
}
