package lifecycle

import (
	"context"
	"maps"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const gitHooksProviderTimeout = 5 * time.Second

// GitHooksProvider returns the stored per-workspace-entry "run Git hooks"
// decision for a task, keyed like the base-branch map. Wired to the task
// service, which reads the repository records at call time.
type GitHooksProvider func(ctx context.Context, taskID string) (map[string]bool, error)

// SetGitHooksProvider wires the provider read by seedExecutionGitHooks.
func (m *Manager) SetGitHooksProvider(fn GitHooksProvider) {
	m.gitHooksProvider = fn
}

// seedExecutionGitHooks is the only code that sets MetadataKeyGitHooks. It runs
// as the last metadata step before the Manager's create call and reads the
// stored decision once. Any value already present is removed first; every
// failure leaves no key, which means hooks off.
func (m *Manager) seedExecutionGitHooks(ctx context.Context, taskID, executorType string, metadata map[string]interface{}) {
	delete(metadata, MetadataKeyGitHooks)
	if taskID == "" || executorType == string(models.ExecutorTypeKubernetes) ||
		executorType == string(models.ExecutorTypePluginRemote) {
		return
	}
	if m.gitHooksProvider == nil {
		m.logger.Warn("git hooks provider not wired; managed Git runs without hooks", zap.String("task_id", taskID))
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, gitHooksProviderTimeout)
	defer cancel()
	hooks, err := m.gitHooksProvider(readCtx, taskID)
	if err != nil || hooks == nil {
		m.logger.Warn("git hooks unavailable; managed Git runs without hooks",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if len(hooks) > 0 {
		metadata[MetadataKeyGitHooks] = maps.Clone(hooks)
	}
}

// gitHooksFromMetadata returns a copy of the seeded decision, or nil.
func gitHooksFromMetadata(metadata map[string]interface{}) map[string]bool {
	hooks, _ := metadata[MetadataKeyGitHooks].(map[string]bool)
	if len(hooks) == 0 {
		return nil
	}
	return maps.Clone(hooks)
}
