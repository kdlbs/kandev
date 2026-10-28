package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
)

// creationRejectedField is the optional REST create-task error marker. It is
// present only when the handler can prove this request allocated no task row.
// A caller may then retire its client-side create identity and submit a
// corrected request under a new one.
const creationRejectedField = "creation_rejected"

// rejectedCreateBody adds the marker to an error body the handler already
// proved was produced before task allocation.
func rejectedCreateBody(body gin.H) gin.H {
	body[creationRejectedField] = true
	return body
}

// rejectedCreateMessage builds a create-only validation body with the proof
// marker. statuses and other fields stay exactly as their original response.
func rejectedCreateMessage(message string) gin.H {
	return gin.H{"error": message, creationRejectedField: true}
}

// isPreAllocationCreateRejection reports whether err proves the create request
// was refused before a task row was allocated. Service.CreateTask wraps every
// pre-insert validation or reference failure in PreAllocationTaskCreateError;
// the allowlist below keeps infrastructure failures out even when that marker
// is present.
func isPreAllocationCreateRejection(err error) bool {
	var preAllocationErr *service.PreAllocationTaskCreateError
	if !errors.As(err, &preAllocationErr) {
		return false
	}
	switch {
	case errors.Is(err, service.ErrTaskReferenceNotFound),
		errors.Is(err, taskrepository.ErrRepositoryNotFound),
		errors.Is(err, service.ErrInvalidRepositorySet),
		errors.Is(err, service.ErrInvalidInitialWorkspaceLayout),
		errors.Is(err, service.ErrInvalidWorkspaceRepositoryPlacement),
		errors.Is(err, service.ErrInvalidWorkspaceSource),
		errors.Is(err, service.ErrInvalidTaskWorkflow),
		errors.Is(err, service.ErrTaskTitleTooLong),
		errors.Is(err, service.ErrInvalidParent),
		errors.Is(err, service.ErrRepositoryBranchPolicyStale),
		errors.Is(err, service.ErrAssigneeCannotReachWorkspace):
		return true
	}
	var selectionErr *service.RepositorySelectionError
	return errors.As(err, &selectionErr)
}

// handleTaskCreateFailure writes the response for a create request that
// reached Service.CreateTask. It preserves every existing status and body,
// adding the rejection marker only for proven pre-allocation failures.
func handleTaskCreateFailure(c *gin.Context, log *logger.Logger, err error) {
	if !isPreAllocationCreateRejection(err) {
		handleNotFound(c, log, err, "task not created")
		return
	}
	if isClientDisconnect(err) {
		abortClientDisconnect(c)
		return
	}
	switch {
	case errors.Is(err, service.ErrTaskReferenceNotFound),
		errors.Is(err, taskrepository.ErrRepositoryNotFound):
		c.JSON(http.StatusNotFound, rejectedCreateMessage("task not created"))
	case errors.Is(err, service.ErrAssigneeCannotReachWorkspace):
		c.JSON(http.StatusBadRequest, rejectedCreateMessage(err.Error()))
	default:
		if status, ok := repositorySelectionHTTPStatus(err); ok {
			c.JSON(status, rejectedCreateBody(taskErrorBody(err)))
			return
		}
		c.JSON(http.StatusBadRequest, rejectedCreateBody(taskErrorBody(err)))
	}
}
