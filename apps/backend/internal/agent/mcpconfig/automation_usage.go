package mcpconfig

import "context"

// Automation usage stays with the automation store. Deleted definitions remain
// visible there as unavailable references until an explicit editor save.
func (s *CatalogService) SetAutomationUsageReader(reader func(context.Context, string, string) (int, error)) {
	s.automationUsage = reader
}
func (s *SelectionService) SetAutomationUsageReader(reader func(context.Context, string, string) (int, error)) {
	s.automationUsage = reader
}
