package checkoutadoption

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

type attachmentQuery interface {
	SelectContext(context.Context, any, string, ...any) error
}

// ValidateAttachmentsTx locks and compares the complete branch-driving attachment set.
func ValidateAttachmentsTx(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, p models.WorkspaceAdoptionPreview) error {
	return validateAttachments(ctx, db, tx, p, dialect.IsPostgres(db.DriverName()))
}

func validateAttachments(ctx context.Context, db *sqlx.DB, q attachmentQuery, p models.WorkspaceAdoptionPreview, lock bool) error {
	query := `SELECT id,task_id,repository_id,COALESCE(checkout_branch,'') AS checkout_branch,COALESCE(base_branch,'') AS base_branch,COALESCE(metadata,'{}') AS metadata,COALESCE(position,0) AS position FROM task_repositories WHERE task_id=? ORDER BY id`
	if lock {
		query += ` FOR UPDATE`
	}
	var rows []struct {
		models.WorkspaceAdoptionAttachment
		Metadata string `db:"metadata"`
	}
	if err := q.SelectContext(ctx, &rows, db.Rebind(query), p.Selection.Snapshot.TaskID); err != nil {
		return err
	}
	attachments := make([]models.WorkspaceAdoptionAttachment, 0, len(rows))
	for _, row := range rows {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(row.Metadata), &metadata); err != nil {
			return fmt.Errorf("decode workspace adoption attachment metadata: %w", err)
		}
		row.MetadataDigest = models.WorkspaceAdoptionMetadataDigest(metadata)
		attachments = append(attachments, row.WorkspaceAdoptionAttachment)
	}
	if models.WorkspaceAdoptionDigest(attachments) != p.Selection.AttachmentsDigest {
		return models.ErrWorkspaceAdoptionStale
	}
	return nil
}
