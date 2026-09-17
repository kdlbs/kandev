package failedinbox

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

const (
	defaultFailedInboxLimit = 50
	maxFailedInboxLimit     = 200

	errFailedInboxInternal = "failed to read failed inbox"
)

// workspaceAuthorizer is the workspace-scope check the Failed tab's read
// endpoint needs (design "Security"): the same rule AuthorizeWorkspaceScope
// already applies everywhere else.
type workspaceAuthorizer interface {
	AuthorizeWorkspaceScope(ctx context.Context, workspaceID string, scope authz.Scope) error
}

// failedTaskStore is the bounded read surface the Failed tab needs from the
// task repository.
type failedTaskStore interface {
	ListFailedInboxTasks(ctx context.Context, opts taskmodels.ListFailedInboxOptions) (*taskmodels.FailedInboxPage, error)
}

// Handlers serves the Inbox Failed tab's HTTP surface.
type Handlers struct {
	authz  workspaceAuthorizer
	store  failedTaskStore
	logger *logger.Logger
}

// NewHandlers constructs the Failed-tab handlers.
func NewHandlers(authz workspaceAuthorizer, store failedTaskStore, log *logger.Logger) *Handlers {
	return &Handlers{
		authz:  authz,
		store:  store,
		logger: log.WithFields(zap.String("component", "failed-inbox-handlers")),
	}
}

// RegisterRoutes registers the Failed tab's HTTP route. enabled gates it
// behind the same Inbox feature flag that already governs the Inbox
// destination (AC-UI-INBOX-FAILED-001.3).
func RegisterRoutes(router *gin.Engine, authz workspaceAuthorizer, store failedTaskStore, log *logger.Logger, enabled bool) {
	if !enabled {
		return
	}
	h := NewHandlers(authz, store, log)
	router.GET("/api/v1/failed-inbox", h.httpListFailedInbox)
}

// failedInboxRow is one wire row. task_id, title, workspace_id, origin and
// reason are always present as an empty string rather than absent;
// failure_instant is omitted (never null, never a zero instant) when
// unresolvable -- the wire contract F22 fixes.
type failedInboxRow struct {
	TaskID         string  `json:"task_id"`
	Title          string  `json:"title"`
	WorkspaceID    string  `json:"workspace_id"`
	Origin         string  `json:"origin"`
	FailureInstant *string `json:"failure_instant,omitempty"`
	Reason         string  `json:"reason"`
}

// failedInboxListResponse is GET /api/v1/failed-inbox's response envelope.
type failedInboxListResponse struct {
	Rows      []failedInboxRow `json:"rows"`
	Count     int              `json:"count"`
	Truncated bool             `json:"truncated"`
}

func respondFailedInboxError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

// httpListFailedInbox backs GET /api/v1/failed-inbox.
func (h *Handlers) httpListFailedInbox(c *gin.Context) {
	ctx := c.Request.Context()
	workspaceID := strings.TrimSpace(c.Query("workspace_id"))
	if workspaceID == "" {
		respondFailedInboxError(c, http.StatusBadRequest, "workspace_id is required")
		return
	}
	limit, err := parseFailedInboxLimit(c.Query("limit"))
	if err != nil {
		respondFailedInboxError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		h.respondWorkspaceAuthzError(c, err)
		return
	}

	page, err := h.store.ListFailedInboxTasks(ctx, taskmodels.ListFailedInboxOptions{WorkspaceID: workspaceID, Limit: limit})
	if err != nil {
		h.logger.Error("failed to list failed inbox tasks", zap.Error(err))
		respondFailedInboxError(c, http.StatusInternalServerError, errFailedInboxInternal)
		return
	}

	rows := make([]failedInboxRow, 0, len(page.Rows))
	for _, row := range page.Rows {
		rows = append(rows, toFailedInboxRow(row))
	}
	c.JSON(http.StatusOK, failedInboxListResponse{Rows: rows, Count: len(rows), Truncated: page.HasMore})
}

// toFailedInboxRow projects a store row onto the wire shape, applying
// AC-UI-INBOX-FAILED-001.19a's sanitize-then-truncate reason contract.
func toFailedInboxRow(row taskmodels.FailedInboxTaskRow) failedInboxRow {
	out := failedInboxRow{
		TaskID:      row.TaskID,
		Title:       row.Title,
		WorkspaceID: row.WorkspaceID,
		Origin:      row.Origin,
		Reason:      SanitizeAndTruncateReason(row.Reason),
	}
	if row.FailureInstant != nil {
		instant := row.FailureInstant.UTC().Format(time.RFC3339)
		out.FailureInstant = &instant
	}
	return out
}

// parseFailedInboxLimit mirrors the Needs-you bucket's limit contract
// (AC-UI-INBOX-FAILED-001.12): absent defaults to 50, non-numeric or <= 0 is
// rejected, and > 200 clamps to 200.
func parseFailedInboxLimit(raw string) (int, error) {
	if raw == "" {
		return defaultFailedInboxLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, errors.New("limit must be a positive integer")
	}
	if n > maxFailedInboxLimit {
		return maxFailedInboxLimit, nil
	}
	return n, nil
}

// respondWorkspaceAuthzError maps AuthorizeWorkspaceScope's errors per
// AC-UI-INBOX-FAILED-001.24: an unreachable workspace is a non-disclosing 404.
func (h *Handlers) respondWorkspaceAuthzError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		respondFailedInboxError(c, http.StatusNotFound, "workspace not found")
	case errors.Is(err, service.ErrForbidden):
		respondFailedInboxError(c, http.StatusForbidden, "insufficient permissions")
	default:
		h.logger.Error("failed to authorize failed inbox workspace access", zap.Error(err))
		respondFailedInboxError(c, http.StatusInternalServerError, errFailedInboxInternal)
	}
}
