package service

import (
	"fmt"
	"github.com/kandev/kandev/internal/user/models"
)

func applySidebarPresentationSettings(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.SidebarNewTaskStyle != nil && *req.SidebarNewTaskStyle != "simple" && *req.SidebarNewTaskStyle != "compact" {
		return fmt.Errorf("unsupported sidebar_new_task_style")
	}
	if req.SidebarFastActionsEnabled != nil {
		settings.SidebarFastActionsEnabled = *req.SidebarFastActionsEnabled
	}
	if req.SidebarNewTaskStyle != nil {
		settings.SidebarNewTaskStyle = *req.SidebarNewTaskStyle
	}
	return nil
}
