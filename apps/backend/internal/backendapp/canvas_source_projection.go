package backendapp

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

type canvasSourceLookup interface {
	GetTask(context.Context, string) (*models.Task, error)
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
}

type canvasSourceLabels struct {
	TaskTitle   string
	SessionName string
}

// projectCanvasSourceLabels adds only labels that the current caller can read.
// The source identifiers remain action and lookup identities, never fallbacks.
func projectCanvasSourceLabels(
	ctx context.Context,
	lookup canvasSourceLookup,
	workspaceID string,
	taskID string,
	sessionID string,
) canvasSourceLabels {
	if lookup == nil {
		return canvasSourceLabels{}
	}

	if taskID == "" && sessionID != "" {
		session, err := lookup.GetTaskSession(ctx, sessionID)
		if err != nil || session == nil {
			return canvasSourceLabels{}
		}
		taskID = session.TaskID
	}
	if taskID == "" {
		return canvasSourceLabels{}
	}

	task, err := lookup.GetTask(ctx, taskID)
	if err != nil || task == nil || task.WorkspaceID != workspaceID {
		return canvasSourceLabels{}
	}
	labels := canvasSourceLabels{TaskTitle: strings.TrimSpace(task.Title)}
	if sessionID == "" {
		return labels
	}

	session, err := lookup.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != task.ID {
		return labels
	}
	labels.SessionName = strings.TrimSpace(session.Name)
	return labels
}

func (h *canvasHTTPHandler) decorateCanvasReleaseSources(
	ctx context.Context,
	workspaceID string,
	responses ...*canvasReleaseResponse,
) {
	for _, response := range responses {
		if response == nil || (response.SourceTaskID == "" && response.SourceSessionID == "") {
			continue
		}
		labels := projectCanvasSourceLabels(
			ctx,
			h.tasks,
			workspaceID,
			response.SourceTaskID,
			response.SourceSessionID,
		)
		response.SourceTaskTitle = labels.TaskTitle
		response.SourceSessionName = labels.SessionName
	}
}
