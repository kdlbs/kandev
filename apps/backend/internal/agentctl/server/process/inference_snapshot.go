package process

import "github.com/kandev/kandev/internal/agentctl/server/utility"

// InferenceOwner returns the authority initialized before cancellable startup.
func (m *Manager) InferenceOwner() *utility.InferenceOwner { return m.inferenceOwner }

// RequireInferenceCapacity closes the unreserved utility start path.
func (m *Manager) RequireInferenceCapacity(required bool) {
	if m.inferenceOwner != nil {
		m.inferenceOwner.RequireCapacity(required)
	}
}

// SetInferenceSnapshotter connects the instance-owned inference journal before admission.
func (m *Manager) SetInferenceSnapshotter(snapshotter func() utility.AttemptSnapshot) {
	m.inferenceSnapshotMu.Lock()
	defer m.inferenceSnapshotMu.Unlock()
	m.inferenceSnapshotter = snapshotter
}

func (m *Manager) captureFinalInferenceSnapshot() {
	m.inferenceSnapshotMu.Lock()
	defer m.inferenceSnapshotMu.Unlock()
	if m.finalInferenceSnapshot == nil && m.inferenceSnapshotter != nil {
		snapshot := m.inferenceSnapshotter()
		m.finalInferenceSnapshot = &snapshot
	}
}

// FinalInferenceSnapshot is available only after successful owned drain and process cleanup.
func (m *Manager) FinalInferenceSnapshot() *utility.AttemptSnapshot {
	m.inferenceSnapshotMu.RLock()
	defer m.inferenceSnapshotMu.RUnlock()
	if m.finalInferenceSnapshot == nil {
		return nil
	}
	snapshot := *m.finalInferenceSnapshot
	snapshot.Attempts = append([]utility.AttemptStatus(nil), snapshot.Attempts...)
	return &snapshot
}
