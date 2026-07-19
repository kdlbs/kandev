package marketplace

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// officialSourceID is the stable primary key of the built-in kandev source,
// so EnsureBuiltin is idempotent across restarts and can keep the row's URL in
// sync with the OfficialSourceURL constant without ever duplicating it.
const officialSourceID = "official"

// ErrSourceNotFound is returned by Get/Update/Delete for an unknown id.
var ErrSourceNotFound = errors.New("marketplace source not found")

// ErrBuiltinImmutable is returned when an operator tries to delete the
// built-in official source (it may be disabled, but never removed).
var ErrBuiltinImmutable = errors.New("the built-in source cannot be deleted")

// ErrDuplicateSource is returned by Add when a source with the same URL is
// already configured (maps to 409, not a leaked raw SQL constraint error).
var ErrDuplicateSource = errors.New("a source with this url already exists")

// SourceStore persists configured marketplace sources in the
// plugin_marketplace_source table.
type SourceStore struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewSourceStore creates a SourceStore and initializes its schema.
func NewSourceStore(pool *db.Pool) (*SourceStore, error) {
	s := &SourceStore{db: pool.Writer(), ro: pool.Reader()}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("marketplace source schema init: %w", err)
	}
	return s, nil
}

func (s *SourceStore) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_marketplace_source (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 1,
			builtin INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		);
	`)
	return err
}

// EnsureBuiltin upserts the official source row. It keeps name/url/builtin in
// sync with the passed values (so changing the constant re-points the builtin
// on next boot) but never touches `enabled`, so an operator who disabled the
// official source keeps it disabled across restarts.
func (s *SourceStore) EnsureBuiltin(name, url string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(s.db.Rebind(`
		INSERT INTO plugin_marketplace_source (id, name, url, enabled, builtin, created_at)
		VALUES (?, ?, ?, 1, 1, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, url = excluded.url, builtin = 1
	`), officialSourceID, name, url, now)
	return err
}

// List returns every configured source, built-in first, then by creation time.
func (s *SourceStore) List() ([]SourceRecord, error) {
	rows, err := s.ro.Query(`
		SELECT id, name, url, enabled, builtin, created_at
		FROM plugin_marketplace_source
		ORDER BY builtin DESC, created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []SourceRecord
	for rows.Next() {
		rec, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Get returns one source by id, or ErrSourceNotFound.
func (s *SourceStore) Get(id string) (*SourceRecord, error) {
	row := s.ro.QueryRow(s.ro.Rebind(`
		SELECT id, name, url, enabled, builtin, created_at
		FROM plugin_marketplace_source WHERE id = ?
	`), id)
	rec, err := scanSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSourceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// Add inserts a new operator-configured (non-builtin) source. url must be a
// non-empty http(s) URL; a duplicate url returns an error.
func (s *SourceStore) Add(name, url string) (*SourceRecord, error) {
	name, url = strings.TrimSpace(name), strings.TrimSpace(url)
	if err := validateSourceURL(url); err != nil {
		return nil, err
	}
	if name == "" {
		name = url
	}
	if s.urlExists(url) {
		return nil, ErrDuplicateSource
	}
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := s.db.Exec(s.db.Rebind(`
		INSERT INTO plugin_marketplace_source (id, name, url, enabled, builtin, created_at)
		VALUES (?, ?, ?, 1, 0, ?)
	`), id, name, url, now.Format(time.RFC3339))
	if err != nil {
		// A concurrent insert may have won the UNIQUE(url) race; report that as
		// a duplicate rather than leaking the raw driver constraint message.
		if s.urlExists(url) {
			return nil, ErrDuplicateSource
		}
		return nil, errors.New("failed to add marketplace source")
	}
	return &SourceRecord{ID: id, Name: name, URL: url, Enabled: true, CreatedAt: now}, nil
}

// urlExists reports whether a source with the given URL is already stored.
func (s *SourceStore) urlExists(url string) bool {
	var one int
	err := s.ro.QueryRow(s.ro.Rebind(`
		SELECT 1 FROM plugin_marketplace_source WHERE url = ?
	`), url).Scan(&one)
	return err == nil
}

// Update changes a source's name and/or enabled flag. nil fields are left
// unchanged. The url and builtin flag are immutable.
func (s *SourceStore) Update(id string, name *string, enabled *bool) (*SourceRecord, error) {
	rec, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if name != nil && strings.TrimSpace(*name) != "" {
		rec.Name = strings.TrimSpace(*name)
	}
	if enabled != nil {
		rec.Enabled = *enabled
	}
	if _, err := s.db.Exec(s.db.Rebind(`
		UPDATE plugin_marketplace_source SET name = ?, enabled = ? WHERE id = ?
	`), rec.Name, boolToInt(rec.Enabled), id); err != nil {
		return nil, err
	}
	return rec, nil
}

// Delete removes a non-builtin source. Deleting the built-in source returns
// ErrBuiltinImmutable.
func (s *SourceStore) Delete(id string) error {
	rec, err := s.Get(id)
	if err != nil {
		return err
	}
	if rec.Builtin {
		return ErrBuiltinImmutable
	}
	_, err = s.db.Exec(s.db.Rebind(`DELETE FROM plugin_marketplace_source WHERE id = ?`), id)
	return err
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSource(sc rowScanner) (SourceRecord, error) {
	var (
		rec              SourceRecord
		enabled, builtin int
		createdAt        string
	)
	if err := sc.Scan(&rec.ID, &rec.Name, &rec.URL, &enabled, &builtin, &createdAt); err != nil {
		return SourceRecord{}, err
	}
	rec.Enabled = enabled != 0
	rec.Builtin = builtin != 0
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		rec.CreatedAt = t
	}
	return rec, nil
}

func validateSourceURL(url string) error {
	if url == "" {
		return errors.New("source url is required")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return errors.New("source url must be an http(s) URL")
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
