package messagequeue

import (
	"context"
	"slices"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// AppendPlanCommentFiles appends canonical plan descriptors without mutating the original queue attachments.
func AppendPlanCommentFiles(existing []MessageAttachment, files []v1.MessageAttachment) []MessageAttachment {
	out := slices.Clone(existing)
	for _, file := range files {
		out = append(out, MessageAttachment{
			CommentID: file.CommentID, Type: file.Type, AttachmentID: file.AttachmentID,
			MimeType: file.MimeType, Name: file.Name, SizeBytes: file.SizeBytes, DeliveryMode: file.DeliveryMode,
		})
	}
	return out
}

func applyQueuedPlanCommentFiles(
	ctx context.Context, tx *sqlx.Tx, candidate *QueuedMessage,
	resolved *plancommenttx.Resolution, claim *QueueAttachmentClaim,
) error {
	files := resolved.Files()
	if len(files) == 0 {
		return nil
	}
	list := make([]v1.MessageAttachment, 0, len(candidate.Attachments)+len(files))
	for _, file := range candidate.Attachments {
		list = append(list, v1.MessageAttachment{AttachmentID: file.AttachmentID, Data: file.Data, CommentID: file.CommentID, DeliveryMode: file.DeliveryMode})
	}
	list = append(list, files...)
	if err := v1.ValidateCommentAttachments(candidate.Content, list); err != nil {
		return models.ErrAttachmentInvalid
	}
	var claimIDs []string
	if claim != nil {
		claimIDs = claim.IDs
	}
	if err := plancommenttx.ValidateFileBatch(ctx, tx, list, claimIDs); err != nil {
		return err
	}
	if err := resolved.TransferFiles(ctx, tx, candidate.SessionID, "", candidate.ID); err != nil {
		return err
	}
	candidate.Attachments = AppendPlanCommentFiles(candidate.Attachments, files)
	return nil
}
