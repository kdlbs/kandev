package sshconnections

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const (
	migrationSystemActor = "system:ssh-migration"
	migrationInvalid     = "ssh_migration_invalid_config"
	migrationMixed       = "ssh_migration_mixed_config"
	migrationAlias       = "ssh_migration_alias_missing"
	migrationRoute       = "ssh_migration_unsupported_route"
	migrationIdentity    = "ssh_migration_identity_unavailable"
)

var migrationInlineKeys = [...]string{
	sshHostConfigKey, "ssh_port", "ssh_user", "ssh_host_alias",
	"ssh_host_fingerprint", "ssh_identity_source", "ssh_identity_file", "ssh_proxy_jump",
}

type sshMigration struct {
	Blocked           bool              `json:"blocked,omitempty"`
	Verified          bool              `json:"verified,omitempty"`
	ConnectionID      string            `json:"connection_id"`
	SourceDigest      string            `json:"source_digest"`
	RouteStamp        string            `json:"route_stamp"`
	Reason            string            `json:"reason,omitempty"`
	Source            map[string]string `json:"source,omitempty"`
	BastionKnownHosts bool              `json:"bastion_known_hosts,omitempty"`
}

func migrationIn(ctx context.Context, db sqlx.ExtContext, executorID string) (sshMigration, error) {
	var data string
	if err := sqlx.GetContext(ctx, db, &data, db.Rebind(`SELECT data FROM ssh_migrations WHERE executor_id=?`), executorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sshMigration{}, ErrNotFound
		}
		return sshMigration{}, err
	}
	var result sshMigration
	err := json.Unmarshal([]byte(data), &result)
	return result, err
}

func putMigration(ctx context.Context, tx *sqlx.Tx, executorID string, migration sshMigration) error {
	data, err := json.Marshal(migration)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_migrations(executor_id,data) VALUES (?,?)
		ON CONFLICT(executor_id) DO UPDATE SET data=excluded.data`), executorID, string(data))
	return err
}

// MigrateLegacyExecutors converts saved SSH executors before the backend serves
// requests. It uses only the database and the backend user's local SSH config.
func (s *Store) MigrateLegacyExecutors(ctx context.Context) error {
	var ids []string
	if err := s.db.SelectContext(ctx, &ids, s.db.Rebind(`SELECT id FROM executors WHERE type=? AND deleted_at IS NULL ORDER BY id`), executorKindSSH); err != nil {
		return err
	}
	for _, id := range ids {
		for {
			err := s.migrateLegacyExecutor(ctx, id)
			if err == nil {
				break
			}
			if !errors.Is(err, ErrConflict) {
				return fmt.Errorf("import saved SSH executor %s: %w", id, err)
			}
		}
	}
	return nil
}

func (s *Store) migrateLegacyExecutor(ctx context.Context, id string) error {
	var snapshot string
	query := s.db.Rebind(`SELECT COALESCE(config,'{}') FROM executors WHERE id=? AND type=? AND deleted_at IS NULL`)
	if err := s.db.GetContext(ctx, &snapshot, query, id, executorKindSSH); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	config, migration, skip := splitMigrationConfig(snapshot)
	if skip {
		return nil
	}
	migration.ConnectionID = uuid.NewString()
	plan := resolveMigration(migration)
	return s.write(ctx, func(tx *sqlx.Tx) error {
		_, err := migrationIn(ctx, tx, id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		return bindMigratedExecutorIn(ctx, tx, id, snapshot, config, migration, plan)
	})
}

func splitMigrationConfig(snapshot string) (map[string]json.RawMessage, sshMigration, bool) {
	migration := sshMigration{SourceDigest: fmt.Sprintf("%x", sha256.Sum256([]byte(snapshot))), Source: make(map[string]string)}
	var config map[string]json.RawMessage
	if err := json.Unmarshal([]byte(snapshot), &config); err != nil {
		migration.Reason = migrationInvalid
		return make(map[string]json.RawMessage), migration, false
	}
	if config == nil {
		config = make(map[string]json.RawMessage)
	}
	hasInline := false
	for _, key := range migrationInlineKeys {
		value, present := config[key]
		if !present {
			continue
		}
		hasInline = true
		selector, valid := decodeMigrationSelector(value)
		if !valid {
			migration.Reason = migrationInvalid
		} else if selector != "" {
			migration.Source[key] = selector
		}
		delete(config, key)
	}
	connectionID, hasBinding := migrationBindingConfig(config)
	if !hasInline && connectionID != "" {
		return config, migration, true
	}
	if discardInvalidMigrationSettings(config) && migration.Reason == "" {
		migration.Reason = migrationInvalid
	}
	if hasBinding {
		migration.Reason = migrationMixed
		// Retain the old reference as repair evidence, outside executable config.
		if connectionID != "" {
			migration.Source[ConnectionConfigKey] = connectionID
		}
	}
	return config, migration, false
}

func migrationBindingConfig(config map[string]json.RawMessage) (string, bool) {
	binding, hasBinding := config[ConnectionConfigKey]
	if hasBinding {
		id, _ := decodeMigrationSelector(binding)
		return id, true
	}
	_, hasStamp := config["ssh_connection_stamp"]
	return "", hasStamp
}

// Keep the executor decodable alongside other executors. Its repository
// supports strings for unrelated settings, including empty values.
func discardInvalidMigrationSettings(config map[string]json.RawMessage) bool {
	invalid := false
	for key, raw := range config {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			delete(config, key)
			invalid = true
		}
	}
	return invalid
}

func decodeMigrationSelector(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || string(raw) == "null" {
		return "", false
	}
	return value, validMigrationSelector(value)
}

func validMigrationSelector(value string) bool {
	return len(value) <= 4096 && !strings.ContainsAny(value, "\r\n\x00")
}

func bindMigratedExecutorIn(ctx context.Context, tx *sqlx.Tx, id, snapshot string, config map[string]json.RawMessage, migration sshMigration, plan migrationPlan) error {
	route, err := putMigrationRoute(ctx, tx, migration.ConnectionID, plan)
	if err != nil {
		return err
	}
	if err = updateMigratedExecutorIn(ctx, tx, id, snapshot, config, route); err != nil {
		return err
	}
	return commitMigrationIn(ctx, tx, id, migration, plan, route, "executor.migrate")
}

func commitMigrationIn(ctx context.Context, tx *sqlx.Tx, id string, migration sshMigration, plan migrationPlan, route Route, operation string) error {
	migration.Reason = plan.reason
	migration.RouteStamp = route.Stamp
	migration.BastionKnownHosts = plan.bastionKnownHosts
	migration.Blocked = migrationNeedsRepair(route, plan.bastionKnownHosts)
	if migration.Reason == "" {
		migration.Source = nil
	}
	if err := putMigration(ctx, tx, id, migration); err != nil {
		return err
	}
	result := migration.Reason
	if result == "" {
		result = migrationCountState(migration, route.Stamp)
	}
	return audit(ctx, tx, migrationSystemActor, id, operation, result)
}

func migrationNeedsRepair(route Route, bastionKnownHosts bool) bool {
	if route.Destination.RepairReason != "" || route.Destination.Fingerprint == "" {
		return true
	}
	return route.Bastion != nil &&
		(route.Bastion.RepairReason != "" || route.Bastion.Fingerprint == "" && !bastionKnownHosts)
}

func updateMigratedExecutorIn(ctx context.Context, tx *sqlx.Tx, id, snapshot string, config map[string]json.RawMessage, route Route) error {
	config[ConnectionConfigKey], _ = json.Marshal(route.Destination.ID)
	config["ssh_connection_stamp"], _ = json.Marshal(route.Stamp)
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	out, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE executors SET config=?,updated_at=?
		WHERE id=? AND type=? AND deleted_at IS NULL AND COALESCE(config,'{}')=?`),
		string(data), time.Now().UTC(), id, executorKindSSH, snapshot)
	if err != nil {
		return err
	}
	count, err := out.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_connection_refs(owner_type,owner_id,connection_id,route_stamp) VALUES ('executor',?,?,?)
		ON CONFLICT(owner_type,owner_id) DO UPDATE SET connection_id=excluded.connection_id,route_stamp=excluded.route_stamp`),
		id, route.Destination.ID, route.Stamp)
	return err
}
