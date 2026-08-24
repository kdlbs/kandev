package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	inboxDefaultLimit = 100
	inboxMaxLimit     = 500
)

type taskInboxRequest struct {
	TaskID           string `json:"task_id"`
	CallerTaskID     string `json:"caller_task_id"`
	CurrentSessionID string `json:"current_session_id"`
	Cursor           string `json:"cursor"`
	Limit            int    `json:"limit"`
}

type taskInboxCursor struct {
	Timestamp time.Time `json:"t"`
	ID        string    `json:"id"`
}

// inboxItem is deliberately a safe projection. In particular it does not
// return message metadata or attachment bytes, both of which can contain
// internal context that a polling coordinator does not need.
type inboxItem struct {
	ID           string            `json:"id"`
	TransitionID string            `json:"transition_id,omitempty"`
	State        string            `json:"state"`
	SessionID    string            `json:"session_id"`
	SessionName  string            `json:"session_name,omitempty"`
	IsPrimary    bool              `json:"is_primary"`
	IsCurrent    bool              `json:"is_current"`
	Sender       string            `json:"sender"`
	Content      string            `json:"content"`
	Attachments  []inboxAttachment `json:"attachments,omitempty"`
	Timestamp    time.Time         `json:"timestamp"`
}

type inboxAttachment struct {
	AttachmentID string `json:"attachment_id,omitempty"`
	Type         string `json:"type"`
	MimeType     string `json:"mime_type,omitempty"`
	Name         string `json:"name,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
}

const inboxTransitionIDKey = messagequeue.MetadataInboxTransitionID

func (h *Handlers) handleListTaskInbox(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, cursor, errResp := h.parseTaskInboxRequest(ctx, msg)
	if errResp != nil {
		return errResp, nil
	}
	sessions, err := h.taskSvc.ListTaskSessions(ctx, req.TaskID)
	if err != nil {
		h.logger.Error("failed to list inbox sessions", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list task inbox", nil)
	}
	limit := inboxLimit(req.Limit)
	items, perSession, messagesHasMore, err := h.buildTaskInboxItems(ctx, req, cursor, sessions, limit)
	if err != nil {
		h.logger.Error("failed to build task inbox", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list task inbox", nil)
	}
	page, hasMore, next := paginateTaskInbox(items, cursor, limit, messagesHasMore)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"task_id": req.TaskID, "items": page, "total": inboxCountTotal(perSession), "returned": len(page),
		"has_more": hasMore, "cursor": next, "per_session_counts": perSession,
	})
}

func (h *Handlers) buildTaskInboxItems(
	ctx context.Context,
	req *taskInboxRequest,
	cursor *taskInboxCursor,
	sessions []*models.TaskSession,
	limit int,
) ([]inboxItem, map[string]int, bool, error) {
	messageOptions := models.TaskInboxMessagesOptions{Limit: limit}
	if cursor != nil {
		messageOptions.AfterCreatedAt = cursor.Timestamp
		messageOptions.AfterID = cursor.ID
	}
	messages, messagesHasMore, messageCounts, err := h.taskSvc.ListTaskInboxMessages(ctx, req.TaskID, messageOptions)
	if err != nil {
		return nil, nil, false, err
	}
	sessionsByID := make(map[string]*models.TaskSession, len(sessions))
	for _, session := range sessions {
		sessionsByID[session.ID] = session
	}
	items := make([]inboxItem, 0)
	perSession := make(map[string]int, len(messageCounts)+len(sessions))
	for sessionID, count := range messageCounts {
		perSession[sessionID] = count
	}
	for _, message := range messages {
		session := sessionsByID[message.TaskSessionID]
		if session == nil {
			continue
		}
		items = append(items, inboxItem{
			ID:           message.ID,
			TransitionID: inboxTransitionID(message.Metadata, inboxMetadataString(message.Metadata, "queue_entry_id")),
			State:        "delivered",
			SessionID:    session.ID,
			SessionName:  session.Name,
			IsPrimary:    session.IsPrimary,
			IsCurrent:    session.ID == req.CurrentSessionID,
			Sender:       string(message.AuthorType),
			Content:      sysprompt.StripSystemContent(message.Content),
			Attachments:  safeInboxMessageAttachments(message.Metadata),
			Timestamp:    message.CreatedAt,
		})
	}
	for _, session := range sessions {
		items, err = h.appendSessionQueueInbox(ctx, items, perSession, session, req.CurrentSessionID)
		if err != nil {
			return nil, nil, false, err
		}
	}
	return items, perSession, messagesHasMore, nil
}

func paginateTaskInbox(items []inboxItem, cursor *taskInboxCursor, limit int, messagesHasMore bool) ([]inboxItem, bool, string) {
	sort.Slice(items, func(i, j int) bool { return inboxBefore(items[i], items[j]) })
	start := 0
	if cursor != nil {
		for start < len(items) && !inboxAfter(items[start], *cursor) {
			start++
		}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]
	hasMore := messagesHasMore || end < len(items)
	next := ""
	if hasMore && len(page) > 0 {
		next = encodeInboxCursor(page[len(page)-1])
	}
	return page, hasMore, next
}

func (h *Handlers) parseTaskInboxRequest(ctx context.Context, msg *ws.Message) (*taskInboxRequest, *taskInboxCursor, *ws.Message) {
	var req taskInboxRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return nil, nil, wsError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error())
	}
	if req.TaskID == "" || req.CallerTaskID == "" || req.TaskID != req.CallerTaskID {
		return nil, nil, wsError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found")
	}
	if req.Limit < 0 {
		return nil, nil, wsError(msg.ID, msg.Action, ws.ErrorCodeValidation, "limit must be non-negative")
	}
	cursor, err := decodeInboxCursor(req.Cursor)
	if err != nil {
		return nil, nil, wsError(msg.ID, msg.Action, ws.ErrorCodeValidation, "invalid cursor")
	}
	if _, err = h.taskSvc.GetTask(ctx, req.TaskID); err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) || errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			return nil, nil, wsError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found")
		}
		h.logger.Error("failed to look up inbox task", zap.Error(err))
		return nil, nil, wsError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list task inbox")
	}
	return &req, cursor, nil
}

func (h *Handlers) appendSessionQueueInbox(ctx context.Context, items []inboxItem, counts map[string]int, session *models.TaskSession, currentID string) ([]inboxItem, error) {
	if h.sessionLauncher == nil || h.sessionLauncher.GetMessageQueue() == nil {
		return items, nil
	}
	for _, entry := range h.sessionLauncher.GetMessageQueue().GetStatus(ctx, session.ID).Entries {
		if entry.IsReservedInFlight() {
			continue
		}
		items = append(items, inboxItem{
			ID:           entry.ID,
			TransitionID: inboxTransitionID(entry.Metadata, entry.ID),
			State:        "queued",
			SessionID:    session.ID,
			SessionName:  session.Name,
			IsPrimary:    session.IsPrimary,
			IsCurrent:    session.ID == currentID,
			Sender:       entry.QueuedBy,
			Content:      entry.Content,
			Attachments:  safeInboxAttachments(entry.Attachments),
			Timestamp:    entry.QueuedAt,
		})
		counts[session.ID]++
	}
	return items, nil
}

func inboxCountTotal(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

func inboxMetadataString(metadata map[string]interface{}, key string) string {
	value, _ := metadata[key].(string)
	return value
}
func inboxTransitionID(metadata map[string]interface{}, fallback string) string {
	if id := inboxMetadataString(metadata, inboxTransitionIDKey); id != "" {
		return id
	}
	return fallback
}
func inboxLimit(requested int) int {
	if requested <= 0 {
		return inboxDefaultLimit
	}
	if requested > inboxMaxLimit {
		return inboxMaxLimit
	}
	return requested
}
func safeInboxAttachments(entries []messagequeue.MessageAttachment) []inboxAttachment {
	out := make([]inboxAttachment, 0, len(entries))
	for _, entry := range entries {
		out = append(out, inboxAttachment{AttachmentID: entry.AttachmentID, Type: entry.Type, MimeType: entry.MimeType, Name: entry.Name, SizeBytes: entry.SizeBytes})
	}
	return out
}

func safeInboxMessageAttachments(metadata map[string]interface{}) []inboxAttachment {
	attachments, ok := metadata["attachments"]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(attachments)
	if err != nil {
		return nil
	}
	var queued []messagequeue.MessageAttachment
	if err := json.Unmarshal(encoded, &queued); err != nil {
		return nil
	}
	return safeInboxAttachments(queued)
}
func inboxBefore(a, b inboxItem) bool {
	aTimestamp := inboxOrderTime(a.Timestamp)
	bTimestamp := inboxOrderTime(b.Timestamp)
	if !aTimestamp.Equal(bTimestamp) {
		return aTimestamp.Before(bTimestamp)
	}
	return a.ID < b.ID
}
func inboxAfter(item inboxItem, cursor taskInboxCursor) bool {
	itemTimestamp := inboxOrderTime(item.Timestamp)
	cursorTimestamp := inboxOrderTime(cursor.Timestamp)
	return itemTimestamp.After(cursorTimestamp) || (itemTimestamp.Equal(cursorTimestamp) && item.ID > cursor.ID)
}
func encodeInboxCursor(item inboxItem) string {
	raw, _ := json.Marshal(taskInboxCursor{Timestamp: inboxOrderTime(item.Timestamp), ID: item.ID})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func inboxOrderTime(timestamp time.Time) time.Time {
	return timestamp.UTC().Truncate(time.Microsecond)
}
func decodeInboxCursor(encoded string) (*taskInboxCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var cursor taskInboxCursor
	if err = json.Unmarshal(raw, &cursor); err != nil || cursor.ID == "" || cursor.Timestamp.IsZero() {
		return nil, errors.New("invalid inbox cursor")
	}
	return &cursor, nil
}
