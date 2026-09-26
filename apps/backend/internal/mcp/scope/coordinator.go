package scope

import (
	"context"
	"os"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	// CanonicalCoordinatorTaskEnv names the one task granted the Coordinator
	// surface by server configuration. Repository paths are mutable task input
	// and must never confer this authority.
	CanonicalCoordinatorTaskEnv = "KANDEV_COORDINATOR_TASK_ID"
)

// IsCanonicalCoordinatorTask verifies the server-owned task grant used to
// grant one normal Kanban task the narrow Coordinator control surface.
func IsCanonicalCoordinatorTask(_ context.Context, task *models.Task, _ any) bool {
	if task == nil || task.ID == "" || task.WorkspaceID == "" || task.ArchivedAt != nil {
		return false
	}
	return task.ID == strings.TrimSpace(os.Getenv(CanonicalCoordinatorTaskEnv))
}
