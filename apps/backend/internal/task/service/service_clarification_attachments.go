package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

// CompleteActiveClarificationBundleWithAttachments claims a current-turn
// response and adopts its complete answer-file set in one transaction. The
// caller resolves authenticated ownership here; no descriptor field can select
// its own owner or scope.
func (s *Service) CompleteActiveClarificationBundleWithAttachments(
	ctx context.Context,
	pendingID, status string,
	responses map[string]interface{},
	claims []models.ClarificationAttachmentClaim,
) ([]*models.Message, bool, error) {
	scope, err := s.clarificationAttachmentScope(ctx, pendingID, claims)
	if err != nil {
		return nil, false, err
	}
	if clarificationClaimsNeedOwner(claims) && s.attachmentSvc == nil {
		return nil, false, errors.New("file-backed attachments are unavailable")
	}
	messages, claimed, released, err := s.messages.CompleteActiveClarificationBundleWithAttachments(
		ctx, pendingID, status, responses, claims, scope,
	)
	if err != nil {
		return nil, false, err
	}
	if claimed && len(released) > 0 {
		s.releaseRemovedClarificationAttachments(ctx, scope, released)
	}
	return messages, claimed, nil
}

func (s *Service) clarificationAttachmentScope(
	ctx context.Context,
	pendingID string,
	claims []models.ClarificationAttachmentClaim,
) (models.ClarificationAttachmentScope, error) {
	messages, err := s.messages.FindMessagesByPendingID(ctx, pendingID)
	if err != nil {
		return models.ClarificationAttachmentScope{}, err
	}
	sessionID, taskID := clarificationBundleIdentity(messages)
	if sessionID != "" && taskID == "" {
		session, sessionErr := s.sessions.GetTaskSession(ctx, sessionID)
		if sessionErr != nil {
			return models.ClarificationAttachmentScope{}, sessionErr
		}
		if session != nil {
			taskID = session.TaskID
		}
	}
	if sessionID == "" || taskID == "" {
		return models.ClarificationAttachmentScope{}, fmt.Errorf("clarification bundle %s has no task or session identity", pendingID)
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return models.ClarificationAttachmentScope{}, err
	}
	identity, _ := authn.IdentityFromContext(ctx)
	if clarificationClaimsNeedOwner(claims) && identity.UserID == "" {
		return models.ClarificationAttachmentScope{}, models.ErrAttachmentForbidden
	}
	ownerID := identity.UserID
	if ownerID == "" {
		ownerID, err = s.clarificationManifestOwner(ctx, messages)
		if err != nil {
			return models.ClarificationAttachmentScope{}, err
		}
	}
	return models.ClarificationAttachmentScope{
		OwnerID:     ownerID,
		WorkspaceID: task.WorkspaceID,
		TaskID:      taskID,
		SessionID:   sessionID,
	}, nil
}

func clarificationClaimsNeedOwner(claims []models.ClarificationAttachmentClaim) bool {
	for _, claim := range claims {
		if len(claim.AttachmentIDs) > 0 {
			return true
		}
	}
	return false
}

func (s *Service) clarificationManifestOwner(
	ctx context.Context,
	messages []*models.Message,
) (string, error) {
	ids := clarificationManifestAttachmentIDs(messages)
	if len(ids) == 0 {
		return "", nil
	}
	rows, err := s.attachments.ListMessageAttachments(ctx, ids)
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		if row != nil && row.OwnerID != "" {
			return row.OwnerID, nil
		}
	}
	return "", nil
}

func clarificationManifestAttachmentIDs(messages []*models.Message) []string {
	var ids []string
	for _, message := range messages {
		if message == nil {
			continue
		}
		raw, ok := message.Metadata["attachments"]
		if !ok || raw == nil {
			continue
		}
		encoded, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		var manifest []models.ClarificationAnswerAttachment
		if err := json.Unmarshal(encoded, &manifest); err != nil {
			continue
		}
		for _, attachment := range manifest {
			if attachment.AttachmentID != "" {
				ids = append(ids, attachment.AttachmentID)
			}
		}
	}
	return ids
}

func clarificationBundleIdentity(messages []*models.Message) (sessionID, taskID string) {
	for _, message := range messages {
		if message == nil {
			continue
		}
		if sessionID == "" {
			sessionID = message.TaskSessionID
		}
		if taskID == "" && message.TaskID != "" {
			taskID = message.TaskID
		}
	}
	return sessionID, taskID
}

// releaseRemovedClarificationAttachments is best-effort: the response is
// already durable, and the repository's reference-aware cleanup refuses to
// delete any attachment a surviving transcript row still references.
func (s *Service) releaseRemovedClarificationAttachments(
	ctx context.Context,
	scope models.ClarificationAttachmentScope,
	ids []string,
) {
	if s.attachmentSvc == nil {
		return
	}
	if err := s.attachmentSvc.Release(ctx, scope.OwnerID, scope.TaskID, scope.SessionID, ids); err != nil {
		s.logger.Warn("failed to release removed clarification answer attachments",
			zap.String("task_id", scope.TaskID),
			zap.String("session_id", scope.SessionID),
			zap.Int("attachments", len(ids)),
			zap.Error(err))
	}
}
