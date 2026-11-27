package process

import "github.com/kandev/kandev/internal/agentctl/types"

// Completed enrichment advances the publication revision while retaining the
// revision of its basic evidence. A waiter can use that completion only when
// it belongs to the observation the waiter accepted.
func (wt *WorkspaceTracker) gitStatusDetailsWaitTarget(observed types.GitStatusUpdate) (types.GitStatusUpdate, string, *gitStatusEnrichmentJob, error) {
	wt.gitStatusEnrichmentMu.Lock()
	defer wt.gitStatusEnrichmentMu.Unlock()
	wt.mu.RLock()
	current := cloneGitStatusUpdate(wt.currentStatus)
	fingerprint := wt.gitStatusFingerprint
	sourceRevision := wt.gitStatusDetailSourceRevision
	wt.mu.RUnlock()

	completedObserved := current.DetailState == gitStatusDetailReady && sourceRevision == observed.SnapshotRevision
	if current.TrackerEpoch != observed.TrackerEpoch || (current.SnapshotRevision != observed.SnapshotRevision && !completedObserved) {
		return current, fingerprint, nil, errGitStatusEvidenceChanged
	}
	if current.DetailState == gitStatusDetailReady {
		return current, fingerprint, nil, nil
	}
	if current.DetailState != gitStatusDetailPending {
		return current, fingerprint, nil, errGitStatusDetailsUnavailable
	}
	job := wt.gitStatusEnrichmentForFingerprintLocked(fingerprint)
	if job == nil {
		return current, fingerprint, nil, errGitStatusDetailsUnavailable
	}
	return current, fingerprint, job, nil
}
