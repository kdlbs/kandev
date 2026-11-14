package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

func handleBulkMoveError(c *gin.Context, err error) bool {
	var refused *service.BulkMoveRefusedError
	if errors.As(err, &refused) {
		c.JSON(http.StatusConflict, dto.BulkMoveErrorResponse{
			Code: service.BulkMoveRefusedCode, MovedTaskIDs: []string{}, Refusals: bulkMoveRefusals(refused.Refusals),
		})
		return true
	}
	var partial *service.BulkMovePartialError
	if errors.As(err, &partial) {
		c.JSON(http.StatusConflict, dto.BulkMoveErrorResponse{
			Code: service.BulkMovePartialCode, MovedTaskIDs: append([]string{}, partial.MovedTaskIDs...),
			Refusals: bulkMoveRefusals([]service.TaskRefusal{partial.Refusal}),
		})
		return true
	}
	return false
}

func bulkMoveRefusals(refusals []service.TaskRefusal) []dto.BulkMoveTaskRefusal {
	result := make([]dto.BulkMoveTaskRefusal, 0, len(refusals))
	for _, refusal := range refusals {
		item := dto.BulkMoveTaskRefusal{
			TaskID: refusal.TaskID, TaskTitle: refusal.TaskTitle,
			Code: refusal.Code, Message: refusal.Message,
		}
		if len(refusal.PendingApprovers) > 0 {
			item.PendingApprovers = make([]dto.BulkMovePendingApprover, 0, len(refusal.PendingApprovers))
			for _, approver := range refusal.PendingApprovers {
				item.PendingApprovers = append(item.PendingApprovers, dto.BulkMovePendingApprover{
					AgentProfileID: approver.AgentProfileID, Name: approver.Name,
				})
			}
		}
		result = append(result, item)
	}
	return result
}
