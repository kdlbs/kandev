package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type queuedMessageOnceWriter interface {
	CreateMessageAndQueueOnce(context.Context, *models.Message, *messagequeue.QueuedMessage, int) (bool, error)
}

func validateQueuedOnceInput(id string, req *CreateMessageRequest, queued *messagequeue.QueuedMessage) error {
	if id == "" || req == nil || queued == nil {
		return errors.New("message id, request and queue entry are required")
	}
	if req.AuthorType != string(models.MessageAuthorUser) {
		return errors.New("queued messages are authored by a user")
	}
	return nil
}

// CreateQueuedMessageOnce stores a user message and its queue entry, both
// under id, at most once. created is true when this call inserted them; when a
// message with id already exists nothing is inserted and that stored message
// is returned with created false, without starting a turn. The message is
// authored by req.AuthorID as given (the caller has already authenticated that
// user); it is not re-resolved to the identity of the calling context.
func (s *Service) CreateQueuedMessageOnce(
	ctx context.Context,
	id string,
	req *CreateMessageRequest,
	queued *messagequeue.QueuedMessage,
	maxPerSession int,
) (*models.Message, bool, error) {
	if err := validateQueuedOnceInput(id, req, queued); err != nil {
		return nil, false, err
	}
	writer, ok := s.messages.(queuedMessageOnceWriter)
	if !ok {
		return nil, false, errors.New("once-only queued message admission is unavailable")
	}
	if err := s.authorizeMessageCreate(ctx, req); err != nil {
		return nil, false, err
	}
	ctx, release, err := s.acquireMessageCreateAdmission(ctx, id, req)
	if err != nil {
		return nil, false, err
	}
	defer release()
	if existing, found, err := s.findStoredMessage(ctx, id); err != nil || found {
		return existing, false, err
	}
	session, err := s.getSessionWithRetry(ctx, req.TaskSessionID, id, messageCreateMaxRetries, messageCreateRetryDelay)
	if err != nil {
		return nil, false, err
	}
	message, err := s.buildMessage(ctx, id, req, session)
	if err != nil {
		return nil, false, err
	}
	message.AuthorID = req.AuthorID
	queued.ID = id
	queued.SessionID = message.TaskSessionID
	queued.TaskID = message.TaskID
	queued.Content = message.Content
	queued.QueuedBy = messagequeue.QueuedByUser
	created, err := writer.CreateMessageAndQueueOnce(ctx, message, queued, maxPerSession)
	if err != nil {
		return nil, false, err
	}
	if !created {
		existing, found, err := s.findStoredMessage(ctx, id)
		if err != nil {
			return nil, false, err
		}
		if !found {
			return nil, false, fmt.Errorf("message %s vanished after an insert conflict", id)
		}
		return existing, false, nil
	}
	_ = s.publishMessageEvent(ctx, events.MessageAdded, message)
	return message, true, nil
}

func (s *Service) findStoredMessage(ctx context.Context, id string) (*models.Message, bool, error) {
	existing, err := s.messages.GetMessageWithPromptIndex(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && existing == nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("check existing message: %w", err)
	}
	if err := s.AuthorizeSessionScope(ctx, existing.TaskSessionID, authz.ScopeSessionPrompt); err != nil {
		return nil, false, err
	}
	return existing, true, nil
}
