package clarification

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// inboxWorkspaceAuthorizer is the workspace-scope check the Needs-you Inbox
// read endpoints need (needs-you-inbox design, "Security"): the same rule
// AuthorizeWorkspaceScope already applies everywhere else, reached through a
// narrow interface so this package does not depend on service.Service's full
// surface.
type inboxWorkspaceAuthorizer interface {
	AuthorizeWorkspaceScope(ctx context.Context, workspaceID string, scope authz.Scope) error
}

// inboxTaskLookup resolves the task_title/session_state enrichment fields
// (F41): both degrade to "" on lookup failure rather than failing the page.
type inboxTaskLookup interface {
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
	GetTaskSession(ctx context.Context, id string) (*taskmodels.TaskSession, error)
}

// inboxTaskService is the combined dependency the Needs-you Inbox handlers
// need from the task service.
type inboxTaskService interface {
	inboxWorkspaceAuthorizer
	inboxTaskLookup
}

// inboxBundleStore is the bounded read/write surface the Needs-you Inbox
// endpoints need: the existing bundle-query machinery plus the new per-user
// dismiss/snooze sidecar (needs-you-inbox design, "Persistence").
type inboxBundleStore interface {
	ListUnresolvedClarificationBundles(ctx context.Context, opts taskmodels.ListClarificationBundlesOptions) (*taskmodels.ClarificationBundlePage, error)
	FindMessagesByPendingID(ctx context.Context, pendingID string) ([]*taskmodels.Message, error)
	UpsertClarificationInboxSidecar(ctx context.Context, userID, pendingID string, state taskmodels.ClarificationSidecarState, snoozeUntil *time.Time, now time.Time) error
	DeleteClarificationInboxSidecar(ctx context.Context, userID, pendingID string) error
	CountHiddenClarificationBundles(ctx context.Context, opts taskmodels.ListClarificationBundlesOptions) (taskmodels.ClarificationInboxHiddenSummary, error)
	GetClarificationInboxSidecarStates(ctx context.Context, userID string, pendingIDs []string) (map[string]taskmodels.ClarificationInboxHiddenBundle, error)
}

const (
	defaultInboxLimit = 50
	maxInboxLimit     = 200

	inboxStateDismissed = string(taskmodels.ClarificationSidecarDismissed)
	inboxStateSnoozed   = string(taskmodels.ClarificationSidecarSnoozed)

	errInboxInternal = "failed to read needs-you inbox"
)

// inboxSnoozeDurations is the closed set of accepted snooze durations: any
// other value, including a well-formed "2h", is 400
// (design-01#Sidecar-write-and-restore-contracts).
var inboxSnoozeDurations = map[string]time.Duration{
	"1h":  time.Hour,
	"4h":  4 * time.Hour,
	"24h": 24 * time.Hour,
}

// inboxBundleView is one bundle row shared by the main list and the hidden
// enumeration (needs-you-inbox design, "Data and contracts").
type inboxBundleView struct {
	PendingID    string        `json:"pending_id"`
	TaskID       string        `json:"task_id"`
	SessionID    string        `json:"session_id"`
	SessionState string        `json:"session_state"`
	TaskTitle    string        `json:"task_title"`
	CreatedAt    string        `json:"created_at"`
	Context      string        `json:"context"`
	Messages     []*v1.Message `json:"messages"`
}

// inboxListResponse is GET /api/v1/clarification-inbox's response envelope.
type inboxListResponse struct {
	Bundles          []inboxBundleView `json:"bundles"`
	Count            int               `json:"count"`
	HiddenCount      int               `json:"hidden_count"`
	NextSnoozeExpiry *string           `json:"next_snooze_expiry"`
	NextCursor       string            `json:"next_cursor,omitempty"`
}

// inboxHiddenBundleView is one row of the hidden-bundles enumeration: the
// same shape as inboxBundleView plus why it is hidden.
type inboxHiddenBundleView struct {
	inboxBundleView
	State       string  `json:"state"`
	SnoozeUntil *string `json:"snooze_until"`
}

// inboxHiddenListResponse is GET /api/v1/clarification-inbox/hidden's
// response envelope. total is workspace-wide; count is page-scoped.
type inboxHiddenListResponse struct {
	Bundles    []inboxHiddenBundleView `json:"bundles"`
	Count      int                     `json:"count"`
	Total      int                     `json:"total"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

// inboxSidecarPutBody is PUT /api/v1/clarification-inbox/sidecar/:pendingID's
// request body.
type inboxSidecarPutBody struct {
	State          string `json:"state"`
	SnoozeDuration string `json:"snooze_duration"`
}

// respondInboxError writes a {"error": message} JSON body. Routed through one
// helper rather than repeating the gin.H{"error": ...} literal at every call
// site. The "error" key itself already recurs across this package's other,
// pre-existing handlers (handlers.go), so goconst's package-wide count stays
// over threshold regardless; consolidating to this one call site is the
// improvement available without touching that unrelated code.
//
//nolint:goconst // see comment above
func respondInboxError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

// httpListInbox backs GET /api/v1/clarification-inbox.
func (h *Handlers) httpListInbox(c *gin.Context) {
	ctx := c.Request.Context()
	workspaceID, limit, cursorCreatedAt, cursorPendingID, ok := h.parseInboxListQuery(c)
	if !ok {
		return
	}
	if err := h.inboxTasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		h.respondInboxWorkspaceAuthzError(c, err)
		return
	}

	userID := InboxUserID(ctx)
	now := h.now()
	page, err := h.inboxBundles.ListUnresolvedClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped:        true,
		WorkspaceID:     workspaceID,
		CursorCreatedAt: cursorCreatedAt,
		CursorPendingID: cursorPendingID,
		Limit:           limit,
		Sidecar:         &taskmodels.ClarificationSidecarFilter{UserID: userID, Now: now},
	})
	if err != nil {
		h.logger.Error("failed to list needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}
	views, err := h.buildInboxBundleViews(ctx, page.Bundles)
	if err != nil {
		h.logger.Error("failed to hydrate needs-you inbox bundle messages", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}

	// F45: the hidden-count query failing fails the whole read rather than
	// defaulting hidden_count/next_snooze_expiry, which would misreport both
	// the empty state and the snooze timer.
	summary, err := h.inboxBundles.CountHiddenClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: workspaceID, Limit: 1,
		Sidecar: &taskmodels.ClarificationSidecarFilter{UserID: userID, Only: true, Now: now},
	})
	if err != nil {
		h.logger.Error("failed to count hidden needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}

	resp := inboxListResponse{
		Bundles:          views,
		Count:            len(views),
		HiddenCount:      summary.HiddenCount,
		NextSnoozeExpiry: formatOptionalInboxTime(summary.NextSnoozeExpiry),
	}
	if page.HasMore && len(page.Bundles) > 0 {
		last := page.Bundles[len(page.Bundles)-1]
		resp.NextCursor = encodeInboxCursor(last.CreatedAt, last.PendingID)
	}
	c.JSON(http.StatusOK, resp)
}

// httpListInboxHidden backs GET /api/v1/clarification-inbox/hidden.
func (h *Handlers) httpListInboxHidden(c *gin.Context) {
	ctx := c.Request.Context()
	workspaceID, limit, cursorCreatedAt, cursorPendingID, ok := h.parseInboxListQuery(c)
	if !ok {
		return
	}
	if err := h.inboxTasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		h.respondInboxWorkspaceAuthzError(c, err)
		return
	}

	userID := InboxUserID(ctx)
	now := h.now()
	sidecar := &taskmodels.ClarificationSidecarFilter{UserID: userID, Only: true, Now: now}
	page, err := h.inboxBundles.ListUnresolvedClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: workspaceID,
		CursorCreatedAt: cursorCreatedAt, CursorPendingID: cursorPendingID, Limit: limit,
		Sidecar: sidecar,
	})
	if err != nil {
		h.logger.Error("failed to list hidden needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}
	resp, err := h.buildInboxHiddenResponse(ctx, userID, page)
	if err != nil {
		h.logger.Error("failed to hydrate hidden needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}

	summary, err := h.inboxBundles.CountHiddenClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: workspaceID, Limit: 1, Sidecar: sidecar,
	})
	if err != nil {
		h.logger.Error("failed to count hidden needs-you inbox bundles", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
		return
	}
	resp.Total = summary.HiddenCount
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) buildInboxHiddenResponse(
	ctx context.Context, userID string, page *taskmodels.ClarificationBundlePage,
) (inboxHiddenListResponse, error) {
	views, err := h.buildInboxBundleViews(ctx, page.Bundles)
	if err != nil {
		return inboxHiddenListResponse{}, err
	}
	pendingIDs := make([]string, 0, len(page.Bundles))
	for _, b := range page.Bundles {
		pendingIDs = append(pendingIDs, b.PendingID)
	}
	states, err := h.inboxBundles.GetClarificationInboxSidecarStates(ctx, userID, pendingIDs)
	if err != nil {
		return inboxHiddenListResponse{}, err
	}

	resp := inboxHiddenListResponse{Bundles: make([]inboxHiddenBundleView, 0, len(views))}
	for _, view := range views {
		entry := inboxHiddenBundleView{inboxBundleView: view}
		if state, ok := states[view.PendingID]; ok {
			entry.State = string(state.State)
			entry.SnoozeUntil = formatOptionalInboxTime(state.SnoozeUntil)
		}
		resp.Bundles = append(resp.Bundles, entry)
	}
	resp.Count = len(resp.Bundles)
	if page.HasMore && len(page.Bundles) > 0 {
		last := page.Bundles[len(page.Bundles)-1]
		resp.NextCursor = encodeInboxCursor(last.CreatedAt, last.PendingID)
	}
	return resp, nil
}

// buildInboxBundleViews hydrates each bundle's durable messages (F43, F44)
// and task_title/session_state enrichment (F41). A bundle whose
// messages cannot be read fails the whole page rather than omitting a row
// silently; a bundle with zero resolvable messages (should not happen given
// the bundle query's own filters) is skipped and logged.
func (h *Handlers) buildInboxBundleViews(
	ctx context.Context, bundles []taskmodels.ClarificationBundleSummary,
) ([]inboxBundleView, error) {
	views := make([]inboxBundleView, 0, len(bundles))
	for _, b := range bundles {
		msgs, err := h.inboxBundles.FindMessagesByPendingID(ctx, b.PendingID)
		if err != nil {
			return nil, err
		}
		if len(msgs) == 0 {
			h.logger.Warn("needs-you inbox bundle has no resolvable messages; omitting from page",
				zap.String("pending_id", b.PendingID))
			continue
		}
		ordered := orderBundleMessages(msgs)
		views = append(views, inboxBundleView{
			PendingID:    b.PendingID,
			TaskID:       b.TaskID,
			SessionID:    b.SessionID,
			CreatedAt:    b.CreatedAt.UTC().Format(time.RFC3339),
			Context:      inboxBundleContext(ordered),
			Messages:     renderInboxMessages(ordered),
			TaskTitle:    h.resolveTaskTitle(ctx, b.TaskID),
			SessionState: h.resolveSessionState(ctx, b.SessionID),
		})
	}
	return views, nil
}

// renderInboxMessages projects each message through Message.ToAPI() and
// rewrites its emitted metadata.question_index to its 0-based rank within
// this order (F44, design-01's "rewrite" rule): the shared client sort is
// then a no-op against normalized ranks. A fresh metadata map is
// built rather than written through the map ToAPI() returns, because that
// map may be the SAME reference as the source message's own metadata.
func renderInboxMessages(ordered []*taskmodels.Message) []*v1.Message {
	out := make([]*v1.Message, 0, len(ordered))
	for i, m := range ordered {
		api := m.ToAPI()
		meta := make(map[string]interface{}, len(api.Metadata)+1)
		for k, v := range api.Metadata {
			meta[k] = v
		}
		meta["question_index"] = i
		api.Metadata = meta
		out = append(out, api)
	}
	return out
}

// inboxBundleContext implements F40+F43: the bundle's context is its FIRST
// message's metadata.context when that is a non-empty string after
// trimming, else "". This mirrors readSharedContext's
// `context?.trim() ? context : null` exactly (clarification-input-overlay.tsx),
// so the row and the panel it expands into can never disagree.
func inboxBundleContext(ordered []*taskmodels.Message) string {
	if len(ordered) == 0 {
		return ""
	}
	v, ok := ordered[0].Metadata["context"].(string)
	if !ok || strings.TrimSpace(v) == "" {
		return ""
	}
	return v
}

// resolveTaskTitle implements F41: degrades to "" on any lookup failure.
func (h *Handlers) resolveTaskTitle(ctx context.Context, taskID string) string {
	if taskID == "" {
		return ""
	}
	task, err := h.inboxTasks.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return ""
	}
	return task.Title
}

// resolveSessionState implements F41: degrades to "" on any lookup failure.
func (h *Handlers) resolveSessionState(ctx context.Context, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	session, err := h.inboxTasks.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return ""
	}
	return string(session.State)
}

// parseInboxListQuery implements the query-validation table shared by the
// main read and the hidden enumeration.
func (h *Handlers) parseInboxListQuery(c *gin.Context) (
	workspaceID string, limit int, cursorCreatedAt time.Time, cursorPendingID string, ok bool,
) {
	workspaceID = strings.TrimSpace(c.Query("workspace_id"))
	if workspaceID == "" {
		respondInboxError(c, http.StatusBadRequest, "workspace_id is required")
		return "", 0, time.Time{}, "", false
	}
	var err error
	limit, err = parseInboxLimit(c.Query("limit"))
	if err != nil {
		respondInboxError(c, http.StatusBadRequest, err.Error())
		return "", 0, time.Time{}, "", false
	}
	if raw := c.Query("cursor"); raw != "" {
		cursorCreatedAt, cursorPendingID, err = decodeInboxCursor(raw)
		if err != nil {
			respondInboxError(c, http.StatusBadRequest, "cursor: "+err.Error())
			return "", 0, time.Time{}, "", false
		}
	}
	return workspaceID, limit, cursorCreatedAt, cursorPendingID, true
}

// parseInboxLimit implements the limit validation rules: absent defaults to
// 50, non-numeric or <= 0 is rejected (not clamped), and > 200 clamps to 200.
func parseInboxLimit(raw string) (int, error) {
	if raw == "" {
		return defaultInboxLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, errors.New("limit must be a positive integer")
	}
	if n > maxInboxLimit {
		return maxInboxLimit, nil
	}
	return n, nil
}

// respondInboxWorkspaceAuthzError maps AuthorizeWorkspaceScope's errors per
// the read's Security rule: an unreachable workspace is 404, and 403 never
// arises on this endpoint (needs-you-inbox design, "Security").
func (h *Handlers) respondInboxWorkspaceAuthzError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		respondInboxError(c, http.StatusNotFound, "workspace not found")
	case errors.Is(err, service.ErrForbidden):
		respondInboxError(c, http.StatusForbidden, "insufficient permissions")
	default:
		h.logger.Error("failed to authorize needs-you inbox workspace access", zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
	}
}

// httpUpsertInboxSidecar backs PUT /api/v1/clarification-inbox/sidecar/:pendingID.
func (h *Handlers) httpUpsertInboxSidecar(c *gin.Context) {
	pendingID := c.Param("pendingID")
	var body inboxSidecarPutBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respondInboxError(c, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}

	var state taskmodels.ClarificationSidecarState
	switch body.State {
	case inboxStateDismissed:
		state = taskmodels.ClarificationSidecarDismissed
	case inboxStateSnoozed:
		state = taskmodels.ClarificationSidecarSnoozed
	default:
		respondInboxError(c, http.StatusBadRequest, `state must be "dismissed" or "snoozed"`)
		return
	}

	now := h.now()
	var snoozeUntil *time.Time
	if state == taskmodels.ClarificationSidecarSnoozed {
		duration := body.SnoozeDuration
		if duration == "" {
			duration = "4h"
		}
		d, ok := inboxSnoozeDurations[duration]
		if !ok {
			respondInboxError(c, http.StatusBadRequest, `snooze_duration must be one of "1h", "4h", "24h"`)
			return
		}
		until := now.Add(d)
		snoozeUntil = &until
	}

	if !h.authorizeBundleAccessOrRespond(c, pendingID) {
		return
	}
	if err := h.inboxBundles.UpsertClarificationInboxSidecar(c.Request.Context(), InboxUserID(c.Request.Context()), pendingID, state, snoozeUntil, now); err != nil {
		h.logger.Error("failed to upsert needs-you inbox sidecar",
			zap.String("pending_id", pendingID), zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, "failed to update needs-you inbox")
		return
	}
	c.Status(http.StatusNoContent)
}

// httpDeleteInboxSidecar backs DELETE /api/v1/clarification-inbox/sidecar/:pendingID:
// restore is deleting the sidecar row, idempotent by construction.
func (h *Handlers) httpDeleteInboxSidecar(c *gin.Context) {
	pendingID := c.Param("pendingID")
	if !h.authorizeBundleAccessOrRespond(c, pendingID) {
		return
	}
	if err := h.inboxBundles.DeleteClarificationInboxSidecar(c.Request.Context(), InboxUserID(c.Request.Context()), pendingID); err != nil {
		h.logger.Error("failed to delete needs-you inbox sidecar",
			zap.String("pending_id", pendingID), zap.Error(err))
		respondInboxError(c, http.StatusInternalServerError, "failed to update needs-you inbox")
		return
	}
	c.Status(http.StatusNoContent)
}

// encodeInboxCursor/decodeInboxCursor implement the opaque (created_at,
// pending_id) cursor, mirroring the MCP list_pending_questions_kandev cursor
// codec (internal/mcp/handlers/question_handlers.go) so the two independent
// callers of the same bundle query encode pages identically.
func encodeInboxCursor(createdAt time.Time, pendingID string) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + pendingID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeInboxCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", errors.New("malformed cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return createdAt, parts[1], nil
}

// formatOptionalInboxTime renders a *time.Time as an RFC3339 string pointer,
// or nil (JSON null) when absent -- next_snooze_expiry and snooze_until are
// never omitted, always either a string or null.
func formatOptionalInboxTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// InboxUserID resolves the calling operator's identity for sidecar scoping.
// A real identity uses its own user id; an unscoped caller (auth disabled, or
// no identity at all) uses the pre-auth default user, keeping single-user
// behavior byte-identical -- the same convention internal/notifications uses
// for pre-auth ownership. Exported so the boot-state builder can resolve the
// same identity for the boot-hydration producer (InboxBootSummary).
func InboxUserID(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.UserID == "" {
		return userstore.DefaultUserID
	}
	return identity.UserID
}

// InboxBootSummary computes the Needs-you Inbox boot-hydration producer: the
// same bounded count, truncation flag, and next snooze expiry the list
// endpoint returns, at the same default limit and workspace/sidecar scope,
// for a workspace the boot builder has already resolved as the caller's own.
func InboxBootSummary(
	ctx context.Context,
	bundles inboxBundleStore,
	workspaceID, userID string,
	now time.Time,
) (count int, hasMore bool, nextSnoozeExpiry *time.Time, err error) {
	page, err := bundles.ListUnresolvedClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped:    true,
		WorkspaceID: workspaceID,
		Limit:       defaultInboxLimit,
		Sidecar:     &taskmodels.ClarificationSidecarFilter{UserID: userID, Now: now},
	})
	if err != nil {
		return 0, false, nil, err
	}
	summary, err := bundles.CountHiddenClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: workspaceID, Limit: 1,
		Sidecar: &taskmodels.ClarificationSidecarFilter{UserID: userID, Only: true, Now: now},
	})
	if err != nil {
		return 0, false, nil, err
	}
	return len(page.Bundles), page.HasMore, summary.NextSnoozeExpiry, nil
}
