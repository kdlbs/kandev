package sshconnections

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/jmoiron/sqlx"
)

type Store struct{ db *sqlx.DB }

// NewStore uses the shared database; ownership of its pools stays with backendapp.
func NewStore(db *sqlx.DB) (*Store, error) {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS ssh_enrollments (actor TEXT NOT NULL,request_id TEXT NOT NULL,connection_id TEXT NOT NULL,digest TEXT NOT NULL,data TEXT NOT NULL,PRIMARY KEY(actor,request_id))`,
		`CREATE TABLE IF NOT EXISTS ssh_connection_requests (actor TEXT NOT NULL, request_id TEXT NOT NULL, digest TEXT NOT NULL, connection_id TEXT NOT NULL, PRIMARY KEY(actor,request_id))`,
		`CREATE TABLE IF NOT EXISTS ssh_identity_requests (actor TEXT NOT NULL, request_id TEXT NOT NULL, digest TEXT NOT NULL, identity_id TEXT NOT NULL, PRIMARY KEY(actor,request_id))`,
		`CREATE TABLE IF NOT EXISTS ssh_connection_lock (id INTEGER PRIMARY KEY, revision BIGINT NOT NULL)`,
		`INSERT INTO ssh_connection_lock (id,revision) VALUES (1,0) ON CONFLICT (id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS ssh_connections (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ssh_identities (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ssh_connection_refs (owner_type TEXT NOT NULL, owner_id TEXT NOT NULL, connection_id TEXT NOT NULL, route_stamp TEXT NOT NULL, PRIMARY KEY (owner_type,owner_id))`,
		`CREATE TABLE IF NOT EXISTS ssh_migrations (executor_id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ssh_connection_audit (id TEXT PRIMARY KEY, actor TEXT NOT NULL, entity_id TEXT NOT NULL, operation TEXT NOT NULL, result TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
	}
	for _, q := range schema {
		if _, err := db.Exec(q); err != nil {
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

// write serializes graph mutations across backend processes on both databases.
// The lock precedes every snapshot read, avoiding write skew when adding hops.
func (s *Store) write(ctx context.Context, fn func(*sqlx.Tx) error) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `UPDATE ssh_connection_lock SET revision=revision+1 WHERE id=1`); err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func connectionIn(ctx context.Context, tx *sqlx.Tx, id string) (Connection, error) {
	var data string
	if err := tx.GetContext(ctx, &data, tx.Rebind(`SELECT data FROM ssh_connections WHERE id=?`), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Connection{}, ErrNotFound
		}
		return Connection{}, err
	}
	var c Connection
	err := json.Unmarshal([]byte(data), &c)
	return c, err
}
func connectionsIn(ctx context.Context, tx *sqlx.Tx) ([]Connection, error) {
	var rows []string
	if err := tx.SelectContext(ctx, &rows, `SELECT data FROM ssh_connections ORDER BY id`); err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, data := range rows {
		var c Connection
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
func putConnection(ctx context.Context, tx *sqlx.Tx, c Connection) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_connections (id,data) VALUES (?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data`), c.ID, string(data))
	return err
}
func sameTransport(a, b Connection) bool {
	return a.Host == b.Host && a.Port == b.Port && a.User == b.User && a.IdentityID == b.IdentityID && a.BastionID == b.BastionID
}
func endpointChanged(a, b Connection) bool {
	return a.Host != b.Host || a.Port != b.Port || a.User != b.User || a.BastionID != b.BastionID
}
func routeInUse(ctx context.Context, tx *sqlx.Tx, id string, runtimeOnly bool) error {
	routes, err := connectionsIn(ctx, tx)
	if err != nil {
		return err
	}
	ids := []string{id}
	for _, c := range routes {
		if c.BastionID == id {
			ids = append(ids, c.ID)
		}
	}
	for _, id := range ids {
		query := `SELECT COUNT(*) FROM ssh_connection_refs WHERE connection_id=?`
		if runtimeOnly {
			query += ` AND owner_type IN ('runtime','runtime_pending','enrollment','probe')`
		}
		var n int
		if err := tx.GetContext(ctx, &n, tx.Rebind(query), id); err != nil {
			return err
		}
		if n > 0 {
			return ErrInUse
		}
	}
	return nil
}
func validateHop(ctx context.Context, tx *sqlx.Tx, c Connection) error {
	if c.BastionID == "" {
		return nil
	}
	if c.BastionID == c.ID {
		return ErrInvalidRoute
	}
	b, err := connectionIn(ctx, tx, c.BastionID)
	if err != nil || b.BastionID != "" {
		return ErrInvalidRoute
	}
	routes, err := connectionsIn(ctx, tx)
	if err != nil {
		return err
	}
	for _, r := range routes {
		if r.BastionID == c.ID {
			return ErrInvalidRoute
		}
	}
	return nil
}
func (s *Store) SaveConnection(ctx context.Context, c Connection, expected int64) (Connection, error) {
	err := s.write(ctx, func(tx *sqlx.Tx) error { return saveConnectionIn(ctx, tx, &c, expected) })
	return c, err
}
func saveConnectionIn(ctx context.Context, tx *sqlx.Tx, c *Connection, expected int64) error {
	old, err := connectionIn(ctx, tx, c.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if old.Version != expected || c.ID == "" {
		return ErrConflict
	}
	if err = validateHop(ctx, tx, *c); err != nil {
		return err
	}
	c.Version = old.Version + 1
	c.Revision = old.Revision
	c.Fingerprint = old.Fingerprint
	c.PublicKey = old.PublicKey
	c.VerifiedRoute = old.VerifiedRoute
	c.ImportedPending = old.ImportedPending
	c.KnownHostsPending = old.KnownHostsPending
	c.RepairReason = old.RepairReason
	if old.ID == "" || !sameTransport(old, *c) {
		if err = routeInUse(ctx, tx, c.ID, true); err != nil {
			return err
		}
		c.Revision++
		c.VerifiedRoute = ""
		c.ImportedPending = false
		c.KnownHostsPending = false
		c.RepairReason = ""
		if endpointChanged(old, *c) {
			c.Fingerprint = ""
			c.PublicKey = ""
		}
	}
	return putConnection(ctx, tx, *c)
}
func (s *Store) GetConnection(ctx context.Context, id string) (Connection, error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Connection{}, err
	}
	defer func() { _ = tx.Rollback() }()
	return connectionIn(ctx, tx, id)
}
func (s *Store) DeleteConnection(ctx context.Context, id string, expected int64) error {
	return s.write(ctx, func(tx *sqlx.Tx) error { return deleteConnectionIn(ctx, tx, id, expected) })
}
func deleteConnectionIn(ctx context.Context, tx *sqlx.Tx, id string, expected int64) error {
	c, err := connectionIn(ctx, tx, id)
	if err != nil {
		return err
	}
	if c.Version != expected {
		return ErrConflict
	}
	routes, err := connectionsIn(ctx, tx)
	if err != nil {
		return err
	}
	for _, r := range routes {
		if r.BastionID == id {
			return ErrInUse
		}
	}
	if err = routeInUse(ctx, tx, id, false); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM ssh_connections WHERE id=?`), id)
	return err
}
