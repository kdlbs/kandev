package mcpconfig

import "context"

// Automation usage stays with the automation store. Deleted definitions remain
// visible there as unavailable references until an explicit editor save.
func (s *CatalogService) SetAutomationUsageReader(reader func(context.Context, string) (map[string]int, error)) {
	s.automationUsage = reader
}
func (s *SelectionService) SetAutomationUsageReader(reader func(context.Context, string) (map[string]int, error)) {
	s.automationUsage = reader
}

// ConfigureCatalogUsage shares the reference guards with HTTP-owned catalogs.
func (s *SelectionService) ConfigureCatalogUsage(catalog *CatalogService) {
	catalog.SetSelectionRepository(s.repo)
	catalog.SetAutomationUsageReader(s.automationUsage)
}
