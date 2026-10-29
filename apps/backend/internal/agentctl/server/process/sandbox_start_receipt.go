package process

import (
	"context"
	"errors"
	"os"

	"github.com/kandev/kandev/internal/common/sandbox"
)

// sandboxStartRecord is protected by startMu. A completed no-spawn result is
// authoritative only while its attempt remains non-replayable.
type sandboxStartRecord struct {
	launch     *sandbox.Launch
	dispatched bool
	completed  bool
	owner      *sandbox.ProcessOwner
}

func (m *Manager) beginSandboxStart() (*sandboxStartRecord, error) {
	if m.cfg.Sandbox == nil {
		return nil, nil
	}
	launch := m.cfg.Sandbox
	if launch.Validate() != nil {
		return nil, sandbox.ErrInvalidPolicy
	}
	if m.sandboxStarts == nil {
		m.sandboxStarts = map[string]*sandboxStartRecord{}
	}
	if m.sandboxStarts[launch.AttemptID] != nil {
		return nil, sandbox.ErrInvalidPolicy
	}
	for key, record := range m.sandboxStarts {
		if _, err := os.Lstat(record.launch.Root); record.completed && errors.Is(err, os.ErrNotExist) {
			delete(m.sandboxStarts, key)
		}
	}
	if len(m.sandboxStarts) >= 256 {
		return nil, sandbox.ErrInvalidPolicy
	}
	record := &sandboxStartRecord{launch: launch.Clone()}
	m.sandboxStarts[launch.AttemptID] = record
	m.sandboxOwner.Store(nil)
	return record, nil
}

func (m *Manager) completeSandboxStart(record *sandboxStartRecord) {
	if record == nil {
		return
	}
	record.completed = true
	if owner := m.sandboxOwner.Load(); owner != nil {
		copied := *owner
		record.owner = &copied
	}
}

// SandboxStartReceipt exposes only a completed, exact attempt on the private
// control route. It never treats a missing record as evidence of no spawn.
func (m *Manager) SandboxStartReceipt(attemptID string) (*sandbox.StartReceipt, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	record := m.sandboxStarts[attemptID]
	if record == nil || !record.completed {
		return nil, sandbox.ErrInvalidPolicy
	}
	return record.receipt()
}

func (record *sandboxStartRecord) receipt() (*sandbox.StartReceipt, error) {
	receipt := &sandbox.StartReceipt{Version: 1, AttemptID: record.launch.AttemptID, Generation: record.launch.Generation, PolicyDigest: record.launch.PolicyDigest, Outcome: sandbox.StartNotStarted}
	if record.dispatched {
		if sandbox.RetireDescendants(context.Background(), record.launch, record.owner) != nil {
			return nil, sandbox.ErrInvalidPolicy
		}
		copied := *record.owner
		receipt.Owner = &copied
		receipt.Outcome = sandbox.StartTerminated
	}
	if receipt.ValidateStopped(record.launch) != nil {
		return nil, sandbox.ErrInvalidPolicy
	}
	return receipt, nil
}
