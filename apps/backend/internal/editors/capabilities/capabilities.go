// Package capabilities resolves editor support from a task session's executor.
package capabilities

import "github.com/kandev/kandev/internal/task/models"

const (
	hostOSLinux  = "linux"
	hostOSDarwin = "darwin"
)

// SupportsEmbeddedVscode reports whether the executor can run Kandev's
// code-server-based embedded editor. Unknown and test-only executors fail
// closed so callers never advertise an unsupported launch path.
func SupportsEmbeddedVscode(executorType models.ExecutorType, hostOS string) bool {
	switch executorType {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree:
		return hostOS == hostOSLinux || hostOS == hostOSDarwin
	case models.ExecutorTypeLocalDocker, models.ExecutorTypeRemoteDocker,
		models.ExecutorTypeSprites, models.ExecutorTypeSSH, models.ExecutorTypeKubernetes:
		return true
	default:
		return false
	}
}
