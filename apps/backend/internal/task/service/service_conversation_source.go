package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

var errConversationSourceUnavailable = errors.New("conversation source repository is unavailable")

// ReadConversationRevision returns the authorized current-state revision for a
// session. The repository performs the existence and revision read together.
func (s *Service) ReadConversationRevision(ctx context.Context, sessionID string) (models.ConversationRevision, error) {
	if err := s.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return models.ConversationRevision{}, err
	}
	reader, ok := s.messages.(taskrepo.ConversationSourceRepository)
	if !ok {
		return models.ConversationRevision{}, errConversationSourceUnavailable
	}
	return reader.ReadConversationRevision(ctx, sessionID)
}

// ReadConversationMessagesPage returns one authorized, revision-bound source
// page without touching the first-party session message cache.
func (s *Service) ReadConversationMessagesPage(ctx context.Context, req models.ConversationMessagePageRequest) (models.ConversationMessagePage, error) {
	if err := s.AuthorizeSessionAccess(ctx, req.SessionID); err != nil {
		return models.ConversationMessagePage{}, err
	}
	reader, ok := s.messages.(taskrepo.ConversationSourceRepository)
	if !ok {
		return models.ConversationMessagePage{}, errConversationSourceUnavailable
	}
	return reader.ReadConversationMessagesPage(ctx, req)
}

// ReadConversationTurnsPage returns one authorized, revision-bound source
// page without mutating the first-party turn cache.
func (s *Service) ReadConversationTurnsPage(ctx context.Context, req models.ConversationTurnPageRequest) (models.ConversationTurnPage, error) {
	if err := s.AuthorizeSessionAccess(ctx, req.SessionID); err != nil {
		return models.ConversationTurnPage{}, err
	}
	reader, ok := s.turns.(taskrepo.ConversationSourceRepository)
	if !ok {
		return models.ConversationTurnPage{}, errConversationSourceUnavailable
	}
	return reader.ReadConversationTurnsPage(ctx, req)
}
