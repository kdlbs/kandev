package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
)

const (
	messageCreateMaxRetries    = 5
	messageCreateRetryDelay    = 50 * time.Millisecond
	clarificationPendingStatus = "pending"
)

var ErrMessageIDConflict = errors.New("client message id is already used")

type planCommentMessageWriter interface {
	CreateMessageWithPlanComments(
		context.Context,
		*models.Message,
		[]models.TaskPlanCommentRef,
		bool,
	) (*models.TaskPlanCommentSnapshot, error)
}

type queuedPlanCommentMessageWriter interface {
	CreateMessageWithPlanCommentsAndQueue(
		context.Context,
		*models.Message,
		*messagequeue.QueuedMessage,
		[]models.TaskPlanCommentRef,
		bool,
		int,
	) (*models.TaskPlanCommentSnapshot, error)
}

// CreateMessage creates a new message on an agent session
func (s *Service) CreateMessage(ctx context.Context, req *CreateMessageRequest) (*models.Message, error) {
	if err := preparePlanCommentMessageRequest(req); err != nil {
		return nil, err
	}
	messageID := uuid.New().String()
	session, err := s.getSessionWithRetry(
		ctx,
		req.TaskSessionID,
		messageID,
		messageCreateMaxRetries,
		messageCreateRetryDelay,
	)
	if err != nil {
		return nil, err
	}

	authorType := models.MessageAuthorUser
	if req.AuthorType == "agent" {
		authorType = models.MessageAuthorAgent
	}

	messageType := models.MessageType(req.Type)
	if messageType == "" {
		messageType = models.MessageTypeMessage
	}

	taskID := req.TaskID
	if taskID == "" && session != nil {
		taskID = session.TaskID
	}

	// Ensure we have a turn ID - get active turn or start a new one
	turnID := req.TurnID
	if turnID == "" {
		var turn *models.Turn
		if req.CompletedTurn {
			turn, err = s.createCompletedTurn(ctx, session)
		} else {
			turn, err = s.getOrStartTurnWithRetry(
				ctx,
				req.TaskSessionID,
				messageID,
				messageCreateMaxRetries,
				messageCreateRetryDelay,
			)
		}
		if err != nil {
			s.logger.Warn("failed to get or start turn for message",
				zap.String("session_id", req.TaskSessionID),
				zap.Error(err))
			return nil, fmt.Errorf("failed to get or start turn: %w", err)
		} else if turn != nil {
			turnID = turn.ID
		}
	}

	message := &models.Message{
		ID:            messageID,
		TaskSessionID: req.TaskSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    authorType,
		AuthorID:      req.AuthorID,
		Content:       req.Content,
		Type:          messageType,
		Metadata:      req.Metadata,
		RequestsInput: req.RequestsInput,
		// CreatedAt deliberately left zero: the repository assigns it inside
		// the atomic per-session create boundary, which treats a zero
		// timestamp as a LIVE create (advancing a colliding or backward key
		// by one tick). Pre-populating it here would misclassify the message
		// as an explicit import and reject same-microsecond creates.
	}

	snapshot, err := s.persistMessage(ctx, message, req)
	if err != nil {
		s.logger.Error("failed to create message", zap.Error(err))
		return nil, err
	}

	// Publish message.added event
	s.publishMessageEvent(ctx, events.MessageAdded, message)
	s.publishMessagePlanCommentSnapshot(ctx, snapshot)

	s.logger.Info("message created",
		zap.String("message_id", message.ID),
		zap.String("session_id", message.TaskSessionID),
		zap.String("author_type", string(message.AuthorType)))

	return message, nil
}

// CreateMessageIdempotent persists a caller-owned message ID and returns the
// existing row when the request is replayed. A client can lose the response
// after the database commit, so retrying must not create a second user turn.
// The handler performs the fast preflight before session-state side effects;
// this method also closes the concurrent two-request race at the repository
// primary-key boundary. Replay reads go through GetMessageWithPromptIndex so
// the returned row carries its stable prompt ordinal.
func (s *Service) CreateMessageIdempotent(ctx context.Context, id string, req *CreateMessageRequest) (*models.Message, error) {
	if id == "" {
		return nil, errors.New("message id is required for idempotent creation")
	}
	if err := preparePlanCommentMessageRequest(req); err != nil {
		return nil, err
	}

	existing, err := s.messages.GetMessageWithPromptIndex(ctx, id)
	if err == nil && existing != nil {
		if !matchesPlanCommentMessageReplay(existing, req) {
			return nil, ErrMessageIDConflict
		}
		return existing, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check existing message: %w", err)
	}

	message, err := s.CreateMessageWithID(ctx, id, req)
	if err == nil {
		return message, nil
	}

	// Another request may have won the insert while this request was building
	// its turn. Read the committed row and treat that duplicate as success.
	existing, lookupErr := s.messages.GetMessageWithPromptIndex(ctx, id)
	if lookupErr == nil && existing != nil {
		if !matchesPlanCommentMessageReplay(existing, req) {
			return nil, ErrMessageIDConflict
		}
		return existing, nil
	}
	return nil, err
}

type planCommentMessageValidator interface {
	ValidateMessagePlanComments(
		context.Context,
		string,
		string,
		string,
		[]models.TaskPlanCommentRef,
		bool,
	) error
}

// ValidatePlanCommentMessage rejects stale references before callers execute
// stateful turn-start hooks. Persistence repeats the same validation under its
// atomic message/comment boundary to close races.
func (s *Service) ValidatePlanCommentMessage(
	ctx context.Context,
	taskID, sessionID, content string,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
) error {
	if len(refs) == 0 {
		return nil
	}
	validator, ok := s.messages.(planCommentMessageValidator)
	if !ok {
		return errors.New("plan comment message validation is unavailable")
	}
	return validator.ValidateMessagePlanComments(
		ctx, taskID, sessionID, plancomments.WithPlaceholder(content), refs, requirePrimary,
	)
}

// CreateQueuedMessageIdempotent commits a comment-bearing user message and
// its deferred queue delivery in the same repository transaction. Exact
// caller-ID replays return the original message without adding another queue
// entry or consuming another comment snapshot.
func (s *Service) CreateQueuedMessageIdempotent(
	ctx context.Context,
	id string,
	req *CreateMessageRequest,
	queued *messagequeue.QueuedMessage,
	maxPerSession int,
) (*models.Message, error) {
	if err := validateQueuedPlanCommentMessage(id, req, queued); err != nil {
		return nil, err
	}
	existing, found, err := s.findPlanCommentMessageReplay(ctx, id, req)
	if err != nil || found {
		return existing, err
	}

	session, err := s.getSessionWithRetry(ctx, req.TaskSessionID, id, messageCreateMaxRetries, messageCreateRetryDelay)
	if err != nil {
		return nil, err
	}
	message, err := s.buildMessage(ctx, id, req, session)
	if err != nil {
		return nil, err
	}
	queued.ID = id
	queued.SessionID = message.TaskSessionID
	queued.TaskID = message.TaskID
	queued.Content = message.Content
	queued.QueuedBy = messagequeue.QueuedByUser

	writer, ok := s.messages.(queuedPlanCommentMessageWriter)
	if !ok {
		return nil, errors.New("queued plan comment message admission is unavailable")
	}
	snapshot, err := writer.CreateMessageWithPlanCommentsAndQueue(
		ctx, message, queued, req.PlanCommentRefs, req.RequirePrimarySession, maxPerSession,
	)
	if err != nil {
		existing, found, lookupErr := s.findPlanCommentMessageReplay(ctx, id, req)
		if lookupErr == nil && found {
			return existing, nil
		}
		return nil, err
	}

	_ = s.publishMessageEvent(ctx, events.MessageAdded, message)
	s.publishMessagePlanCommentSnapshot(ctx, snapshot)
	s.logger.Info("queued message created with ID",
		zap.String("message_id", message.ID),
		zap.String("session_id", message.TaskSessionID),
		zap.String("author_type", string(message.AuthorType)))
	return message, nil
}

func validateQueuedPlanCommentMessage(
	id string,
	req *CreateMessageRequest,
	queued *messagequeue.QueuedMessage,
) error {
	if id == "" {
		return errors.New("message id is required for idempotent creation")
	}
	if req == nil || len(req.PlanCommentRefs) == 0 {
		return errors.New("plan comment refs are required for queued message creation")
	}
	if queued == nil {
		return errors.New("queued message is required")
	}
	return preparePlanCommentMessageRequest(req)
}

func (s *Service) findPlanCommentMessageReplay(
	ctx context.Context,
	id string,
	req *CreateMessageRequest,
) (*models.Message, bool, error) {
	existing, err := s.messages.GetMessageWithPromptIndex(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && existing == nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("check existing queued message: %w", err)
	}
	if !matchesPlanCommentMessageReplay(existing, req) {
		return nil, false, ErrMessageIDConflict
	}
	return existing, true, nil
}

// CreateMessageWithID creates a new message with a pre-generated ID.
// This is used for streaming messages where the ID is generated by the caller.
// It includes retry logic to handle transient database errors and ensure
// message chunks are not lost during streaming.
func (s *Service) CreateMessageWithID(ctx context.Context, id string, req *CreateMessageRequest) (*models.Message, error) {
	if err := preparePlanCommentMessageRequest(req); err != nil {
		return nil, err
	}
	session, err := s.getSessionWithRetry(ctx, req.TaskSessionID, id, messageCreateMaxRetries, messageCreateRetryDelay)
	if err != nil {
		return nil, err
	}

	message, err := s.buildMessage(ctx, id, req, session)
	if err != nil {
		return nil, err
	}

	snapshot, err := s.createMessageWithRequestRetry(
		ctx, message, req, messageCreateMaxRetries, messageCreateRetryDelay,
	)
	if err != nil {
		return nil, err
	}

	// Publish message.added event
	s.publishMessageEvent(ctx, events.MessageAdded, message)
	s.publishMessagePlanCommentSnapshot(ctx, snapshot)

	s.logger.Info("message created with ID",
		zap.String("message_id", message.ID),
		zap.String("session_id", message.TaskSessionID),
		zap.String("author_type", string(message.AuthorType)))

	return message, nil
}

// getSessionWithRetry fetches a session, retrying on transient errors caused by out-of-order events.
func (s *Service) getSessionWithRetry(ctx context.Context, sessionID, messageID string, maxRetries int, retryDelay time.Duration) (*models.TaskSession, error) {
	var session *models.TaskSession
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		session, err = s.sessions.GetTaskSession(ctx, sessionID)
		if err == nil {
			return session, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt < maxRetries-1 {
			s.logger.Debug("session not found for message create, retrying",
				zap.String("session_id", sessionID),
				zap.String("message_id", messageID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(retryDelay)
		}
	}
	s.logger.Warn("session not found for message create after retries",
		zap.String("session_id", sessionID),
		zap.String("message_id", messageID),
		zap.Int("retries", maxRetries),
		zap.Error(err))
	return nil, err
}

// getOrStartTurnWithRetry fetches or starts a turn, retrying short-lived FK/session visibility races.
func (s *Service) getOrStartTurnWithRetry(ctx context.Context, sessionID, messageID string, maxRetries int, retryDelay time.Duration) (*models.Turn, error) {
	var turn *models.Turn
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		turn, err = s.getOrStartTurn(ctx, sessionID)
		if err == nil {
			return turn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt < maxRetries-1 {
			s.logger.Debug("failed to get or start turn for message, retrying",
				zap.String("session_id", sessionID),
				zap.String("message_id", messageID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries),
				zap.Error(err))
			time.Sleep(retryDelay)
		}
	}
	return nil, err
}

// buildMessage constructs a Message model from a CreateMessageRequest and resolved session.
func (s *Service) buildMessage(ctx context.Context, id string, req *CreateMessageRequest, session *models.TaskSession) (*models.Message, error) {
	authorType := models.MessageAuthorUser
	if req.AuthorType == "agent" {
		authorType = models.MessageAuthorAgent
	}

	messageType := models.MessageType(req.Type)
	if messageType == "" {
		messageType = models.MessageTypeMessage
	}

	taskID := req.TaskID
	if taskID == "" && session != nil {
		taskID = session.TaskID
	}

	turnID := req.TurnID
	if turnID == "" {
		if turn, err := s.getOrStartTurnWithRetry(
			ctx,
			req.TaskSessionID,
			id,
			messageCreateMaxRetries,
			messageCreateRetryDelay,
		); err != nil {
			s.logger.Warn("failed to get or start turn for streaming message",
				zap.String("session_id", req.TaskSessionID),
				zap.Error(err))
			return nil, fmt.Errorf("failed to get or start turn: %w", err)
		} else if turn != nil {
			turnID = turn.ID
		}
	}

	return &models.Message{
		ID:            id,
		TaskSessionID: req.TaskSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    authorType,
		AuthorID:      req.AuthorID,
		Content:       req.Content,
		Type:          messageType,
		Metadata:      req.Metadata,
		RequestsInput: req.RequestsInput,
		// CreatedAt deliberately left zero: the repository's atomic per-session
		// create boundary assigns it (live creates advance a colliding key).
	}, nil
}

// createMessageWithRetry persists a message with retry logic for transient DB errors.
func (s *Service) createMessageWithRequestRetry(
	ctx context.Context,
	message *models.Message,
	req *CreateMessageRequest,
	maxRetries int,
	retryDelay time.Duration,
) (*models.TaskPlanCommentSnapshot, error) {
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		var snapshot *models.TaskPlanCommentSnapshot
		snapshot, err = s.persistMessage(ctx, message, req)
		if err == nil {
			return snapshot, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt < maxRetries-1 {
			s.logger.Debug("failed to create message, retrying",
				zap.String("message_id", message.ID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries),
				zap.Error(err))
			time.Sleep(retryDelay)
		}
	}
	s.logger.Error("failed to create message with ID after retries",
		zap.String("id", message.ID),
		zap.Int("retries", maxRetries),
		zap.Error(err))
	return nil, err
}

func (s *Service) persistMessage(
	ctx context.Context,
	message *models.Message,
	req *CreateMessageRequest,
) (*models.TaskPlanCommentSnapshot, error) {
	if len(req.PlanCommentRefs) == 0 {
		return nil, s.messages.CreateMessage(ctx, message)
	}
	writer, ok := s.messages.(planCommentMessageWriter)
	if !ok {
		return nil, errors.New("plan comment message admission is unavailable")
	}
	return writer.CreateMessageWithPlanComments(
		ctx, message, req.PlanCommentRefs, req.RequirePrimarySession,
	)
}

type planCommentMessageReplayIdentity struct {
	TaskSessionID  string                      `json:"session_id"`
	TaskID         string                      `json:"task_id"`
	TurnID         string                      `json:"turn_id"`
	Content        string                      `json:"content"`
	AuthorID       string                      `json:"author_id"`
	MessageType    string                      `json:"message_type"`
	Metadata       map[string]interface{}      `json:"metadata"`
	Refs           []models.TaskPlanCommentRef `json:"refs"`
	RequirePrimary bool                        `json:"require_primary"`
}

func preparePlanCommentMessageRequest(req *CreateMessageRequest) error {
	if req == nil || len(req.PlanCommentRefs) == 0 {
		return nil
	}
	metadata := make(map[string]interface{}, len(req.Metadata)+2)
	for key, value := range req.Metadata {
		if key != plancomments.MetadataRefs && key != plancomments.MetadataRequestFingerprint {
			metadata[key] = value
		}
	}
	identity := planCommentMessageReplayIdentity{
		TaskSessionID: req.TaskSessionID, TaskID: req.TaskID, TurnID: req.TurnID,
		Content: req.Content, AuthorID: req.AuthorID, MessageType: req.Type,
		Metadata: metadata, Refs: req.PlanCommentRefs, RequirePrimary: req.RequirePrimarySession,
	}
	fingerprint, err := plancomments.Fingerprint(identity)
	if err != nil {
		return err
	}
	metadata[plancomments.MetadataRefs] = req.PlanCommentRefs
	metadata[plancomments.MetadataRequestFingerprint] = fingerprint
	req.Metadata = metadata
	return nil
}

func matchesPlanCommentMessageReplay(existing *models.Message, req *CreateMessageRequest) bool {
	if len(req.PlanCommentRefs) == 0 {
		return true
	}
	want, _ := req.Metadata[plancomments.MetadataRequestFingerprint].(string)
	got, _ := existing.Metadata[plancomments.MetadataRequestFingerprint].(string)
	return want != "" && got == want
}

func (s *Service) publishMessagePlanCommentSnapshot(
	ctx context.Context,
	snapshot *models.TaskPlanCommentSnapshot,
) {
	if snapshot == nil || s.eventBus == nil {
		return
	}
	if err := s.eventBus.Publish(ctx, events.TaskPlanCommentsChanged,
		bus.NewEvent(events.TaskPlanCommentsChanged, "task-service", snapshot)); err != nil {
		s.logger.Error("publish consumed plan comments",
			zap.String("task_id", snapshot.TaskID),
			zap.String("plan_id", snapshot.PlanID),
			zap.Int64("comments_revision", snapshot.Revision),
			zap.Int("comment_count", len(snapshot.Comments)),
			zap.Error(err),
		)
	}
}

// GetMessage retrieves a message by ID
func (s *Service) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	message, err := s.messages.GetMessage(ctx, id)
	if err != nil {
		return nil, err
	}
	// Scope like ListMessages: the shell-output route reaches a message by ID,
	// so without this a caller holding someone else's (session_id, message_id)
	// pair could read their command output.
	if message.TaskSessionID != "" {
		if err := s.AuthorizeSessionAccess(ctx, message.TaskSessionID); err != nil {
			return nil, err
		}
	}
	return message, nil
}

// GetMessageWithPromptIndex retrieves a message by ID with its computed
// prompt ordinal, scoped like GetMessage. Used by the idempotent WS
// replay/response path so a retried prompt answers with its stable index.
func (s *Service) GetMessageWithPromptIndex(ctx context.Context, id string) (*models.Message, error) {
	message, err := s.messages.GetMessageWithPromptIndex(ctx, id)
	if err != nil {
		return nil, err
	}
	if message.TaskSessionID != "" {
		if err := s.AuthorizeSessionAccess(ctx, message.TaskSessionID); err != nil {
			return nil, err
		}
	}
	return message, nil
}

// ListMessages returns all messages for a session.
func (s *Service) ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error) {
	if err := s.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	return s.messages.ListMessages(ctx, sessionID)
}

// ListMessagesPaginated returns messages for a session with pagination options.
func (s *Service) ListMessagesPaginated(ctx context.Context, req ListMessagesRequest) ([]*models.Message, bool, error) {
	if err := s.AuthorizeSessionAccess(ctx, req.TaskSessionID); err != nil {
		return nil, false, err
	}
	limit := req.Limit
	if limit <= 0 && (req.Before != "" || req.After != "" || req.Around != "" || req.AuthorType != "") {
		limit = DefaultMessagesPageSize
	}
	if limit > MaxMessagesPageSize {
		limit = MaxMessagesPageSize
	}
	return s.messages.ListMessagesPaginated(ctx, req.TaskSessionID, models.ListMessagesOptions{
		Limit:      limit,
		Before:     req.Before,
		After:      req.After,
		Sort:       req.Sort,
		AuthorType: req.AuthorType,
		Around:     req.Around,
	})
}

// ListMessagesForPlugin returns messages matching the plugin Host data API
// filter (ADR 0047), backing internal/plugins' capability-gated
// Messages().List reader. Reads go through the service layer, never a
// repository directly, per ADR 0043.
func (s *Service) ListMessagesForPlugin(ctx context.Context, filter models.PluginMessageFilter) ([]*models.Message, error) {
	return s.messages.ListMessagesForPlugin(ctx, filter)
}

// SearchMessages returns messages whose content matches the query in the given session.
func (s *Service) SearchMessages(ctx context.Context, sessionID, query string, limit int) ([]*models.Message, error) {
	if err := s.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	return s.messages.SearchMessages(ctx, sessionID, models.SearchMessagesOptions{
		Query: query,
		Limit: limit,
	})
}

// DeleteMessage deletes a message
func (s *Service) DeleteMessage(ctx context.Context, id string) error {
	message, getErr := s.messages.GetMessage(ctx, id)
	if err := s.messages.DeleteMessage(ctx, id); err != nil {
		s.logger.Error("failed to delete message", zap.String("message_id", id), zap.Error(err))
		return err
	}

	if getErr == nil && message != nil {
		s.publishMessageEvent(ctx, events.MessageDeleted, message)
	}
	s.logger.Info("message deleted", zap.String("message_id", id))
	return nil
}

// UpdateMessage updates an existing message and publishes an event.
func (s *Service) UpdateMessage(ctx context.Context, message *models.Message) error {
	if err := s.messages.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to update message",
			zap.String("message_id", message.ID),
			zap.Error(err))
		return err
	}

	// User rows carry the stable prompt ordinal in message.updated events:
	// re-read the affected row through GetMessageWithPromptIndex before
	// publishing. Agent messages and streaming agent content/thinking updates
	// stay on the hot 12-column loaded model and never carry the field.
	published := message
	if message.AuthorType == models.MessageAuthorUser {
		if indexed, err := s.messages.GetMessageWithPromptIndex(ctx, message.ID); err == nil {
			published = indexed
		} else {
			s.logger.Warn("failed to re-read indexed message for update event",
				zap.String("message_id", message.ID),
				zap.Error(err))
		}
	}

	// Publish message.updated event for real-time streaming. Delivery is best
	// effort after the durable write succeeded (see publishMessageEvent).
	_ = s.publishMessageEvent(ctx, events.MessageUpdated, published)

	return nil
}

// AppendMessageContent appends additional content to an existing message.
// This is used for streaming agent responses where content arrives incrementally.
func (s *Service) AppendMessageContent(ctx context.Context, messageID, additionalContent string) error {
	message, err := s.messages.GetMessage(ctx, messageID)
	if err != nil {
		s.logger.Warn("message not found for append",
			zap.String("message_id", messageID),
			zap.Error(err))
		return err
	}

	// Append the new content
	message.Content += additionalContent

	if err := s.messages.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to append message content",
			zap.String("message_id", messageID),
			zap.Error(err))
		return err
	}

	// Publish message.updated event for real-time streaming
	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	s.logger.Debug("message content appended",
		zap.String("message_id", messageID),
		zap.Int("appended_length", len(additionalContent)),
		zap.Int("total_length", len(message.Content)))

	return nil
}

// AppendThinkingContent appends additional thinking content to an existing thinking message.
// This updates the metadata.thinking field for streaming agent reasoning.
func (s *Service) AppendThinkingContent(ctx context.Context, messageID, additionalContent string) error {
	message, err := s.messages.GetMessage(ctx, messageID)
	if err != nil {
		s.logger.Warn("thinking message not found for append",
			zap.String("message_id", messageID),
			zap.Error(err))
		return err
	}

	// Initialize metadata if nil
	if message.Metadata == nil {
		message.Metadata = make(map[string]interface{})
	}

	// Get existing thinking content and append
	existingThinking := ""
	if existing, ok := message.Metadata["thinking"].(string); ok {
		existingThinking = existing
	}
	message.Metadata["thinking"] = existingThinking + additionalContent

	if err := s.messages.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to append thinking content",
			zap.String("message_id", messageID),
			zap.Error(err))
		return err
	}

	// Publish message.updated event for real-time streaming
	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	s.logger.Debug("thinking content appended",
		zap.String("message_id", messageID),
		zap.Int("appended_length", len(additionalContent)))

	return nil
}

// UpdateToolCallMessage updates a tool call message's status, optionally title and normalized data.
// It includes retry logic to handle race conditions where the complete event
// may arrive before the message has been created by the start event.
// If the message is not found after retries and taskID/turnID/msgType are provided, it creates the message.
// The normalized parameter contains typed tool payload data that gets added to metadata.
func (s *Service) UpdateToolCallMessage(ctx context.Context, sessionID, toolCallID, status, result, title string, normalized *streams.NormalizedPayload) error {
	return s.UpdateToolCallMessageWithCreate(ctx, sessionID, toolCallID, "", status, result, title, normalized, "", "", "")
}

// UpdateToolCallMessageWithCreate is like UpdateToolCallMessage but can create the message if not found.
// If taskID, turnID, and msgType are provided, the message will be created if it doesn't exist.
// parentToolCallID is used for subagent nesting (empty for top-level).
func (s *Service) UpdateToolCallMessageWithCreate(ctx context.Context, sessionID, toolCallID, parentToolCallID, status, result, title string, normalized *streams.NormalizedPayload, taskID, turnID, msgType string) error {
	const maxRetries = 5
	const retryDelay = 100 * time.Millisecond

	message, err := s.getToolCallMessageWithRetry(ctx, sessionID, toolCallID, maxRetries, retryDelay)

	// If message not found and we have enough info to create it, do so
	if err != nil && taskID != "" && msgType != "" {
		return s.createToolCallMessageFallback(ctx, sessionID, toolCallID, parentToolCallID, status, title, turnID, taskID, msgType, normalized)
	}

	if err != nil {
		s.logger.Warn("tool call message not found for update after retries",
			zap.String("session_id", sessionID),
			zap.String("tool_call_id", toolCallID),
			zap.Int("retries", maxRetries),
			zap.Error(err))
		return err
	}

	s.applyToolCallMessageUpdate(message, status, result, title, normalized)

	if err := s.messages.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to update tool call message",
			zap.String("message_id", message.ID),
			zap.String("tool_call_id", toolCallID),
			zap.Error(err))
		return err
	}

	// Publish message.updated event
	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	s.logger.Info("tool call message updated",
		zap.String("message_id", message.ID),
		zap.String("tool_call_id", toolCallID),
		zap.String("status", status))

	return nil
}

// getToolCallMessageWithRetry fetches a tool call message with retry logic for race conditions.
func (s *Service) getToolCallMessageWithRetry(ctx context.Context, sessionID, toolCallID string, maxRetries int, retryDelay time.Duration) (*models.Message, error) {
	var message *models.Message
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		message, err = s.messages.GetMessageByToolCallID(ctx, sessionID, toolCallID)
		if err == nil {
			return message, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt < maxRetries-1 {
			s.logger.Debug("tool call message not found, retrying",
				zap.String("session_id", sessionID),
				zap.String("tool_call_id", toolCallID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(retryDelay)
		}
	}
	return nil, err
}

// createToolCallMessageFallback creates a tool call message when it cannot be found via GetMessageByToolCallID.
func (s *Service) createToolCallMessageFallback(ctx context.Context, sessionID, toolCallID, parentToolCallID, status, title, turnID, taskID, msgType string, normalized *streams.NormalizedPayload) error {
	s.logger.Info("tool call message not found, creating it",
		zap.String("session_id", sessionID),
		zap.String("tool_call_id", toolCallID),
		zap.String("task_id", taskID),
		zap.String("msg_type", msgType))

	metadata := map[string]interface{}{
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}
	if parentToolCallID != "" {
		metadata["parent_tool_call_id"] = parentToolCallID
	}
	if normalized != nil {
		metadata["normalized"] = normalized
	}

	msg, createErr := s.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       title,
		AuthorType:    "agent",
		Type:          msgType,
		Metadata:      metadata,
	})
	if createErr != nil {
		s.logger.Error("failed to create tool call message as fallback",
			zap.String("session_id", sessionID),
			zap.String("tool_call_id", toolCallID),
			zap.Error(createErr))
		return createErr
	}

	s.logger.Info("created tool call message as fallback",
		zap.String("message_id", msg.ID),
		zap.String("tool_call_id", toolCallID),
		zap.String("status", status))
	return nil
}

// applyToolCallMessageUpdate applies status, result, normalized data, and title to a tool call message.
//
// Defensive guard: a permission_request row also carries `tool_call_id` in metadata, so
// any future code path that hands such a row in here would otherwise silently overwrite
// the user's approve/reject decision and retype it to tool_execute. The repository's
// GetMessageByToolCallID excludes permission_request, but the guard makes the invariant
// explicit at the layer that does the writing.
func (s *Service) applyToolCallMessageUpdate(message *models.Message, status, result, title string, normalized *streams.NormalizedPayload) {
	if message.Type == models.MessageTypePermissionRequest {
		// Error severity: the repo-layer GetMessageByToolCallID filter is supposed
		// to make this branch unreachable. Reaching it means a caller bug; surface
		// it loudly so the invariant violation isn't silently swallowed.
		s.logger.Error("applyToolCallMessageUpdate refusing to overwrite permission_request",
			zap.String("message_id", message.ID),
			zap.String("incoming_status", status))
		return
	}
	if message.Metadata == nil {
		message.Metadata = make(map[string]interface{})
	}
	message.Metadata["status"] = status
	if result != "" {
		message.Metadata["result"] = result
	}

	if normalized != nil {
		message.Metadata["normalized"] = normalized
		// Update message type if the normalized kind changed
		// This handles cases like Read on a directory converting to code_search
		newMsgType := models.MessageType(normalized.Kind().ToMessageType())
		if newMsgType != message.Type {
			s.logger.Debug("updating message type based on normalized kind",
				zap.String("message_id", message.ID),
				zap.String("old_type", string(message.Type)),
				zap.String("new_type", string(newMsgType)),
				zap.String("normalized_kind", string(normalized.Kind())))
			message.Type = newMsgType
		}
	}

	// Update title/content if provided and different from current
	if title != "" && title != message.Content {
		message.Content = title
		message.Metadata["title"] = title
	}
}

// UpdatePermissionMessage updates a permission request message's status.
// It includes retry logic to handle race conditions.
//
// The lookup is qualified by the full (task, session, request, pending)
// identity rather than pending_id alone: a provider may reuse a pending_id
// for a later, unrelated request once the original is resolved, and a
// delayed event about the old request must not be able to expire the new
// one's message.
func (s *Service) UpdatePermissionMessage(ctx context.Context, taskID, sessionID, requestID, pendingID string, status models.PermissionStatus) error {
	const maxRetries = 5
	const retryDelay = 100 * time.Millisecond

	var message *models.Message
	var err error

	// Retry loop to handle race condition
	for attempt := 0; attempt < maxRetries; attempt++ {
		message, err = s.messages.GetPermissionMessageByIdentity(ctx, taskID, sessionID, requestID, pendingID)
		if err == nil {
			break
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if attempt < maxRetries-1 {
			s.logger.Debug("permission message not found, retrying",
				zap.String("session_id", sessionID),
				zap.String("request_id", requestID),
				zap.String("pending_id", pendingID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		s.logger.Warn("permission message not found for update after retries",
			zap.String("session_id", sessionID),
			zap.String("pending_id", pendingID),
			zap.Int("retries", maxRetries),
			zap.Error(err))
		return err
	}

	if message.Metadata == nil {
		message.Metadata = make(map[string]interface{})
	}
	message.Metadata["status"] = string(status)

	if err := s.messages.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to update permission message",
			zap.String("message_id", message.ID),
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return err
	}

	// Publish message.updated event
	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	// When a permission expires, also mark the related tool call as cancelled
	// so the UI no longer shows a loading spinner on the tool call.
	if status == models.PermissionStatusExpired {
		if toolCallID, ok := message.Metadata["tool_call_id"].(string); ok && toolCallID != "" {
			if err := s.UpdateToolCallMessage(ctx, sessionID, toolCallID, "error", "", "", nil); err != nil {
				s.logger.Warn("failed to cancel related tool call message",
					zap.String("tool_call_id", toolCallID),
					zap.String("pending_id", pendingID),
					zap.Error(err))
			}
		}
	}

	s.logger.Info("permission message updated",
		zap.String("message_id", message.ID),
		zap.String("pending_id", pendingID),
		zap.String("status", string(status)))

	return nil
}

// ClaimPermissionResolution durably serializes the first resolver before any
// option is delivered to the live agent process.
func (s *Service) ClaimPermissionResolution(ctx context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
	result, err := s.messages.ClaimPermissionResolution(ctx, request)
	if err != nil {
		s.logger.Error("failed to claim permission resolution",
			zap.String("task_id", request.TaskID),
			zap.String("session_id", request.SessionID),
			zap.String("request_id", request.Audit.RequestID),
			zap.String("pending_id", request.Audit.PendingID),
			zap.Error(err))
		return nil, err
	}
	if result.Outcome == models.PermissionClaimed && result.Message != nil {
		_ = s.publishMessageEvent(ctx, events.MessageUpdated, result.Message)
	}
	return result, nil
}

// FinalizePermissionResolution records the outcome for the exact durable
// claim. Only successful writes publish the existing message update event.
func (s *Service) FinalizePermissionResolution(ctx context.Context, request models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error) {
	result, err := s.messages.FinalizePermissionResolution(ctx, request)
	if err != nil {
		s.logger.Error("failed to finalize permission resolution",
			zap.String("task_id", request.TaskID),
			zap.String("session_id", request.SessionID),
			zap.String("request_id", request.RequestID),
			zap.String("pending_id", request.PendingID),
			zap.String("result", string(request.Result)),
			zap.Error(err))
		return nil, err
	}
	if result.Outcome == models.PermissionFinalized && result.Message != nil {
		_ = s.publishMessageEvent(ctx, events.MessageUpdated, result.Message)
	}
	return result, nil
}

func (s *Service) GetPermissionResolutionAudit(ctx context.Context, taskID, sessionID, requestID, pendingID string) (*models.PermissionResolutionAudit, error) {
	return s.messages.GetPermissionResolutionAudit(ctx, taskID, sessionID, requestID, pendingID)
}

// UpdateClarificationMessageForQuestion updates a single clarification message
// (identified by pending_id + question_id) with the new status and the answer
// payload. Used by both single- and multi-question clarification bundles since
// each question lives in its own chat message.
func (s *Service) UpdateClarificationMessageForQuestion(ctx context.Context, sessionID, pendingID, questionID, status string, answer interface{}) error {
	const maxRetries = 5
	const retryDelay = 100 * time.Millisecond

	var message *models.Message
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		message, err = s.messages.FindMessageByPendingIDAndQuestion(ctx, sessionID, pendingID, questionID)
		if err == nil {
			break
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if attempt < maxRetries-1 {
			s.logger.Debug("clarification message not found, retrying",
				zap.String("session_id", sessionID),
				zap.String("pending_id", pendingID),
				zap.String("question_id", questionID),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		s.logger.Warn("clarification message not found for update after retries",
			zap.String("session_id", sessionID),
			zap.String("pending_id", pendingID),
			zap.String("question_id", questionID),
			zap.Int("retries", maxRetries),
			zap.Error(err))
		return err
	}

	if message.Metadata == nil {
		message.Metadata = make(map[string]interface{})
	}
	message.Metadata["status"] = status
	if answer != nil {
		message.Metadata["response"] = answer
	}

	if err := s.messages.UpdateMessage(ctx, message); err != nil {
		s.logger.Error("failed to update clarification message",
			zap.String("message_id", message.ID),
			zap.String("pending_id", pendingID),
			zap.String("question_id", questionID),
			zap.Error(err))
		return err
	}

	s.publishMessageEvent(ctx, events.MessageUpdated, message)

	s.logger.Info("clarification message updated",
		zap.String("message_id", message.ID),
		zap.String("pending_id", pendingID),
		zap.String("question_id", questionID),
		zap.String("status", status))

	return nil
}

// CompleteActiveClarificationBundle atomically transitions a current-turn
// bundle. The caller publishes the returned messages only after response
// delivery succeeds, so a failed detached resume can be restored for retry.
func (s *Service) CompleteActiveClarificationBundle(
	ctx context.Context,
	pendingID, status string,
	responses map[string]interface{},
) ([]*models.Message, bool, error) {
	return s.messages.CompleteActiveClarificationBundle(ctx, pendingID, status, responses)
}

// FinalizeClarificationResponseDelivery retires the durable recovery intent
// after the claimed response reaches its live or detached handoff boundary.
func (s *Service) FinalizeClarificationResponseDelivery(
	ctx context.Context,
	pendingID, terminalStatus string,
	claimedMessages []*models.Message,
) ([]*models.Message, bool, error) {
	return s.messages.FinalizeClarificationResponseDelivery(
		ctx,
		pendingID,
		terminalStatus,
		claimedMessages,
	)
}

// RestoreActiveClarificationBundle reopens a terminal bundle after detached
// resume acceptance fails and returns the committed pending rows for publication.
func (s *Service) RestoreActiveClarificationBundle(
	ctx context.Context,
	pendingID, terminalStatus string,
	claimedMessages []*models.Message,
) ([]*models.Message, bool, error) {
	return s.messages.RestoreActiveClarificationBundle(
		ctx,
		pendingID,
		terminalStatus,
		claimedMessages,
	)
}

// PublishClarificationBundleUpdates exposes committed rows to ordinary bus
// subscribers. A restored pending bundle first drives the live summary
// projector synchronously. Projection failure is returned to the caller, but
// does not make the durably restored bundle unsafe to retry; later events and
// reads repair that cache. Committed rows are still published on failure so
// clients do not retain the terminal snapshot.
func (s *Service) PublishClarificationBundleUpdates(ctx context.Context, messages []*models.Message) error {
	var resultErr error
	if restored := firstRestoredClarification(messages); restored != nil {
		if s.statusSummaryProjector == nil {
			resultErr = errors.New("task status summary projector is unavailable")
		} else if err := s.statusSummaryProjector.HandleEvent(
			ctx,
			newMessageEvent(events.MessageUpdated, restored),
		); err != nil {
			resultErr = fmt.Errorf("converge clarification status summary: %w", err)
		}
	}
	for _, message := range messages {
		if message == nil {
			continue
		}
		if err := s.publishMessageEvent(ctx, events.MessageUpdated, message); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("publish clarification message update: %w", err)
		}
	}
	return resultErr
}

func firstRestoredClarification(messages []*models.Message) *models.Message {
	for _, message := range messages {
		if message == nil || message.Type != models.MessageTypeClarificationRequest {
			continue
		}
		if status, _ := message.Metadata["status"].(string); status == clarificationPendingStatus {
			return message
		}
	}
	return nil
}
