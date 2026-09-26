package service

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// PrepareConversationForkAgentAdmission authorizes a new-agent fork and
// returns the transaction input that the session repository consumes when it
// inserts the destination session.
func (s *Service) PrepareConversationForkAgentAdmission(
	ctx context.Context,
	taskID, forkID, requestID, fingerprint string,
) (models.ConversationForkAdmission, models.ConversationForkDraft, error) {
	task, err := s.authorizedConversationForkAgentTask(ctx, taskID, forkID, requestID, fingerprint)
	if err != nil {
		return models.ConversationForkAdmission{}, models.ConversationForkDraft{}, err
	}
	ownerID, err := s.conversationForkOwnerID(ctx, task.WorkspaceID)
	if err != nil {
		return models.ConversationForkAdmission{}, models.ConversationForkDraft{}, models.ErrConversationForkNotFound
	}
	destinations, ok := s.messages.(taskrepo.ConversationForkDestinationRepository)
	if !ok {
		return models.ConversationForkAdmission{}, models.ConversationForkDraft{}, models.ErrConversationForkSourceUnavailable
	}
	admission := models.ConversationForkAdmission{
		OwnerID: ownerID, WorkspaceID: task.WorkspaceID, ForkID: forkID, DestinationKind: createdByAgent,
		DestinationRequestID: requestID, RequestFingerprint: fingerprint, DestinationTaskID: taskID,
	}
	if existing, found, err := s.existingConversationForkAgentAdmission(ctx, destinations, admission); found || err != nil {
		return admission, existing, err
	}
	draft, err := s.loadConversationForkAgentDraft(ctx, admission)
	if err != nil {
		return models.ConversationForkAdmission{}, models.ConversationForkDraft{}, err
	}
	return admission, draft, nil
}

func (s *Service) authorizedConversationForkAgentTask(ctx context.Context, taskID, forkID, requestID, fingerprint string) (*models.Task, error) {
	if taskID == "" || forkID == "" || requestID == "" || len(requestID) > 128 || fingerprint == "" {
		return nil, models.ErrConversationForkConflict
	}
	if err := s.AuthorizeTaskScope(ctx, taskID, authz.ScopeSessionPrompt); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil {
		if err != nil {
			return nil, err
		}
		return nil, models.ErrConversationForkNotFound
	}
	if task.IsEphemeral || task.ProjectID != "" {
		return nil, models.ErrConversationForkUnsupportedDestination
	}
	return task, nil
}

func (s *Service) existingConversationForkAgentAdmission(
	ctx context.Context,
	destinations taskrepo.ConversationForkDestinationRepository,
	admission models.ConversationForkAdmission,
) (models.ConversationForkDraft, bool, error) {
	existing, err := destinations.GetConversationForkByDestinationRequest(ctx, admission.OwnerID, admission.DestinationRequestID)
	if errors.Is(err, models.ErrConversationForkNotFound) {
		return models.ConversationForkDraft{}, false, nil
	}
	if err != nil {
		return models.ConversationForkDraft{}, true, err
	}
	if existing.Descriptor.ID != admission.ForkID || existing.Descriptor.DestinationKind != createdByAgent ||
		existing.Descriptor.DestinationTaskID != admission.DestinationTaskID || existing.DestinationFingerprint != admission.RequestFingerprint || existing.WorkspaceID != admission.WorkspaceID {
		return models.ConversationForkDraft{}, true, models.ErrConversationForkConflict
	}
	return existing, true, nil
}

func (s *Service) loadConversationForkAgentDraft(ctx context.Context, admission models.ConversationForkAdmission) (models.ConversationForkDraft, error) {
	drafts, ok := s.messages.(taskrepo.ConversationForkDraftRepository)
	if !ok {
		return models.ConversationForkDraft{}, models.ErrConversationForkSourceUnavailable
	}
	draft, err := drafts.GetConversationForkDraft(ctx, admission.OwnerID, admission.ForkID, time.Now().UTC())
	if err != nil {
		return models.ConversationForkDraft{}, err
	}
	if draft.Descriptor.State != conversationForkStateDraft || draft.WorkspaceID != admission.WorkspaceID {
		return models.ConversationForkDraft{}, models.ErrConversationForkConflict
	}
	if err := s.AuthorizeSessionAccess(ctx, draft.Descriptor.SourceSessionID); err != nil {
		return models.ConversationForkDraft{}, err
	}
	return draft, nil
}

// GetConversationForkForDestination reads historical text through the
// destination task/session grant. The source session is no longer an authority
// after admission because its owner may delete it.
func (s *Service) GetConversationForkForDestination(ctx context.Context, taskID, sessionID string) (models.ConversationForkDraft, error) {
	if taskID == "" || sessionID == "" {
		return models.ConversationForkDraft{}, models.ErrConversationForkNotFound
	}
	if err := s.AuthorizeTaskSessionAccess(ctx, taskID, sessionID); err != nil {
		return models.ConversationForkDraft{}, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil {
		if err != nil {
			return models.ConversationForkDraft{}, err
		}
		return models.ConversationForkDraft{}, models.ErrConversationForkNotFound
	}
	ownerID, err := s.conversationForkOwnerID(ctx, task.WorkspaceID)
	if err != nil {
		return models.ConversationForkDraft{}, models.ErrConversationForkNotFound
	}
	destinations, ok := s.messages.(taskrepo.ConversationForkDestinationRepository)
	if !ok {
		return models.ConversationForkDraft{}, models.ErrConversationForkSourceUnavailable
	}
	return destinations.GetConversationForkByDestinationSession(ctx, ownerID, taskID, sessionID)
}
