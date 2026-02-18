package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

// UpsertSessionFileReview inserts or updates a file review record.
func (r *Repository) UpsertSessionFileReview(ctx context.Context, review *models.SessionFileReview) error {
	if review.ID == "" {
		review.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if review.CreatedAt.IsZero() {
		review.CreatedAt = now
	}
	review.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO session_file_reviews (
			id, session_id, file_path, reviewed, diff_hash, reviewed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, file_path) DO UPDATE SET
			reviewed = excluded.reviewed,
			diff_hash = excluded.diff_hash,
			reviewed_at = excluded.reviewed_at,
			updated_at = excluded.updated_at
	`), review.ID, review.SessionID, review.FilePath, review.Reviewed,
		review.DiffHash, review.ReviewedAt, review.CreatedAt, review.UpdatedAt)

	return err
}

// GetSessionFileReviews retrieves all file reviews for a session.
func (r *Repository) GetSessionFileReviews(ctx context.Context, sessionID string) ([]*models.SessionFileReview, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, session_id, file_path, reviewed, diff_hash, reviewed_at, created_at, updated_at
		FROM session_file_reviews
		WHERE session_id = ?
		ORDER BY file_path
	`), sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.SessionFileReview
	for rows.Next() {
		review := &models.SessionFileReview{}
		err := rows.Scan(
			&review.ID, &review.SessionID, &review.FilePath, &review.Reviewed,
			&review.DiffHash, &review.ReviewedAt, &review.CreatedAt, &review.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, review)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteSessionFileReviews deletes all file reviews for a session.
func (r *Repository) DeleteSessionFileReviews(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM session_file_reviews WHERE session_id = ?`), sessionID)
	return err
}
