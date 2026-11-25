package sshconnections

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/sshtransport"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type migrationAdmission struct {
	migration sshMigration
	snapshot  RuntimeSnapshot
}

// SetMigrationPlatformCheck wires the workload's supported-platform check before service startup.
func (s *Service) SetMigrationPlatformCheck(check func(context.Context, *ssh.Client) error) {
	s.migrationPlatformCheck = check
}

func migrationMatchesBinding(migration sshMigration, config map[string]string, route Route) bool {
	return migration.ConnectionID == route.Destination.ID && migration.RouteStamp == route.Stamp &&
		config[ConnectionConfigKey] == route.Destination.ID && config["ssh_connection_stamp"] == route.Stamp
}

func (s *Service) verifyMigratedRuntime(ctx context.Context, sessionID, id string) error {
	var executorID string
	if err := s.store.db.GetContext(ctx, &executorID, s.store.db.Rebind(`SELECT executor_id FROM task_sessions WHERE id=?`), sessionID); err != nil {
		return ErrInvalidRoute
	}
	if err := s.refreshSSHMigration(ctx, executorID); err != nil {
		return err
	}
	admission, err := s.readMigrationAdmission(ctx, executorID, sessionID, id)
	if err != nil || admission == nil || admission.migration.Verified {
		return err
	}
	if s.migrationPlatformCheck == nil {
		return ErrInvalidRoute
	}
	client, key, err := s.dialMigrationRoute(ctx, admission.snapshot.Route, admission.migration.BastionKnownHosts)
	if err != nil {
		return transportFailure(err)
	}
	defer func() { _ = client.Close() }()
	if err = s.checkMigrationPlatform(ctx, client); err != nil {
		return err
	}
	return s.store.write(ctx, func(tx *sqlx.Tx) error {
		current, err := migrationAdmissionIn(ctx, tx, executorID, sessionID, id)
		if errors.Is(err, ErrInvalidRoute) || errors.Is(err, ErrNotFound) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		if current == nil {
			return ErrConflict
		}
		if current.migration.Verified {
			return checkConcurrentPromotion(*admission, *current, key)
		}
		return promoteMigrationIn(ctx, tx, executorID, *admission, *current, key)
	})
}

func (s *Service) checkMigrationPlatform(ctx context.Context, client *ssh.Client) error {
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(checkCtx, func() { _ = client.Close() })
	defer stop()
	if err := s.migrationPlatformCheck(checkCtx, client); err != nil {
		return err
	}
	return checkCtx.Err()
}

func (s *Service) readMigrationAdmission(ctx context.Context, executorID, sessionID, id string) (*migrationAdmission, error) {
	tx, err := s.store.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	return migrationAdmissionIn(ctx, tx, executorID, sessionID, id)
}

func migrationAdmissionIn(ctx context.Context, tx *sqlx.Tx, executorID, sessionID, id string) (*migrationAdmission, error) {
	migration, err := migrationIn(ctx, tx, executorID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	config, kind, err := executorConfigIn(ctx, tx, executorID)
	if err != nil {
		return nil, err
	}
	if err := validateMigrationSessionIn(ctx, tx, executorID, sessionID); err != nil {
		return nil, err
	}
	if kind != executorKindSSH || config[ConnectionConfigKey] != id || hasInlineSSHConfig(config) {
		return nil, ErrInvalidRoute
	}
	route, err := routeIn(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if !migrationMatchesBinding(migration, config, route) {
		// A deliberate rebind uses the ordinary verified-route admission path.
		if _, err = readyExecutorRouteIn(ctx, tx, executorID, kind, config); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := checkMigrationEvidenceIn(ctx, tx, executorID, migration, route); err != nil {
		return nil, err
	}
	snapshot, err := runtimeSnapshotIn(ctx, tx, sessionID, route)
	if err != nil {
		return nil, err
	}
	return &migrationAdmission{migration: migration, snapshot: snapshot}, nil
}

func checkMigrationEvidenceIn(ctx context.Context, tx *sqlx.Tx, executorID string, migration sshMigration, route Route) error {
	if migration.Reason != "" {
		hop := hopDestination
		if route.Bastion != nil && route.Bastion.RepairReason == migration.Reason ||
			migration.Reason == migrationRoute && migration.Source["ssh_proxy_jump"] != "" {
			hop = hopBastion
		}
		return &ProbeFailure{Code: migration.Reason, Hop: hop}
	}
	if route.Destination.Fingerprint == "" {
		return &ProbeFailure{Code: ErrTrust.Error(), Hop: hopDestination}
	}
	matches, err := executorRefMatchesIn(ctx, tx, executorID, route.Destination.ID, route.Stamp)
	if err != nil {
		return err
	}
	if !matches {
		return ErrConflict
	}
	return nil
}

func validateMigrationSessionIn(ctx context.Context, tx *sqlx.Tx, executorID, sessionID string) error {
	var currentExecutor string
	if err := tx.GetContext(ctx, &currentExecutor, tx.Rebind(`SELECT executor_id FROM task_sessions WHERE id=?`), sessionID); err != nil || currentExecutor != executorID {
		return ErrConflict
	}
	return nil
}

func checkConcurrentPromotion(expected, current migrationAdmission, key ssh.PublicKey) error {
	if !sameMigrationIdentity(expected, current) ||
		current.snapshot.Destination.VerifiedRoute != current.snapshot.Stamp {
		return ErrConflict
	}
	if expected.snapshot.Stamp == current.snapshot.Stamp {
		return nil
	}
	oldBastion, newBastion := expected.snapshot.Bastion, current.snapshot.Bastion
	if !expected.migration.BastionKnownHosts || key == nil || oldBastion == nil || newBastion == nil {
		return ErrConflict
	}
	// The only route revision changed by promotion is the first pin for
	// an imported bastion. A concurrent check must have seen that exact key.
	if expected.snapshot.Destination.Revision != current.snapshot.Destination.Revision ||
		oldBastion.ID != newBastion.ID || !sameTransport(*oldBastion, *newBastion) ||
		newBastion.Revision != oldBastion.Revision+1 || newBastion.Fingerprint != ssh.FingerprintSHA256(key) {
		return ErrConflict
	}
	return nil
}

func sameMigrationIdentity(expected, current migrationAdmission) bool {
	return expected.migration.SourceDigest == current.migration.SourceDigest &&
		expected.migration.ConnectionID == current.migration.ConnectionID &&
		expected.snapshot.IdentityGeneration == current.snapshot.IdentityGeneration &&
		expected.snapshot.BastionIdentityGeneration == current.snapshot.BastionIdentityGeneration
}

func promoteMigrationIn(ctx context.Context, tx *sqlx.Tx, executorID string, expected, current migrationAdmission, bastionKey ssh.PublicKey) error {
	if !sameMigrationIdentity(expected, current) ||
		expected.migration.BastionKnownHosts != current.migration.BastionKnownHosts ||
		expected.snapshot.Stamp != current.snapshot.Stamp {
		return ErrConflict
	}
	route := current.snapshot.Route
	if err := verifyMigrationBastionIn(ctx, tx, route.Bastion, bastionKey); err != nil {
		return err
	}
	route, err := routeIn(ctx, tx, route.Destination.ID)
	if err != nil {
		return err
	}
	destination := route.Destination
	destination.VerifiedRoute = route.Stamp
	destination.ImportedPending = false
	destination.RepairReason = ""
	destination.Version++
	if err = putConnection(ctx, tx, destination); err != nil {
		return err
	}
	if err = updateMigrationBindingIn(ctx, tx, executorID, destination.ID, current.snapshot.Stamp, route.Stamp); err != nil {
		return err
	}
	migration := current.migration
	migration.RouteStamp = route.Stamp
	migration.BastionKnownHosts = false
	migration.Blocked = false
	migration.Verified = true
	if err = putMigration(ctx, tx, executorID, migration); err != nil {
		return err
	}
	return audit(ctx, tx, migrationSystemActor, executorID, "executor.migration.verify", outcomeVerified)
}

func verifyMigrationBastionIn(ctx context.Context, tx *sqlx.Tx, bastion *Connection, key ssh.PublicKey) error {
	if bastion == nil {
		return nil
	}
	if key != nil {
		if err := routeInUse(ctx, tx, bastion.ID, true); err != nil {
			return err
		}
		bastion.Fingerprint = ssh.FingerprintSHA256(key)
		bastion.PublicKey = string(ssh.MarshalAuthorizedKey(key))
		bastion.Revision++
		if err := putConnection(ctx, tx, *bastion); err != nil {
			return err
		}
	}
	route, err := routeIn(ctx, tx, bastion.ID)
	if err != nil {
		return err
	}
	bastion.VerifiedRoute = route.Stamp
	bastion.RepairReason = ""
	bastion.KnownHostsPending = false
	bastion.Version++
	return putConnection(ctx, tx, *bastion)
}

func updateMigrationBindingIn(ctx context.Context, tx *sqlx.Tx, executorID, id, oldStamp, stamp string) error {
	config, _, err := executorConfigIn(ctx, tx, executorID)
	if err != nil {
		return err
	}
	config["ssh_connection_stamp"] = stamp
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE executors SET config=?,updated_at=? WHERE id=?`), string(data), time.Now().UTC(), executorID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE ssh_connection_refs SET route_stamp=? WHERE owner_type='executor' AND owner_id=? AND connection_id=? AND route_stamp=?`), stamp, executorID, id, oldStamp)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return ErrConflict
	}
	return err
}

func (s *Service) dialMigrationRoute(ctx context.Context, route Route, useKnownHosts bool) (*ssh.Client, ssh.PublicKey, error) {
	if route.Bastion == nil || route.Bastion.Fingerprint != "" {
		client, err := s.dialProtectedRoute(ctx, route)
		return client, nil, err
	}
	if !useKnownHosts {
		return nil, nil, &ProbeFailure{Code: ErrTrust.Error(), Hop: hopBastion}
	}
	file, err := loadMigrationKnownHosts()
	if err != nil {
		return nil, nil, &ProbeFailure{Code: ErrTrust.Error(), Hop: hopBastion}
	}
	keyTypes := importedBastionKeyTypes(file, *route.Bastion)
	if len(keyTypes) == 0 {
		return nil, nil, &ProbeFailure{Code: ErrTrust.Error(), Hop: hopBastion}
	}
	var verifiedKey atomic.Pointer[ssh.PublicKey]
	client, err := s.dialRouteWithBastionVerification(ctx, route, func(host string, remote net.Addr, key ssh.PublicKey) error {
		if err := verifyImportedBastionKey(file, host, remote, key); err != nil {
			return err
		}
		verifiedKey.Store(&key)
		return nil
	}, keyTypes)
	if key := verifiedKey.Load(); key != nil {
		return client, *key, err
	}
	return client, nil, err
}

func importedBastionKeyTypes(file migrationKnownHostFile, bastion Connection) []string {
	address := net.JoinHostPort(bastion.Host, strconv.Itoa(bastion.Port))
	var keyTypes []string
	for _, candidate := range file.candidates(address, migrationHostAddr(address)) {
		if file.isRawKey(candidate) && file.strict(address, migrationHostAddr(address), candidate.Key) == nil {
			keyTypes = append(keyTypes, candidate.Key.Type())
		}
	}
	return keyTypes
}

func verifyImportedBastionKey(file migrationKnownHostFile, host string, remote net.Addr, key ssh.PublicKey) error {
	err := file.checkHostKey(host, remote, key)
	if err == nil {
		return nil
	}
	var mismatch *knownhosts.KeyError
	if errors.As(err, &mismatch) && len(mismatch.Want) != 0 {
		return errors.Join(&ProbeFailure{Code: codeHostKeyChanged, Hop: hopBastion}, sshtransport.ErrTryOtherKnownHostKey)
	}
	var revoked *knownhosts.RevokedError
	if errors.As(err, &revoked) {
		return &ProbeFailure{Code: codeHostKeyChanged, Hop: hopBastion}
	}
	return &ProbeFailure{Code: ErrTrust.Error(), Hop: hopBastion}
}
