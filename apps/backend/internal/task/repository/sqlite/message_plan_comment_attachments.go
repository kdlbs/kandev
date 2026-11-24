package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func marshalMessagePlanCommentFiles(
	ctx context.Context, tx *sqlx.Tx, message *models.Message, queued *messagequeue.QueuedMessage,
	resolved *plancommenttx.Resolution, claim *messagequeue.QueueAttachmentClaim, originalJSON string,
) (string, error) {
	if !resolved.HasFiles() {
		return originalJSON, nil
	}
	if err := applyMessagePlanCommentFiles(ctx, tx, message, queued, resolved, claim); err != nil {
		return "", err
	}
	encoded, err := marshalTaskFeedbackMetadata(message.Metadata)
	if err != nil {
		return "", fmt.Errorf("serialize plan comment attachment metadata: %w", err)
	}
	return string(encoded), nil
}

func applyMessagePlanCommentFiles(
	ctx context.Context, tx *sqlx.Tx, message *models.Message, queued *messagequeue.QueuedMessage,
	resolved *plancommenttx.Resolution, claim *messagequeue.QueueAttachmentClaim,
) error {
	files := resolved.Files()
	if len(files) == 0 {
		return nil
	}
	attachments, err := readMessageAttachmentMetadata(message.Metadata["attachments"])
	if err != nil {
		return err
	}
	attachments = append(attachments, files...)
	if err := v1.ValidateCommentAttachments(message.Content, attachments); err != nil {
		return models.ErrAttachmentInvalid
	}
	var claimIDs []string
	if claim != nil {
		claimIDs = claim.IDs
	}
	if err := plancommenttx.ValidateFileBatch(ctx, tx, attachments, claimIDs); err != nil {
		return err
	}
	if err := resolved.TransferFiles(ctx, tx, message.TaskSessionID, message.ID, ""); err != nil {
		return err
	}
	metadata := maps.Clone(message.Metadata)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["attachments"] = attachments
	message.Metadata = metadata
	if queued != nil {
		queued.Attachments = messagequeue.AppendPlanCommentFiles(queued.Attachments, files)
	}
	return nil
}

func readMessageAttachmentMetadata(value interface{}) ([]v1.MessageAttachment, error) {
	if value == nil {
		return nil, nil
	}
	if files, ok := value.([]v1.MessageAttachment); ok {
		return append([]v1.MessageAttachment(nil), files...), nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode ordinary attachment metadata: %w", err)
	}
	var files []v1.MessageAttachment
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, fmt.Errorf("decode ordinary attachment metadata: %w", err)
	}
	return files, nil
}
