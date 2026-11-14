package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// CompleteActiveClarificationBundleWithAttachments keeps the production
// message adapter on the attachment-aware claim path.
func (a *messageCreatorAdapter) CompleteActiveClarificationBundleWithAttachments(
	ctx context.Context,
	pendingID, status string,
	responses map[string]interface{},
	claims []models.ClarificationAttachmentClaim,
) ([]*models.Message, bool, error) {
	return a.svc.CompleteActiveClarificationBundleWithAttachments(ctx, pendingID, status, responses, claims)
}

// answerAttachmentRuntime is the narrow trusted surface used by the
// clarification lifecycle. Keeping it local avoids a direct dependency on the
// runtime implementation from this higher-level wiring package.
type answerAttachmentRuntime interface {
	MaterializeAnswerAttachments(
		context.Context,
		string,
		[]v1.MessageAttachment,
	) ([]v1.MessageAttachment, error)
}

// clarificationAnswerAttachmentMaterializer streams already-claimed answer
// files into an existing execution.
type clarificationAnswerAttachmentMaterializer struct {
	runtime answerAttachmentRuntime
}

func (m *clarificationAnswerAttachmentMaterializer) MaterializeAnswerAttachments(
	ctx context.Context,
	sessionID string,
	attachments []models.ClarificationAnswerAttachment,
) (map[string]string, error) {
	if m == nil || m.runtime == nil {
		return nil, errors.New("answer attachment runtime is unavailable")
	}
	input := make([]v1.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		input = append(input, v1.MessageAttachment{
			Type:         attachment.Type,
			AttachmentID: attachment.AttachmentID,
			Name:         attachment.Name,
			MimeType:     attachment.MimeType,
			SizeBytes:    attachment.SizeBytes,
			DeliveryMode: "path",
		})
	}
	materialized, err := m.runtime.MaterializeAnswerAttachments(ctx, sessionID, input)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]string, len(materialized))
	for _, attachment := range materialized {
		if attachment.AttachmentID != "" && attachment.Path != "" {
			paths[attachment.AttachmentID] = attachment.Path
		}
	}
	return paths, nil
}
