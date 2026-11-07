package process

import (
	"context"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

// StopForInferenceClosure keeps only the journal and control transport alive
// after normal owned teardown, so settlement can precede receipt commitment.
func (m *Manager) StopForInferenceClosure(ctx context.Context) (*utility.ManagedProcessStopReceipt, error) {
	if err := m.stopForTeardown(ctx, true); err != nil {
		return nil, err
	}
	proof := m.FinalManagedStopReceipt()
	if proof == nil {
		return nil, utility.ErrInferenceClosureIncomplete
	}
	return proof, nil
}
