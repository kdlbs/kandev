package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// docRevSelectCols lists the task_document_revisions columns in the fixed order
// used by every SELECT in this file (and by scanDocRevisionRow / scanDocRevisionRows).
const docRevSelectCols = `id, task_id, document_key, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at`

// CreateDocument inserts a new task document HEAD row.
func (r *Repository) CreateDocument(ctx context.Context, doc *models.TaskDocument) error {
	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	doc.CreatedAt = now
	doc.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_documents
			(id, task_id, key, type, title, content, author_kind, author_name,
			 filename, mime_type, size_bytes, disk_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), doc.ID, doc.TaskID, doc.Key, doc.Type, doc.Title, doc.Content,
		doc.AuthorKind, doc.AuthorName, doc.Filename, doc.MimeType,
		doc.SizeBytes, doc.DiskPath, doc.CreatedAt, doc.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create document: %w", err)
	}
	return nil
}

// GetDocument retrieves a document HEAD by task ID and key. Returns nil, nil when not found.
func (r *Repository) GetDocument(ctx context.Context, taskID, key string) (*models.TaskDocument, error) {
	doc := &models.TaskDocument{}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, key, type, title, content, author_kind, author_name,
		       filename, mime_type, size_bytes, disk_path, created_at, updated_at
		FROM task_documents WHERE task_id = ? AND key = ?
	`), taskID, key).Scan(
		&doc.ID, &doc.TaskID, &doc.Key, &doc.Type, &doc.Title, &doc.Content,
		&doc.AuthorKind, &doc.AuthorName, &doc.Filename, &doc.MimeType,
		&doc.SizeBytes, &doc.DiskPath, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	return doc, nil
}

// UpdateDocument updates an existing document HEAD row.
func (r *Repository) UpdateDocument(ctx context.Context, doc *models.TaskDocument) error {
	doc.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_documents
		SET type = ?, title = ?, content = ?, author_kind = ?, author_name = ?,
		    filename = ?, mime_type = ?, size_bytes = ?, disk_path = ?, updated_at = ?
		WHERE task_id = ? AND key = ?
	`), doc.Type, doc.Title, doc.Content, doc.AuthorKind, doc.AuthorName,
		doc.Filename, doc.MimeType, doc.SizeBytes, doc.DiskPath, doc.UpdatedAt,
		doc.TaskID, doc.Key)
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("document not found: task=%s key=%s", doc.TaskID, doc.Key)
	}
	return nil
}

// DeleteDocument removes a document HEAD row (revisions cascade via FK).
func (r *Repository) DeleteDocument(ctx context.Context, taskID, key string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM task_documents WHERE task_id = ? AND key = ?`,
	), taskID, key)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("document not found: task=%s key=%s", taskID, key)
	}
	return nil
}

// ListDocuments returns all documents for a task, ordered by key.
func (r *Repository) ListDocuments(ctx context.Context, taskID string) ([]*models.TaskDocument, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, key, type, title, content, author_kind, author_name,
		       filename, mime_type, size_bytes, disk_path, created_at, updated_at
		FROM task_documents WHERE task_id = ? ORDER BY key
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.TaskDocument
	for rows.Next() {
		doc := &models.TaskDocument{}
		if err := rows.Scan(
			&doc.ID, &doc.TaskID, &doc.Key, &doc.Type, &doc.Title, &doc.Content,
			&doc.AuthorKind, &doc.AuthorName, &doc.Filename, &doc.MimeType,
			&doc.SizeBytes, &doc.DiskPath, &doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		out = append(out, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate documents: %w", err)
	}
	return out, nil
}

// InsertDocumentRevision inserts a new revision row.
func (r *Repository) InsertDocumentRevision(ctx context.Context, rev *models.TaskDocumentRevision) error {
	if rev.ID == "" {
		rev.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt = now
	}
	rev.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_document_revisions
			(`+docRevSelectCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		rev.ID, rev.TaskID, rev.DocumentKey, rev.RevisionNumber, rev.Title, rev.Content,
		rev.AuthorKind, rev.AuthorName, rev.RevertOfRevisionID, rev.CreatedAt, rev.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert document revision: %w", err)
	}
	return nil
}

// GetDocumentRevision fetches a single revision by ID.
func (r *Repository) GetDocumentRevision(ctx context.Context, id string) (*models.TaskDocumentRevision, error) {
	return r.scanDocRevisionRow(r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+docRevSelectCols+` FROM task_document_revisions WHERE id = ?`,
	), id))
}

// GetLatestDocumentRevision returns the newest revision for a document (by revision_number DESC).
func (r *Repository) GetLatestDocumentRevision(ctx context.Context, taskID, key string) (*models.TaskDocumentRevision, error) {
	return r.scanDocRevisionRow(r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+docRevSelectCols+`
		FROM task_document_revisions
		WHERE task_id = ? AND document_key = ?
		ORDER BY revision_number DESC LIMIT 1
	`), taskID, key))
}

// ListDocumentRevisions returns revisions newest-first. limit <= 0 returns all.
func (r *Repository) ListDocumentRevisions(ctx context.Context, taskID, key string, limit int) ([]*models.TaskDocumentRevision, error) {
	query := `SELECT ` + docRevSelectCols + ` FROM task_document_revisions WHERE task_id = ? AND document_key = ? ORDER BY revision_number DESC`
	args := []interface{}{taskID, key}
	if limit > 0 {
		query += sqlLimitClause
		args = append(args, limit)
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list document revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.TaskDocumentRevision
	for rows.Next() {
		rev := &models.TaskDocumentRevision{}
		var revertOf sql.NullString
		if err := rows.Scan(
			&rev.ID, &rev.TaskID, &rev.DocumentKey, &rev.RevisionNumber, &rev.Title, &rev.Content,
			&rev.AuthorKind, &rev.AuthorName, &revertOf, &rev.CreatedAt, &rev.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan document revision: %w", err)
		}
		if revertOf.Valid {
			v := revertOf.String
			rev.RevertOfRevisionID = &v
		}
		out = append(out, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate document revisions: %w", err)
	}
	return out, nil
}

// NextDocumentRevisionNumber returns max(revision_number)+1 for a document, or 1 if none exist.
func (r *Repository) NextDocumentRevisionNumber(ctx context.Context, taskID, key string) (int, error) {
	var maxNum sql.NullInt64
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT MAX(revision_number) FROM task_document_revisions WHERE task_id = ? AND document_key = ?
	`), taskID, key).Scan(&maxNum)
	if err != nil {
		return 0, fmt.Errorf("next document revision number: %w", err)
	}
	if !maxNum.Valid {
		return 1, nil
	}
	return int(maxNum.Int64) + 1, nil
}

// WriteDocumentRevision atomically upserts HEAD (task_documents) and either appends a new revision
// or merges into an existing one in a single write transaction.
//
// Coalesce behavior: when coalesceLatestID is non-nil and non-empty, the revision with that
// ID has title/content/updated_at merged in-place. When nil or empty, a new revision row is
// inserted with revision_number = MAX(existing)+1.
//
// On success, rev is mutated to reflect the persisted state.
func (r *Repository) WriteDocumentRevision(
	ctx context.Context,
	head *models.TaskDocument,
	rev *models.TaskDocumentRevision,
	coalesceLatestID *string,
) error {
	upsertHead := func(ctx context.Context, tx *sqlx.Tx, now time.Time) error {
		return upsertDocumentHead(ctx, tx, r.db, head, now)
	}
	merge := func(ctx context.Context, tx *sqlx.Tx, id string, now time.Time) error {
		return mergeDocRevisionInTx(ctx, tx, r.db, rev, id, now)
	}
	insert := func(ctx context.Context, tx *sqlx.Tx, now time.Time) error {
		return insertNewDocRevisionInTx(ctx, tx, r.db, rev, now)
	}
	return r.runRevisionTx(ctx, "document", coalesceLatestID, upsertHead, merge, insert)
}

// runRevisionTx runs the shared begin/upsertHead/coalesce-or-insert/commit transaction
// pattern used by both document and plan revision writes.
func (r *Repository) runRevisionTx(
	ctx context.Context,
	label string,
	coalesceLatestID *string,
	upsertHead func(context.Context, *sqlx.Tx, time.Time) error,
	merge func(context.Context, *sqlx.Tx, string, time.Time) error,
	insert func(context.Context, *sqlx.Tx, time.Time) error,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s revision tx: %w", label, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if err := upsertHead(ctx, tx, now); err != nil {
		return err
	}
	if coalesceLatestID != nil && *coalesceLatestID != "" {
		if err := merge(ctx, tx, *coalesceLatestID, now); err != nil {
			return err
		}
	} else {
		if err := insert(ctx, tx, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func upsertDocumentHead(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, head *models.TaskDocument, now time.Time) error {
	if head.ID == "" {
		head.ID = uuid.New().String()
	}
	if head.CreatedAt.IsZero() {
		head.CreatedAt = now
	}
	head.UpdatedAt = now
	_, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO task_documents
			(id, task_id, key, type, title, content, author_kind, author_name,
			 filename, mime_type, size_bytes, disk_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, key) DO UPDATE SET
			type       = excluded.type,
			title      = excluded.title,
			content    = excluded.content,
			author_kind = excluded.author_kind,
			author_name = excluded.author_name,
			filename   = excluded.filename,
			mime_type  = excluded.mime_type,
			size_bytes = excluded.size_bytes,
			disk_path  = excluded.disk_path,
			updated_at = excluded.updated_at
	`), head.ID, head.TaskID, head.Key, head.Type, head.Title, head.Content,
		head.AuthorKind, head.AuthorName, head.Filename, head.MimeType,
		head.SizeBytes, head.DiskPath, head.CreatedAt, head.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert document head: %w", err)
	}
	return nil
}

func mergeDocRevisionInTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, rev *models.TaskDocumentRevision, latestID string, now time.Time) error {
	result, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE task_document_revisions
		SET title = ?, content = ?, updated_at = ?
		WHERE id = ?
	`), rev.Title, rev.Content, now, latestID)
	if err != nil {
		return fmt.Errorf("merge document revision: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("document revision not found: %s", latestID)
	}
	rev.ID = latestID
	rev.UpdatedAt = now
	return nil
}

func insertNewDocRevisionInTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, rev *models.TaskDocumentRevision, now time.Time) error {
	var maxNum sql.NullInt64
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT MAX(revision_number) FROM task_document_revisions WHERE task_id = ? AND document_key = ?
	`), rev.TaskID, rev.DocumentKey).Scan(&maxNum); err != nil {
		return fmt.Errorf("compute next document revision number: %w", err)
	}
	rev.RevisionNumber = 1
	if maxNum.Valid {
		rev.RevisionNumber = int(maxNum.Int64) + 1
	}
	if rev.ID == "" {
		rev.ID = uuid.New().String()
	}
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt = now
	}
	rev.UpdatedAt = now
	_, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO task_document_revisions
			(`+docRevSelectCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		rev.ID, rev.TaskID, rev.DocumentKey, rev.RevisionNumber, rev.Title, rev.Content,
		rev.AuthorKind, rev.AuthorName, rev.RevertOfRevisionID, rev.CreatedAt, rev.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert document revision: %w", err)
	}
	return nil
}

func (r *Repository) scanDocRevisionRow(row *sql.Row) (*models.TaskDocumentRevision, error) {
	rev := &models.TaskDocumentRevision{}
	var revertOf sql.NullString
	err := row.Scan(
		&rev.ID, &rev.TaskID, &rev.DocumentKey, &rev.RevisionNumber, &rev.Title, &rev.Content,
		&rev.AuthorKind, &rev.AuthorName, &revertOf, &rev.CreatedAt, &rev.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan document revision: %w", err)
	}
	if revertOf.Valid {
		v := revertOf.String
		rev.RevertOfRevisionID = &v
	}
	return rev, nil
}
