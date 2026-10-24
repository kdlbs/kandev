package backendapp

import "github.com/kandev/kandev/internal/plugins"

func themeInventoryForBoot(p routeParams) *plugins.ThemeInventory {
	if p.services == nil || p.services.Plugins == nil {
		return nil
	}
	inventory := p.services.Plugins.ThemeInventory()
	return &inventory
}
