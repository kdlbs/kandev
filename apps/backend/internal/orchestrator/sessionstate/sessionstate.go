package sessionstate

import "github.com/kandev/kandev/internal/task/models"

func IsWorking(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateRunning || state == models.TaskSessionStateStarting
}
