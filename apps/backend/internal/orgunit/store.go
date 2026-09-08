package orgunit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// Store persists the unit tree and its memberships.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore builds the store and creates its schema.
func NewStore(pool *db.Pool) (*Store, error) {
	s := &Store{db: pool.Writer(), ro: pool.Reader()}
	if err := s.initSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) initSchema() error {
	timestamp := "DATETIME"
	if dialect.IsPostgres(s.db.DriverName()) {
		timestamp = "TIMESTAMPTZ"
	}
	statements := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS org_units (
			id TEXT PRIMARY KEY,
			org_id TEXT NOT NULL,
			parent_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT 'standard',
			owner_user_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			path TEXT NOT NULL DEFAULT '',
			created_at %s NOT NULL,
			updated_at %s NOT NULL
		)`, timestamp, timestamp),
		`CREATE INDEX IF NOT EXISTS idx_org_units_org_parent ON org_units(org_id, parent_id)`,
		// Ancestor lookups are prefix matches on this column, so it carries
		// the only index that the reach query depends on.
		`CREATE INDEX IF NOT EXISTS idx_org_units_path ON org_units(path)`,
		// One root per organization, and one personal unit per user, enforced
		// by the database rather than by a check the caller might skip.
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_org_units_root ON org_units(org_id) WHERE kind = 'root'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_org_units_personal ON org_units(owner_user_id) WHERE kind = 'personal'`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS unit_members (
			unit_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			role TEXT NOT NULL,
			added_by TEXT NOT NULL DEFAULT '',
			created_at %s NOT NULL,
			PRIMARY KEY (unit_id, user_id)
		)`, timestamp),
		`CREATE INDEX IF NOT EXISTS idx_unit_members_user ON unit_members(user_id)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("org unit schema: %w", err)
		}
	}
	return nil
}

const unitColumns = `id, org_id, parent_id, kind, owner_user_id, name, path, created_at, updated_at`

func scanUnit(row interface{ Scan(...any) error }) (*Unit, error) {
	u := &Unit{}
	if err := row.Scan(&u.ID, &u.OrgID, &u.ParentID, &u.Kind, &u.OwnerUserID,
		&u.Name, &u.Path, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return u, nil
}

// Insert writes a unit, deriving its path from its parent. The caller has
// already validated the tree invariants; this is the only writer of `path`.
func (s *Store) Insert(ctx context.Context, unit *Unit) (*Unit, error) {
	if unit.ID == "" {
		unit.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	unit.CreatedAt, unit.UpdatedAt = now, now

	parentPath := ""
	if unit.ParentID != "" {
		parent, err := s.Get(ctx, unit.ParentID)
		if err != nil {
			return nil, err
		}
		parentPath = parent.Path
	}
	unit.Path = strings.TrimSuffix(parentPath, "/") + "/" + unit.ID + "/"

	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO org_units (`+unitColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		unit.ID, unit.OrgID, unit.ParentID, string(unit.Kind), unit.OwnerUserID,
		unit.Name, unit.Path, unit.CreatedAt, unit.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert unit: %w", err)
	}
	return unit, nil
}

// Get returns one unit.
func (s *Store) Get(ctx context.Context, id string) (*Unit, error) {
	row := s.ro.QueryRowxContext(ctx, s.ro.Rebind(
		`SELECT `+unitColumns+` FROM org_units WHERE id = ?`), id)
	unit, err := scanUnit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnitNotFound
	}
	return unit, err
}

// Root returns an organization's root unit.
func (s *Store) Root(ctx context.Context, orgID string) (*Unit, error) {
	row := s.ro.QueryRowxContext(ctx, s.ro.Rebind(
		`SELECT `+unitColumns+` FROM org_units WHERE org_id = ? AND kind = 'root'`), orgID)
	unit, err := scanUnit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnitNotFound
	}
	return unit, err
}

// Personal returns a user's personal unit.
func (s *Store) Personal(ctx context.Context, userID string) (*Unit, error) {
	row := s.ro.QueryRowxContext(ctx, s.ro.Rebind(
		`SELECT `+unitColumns+` FROM org_units WHERE owner_user_id = ? AND kind = 'personal'`), userID)
	unit, err := scanUnit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnitNotFound
	}
	return unit, err
}

// ListByOrg returns every unit in an organization, ordered by path so a caller
// receives parents before their children.
func (s *Store) ListByOrg(ctx context.Context, orgID string) ([]*Unit, error) {
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(
		`SELECT `+unitColumns+` FROM org_units WHERE org_id = ? ORDER BY path`), orgID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*Unit
	for rows.Next() {
		unit, err := scanUnit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, unit)
	}
	return out, rows.Err()
}

// Rename updates a unit's display name.
func (s *Store) Rename(ctx context.Context, id, name string) error {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE org_units SET name = ?, updated_at = ? WHERE id = ?`),
		name, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrUnitNotFound
	}
	return nil
}

// Reparent moves a unit and rewrites the path of the unit and every
// descendant in one transaction. Doing it in two statements would leave the
// subtree pointing at an ancestor chain that no longer exists.
func (s *Store) Reparent(ctx context.Context, unit *Unit, newParent *Unit) error {
	oldPrefix := unit.Path
	newPath := strings.TrimSuffix(newParent.Path, "/") + "/" + unit.ID + "/"

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE org_units SET parent_id = ?, path = ?, updated_at = ? WHERE id = ?`),
		newParent.ID, newPath, time.Now().UTC(), unit.ID); err != nil {
		return err
	}
	// Every descendant keeps its own tail and gets the new head.
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE org_units
		    SET path = ? || SUBSTR(path, ?),
		        updated_at = ?
		  WHERE path LIKE ? AND id <> ?`),
		newPath, len(oldPrefix)+1, time.Now().UTC(), oldPrefix+"%", unit.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Delete removes a unit. The caller has already established that it is empty.
func (s *Store) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM unit_members WHERE unit_id = ?`), id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM org_units WHERE id = ?`), id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrUnitNotFound
	}
	return tx.Commit()
}

// ChildCount reports how many child units a unit still holds.
func (s *Store) ChildCount(ctx context.Context, id string) (int, error) {
	var count int
	err := s.ro.QueryRowxContext(ctx, s.ro.Rebind(
		`SELECT COUNT(*) FROM org_units WHERE parent_id = ?`), id).Scan(&count)
	return count, err
}

// SetMember adds or re-roles a member.
func (s *Store) SetMember(ctx context.Context, m *Member) error {
	m.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO unit_members (unit_id, user_id, role, added_by, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (unit_id, user_id) DO UPDATE SET role = EXCLUDED.role`),
		m.UnitID, m.UserID, m.Role, m.AddedBy, m.CreatedAt)
	return err
}

// RemoveMember drops a membership row.
func (s *Store) RemoveMember(ctx context.Context, unitID, userID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM unit_members WHERE unit_id = ? AND user_id = ?`), unitID, userID)
	return err
}

// ListMembers returns a unit's own members, without the inherited ones.
func (s *Store) ListMembers(ctx context.Context, unitID string) ([]*Member, error) {
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(`
		SELECT unit_id, user_id, role, added_by, created_at
		  FROM unit_members WHERE unit_id = ? ORDER BY created_at`), unitID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*Member
	for rows.Next() {
		m := &Member{}
		if err := rows.Scan(&m.UnitID, &m.UserID, &m.Role, &m.AddedBy, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AncestorRoles returns the roles a user holds on the given unit and on every
// ancestor of it. This is the query the reach resolver runs, so it takes the
// path rather than walking the tree row by row.
func (s *Store) AncestorRoles(ctx context.Context, userID, path string) ([]string, error) {
	ids := AncestorIDs(path)
	if len(ids) == 0 || userID == "" {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT role FROM unit_members WHERE user_id = ? AND unit_id IN (?)`, userID, ids)
	if err != nil {
		return nil, err
	}
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

// AncestorIDs splits a materialized path into its unit ids, nearest last.
func AncestorIDs(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// UserRoles returns every unit role a user holds, keyed by unit id. It is one
// query, so a caller resolving many workspaces does not issue one per row.
func (s *Store) UserRoles(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(
		`SELECT unit_id, role FROM unit_members WHERE user_id = ?`), userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var unitID, role string
		if err := rows.Scan(&unitID, &role); err != nil {
			return nil, err
		}
		out[unitID] = role
	}
	return out, rows.Err()
}

// PathsByID returns the materialized path of each requested unit.
func (s *Store) PathsByID(ctx context.Context, unitIDs []string) (map[string]string, error) {
	if len(unitIDs) == 0 {
		return map[string]string{}, nil
	}
	query, args, err := sqlx.In(`SELECT id, path FROM org_units WHERE id IN (?)`, unitIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		out[id] = path
	}
	return out, rows.Err()
}

// AncestorMemberIDs returns every user holding a membership on a unit or any
// of its ancestors. It answers "who reaches a workspace here", which is what
// decides who a workspace-scoped event fans out to.
func (s *Store) AncestorMemberIDs(ctx context.Context, path string) ([]string, error) {
	ids := AncestorIDs(path)
	if len(ids) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT DISTINCT user_id FROM unit_members WHERE unit_id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DeleteByOrg removes every unit in an organization and their memberships. It
// is the deletion path for a whole tenant: without it, deleting an
// organization leaves its tree behind with nothing pointing at it.
func (s *Store) DeleteByOrg(ctx context.Context, orgID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`DELETE FROM unit_members WHERE unit_id IN (SELECT id FROM org_units WHERE org_id = ?)`),
		orgID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`DELETE FROM org_units WHERE org_id = ?`), orgID); err != nil {
		return err
	}
	return tx.Commit()
}
