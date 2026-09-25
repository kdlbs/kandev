package service

import (
	"fmt"
	"github.com/kandev/kandev/internal/user/models"
)

func applySidebarHoverSettings(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.SidebarHoverDelayMs != nil && (*req.SidebarHoverDelayMs < 0 || *req.SidebarHoverDelayMs > 5000) {
		return fmt.Errorf("sidebar_hover_delay_ms must be between 0 and 5000")
	}
	if req.SidebarHoverEnabled != nil {
		settings.SidebarHoverEnabled = *req.SidebarHoverEnabled
	}
	if req.SidebarHoverDelayMs != nil {
		settings.SidebarHoverDelayMs = *req.SidebarHoverDelayMs
	}
	return nil
}
