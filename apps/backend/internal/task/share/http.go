package share

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
)

// HTTPHandlers wires the share endpoints. RegisterRoutes is the public entry
// point used by cmd/kandev.
type HTTPHandlers struct {
	svc      *Service
	ghClient github.Client
	log      *logger.Logger
}

// NewHTTPHandlers returns handlers that delegate to the share Service. The
// github.Client is used only for the upfront IsAuthenticated probe so we
// can return a precondition-failed error instead of letting Upload fail.
func NewHTTPHandlers(svc *Service, ghClient github.Client, log *logger.Logger) *HTTPHandlers {
	return &HTTPHandlers{svc: svc, ghClient: ghClient, log: log}
}

// RegisterRoutes wires the share endpoints onto the given gin engine under
// the /api/v1 prefix.
func (h *HTTPHandlers) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.POST("/tasks/:id/sessions/:sessionId/shares", h.httpCreate)
	api.GET("/tasks/:id/sessions/:sessionId/shares", h.httpList)
	api.DELETE("/shares/:shareId", h.httpRevoke)
}

// Response payloads. Kept on this file so the contract lives next to the
// handlers; the frontend share-api.ts must match these shapes.
type shareResponse struct {
	ID                string `json:"id"`
	URL               string `json:"url"`
	CreatedAt         string `json:"created_at"`
	RevokedAt         string `json:"revoked_at,omitempty"`
	SnapshotSizeBytes int64  `json:"snapshot_size_bytes"`
}

type listSharesResponse struct {
	Shares []shareResponse `json:"shares"`
}

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func toShareResponse(s *Share) shareResponse {
	out := shareResponse{
		ID:                s.ID,
		URL:               displayURL(s.ExternalURL),
		CreatedAt:         s.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		SnapshotSizeBytes: s.SnapshotSizeBytes,
	}
	if s.RevokedAt != nil {
		out.RevokedAt = s.RevokedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	return out
}

// displayURL returns the URL we want clients to use when opening a share.
// We always prefer the gist.githack.com rendered view (it proxies the raw
// gist file directly, avoiding the GitHub gists-API content budget that
// makes gistpreview.github.io render blank for big snapshots). Stored
// formats are normalised on the way out:
//
//   - bare gist URL (https://gist.github.com/<owner>/<id>) — older rows
//     written before share.html landed in the gist
//   - already a githack URL — re-pinned to /raw/share.html so rows that
//     targeted a different file (or no file) land on the styled view
//   - legacy gistpreview URL — passed through (owner is not recoverable
//     from gistpreview shape, so we can't upgrade to githack), but
//     re-pinned with /share.html when stored without a filename so
//     pre-existing rows still hit the styled HTML rather than the
//     alphabetically-first README.md
//
// Anything we don't recognise is returned unchanged.
func displayURL(stored string) string {
	if stored == "" {
		return ""
	}
	// Already a githack URL — re-pin /raw/share.html.
	if owner, id := ownerAndIDFromGithackURL(stored); owner != "" && id != "" {
		return githackURL(owner, id)
	}
	// Bare gist URL → convert to githack.
	if rendered := renderedURLForGist(stored); rendered != "" {
		return rendered
	}
	// Legacy gistpreview URL — pass through, but re-pin /share.html for
	// rows stored without a filename so they don't silently fall back to
	// rendering README.md.
	const previewPrefix = "https://gistpreview.github.io/?"
	if strings.HasPrefix(stored, previewPrefix) {
		id := strings.TrimPrefix(stored, previewPrefix)
		if i := strings.Index(id, "/"); i >= 0 {
			id = id[:i]
		}
		return previewPrefix + id + "/share.html"
	}
	return stored
}

// httpCreate handles POST /api/v1/tasks/:id/sessions/:sessionId/shares.
// Query string dry_run=true returns the snapshot inline without uploading.
func (h *HTTPHandlers) httpCreate(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, errorBody{Error: "session id is required"})
		return
	}
	ctx := c.Request.Context()

	if strings.EqualFold(c.Query("dry_run"), "true") {
		snap, err := h.svc.PreviewSnapshot(ctx, sessionID)
		if mapped, status := mapShareError(err); mapped != nil {
			c.JSON(status, mapped)
			return
		}
		c.JSON(http.StatusOK, snap)
		return
	}

	if err := h.requireGitHubAuth(ctx); err != nil {
		c.JSON(http.StatusPreconditionFailed, errorBody{
			Error: err.Error(),
			Code:  "github_credential_missing",
		})
		return
	}

	share, err := h.svc.CreateShare(ctx, sessionID)
	if mapped, status := mapShareError(err); mapped != nil {
		h.logServerError(err, "create share failed", sessionID)
		c.JSON(status, mapped)
		return
	}
	c.JSON(http.StatusCreated, toShareResponse(share))
}

// httpList returns every share row for the session, including revoked.
func (h *HTTPHandlers) httpList(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, errorBody{Error: "session id is required"})
		return
	}
	rows, err := h.svc.ListBySession(c.Request.Context(), sessionID)
	if err != nil {
		h.logServerError(err, "list shares failed", sessionID)
		c.JSON(http.StatusInternalServerError, errorBody{Error: "failed to list shares"})
		return
	}
	out := listSharesResponse{Shares: make([]shareResponse, 0, len(rows))}
	for _, r := range rows {
		out.Shares = append(out.Shares, toShareResponse(r))
	}
	c.JSON(http.StatusOK, out)
}

// httpRevoke handles DELETE /api/v1/shares/:shareId.
func (h *HTTPHandlers) httpRevoke(c *gin.Context) {
	shareID := c.Param("shareId")
	if shareID == "" {
		c.JSON(http.StatusBadRequest, errorBody{Error: "share id is required"})
		return
	}
	if err := h.svc.RevokeShare(c.Request.Context(), shareID); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, errorBody{Error: "share not found"})
			return
		}
		h.logServerError(err, "revoke share failed", shareID)
		c.JSON(http.StatusBadGateway, errorBody{
			Error: err.Error(),
			Code:  "gist_revoke_failed",
		})
		return
	}
	c.Status(http.StatusNoContent)
}

// requireGitHubAuth returns nil when the client reports an authenticated
// session, or an explanatory error otherwise. A transport-layer failure is
// logged and treated as "not authenticated" so the user still sees a clean
// 412 with a CTA, not a 5xx.
func (h *HTTPHandlers) requireGitHubAuth(ctx context.Context) error {
	if h.ghClient == nil {
		return errors.New("github client is not configured")
	}
	ok, err := h.ghClient.IsAuthenticated(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("connect a GitHub account to share tasks publicly")
	}
	return nil
}

// mapShareError translates a service error into the response body + status
// code. Returns (nil, 0) when err is nil. The status table is part of the
// public contract; keep it in sync with docs/specs/public-share-links/spec.md.
func mapShareError(err error) (*errorBody, int) {
	if err == nil {
		return nil, 0
	}
	switch {
	case errors.Is(err, ErrSessionNotShareable):
		return &errorBody{Error: "session has no shareable content yet", Code: "session_not_shareable"}, http.StatusConflict
	case errors.Is(err, ErrNotFound):
		return &errorBody{Error: "session or task not found"}, http.StatusNotFound
	case errors.Is(err, ErrSnapshotTooLarge):
		return &errorBody{Error: err.Error(), Code: "snapshot_too_large"}, http.StatusRequestEntityTooLarge
	}
	return &errorBody{Error: err.Error(), Code: "gist_upload_failed"}, http.StatusBadGateway
}

func (h *HTTPHandlers) logServerError(err error, msg, ref string) {
	if h.log == nil {
		return
	}
	h.log.Warn(msg, zap.String("ref", ref), zap.Error(err))
}
