package backendapp

import "testing"

func TestThemeBootInventory(t *testing.T) {
	if themeInventoryForBoot(routeParams{}) != nil {
		t.Fatal("unavailable plugins exposed inventory")
	}
	svc := newTestPluginsService(t)
	inventory := themeInventoryForBoot(routeParams{services: &Services{Plugins: svc}})
	if inventory == nil || inventory.BootID == "" || inventory.Themes == nil {
		t.Fatal("missing boot inventory identity")
	}
}
