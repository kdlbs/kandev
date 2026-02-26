//go:build windows

package process

import (
	"github.com/kandev/kandev/internal/common/logger"
)

// logChildProcesses is a no-op on Windows.
func logChildProcesses(_ *logger.Logger, _ int) {}

// CleanupOrphanedCodeServers is a no-op on Windows.
// Windows process groups are handled differently via taskkill /T.
func CleanupOrphanedCodeServers(_ *logger.Logger) {}
