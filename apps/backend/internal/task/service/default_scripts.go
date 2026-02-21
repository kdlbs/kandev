package service

import (
	"github.com/kandev/kandev/internal/scriptengine"
	"github.com/kandev/kandev/internal/task/models"
)

// DefaultPrepareScript returns the default prepare script for a given executor type.
//
// Deprecated: Use scriptengine.DefaultPrepareScript directly.
func DefaultPrepareScript(executorType models.ExecutorType) string {
	return scriptengine.DefaultPrepareScript(string(executorType))
}
