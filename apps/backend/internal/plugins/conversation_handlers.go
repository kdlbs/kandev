package plugins

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/sysprompt"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const conversationNoStore = "no-store"

// ConversationReader is the narrow task-service surface needed by browser
// plugin conversation reads. The task service applies workspace authorization.
type ConversationReader interface {
	GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
	ListMessagesPaginated(
		context.Context,
		taskservice.ListMessagesRequest,
	) ([]*taskmodels.Message, bool, error)
	ListTurnsBySession(context.Context, string) ([]*taskmodels.Turn, error)
}

type conversationError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type conversationErrorEnvelope struct {
	Error conversationError `json:"error"`
}

type conversationBindingResponse struct {
	BindingToken string    `json:"bindingToken"`
	Generation   int64     `json:"generation"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type conversationMessageDTO struct {
	ID           string  `json:"id"`
	TaskID       *string `json:"taskId"`
	SessionID    string  `json:"sessionId"`
	TurnID       *string `json:"turnId,omitempty"`
	AuthorType   string  `json:"authorType"`
	Type         string  `json:"type"`
	Content      string  `json:"content"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
	PromptIndex  *int    `json:"promptIndex,omitempty"`
	SenderTaskID *string `json:"senderTaskId,omitempty"`
}

func loadFallbackTurns(ctx context.Context, reader ConversationReader, sessionID, taskID string) ([]*taskmodels.Turn, error) {
	turns, err := reader.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return filterTurnsByTask(turns, taskID), nil
}

func filterTurnsByTask(turns []*taskmodels.Turn, taskID string) []*taskmodels.Turn {
	if taskID == "" {
		return turns
	}
	filtered := make([]*taskmodels.Turn, 0, len(turns))
	for _, turn := range turns {
		if turn.TaskID == taskID {
			filtered = append(filtered, turn)
		}
	}
	return filtered
}

type conversationMessagesResponse struct {
	Messages []conversationMessageDTO `json:"messages"`
	HasMore  bool                     `json:"hasMore"`
	Cursor   *string                  `json:"cursor"`
}

type conversationTurnDTO struct {
	ID          string  `json:"id"`
	TaskID      *string `json:"taskId"`
	SessionID   string  `json:"sessionId"`
	StartedAt   string  `json:"startedAt"`
	CompletedAt *string `json:"completedAt,omitempty"`
	UpdatedAt   string  `json:"updatedAt"`
}

type conversationTurnsResponse struct {
	Turns []conversationTurnDTO `json:"turns"`
}

type conversationContinuationRenewRequest struct {
	Cursor        string `json:"cursor"`
	SnapshotToken string `json:"snapshot_token"`
}

type conversationContinuationRenewResponse struct {
	Cursor        string    `json:"cursor"`
	SnapshotToken string    `json:"snapshot_token"`
	Cutoff        int64     `json:"cutoff"`
	Fingerprint   string    `json:"fingerprint"`
	ExpiresAt     time.Time `json:"expires_at"`
}
type conversationMessageQuery struct {
	limit      int
	sort       string
	authorType string
	authors    []string
	taskID     *string
	cursor     string
}

func registerConversationRoutes(api *gin.RouterGroup, ctrl *Controller) {
	api.GET("/:id/conversation/binding", ctrl.conversationBinding)
	api.GET("/:id/conversation/task-sessions/:sessionId/messages", ctrl.conversationMessages)
	api.GET("/:id/conversation/task-sessions/:sessionId/turns", ctrl.conversationTurns)
	api.POST("/:id/conversation/continuation/renew", ctrl.conversationContinuationRenew)
}

func (c *Controller) conversationBinding(ctx *gin.Context) {
	ctx.Header("Cache-Control", conversationNoStore)
	identity, ok := authn.IdentityFromContext(ctx.Request.Context())
	if !ok {
		writeConversationError(ctx, http.StatusUnauthorized, "unauthenticated", "authentication required", false)
		return
	}
	record, ok := c.conversationRecord(ctx.Param("id"))
	if !ok {
		writeConversationError(ctx, http.StatusNotFound, "not_found", "plugin not found", false)
		return
	}
	generation := conversationGeneration(record.InstalledAt)
	token, expiresAt, err := c.conversationTokens.mintBinding(record.ID, identity.UserID, generation)
	if err != nil {
		c.log.Error("mint plugin conversation binding", zap.Error(err))
		writeConversationError(ctx, http.StatusInternalServerError, "upstream_failure", "conversation service unavailable", true)
		return
	}
	ctx.JSON(http.StatusOK, conversationBindingResponse{
		BindingToken: token,
		Generation:   generation,
		ExpiresAt:    expiresAt,
	})
}

func (c *Controller) conversationRecord(pluginID string) (*store.Record, bool) {
	record, err := c.svc.Get(pluginID)
	if err != nil || record.Status != StatusActive || !record.Capabilities.CanRead("messages") {
		return nil, false
	}
	return record, true
}

//nolint:cyclop,goconst // This handler owns authorization, snapshot validation, pagination, and cursor minting.
func (c *Controller) conversationMessages(ctx *gin.Context) {
	ctx.Header("Cache-Control", conversationNoStore)
	record, identity, ok := c.authorizeConversationRequest(ctx)
	if !ok || !c.validBinding(ctx, record, identity.UserID) {
		return
	}
	query, ok := parseConversationMessageQuery(ctx)
	if !ok {
		return
	}
	if c.conversationReader == nil {
		writeConversationError(ctx, http.StatusInternalServerError, "upstream_failure", "conversation service unavailable", true)
		return
	}
	sessionID := ctx.Param("sessionId")
	snapshot, ok := c.validSnapshot(ctx, record, identity.UserID, sessionID)
	if !ok {
		return
	}
	session, err := c.conversationReader.GetTaskSession(ctx.Request.Context(), sessionID)
	if err != nil || session == nil || session.ID != sessionID {
		if !c.svc.HasConversationJournal() {
			writeConversationError(ctx, http.StatusNotFound, "not_found", "task session not found", false)
			return
		}
	} else if query.taskID != nil && *query.taskID != session.TaskID {
		writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "task_id does not match the task session", false)
		return
	}
	request := taskservice.ListMessagesRequest{
		TaskSessionID: sessionID,
		Limit:         query.limit,
		Sort:          query.sort,
		AuthorType:    query.authorType,
		// Parity with the journal branch: multi-author filters and an
		// explicit task_id must narrow the fallback page the same way.
		AuthorTypes: query.authors,
		TaskID:      optionalStringValue(query.taskID),
	}
	if !c.applyConversationMessageCursor(ctx, record, identity.UserID, sessionID, query, snapshot, &request) {
		return
	}
	var messages []*taskmodels.Message
	var hasMore bool
	if c.svc.HasConversationJournal() {
		cursorID := request.After
		if query.sort == "desc" {
			cursorID = request.Before
		}
		messages, hasMore, err = c.svc.conversationMessagesAt(
			ctx.Request.Context(), sessionID, uint64(snapshot.Cutoff),
			query.taskID, query.authors, query.sort, cursorID, query.limit,
		)
	} else {
		messages, hasMore, err = c.conversationReader.ListMessagesPaginated(ctx.Request.Context(), request)
	}
	if err != nil {
		writeConversationError(ctx, http.StatusInternalServerError, "upstream_failure", "conversation service unavailable", true)
		return
	}
	response, ok := c.buildConversationMessagesResponse(
		ctx,
		record,
		identity.UserID,
		sessionID,
		query,
		snapshot,
		messages,
		hasMore,
	)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, response)
}
func (c *Controller) applyConversationMessageCursor(
	ctx *gin.Context,
	record *store.Record,
	userID string,
	sessionID string,
	query conversationMessageQuery,
	snapshot conversationTokenClaims,
	request *taskservice.ListMessagesRequest,
) bool {
	if query.cursor == "" {
		return true
	}
	claims, err := c.conversationTokens.parse(query.cursor)
	if err != nil || !validConversationMessageCursor(claims, record, userID, sessionID, query, snapshot) {
		writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "invalid cursor", false)
		return false
	}
	if query.sort == "asc" {
		request.After = claims.LastID
	} else {
		request.Before = claims.LastID
	}
	return true
}

func validConversationMessageCursor(
	claims conversationTokenClaims,
	record *store.Record,
	userID string,
	sessionID string,
	query conversationMessageQuery,
	snapshot conversationTokenClaims,
) bool {
	return claims.Kind == tokenKindCursor &&
		claims.PluginID == record.ID &&
		claims.UserID == userID &&
		claims.Generation == conversationGeneration(record.InstalledAt) &&
		claims.SessionID == sessionID &&
		equalOptionalString(claims.TaskID, query.taskID) &&
		claims.Sort == query.sort &&
		equalStrings(claims.Authors, query.authors) &&
		claims.LastID != "" &&
		claims.Cutoff == snapshot.Cutoff &&
		claims.Fingerprint == snapshot.Fingerprint
}

func (c *Controller) buildConversationMessagesResponse(
	ctx *gin.Context,
	record *store.Record,
	userID string,
	sessionID string,
	query conversationMessageQuery,
	snapshot conversationTokenClaims,
	messages []*taskmodels.Message,
	hasMore bool,
) (conversationMessagesResponse, bool) {
	response := conversationMessagesResponse{
		Messages: make([]conversationMessageDTO, 0, len(messages)),
		HasMore:  hasMore,
	}
	for _, message := range messages {
		response.Messages = append(response.Messages, conversationMessageModelToDTO(message))
	}
	if !hasMore {
		return response, true
	}
	if len(messages) == 0 {
		writeConversationError(ctx, http.StatusInternalServerError, "upstream_failure", "conversation service returned an invalid page", true)
		return conversationMessagesResponse{}, false
	}
	cursor, err := c.conversationTokens.mintCursor(
		record.ID,
		userID,
		conversationGeneration(record.InstalledAt),
		sessionID,
		query.taskID,
		query.sort,
		query.authors,
		messages[len(messages)-1].ID,
		snapshot.Cutoff,
		snapshot.Fingerprint,
	)
	if err != nil {
		writeConversationError(ctx, http.StatusInternalServerError, "upstream_failure", "conversation service unavailable", true)
		return conversationMessagesResponse{}, false
	}
	response.Cursor = &cursor
	return response, true
}

//nolint:cyclop // Turn lookup supports primary, task-scoped, and fallback sources.
func (c *Controller) conversationTurns(ctx *gin.Context) {
	ctx.Header("Cache-Control", conversationNoStore)
	record, identity, ok := c.authorizeConversationRequest(ctx)
	if !ok || !c.validBinding(ctx, record, identity.UserID) {
		return
	}
	if c.conversationReader == nil {
		writeConversationError(ctx, http.StatusInternalServerError, "upstream_failure", "conversation service unavailable", true)
		return
	}
	sessionID := ctx.Param("sessionId")
	snapshot, ok := c.validSnapshot(ctx, record, identity.UserID, sessionID)
	if !ok {
		return
	}
	var taskID *string
	if rawTaskID, exists := ctx.GetQuery("task_id"); exists && rawTaskID != nullJSONValue {
		if rawTaskID == "" {
			writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "task_id does not match the task session", false)
			return
		}
		taskID = &rawTaskID
	}
	session, err := c.conversationReader.GetTaskSession(ctx.Request.Context(), sessionID)
	sessionFound := err == nil && session != nil && session.ID == sessionID
	if !sessionFound {
		if !c.svc.HasConversationJournal() {
			writeConversationError(ctx, http.StatusNotFound, "not_found", "task session not found", false)
			return
		}
	} else if taskID != nil && *taskID != session.TaskID {
		writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "task_id does not match the task session", false)
		return
	}
	var scopedTaskID *string
	if taskID != nil {
		scopedTaskID = taskID
	} else if sessionFound {
		scopedTaskID = &session.TaskID
	}
	var turns []*taskmodels.Turn
	if c.svc.HasConversationJournal() {
		turns, err = c.svc.conversationTurnsAt(ctx.Request.Context(), sessionID, uint64(snapshot.Cutoff), scopedTaskID)
	} else {
		// The no-journal fallback must honor the requested/inherited task
		// scope like the journal branch: a session that ever accumulated
		// mixed-task turn rows must not leak turns outside the validated task.
		turns, err = loadFallbackTurns(ctx.Request.Context(), c.conversationReader, sessionID, *scopedTaskID)
	}
	if err != nil {
		writeConversationError(ctx, http.StatusInternalServerError, "upstream_failure", "conversation service unavailable", true)
		return
	}
	response := conversationTurnsResponse{Turns: make([]conversationTurnDTO, 0, len(turns))}
	for _, turn := range turns {
		response.Turns = append(response.Turns, conversationTurnModelToDTO(turn))
	}
	ctx.JSON(http.StatusOK, response)
}

func (c *Controller) conversationContinuationRenew(ctx *gin.Context) {
	ctx.Header("Cache-Control", conversationNoStore)
	record, identity, ok := c.authorizeConversationRequest(ctx)
	if !ok || !c.validBinding(ctx, record, identity.UserID) {
		return
	}
	var request conversationContinuationRenewRequest
	if err := ctx.ShouldBindJSON(&request); err != nil ||
		request.Cursor == "" ||
		request.SnapshotToken == "" {
		writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "cursor and snapshot_token are required", false)
		return
	}
	cursorClaims, cursorErr := c.conversationTokens.parse(request.Cursor)
	snapshotClaims, snapshotErr := c.conversationTokens.parse(request.SnapshotToken)
	if !validContinuationClaims(cursorClaims, snapshotClaims, cursorErr, snapshotErr) {
		writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "invalid continuation", false)
		return
	}
	if cursorClaims.PluginID != record.ID ||
		cursorClaims.UserID != identity.UserID ||
		cursorClaims.Generation != conversationGeneration(record.InstalledAt) {
		writeConversationError(ctx, http.StatusConflict, "upstream_failure", "conversation generation changed", true)
		return
	}
	cursor, expiresAt, err := c.conversationTokens.renew(cursorClaims)
	if err != nil {
		writeConversationError(ctx, http.StatusInternalServerError, "upstream_failure", "conversation service unavailable", true)
		return
	}
	snapshotToken, _, err := c.conversationTokens.renew(snapshotClaims)
	if err != nil {
		writeConversationError(ctx, http.StatusInternalServerError, "upstream_failure", "conversation service unavailable", true)
		return
	}
	ctx.JSON(http.StatusOK, conversationContinuationRenewResponse{
		Cursor:        cursor,
		SnapshotToken: snapshotToken,
		Cutoff:        cursorClaims.Cutoff,
		Fingerprint:   cursorClaims.Fingerprint,
		ExpiresAt:     expiresAt,
	})
}

func (c *Controller) authorizeConversationRequest(
	ctx *gin.Context,
) (*store.Record, authn.Identity, bool) {
	identity, ok := authn.IdentityFromContext(ctx.Request.Context())
	if !ok {
		writeConversationError(ctx, http.StatusUnauthorized, "unauthenticated", "authentication required", false)
		return nil, authn.Identity{}, false
	}
	record, ok := c.conversationRecord(ctx.Param("id"))
	if !ok {
		writeConversationError(ctx, http.StatusNotFound, "not_found", "plugin not found", false)
		return nil, authn.Identity{}, false
	}
	return record, identity, true
}

func (c *Controller) validBinding(ctx *gin.Context, record *store.Record, userID string) bool {
	claims, err := c.conversationTokens.parse(ctx.GetHeader("X-Kandev-Plugin-Binding"))
	if err != nil ||
		claims.Kind != tokenKindBinding ||
		claims.PluginID != record.ID ||
		claims.UserID != userID ||
		claims.Generation != conversationGeneration(record.InstalledAt) {
		writeConversationError(ctx, http.StatusUnauthorized, "unauthenticated", "invalid conversation binding", false)
		return false
	}
	return true
}

func (c *Controller) validSnapshot(
	ctx *gin.Context,
	record *store.Record,
	userID string,
	sessionID string,
) (conversationTokenClaims, bool) {
	claims, err := c.conversationTokens.parse(ctx.GetHeader("X-Kandev-Snapshot-Token"))
	if err != nil ||
		claims.Kind != tokenKindSnapshot ||
		claims.PluginID != record.ID ||
		claims.UserID != userID ||
		claims.Generation != conversationGeneration(record.InstalledAt) ||
		claims.SessionID != sessionID {
		writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "invalid snapshot token", false)
		return conversationTokenClaims{}, false
	}
	return claims, true
}

func parseConversationMessageQuery(ctx *gin.Context) (conversationMessageQuery, bool) {
	limit, ok := parseConversationMessageLimit(ctx)
	if !ok {
		return conversationMessageQuery{}, false
	}
	sortOrder, ok := parseConversationMessageSort(ctx)
	if !ok {
		return conversationMessageQuery{}, false
	}
	authors, authorType, ok := parseConversationMessageAuthors(ctx)
	if !ok {
		return conversationMessageQuery{}, false
	}
	taskID, ok := parseOptionalConversationQuery(ctx, "task_id")
	if !ok {
		return conversationMessageQuery{}, false
	}
	cursor, ok := parseOptionalConversationQuery(ctx, "cursor")
	if !ok {
		return conversationMessageQuery{}, false
	}
	if taskID != nil && *taskID == nullJSONValue {
		taskID = nil
	}
	return conversationMessageQuery{
		limit: limit, sort: sortOrder, authorType: authorType,
		authors: authors, taskID: taskID, cursor: optionalStringValue(cursor),
	}, true
}

func parseConversationMessageLimit(ctx *gin.Context) (int, bool) {
	raw, exists := ctx.GetQuery("limit")
	if !exists {
		return 20, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "limit must be between 1 and 100", false)
		return 0, false
	}
	return limit, true
}

func parseConversationMessageSort(ctx *gin.Context) (string, bool) {
	raw, exists := ctx.GetQuery("sort")
	if !exists {
		return "desc", true
	}
	if raw != "asc" && raw != "desc" {
		writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "sort must be asc or desc", false)
		return "", false
	}
	return raw, true
}

func parseConversationMessageAuthors(ctx *gin.Context) ([]string, string, bool) {
	authors := ctx.QueryArray("author_type")
	if len(authors) > 2 {
		writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "unsupported author_type", false)
		return nil, "", false
	}
	seen := make(map[string]struct{}, len(authors))
	for _, author := range authors {
		if author != string(taskmodels.MessageAuthorUser) && author != string(taskmodels.MessageAuthorAgent) {
			writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "unsupported author_type", false)
			return nil, "", false
		}
		if _, duplicate := seen[author]; duplicate {
			writeConversationError(ctx, http.StatusBadRequest, "invalid_query", "duplicate author_type", false)
			return nil, "", false
		}
		seen[author] = struct{}{}
	}
	if len(authors) == 1 {
		return authors, authors[0], true
	}
	return authors, "", true
}

func parseOptionalConversationQuery(ctx *gin.Context, name string) (*string, bool) {
	raw, exists := ctx.GetQuery(name)
	if !exists {
		return nil, true
	}
	if raw == "" {
		writeConversationError(ctx, http.StatusBadRequest, "invalid_query", name+" must not be empty", false)
		return nil, false
	}
	return &raw, true
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func conversationMessageModelToDTO(message *taskmodels.Message) conversationMessageDTO {
	messageType := string(message.Type)
	if messageType == "" {
		messageType = string(taskmodels.MessageTypeMessage)
	}
	updatedAt := message.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = message.CreatedAt
	}
	dto := conversationMessageDTO{
		ID:         message.ID,
		TaskID:     nonEmptyStringPointer(message.TaskID),
		SessionID:  message.TaskSessionID,
		TurnID:     nonEmptyStringPointer(message.TurnID),
		AuthorType: string(message.AuthorType),
		Type:       messageType,
		Content:    sysprompt.StripSystemContent(message.Content),
		CreatedAt:  message.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:  updatedAt.UTC().Format(time.RFC3339Nano),
	}
	if message.PromptIndex > 0 {
		dto.PromptIndex = &message.PromptIndex
	}
	if senderTaskID, ok := message.Metadata["sender_task_id"].(string); ok && senderTaskID != "" {
		dto.SenderTaskID = &senderTaskID
	}
	return dto
}

func conversationTurnModelToDTO(turn *taskmodels.Turn) conversationTurnDTO {
	updatedAt := turn.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = turn.StartedAt
	}
	dto := conversationTurnDTO{
		ID:        turn.ID,
		TaskID:    nonEmptyStringPointer(turn.TaskID),
		SessionID: turn.TaskSessionID,
		StartedAt: turn.StartedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339Nano),
	}
	if turn.CompletedAt != nil {
		completedAt := turn.CompletedAt.UTC().Format(time.RFC3339Nano)
		dto.CompletedAt = &completedAt
	}
	return dto
}

func validContinuationClaims(
	cursor conversationTokenClaims,
	snapshot conversationTokenClaims,
	cursorErr error,
	snapshotErr error,
) bool {
	return cursorErr == nil &&
		snapshotErr == nil &&
		cursor.Kind == tokenKindCursor &&
		snapshot.Kind == tokenKindSnapshot &&
		matchingContinuationClaims(cursor, snapshot)
}

func matchingContinuationClaims(
	cursor conversationTokenClaims,
	snapshot conversationTokenClaims,
) bool {
	return cursor.PluginID == snapshot.PluginID &&
		cursor.UserID == snapshot.UserID &&
		cursor.Generation == snapshot.Generation &&
		cursor.SessionID == snapshot.SessionID &&
		cursor.Cutoff == snapshot.Cutoff &&
		cursor.Fingerprint != "" &&
		cursor.Fingerprint == snapshot.Fingerprint
}

func nonEmptyStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func equalOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func writeConversationError(ctx *gin.Context, status int, code, message string, retryable bool) {
	ctx.JSON(status, conversationErrorEnvelope{Error: conversationError{
		Code: code, Message: message, Retryable: retryable,
	}})
}
