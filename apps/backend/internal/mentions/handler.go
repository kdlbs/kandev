package mentions

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const errorResponseField = "error"

// Searcher is the HTTP handler's provider-neutral search boundary.
type Searcher interface {
	Search(context.Context, SearchRequest) (*apiv1.MentionSearchResponse, error)
}

// Handler exposes mention search over HTTP.
type Handler struct {
	searcher Searcher
}

func NewHandler(searcher Searcher) *Handler {
	return &Handler{searcher: searcher}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/api/v1/workspaces/:id/mentions/search", h.search)
}

func (h *Handler) search(c *gin.Context) {
	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeSearchError(c, ErrInvalidRequest)
		return
	}
	response, err := h.searcher.Search(c.Request.Context(), SearchRequest{
		WorkspaceID: c.Param("id"),
		Query:       c.Query("q"),
		Limit:       limit,
	})
	if err != nil {
		writeSearchError(c, err)
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

func writeSearchError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{errorResponseField: "invalid mention search request"})
	case errors.Is(err, ErrWorkspaceNotFound):
		c.JSON(http.StatusNotFound, gin.H{errorResponseField: "mention search workspace not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{errorResponseField: "mention search failed"})
	}
}
