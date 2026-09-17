package runtime

import (
	"context"

	"github.com/kandev/kandev/internal/orchestration/models"
)

// Check after capturing binding authority. If selection changes afterward the
// launch check rejects that snapshot, rather than upgrading an old shared turn.
func (s *Service) privateTurnSource(ctx context.Context, taskID string, payload map[string]any) error {
	owner, err := s.Repo.ConversationUserOwner(ctx, taskID)
	if err != nil || owner == "" {
		return err
	}
	id, _ := payload["comment_id"].(string)
	if id == "" {
		return nil // internal callback, not a human instruction
	}
	comment, err := s.Repo.GetCommentByID(ctx, taskID, id)
	if err != nil {
		return err
	}
	if comment.AuthorType != authorTypeUser || comment.AuthorID != owner {
		return models.ErrConflict
	}
	return nil
}
