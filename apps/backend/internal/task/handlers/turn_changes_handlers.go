package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

func (h *TaskHandlers) httpListTurnChangeHistory(c *gin.Context) {
	offset, limit, ok := parseTurnChangePage(c)
	if !ok {
		return
	}
	sessionID := c.Param("id")
	sets, total, err := h.service.ListTurnChangeHistory(c.Request.Context(), sessionID, offset, limit)
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	response := dto.TurnChangeHistoryResponse{
		ChangeSets: make([]dto.TurnChangeSetSummaryDTO, 0, len(sets)), Total: total, Offset: offset, Limit: limit,
	}
	for _, changeSet := range sets {
		repositories, listErr := h.service.ListTurnChangeRepositories(c.Request.Context(), sessionID, changeSet.ID)
		if listErr != nil {
			h.logger.Error("failed to list turn change repositories", zap.String("change_set_id", changeSet.ID), zap.Error(listErr))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list turn change history"})
			return
		}
		response.ChangeSets = append(response.ChangeSets, dto.TurnChangeSetSummaryFromModel(changeSet, repositories))
	}
	if offset+len(sets) < total {
		next := offset + len(sets)
		response.NextOffset = &next
	}
	c.JSON(http.StatusOK, response)
}

func (h *TaskHandlers) httpGetTurnChangeHistory(c *gin.Context) {
	sessionID, changeSetID := c.Param("id"), c.Param("changeSetID")
	changeSet, err := h.service.GetTurnChangeHistory(c.Request.Context(), sessionID, changeSetID)
	if err != nil {
		handleNotFound(c, h.logger, err, "turn change history not found")
		return
	}
	repositories, err := h.service.ListTurnChangeRepositories(c.Request.Context(), sessionID, changeSetID)
	if err != nil {
		handleNotFound(c, h.logger, err, "turn change history not found")
		return
	}
	c.JSON(http.StatusOK, dto.TurnChangeSetSummaryFromModel(changeSet, repositories))
}

func (h *TaskHandlers) httpListTurnChangeFiles(c *gin.Context) {
	offset, limit, ok := parseTurnChangePage(c)
	if !ok {
		return
	}
	sessionID, changeSetID, repositoryChangeID := c.Param("id"), c.Param("changeSetID"), c.Param("repositoryChangeID")
	files, total, err := h.service.ListTurnChangeFiles(c.Request.Context(), sessionID, changeSetID, repositoryChangeID, offset, limit)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTurnChangeRelationship) || errors.Is(err, repoerrors.ErrTurnChangeSetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "turn change history not found"})
			return
		}
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	response := dto.TurnChangeFilesResponse{Files: make([]dto.TurnFileChangeDTO, 0, len(files)), Total: total, Offset: offset, Limit: limit}
	for _, file := range files {
		response.Files = append(response.Files, dto.TurnFileChangeFromModel(file))
	}
	if offset+len(files) < total {
		next := offset + len(files)
		response.NextOffset = &next
	}
	c.JSON(http.StatusOK, response)
}

func (h *TaskHandlers) httpReadTurnChangeContent(c *gin.Context) {
	variant := models.TurnChangeContentVariant(c.Query("variant"))
	payload, err := h.service.ReadTurnChangeContent(
		c.Request.Context(), c.Param("id"), c.Param("changeSetID"), c.Param("fileChangeID"), variant,
	)
	if err != nil {
		var expired *service.TurnChangeContentExpiredError
		if errors.As(err, &expired) || errors.Is(err, service.ErrTurnChangeContentExpired) {
			reason := models.TurnChangeReasonExpiredAge
			if expired != nil && expired.Reason != "" {
				reason = expired.Reason
			}
			c.JSON(http.StatusGone, gin.H{"availability": models.TurnChangeAvailabilityExpired, "reason": reason})
			return
		}
		if errors.Is(err, repoerrors.ErrTurnChangeContentNotFound) || errors.Is(err, repoerrors.ErrTurnChangeSetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "turn change content not found"})
			return
		}
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	c.JSON(http.StatusOK, dto.TurnChangeContentResponse{
		FileChangeID: payload.FileChangeID, Variant: payload.Variant, Content: payload.Content,
		Digest: payload.Digest, Truncated: payload.Truncated,
	})
}

func parseTurnChangePage(c *gin.Context) (int, int, bool) {
	offset := 0
	limit := 50
	if raw := c.Query("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be a non-negative integer"})
			return 0, 0, false
		}
		offset = parsed
	}
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
			return 0, 0, false
		}
		limit = parsed
	}
	return offset, limit, true
}
