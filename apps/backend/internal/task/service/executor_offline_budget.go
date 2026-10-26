package service

import (
	"fmt"

	"github.com/kandev/kandev/internal/common/offlinebudget"
)

// validateOfflineBudgetConfig rejects a profile whose offline budget is not a
// whole number of minutes in the accepted range. An absent or empty value
// means the default and is accepted.
func validateOfflineBudgetConfig(config map[string]string) error {
	value, ok := config[offlinebudget.Key]
	if !ok {
		return nil
	}
	if _, err := offlinebudget.Parse(value); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorConfig, err)
	}
	return nil
}
