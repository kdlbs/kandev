package mentions

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const errorResponseField = "error"

// Searcher is the HTTP handler's provider-neutral search boundary.
type Searcher interface {
	Search(context.Context, SearchRequest) (*apiv1.MentionSearchResponse, error)
}

// Handler exposes mention search over HTTP.
type Handler struct {
	searcher Searcher
	log      *logger.Logger
}

func NewHandler(searcher Searcher, log *logger.Logger) *Handler {
	return &Handler{searcher: searcher, log: log}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/api/v1/workspaces/:id/mentions/search", h.search)
}

func (h *Handler) search(c *gin.Context) {
	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		h.writeSearchError(c, ErrInvalidRequest)
		return
	}
	response, err := h.searcher.Search(c.Request.Context(), SearchRequest{
		WorkspaceID: c.Param("id"),
		Query:       c.Query("q"),
		Limit:       limit,
	})
	if err != nil {
		h.writeSearchError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func parseLimit(rawLimit string) (int, error) {
	rawLimit = strings.TrimSpace(rawLimit)
	if rawLimit == "" {
		return DefaultLimit, nil
	}
	limit, err := strconv.Atoi(rawLimit)
	if err != nil {
		return 0, ErrInvalidRequest
	}
	if limit < 1 {
		return 1, nil
	}
	if limit > MaxLimit {
		return MaxLimit, nil
	}
	return limit, nil
}

func (h *Handler) writeSearchError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{errorResponseField: "invalid mention search request"})
	case errors.Is(err, ErrWorkspaceNotFound):
		c.JSON(http.StatusNotFound, gin.H{errorResponseField: "mention search workspace not found"})
	case errors.Is(err, context.Canceled):
		c.JSON(499, gin.H{errorResponseField: "mention search request canceled"})
	default:
		h.logUnexpectedFailure(c, err)
		c.JSON(http.StatusInternalServerError, gin.H{errorResponseField: "mention search failed"})
	}
}

func (h *Handler) logUnexpectedFailure(c *gin.Context, err error) {
	if h.log == nil {
		return
	}
	stage, class := SearchFailureMetadata(err)
	h.log.Error(
		"mention search failed",
		zap.String("workspace_id", c.Param("id")),
		zap.String("failure_stage", stage),
		zap.String("failure_class", class),
	)
}
