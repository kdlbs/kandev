package orchestrator

import "context"

func (m *sessionUpdatingAgentManager) IsAgentReadyForPrompt(_ context.Context, _ string) bool {
	return m.onStartCalled != nil && *m.onStartCalled
}
