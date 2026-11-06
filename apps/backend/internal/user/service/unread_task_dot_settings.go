package service

import (
	"fmt"

	"github.com/kandev/kandev/internal/user/models"
)

func applyUnreadTaskDotHideDelay(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.UnreadTaskDotHideDelaySeconds == nil {
		return nil
	}
	if !models.ValidUnreadTaskDotHideDelaySeconds(*req.UnreadTaskDotHideDelaySeconds) {
		return fmt.Errorf("unread_task_dot_hide_delay_seconds must be between 0 and %d",
			models.UnreadTaskDotHideDelayMaxSeconds)
	}
	settings.UnreadTaskDotHideDelaySeconds = *req.UnreadTaskDotHideDelaySeconds
	return nil
}
