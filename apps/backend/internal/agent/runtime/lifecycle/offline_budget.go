package lifecycle

import "github.com/kandev/kandev/internal/common/offlinebudget"

// validateOfflineBudgetMetadata rejects a launch whose offline budget is
// outside the accepted form. An absent or empty value means the default.
func validateOfflineBudgetMetadata(metadata map[string]interface{}) error {
	_, err := offlinebudget.Parse(getMetadataString(metadata, MetadataKeyOfflineBudgetMinutes))
	return err
}

// offlineBudgetFromMetadata returns the budget to send to agentctl. Launch
// validates the value first, so an invalid one falls back to the default.
func offlineBudgetFromMetadata(metadata map[string]interface{}) *int {
	minutes, err := offlinebudget.Parse(getMetadataString(metadata, MetadataKeyOfflineBudgetMinutes))
	if err != nil {
		minutes = offlinebudget.DefaultMinutes
	}
	return &minutes
}
