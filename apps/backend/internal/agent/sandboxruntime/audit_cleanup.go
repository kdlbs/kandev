package sandboxruntime

import (
	"context"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
)

type auditAttemptKey struct{ org, attempt string }

// The audit admission fence must remain held until artifact removal finishes.
func (m *Manager) cancelAuditJobs(ctx context.Context, selected map[auditAttemptKey]bool) error {
	m.mu.Lock()
	var pending []<-chan struct{}
	for _, job := range m.audit.jobs {
		if selected[auditAttemptKey{org: job.orgID, attempt: job.attemptID}] {
			job.cancel()
			pending = append(pending, job.done)
		}
	}
	m.mu.Unlock()
	for _, done := range pending {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
		}
	}
	m.mu.Lock()
	failed := m.audit.persistenceFailed
	m.mu.Unlock()
	if failed {
		return policy.ErrConflict
	}
	return ctx.Err()
}
