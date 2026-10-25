package org

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

// ErrOrgNotFound reports an unknown organization. Callers that reached the
// lookup through a caller-supplied ID surface it as 404.
var ErrOrgNotFound = errors.New("organization not found")

// ErrSlugTaken reports a slug collision.
var ErrSlugTaken = errors.New("that organization slug is already in use")

// Store persists organizations.
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
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS orgs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'active',
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at %s NOT NULL,
			updated_at %s NOT NULL
		)`, timestamp, timestamp),
		`CREATE INDEX IF NOT EXISTS idx_orgs_status ON orgs(status)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("org schema: %w", err)
		}
	}
	return nil
}

const orgColumns = `id, name, slug, status, is_default, created_at, updated_at`

func scanOrg(row interface{ Scan(...any) error }) (*Org, error) {
	o := &Org{}
	var isDefault int
	if err := row.Scan(&o.ID, &o.Name, &o.Slug, &o.Status, &isDefault, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	o.IsDefault = isDefault != 0
	return o, nil
}

// Create inserts an organization, deriving a unique slug when needed.
func (s *Store) Create(ctx context.Context, name, slug string, isDefault bool) (*Org, error) {
	if slug == "" {
		slug = Slugify(name)
	}
	now := time.Now().UTC()
	o := &Org{
		ID: uuid.New().String(), Name: name, Slug: slug,
		Status: string(StatusActive), IsDefault: isDefault, CreatedAt: now, UpdatedAt: now,
	}
	flag := 0
	if isDefault {
		flag = 1
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO orgs (`+orgColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
	), o.ID, o.Name, o.Slug, o.Status, flag, o.CreatedAt, o.UpdatedAt)
	if err != nil {
		if db.IsAlreadyExistsError(err) || isUniqueViolation(err) {
			return nil, ErrSlugTaken
		}
		return nil, err
	}
	return o, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value")
}

// Get returns one organization by ID.
func (s *Store) Get(ctx context.Context, id string) (*Org, error) {
	row := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT `+orgColumns+` FROM orgs WHERE id = ?`), id)
	o, err := scanOrg(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOrgNotFound
	}
	return o, err
}

// List returns every organization, default first then by name.
func (s *Store) List(ctx context.Context) ([]*Org, error) {
	rows, err := s.ro.QueryContext(ctx, `SELECT `+orgColumns+` FROM orgs ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	orgs := make([]*Org, 0)
	for rows.Next() {
		o, err := scanOrg(rows)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

// Default returns the default organization, or ErrOrgNotFound when the
// instance has not been migrated yet.
func (s *Store) Default(ctx context.Context) (*Org, error) {
	row := s.ro.QueryRowContext(ctx, `SELECT `+orgColumns+` FROM orgs WHERE is_default = 1 LIMIT 1`)
	o, err := scanOrg(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOrgNotFound
	}
	return o, err
}

// Count returns how many organizations exist.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.ro.QueryRowContext(ctx, `SELECT COUNT(*) FROM orgs`).Scan(&n)
	return n, err
}

// UpdateNameStatus changes an org's display name and/or status.
func (s *Store) UpdateNameStatus(ctx context.Context, id, name, status string) (*Org, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = existing.Name
	}
	if status == "" {
		status = existing.Status
	}
	status = string(NormalizeStatus(status))
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE orgs SET name = ?, status = ?, updated_at = ? WHERE id = ?`,
	), name, status, now, id); err != nil {
		return nil, err
	}
	existing.Name, existing.Status, existing.UpdatedAt = name, status, now
	return existing, nil
}

// Delete removes the organization row. Callers are responsible for removing
// the org's data first; this is the last step.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM orgs WHERE id = ?`), id)
	return err
}
