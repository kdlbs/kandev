package service

import "github.com/kandev/kandev/internal/user/models"

// applySidebarTaskColors applies a narrow manual-color patch to a copied map.
// Missing-only imports preserve both active colors and clear tombstones.
func applySidebarTaskColors(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.SidebarTaskColorPatch == nil {
		return nil
	}
	patch := *req.SidebarTaskColorPatch
	if err := models.ValidateSidebarTaskColorPatch(patch); err != nil {
		return err
	}

	next := models.CloneSidebarTaskColors(settings.SidebarTaskColors)
	for taskID, color := range patch.Colors {
		if patch.IfMissing {
			if _, exists := next[taskID]; exists {
				continue
			}
		}
		next[taskID] = cloneSidebarTaskColor(color)
	}
	if err := models.ValidateSidebarTaskColors(next); err != nil {
		return err
	}
	settings.SidebarTaskColors = next
	return nil
}

func cloneSidebarTaskColor(color *string) *string {
	if color == nil {
		return nil
	}
	value := *color
	return &value
}
