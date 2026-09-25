package github

import (
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/authcircuit"
)

type reviewCleanupCircuits struct {
	mu         sync.Mutex
	workspaces map[string]authcircuit.State
	records    map[string]authcircuit.State
}

func newReviewCleanupCircuits() *reviewCleanupCircuits {
	return &reviewCleanupCircuits{
		workspaces: make(map[string]authcircuit.State),
		records:    make(map[string]authcircuit.State),
	}
}

func (c *reviewCleanupCircuits) workspaceOpen(workspaceID string, now time.Time) (bool, authcircuit.FailureClass) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.workspaces[workspaceID]
	return state.Open(now), state.FailureClass
}

func (c *reviewCleanupCircuits) recordOpen(recordID string, now time.Time) (bool, authcircuit.FailureClass) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.records[recordID]
	return state.Open(now), state.FailureClass
}

func (c *reviewCleanupCircuits) refreshWorkspaceFingerprint(workspaceID, fingerprint string) bool {
	return refreshReviewCleanupFingerprint(c.workspaces, c, workspaceID, fingerprint)
}

func (c *reviewCleanupCircuits) refreshRecordFingerprint(recordID, fingerprint string) bool {
	return refreshReviewCleanupFingerprint(c.records, c, recordID, fingerprint)
}

func refreshReviewCleanupFingerprint(
	states map[string]authcircuit.State,
	circuits *reviewCleanupCircuits,
	key, fingerprint string,
) bool {
	if key == "" || fingerprint == "" {
		return false
	}
	circuits.mu.Lock()
	defer circuits.mu.Unlock()
	state, exists := states[key]
	if !exists {
		return false
	}
	if state.ResetIfFingerprintChanged(fingerprint) {
		delete(states, key)
		return true
	}
	states[key] = state
	return false
}

func (c *reviewCleanupCircuits) recordWorkspaceOutcome(
	workspaceID, fingerprint string,
	class authcircuit.FailureClass,
	now time.Time,
) {
	if workspaceID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if class == authcircuit.FailureClassNone {
		delete(c.workspaces, workspaceID)
		return
	}
	state := c.workspaces[workspaceID]
	if fingerprint != "" {
		state.Fingerprint = fingerprint
	}
	state.RecordFailure(now, class, nil)
	c.workspaces[workspaceID] = state
}

func (c *reviewCleanupCircuits) recordRecordOutcome(
	recordID, fingerprint string,
	class authcircuit.FailureClass,
	now time.Time,
) {
	if recordID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if class == authcircuit.FailureClassNone {
		delete(c.records, recordID)
		return
	}
	state := c.records[recordID]
	if fingerprint != "" {
		state.Fingerprint = fingerprint
	}
	state.RecordFailure(now, class, nil)
	c.records[recordID] = state
}

func (c *reviewCleanupCircuits) pruneRecords(active map[string]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for recordID := range c.records {
		if _, exists := active[recordID]; !exists {
			delete(c.records, recordID)
		}
	}
}
