package clarification

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/authz"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// inboxRecoveryBlockReader is the optional, read-only list of open session
// recovery blocks waiting on an operator decision. It is resolved from the
// bundle store by type assertion so existing store doubles stay valid; a
// store without it answers 501.
type inboxRecoveryBlockReader interface {
	ListOpenWorkspaceRecoveryBlocks(ctx context.Context, workspaceID string, limit int) ([]taskmodels.WorkspaceRecoveryBlockSummary, bool, error)
}

// inboxEarlierListResponse is GET /api/v1/clarification-inbox/earlier-turns's
// response envelope: questions still marked pending on a turn that is no
// longer the session's current turn. They cannot be claimed, so the rows are
// read-only and answered by an ordinary follow-up message.
type inboxEarlierListResponse struct {
	Bundles    []inboxBundleView `json:"bundles"`
	Count      int               `json:"count"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// inboxRecoveryBlockView is one row of GET /api/v1/clarification-inbox/recovery-blocks.
type inboxRecoveryBlockView struct {
	BlockID           string `json:"block_id"`
	TaskID            string `json:"task_id"`
	TaskTitle         string `json:"task_title"`
	SessionID         string `json:"session_id"`
	SessionState      string `json:"session_state"`
	Reason            string `json:"reason"`
	ConsumerReference string `json:"consumer_reference,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

// inboxRecoveryListResponse is GET /api/v1/clarification-inbox/recovery-blocks's
// response envelope. truncated is true when more blocks exist beyond the page.
type inboxRecoveryListResponse struct {
	Blocks    []inboxRecoveryBlockView `json:"blocks"`
	Count     int                      `json:"count"`
	Truncated bool                     `json:"truncated"`
}

// authorizeInboxWorkspaceRead applies the inbox read's workspace rule and
// writes the error response itself when it fails.
func (h *Handlers) authorizeInboxWorkspaceRead(c *gin.Context, requestCtx, ctx context.Context, workspaceID string) bool {
	err := h.inboxTasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead)
	if err == nil {
		return true
	}
	if !h.respondInboxReadError(c, requestCtx, err) {
		h.respondInboxWorkspaceAuthzError(c, err)
	}
	return false
}

func (h *Handlers) respondInboxReadFailure(c *gin.Context, requestCtx context.Context, err error, message string) {
	if h.respondInboxReadError(c, requestCtx, err) {
		return
	}
	h.logger.Error(message, zap.Error(err))
	respondInboxError(c, http.StatusInternalServerError, errInboxInternal)
}

// httpListInboxEarlierTurns backs GET /api/v1/clarification-inbox/earlier-turns.
func (h *Handlers) httpListInboxEarlierTurns(c *gin.Context) {
	requestCtx := c.Request.Context()
	workspaceID, limit, cursorCreatedAt, cursorPendingID, ok := h.parseInboxListQuery(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(requestCtx, inboxReadTimeout)
	defer cancel()
	if !h.authorizeInboxWorkspaceRead(c, requestCtx, ctx, workspaceID) {
		return
	}
	page, err := h.inboxBundles.ListUnresolvedClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped:        true,
		WorkspaceID:     workspaceID,
		CursorCreatedAt: cursorCreatedAt,
		CursorPendingID: cursorPendingID,
		Limit:           limit,
		Sidecar:         &taskmodels.ClarificationSidecarFilter{UserID: InboxUserID(ctx), Now: h.now()},
		EarlierTurns:    true,
	})
	if err != nil {
		h.respondInboxReadFailure(c, requestCtx, err, "failed to list earlier-turn needs-you bundles")
		return
	}
	views, err := h.buildInboxBundleViews(ctx, page.Bundles)
	if err != nil {
		h.respondInboxReadFailure(c, requestCtx, err, "failed to hydrate earlier-turn needs-you bundles")
		return
	}
	resp := inboxEarlierListResponse{Bundles: views, Count: len(views)}
	if page.HasMore && len(page.Bundles) > 0 {
		last := page.Bundles[len(page.Bundles)-1]
		resp.NextCursor = encodeInboxCursor(last.CreatedAt, last.PendingID)
	}
	c.JSON(http.StatusOK, resp)
}

// httpListInboxRecoveryBlocks backs GET /api/v1/clarification-inbox/recovery-blocks.
// It lists; the decision itself stays on the task chat's recovery card.
func (h *Handlers) httpListInboxRecoveryBlocks(c *gin.Context) {
	requestCtx := c.Request.Context()
	workspaceID, limit, _, _, ok := h.parseInboxListQuery(c)
	if !ok {
		return
	}
	reader, supported := h.inboxBundles.(inboxRecoveryBlockReader)
	if !supported {
		respondInboxError(c, http.StatusNotImplemented, "recovery block listing is not available")
		return
	}
	ctx, cancel := context.WithTimeout(requestCtx, inboxReadTimeout)
	defer cancel()
	if !h.authorizeInboxWorkspaceRead(c, requestCtx, ctx, workspaceID) {
		return
	}
	blocks, hasMore, err := reader.ListOpenWorkspaceRecoveryBlocks(ctx, workspaceID, limit)
	if err != nil {
		h.respondInboxReadFailure(c, requestCtx, err, "failed to list open session recovery blocks")
		return
	}
	resp := inboxRecoveryListResponse{Blocks: make([]inboxRecoveryBlockView, 0, len(blocks)), Truncated: hasMore}
	for _, block := range blocks {
		resp.Blocks = append(resp.Blocks, inboxRecoveryBlockView{
			BlockID: block.BlockID, TaskID: block.TaskID, TaskTitle: block.TaskTitle,
			SessionID: block.SessionID, SessionState: block.SessionState, Reason: block.Reason,
			ConsumerReference: block.ConsumerReference,
			CreatedAt:         block.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:         block.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	resp.Count = len(resp.Blocks)
	c.JSON(http.StatusOK, resp)
}
