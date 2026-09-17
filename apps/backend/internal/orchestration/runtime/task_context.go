package runtime

import "github.com/kandev/kandev/internal/orchestration/models"

func newTaskContextProfile(packet *models.ContextPacket, repositoryID, assigneeID string) (string, error) {
	if packet == nil {
		return assigneeID, nil
	}
	if packet.TaskID != "" || packet.EnvironmentID != "" {
		return "", rejectOperation(422, "new task requires context without an existing task or environment")
	}
	if packet.ProjectID != "" && packet.ProjectID != repositoryID {
		return "", rejectOperation(422, "context repository must match the new task")
	}
	if assigneeID != "" && assigneeID != packet.ProfileID {
		return "", rejectOperation(422, "context account must match the selected profile")
	}
	return packet.ProfileID, nil
}
