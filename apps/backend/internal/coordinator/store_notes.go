package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const noteSelect = `SELECT n.id, n.coordinator_id, n.workspace_id, n.source_item_id, n.current_version,
	n.created_at, n.expires_at, n.deleted_at, n.use_count, n.last_validated_at, n.last_result,
	n.last_attempt_at, n.last_attempt,
	v.note_type, v.body, v.citations, v.prov_kind, v.prov_user_id, v.prov_model, v.actor_id, v.reverted_from, v.created_at
	FROM coordinator_notes n
	JOIN coordinator_note_versions v ON v.note_id = n.id AND v.version = n.current_version`

const versionSelect = `SELECT note_id, version, note_type, body, citations, prov_kind, prov_user_id, prov_model,
	actor_id, reverted_from, created_at FROM coordinator_note_versions`

type noteScanner interface{ Scan(dest ...any) error }

func scanNote(r noteScanner) (*Note, error) {
	var n Note
	var source sql.NullString
	var deleted, validated, attempt sql.NullTime
	var typ, body, cites string
	var v NoteVersion
	err := r.Scan(&n.ID, &n.CoordinatorID, &n.WorkspaceID, &source, &n.CurrentVer,
		&n.CreatedAt, &n.ExpiresAt, &deleted, &n.UseCount, &validated, &n.LastResult, &attempt, &n.LastAttempt,
		&typ, &body, &cites, &v.Provenance.Kind, &v.Provenance.UserID, &v.Provenance.Model, &v.ActorID, &v.RevertedFrom, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	n.SourceItemID = source.String
	n.CreatedAt, n.ExpiresAt = n.CreatedAt.UTC(), n.ExpiresAt.UTC()
	n.DeletedAt = utcPtr(deleted)
	n.LastValidatedAt = utcPtr(validated)
	n.LastAttemptAt = utcPtr(attempt)
	v.NoteID, v.Version, v.Type, v.Text, v.Citations = n.ID, n.CurrentVer, typ, body, unmarshalCitations(cites)
	v.CreatedAt = v.CreatedAt.UTC()
	n.Current = v
	return &n, nil
}

func scanVersion(r noteScanner) (NoteVersion, error) {
	var v NoteVersion
	var cites string
	err := r.Scan(&v.NoteID, &v.Version, &v.Type, &v.Text, &cites, &v.Provenance.Kind, &v.Provenance.UserID,
		&v.Provenance.Model, &v.ActorID, &v.RevertedFrom, &v.CreatedAt)
	v.Citations = unmarshalCitations(cites)
	v.CreatedAt = v.CreatedAt.UTC()
	return v, err
}

// ListNotes returns the coordinator's notes by creation time then id.
func (s *Store) ListNotes(ctx context.Context, coordinatorID string, includeDeleted bool) ([]*Note, error) {
	q := noteSelect + ` WHERE n.coordinator_id = ?`
	if !includeDeleted {
		q += ` AND n.deleted_at IS NULL`
	}
	return s.queryNotes(ctx, s.ro, q+` ORDER BY n.created_at ASC, n.id ASC`, coordinatorID)
}

func (s *Store) queryNotes(ctx context.Context, exec coordinatorExec, q string, args ...any) ([]*Note, error) {
	rows, err := exec.QueryContext(ctx, s.db.Rebind(q), args...)
	if err != nil {
		return nil, fmt.Errorf("list coordinator notes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	notes := []*Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("scan coordinator note: %w", err)
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// GetNote reads one note of the coordinator; ErrNotFound when absent.
func (s *Store) GetNote(ctx context.Context, coordinatorID, noteID string) (*Note, error) {
	return s.noteOn(ctx, s.ro, coordinatorID, noteID)
}

// noteOn reads a note through exec; mutations call it under the coordinator
// lock, which serializes every note write.
func (s *Store) noteOn(ctx context.Context, exec coordinatorExec, coordinatorID, noteID string) (*Note, error) {
	q := noteSelect + ` WHERE n.id = ? AND n.coordinator_id = ?`
	n, err := scanNote(exec.QueryRowContext(ctx, s.db.Rebind(q), noteID, coordinatorID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get coordinator note: %w", err)
	}
	return n, nil
}

// ListNoteVersions returns the note's versions by number ascending.
func (s *Store) ListNoteVersions(ctx context.Context, noteID string) ([]NoteVersion, error) {
	rows, err := s.ro.QueryContext(ctx, s.db.Rebind(versionSelect+` WHERE note_id = ? ORDER BY version ASC`), noteID)
	if err != nil {
		return nil, fmt.Errorf("list note versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []NoteVersion{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("scan note version: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// noteVersionOn reads one version; ErrNotFound when it does not exist.
func (s *Store) noteVersionOn(ctx context.Context, exec coordinatorExec, noteID string, version int) (NoteVersion, error) {
	v, err := scanVersion(exec.QueryRowContext(ctx, s.db.Rebind(versionSelect+` WHERE note_id = ? AND version = ?`), noteID, version))
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

// countLiveNotesOn counts the coordinator's nondeleted notes.
func (s *Store) countLiveNotesOn(ctx context.Context, exec coordinatorExec, coordinatorID string) (int, error) {
	var n int
	err := exec.QueryRowContext(ctx, s.db.Rebind(
		`SELECT COUNT(*) FROM coordinator_notes WHERE coordinator_id = ? AND deleted_at IS NULL`), coordinatorID).Scan(&n)
	return n, err
}

// noteContent is the content of a version being written.
type noteContent struct {
	Type         string
	Text         string
	Citations    []NoteCitation
	Provenance   NoteProvenance
	ActorID      string
	RevertedFrom int
}

func (s *Store) insertVersionOn(ctx context.Context, exec coordinatorExec, noteID string, version int, c noteContent, at time.Time) error {
	_, err := exec.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_note_versions
		(note_id, version, note_type, body, citations, prov_kind, prov_user_id, prov_model, actor_id, reverted_from, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		noteID, version, c.Type, c.Text, marshalCitations(c.Citations), c.Provenance.Kind, c.Provenance.UserID,
		c.Provenance.Model, c.ActorID, c.RevertedFrom, at)
	if err != nil {
		return fmt.Errorf("insert note version: %w", err)
	}
	return nil
}

// insertNoteOn creates a note at version 1 with expiry 30 days out. A
// non-empty sourceItemID makes the insert unique per coordinator.
func (s *Store) insertNoteOn(ctx context.Context, exec coordinatorExec, c *Coordinator, content noteContent, sourceItemID string, at time.Time) (string, error) {
	id := uuid.NewString()
	var source any
	if sourceItemID != "" {
		source = sourceItemID
	}
	if _, err := exec.ExecContext(ctx, s.db.Rebind(`INSERT INTO coordinator_notes
		(id, coordinator_id, workspace_id, source_item_id, current_version, created_at, expires_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)`), id, c.ID, c.WorkspaceID, source, at, at.Add(noteTTL)); err != nil {
		return "", fmt.Errorf("insert coordinator note: %w", err)
	}
	return id, s.insertVersionOn(ctx, exec, id, 1, content, at)
}

// noteBySourceItemOn finds the note created for an approved item.
func (s *Store) noteBySourceItemOn(ctx context.Context, exec coordinatorExec, coordinatorID, item string) (*Note, error) {
	n, err := scanNote(exec.QueryRowContext(ctx, s.db.Rebind(
		noteSelect+` WHERE n.coordinator_id = ? AND n.source_item_id = ?`), coordinatorID, item))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return n, err
}

// appendVersionOn writes the next version of a note and resets its validation
// state, conditional on the note still being at expectedVersion and nondeleted.
// It reports whether the conditional update applied.
func (s *Store) appendVersionOn(ctx context.Context, exec coordinatorExec, noteID string, expectedVersion int, c noteContent, at time.Time) (bool, error) {
	next := expectedVersion + 1
	res, err := exec.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_notes
		SET current_version = ?, last_validated_at = NULL, last_result = '', last_attempt_at = NULL, last_attempt = ''
		WHERE id = ? AND current_version = ? AND deleted_at IS NULL`), next, noteID, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("advance note version: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	return true, s.insertVersionOn(ctx, exec, noteID, next, c, at)
}

// tombstoneNoteOn marks a nondeleted note deleted; no version is created.
func (s *Store) tombstoneNoteOn(ctx context.Context, exec coordinatorExec, noteID string, at time.Time) (bool, error) {
	res, err := exec.ExecContext(ctx, s.db.Rebind(
		`UPDATE coordinator_notes SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`), at, noteID)
	if err != nil {
		return false, fmt.Errorf("delete coordinator note: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RecordNoteValidation persists one validation attempt for a note version
// under the coordinator lock. It applies only when the note is still at
// version and at is strictly later than the stored attempt time; an unavailable
// result never touches the last successful result. It reports whether the
// attempt was persisted.
func (s *Store) RecordNoteValidation(ctx context.Context, coordinatorID, noteID string, version int, result string, at time.Time) (bool, error) {
	persisted := false
	err := s.withCoordinatorLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		n, err := s.noteOn(ctx, tx, coordinatorID, noteID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if n.CurrentVer != version || (n.LastAttemptAt != nil && !at.After(*n.LastAttemptAt)) {
			return nil
		}
		q := `UPDATE coordinator_notes SET last_attempt_at = ?, last_attempt = ? WHERE id = ? AND current_version = ?`
		args := []any{at, result, noteID, version}
		if result != validationUnavailable {
			q = `UPDATE coordinator_notes SET last_attempt_at = ?, last_attempt = ?, last_validated_at = ?, last_result = ?
				WHERE id = ? AND current_version = ?`
			args = []any{at, result, at, result, noteID, version}
		}
		res, err := tx.ExecContext(ctx, s.db.Rebind(q), args...)
		if err != nil {
			return fmt.Errorf("record note validation: %w", err)
		}
		rows, _ := res.RowsAffected()
		persisted = rows > 0
		return nil
	})
	return persisted, err
}
