package sandboxruntime

import (
	"context"
	"github.com/kandev/kandev/internal/auth/authn"
	"sync"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
)

type auditJobState struct {
	admission         sync.Mutex
	wait              sync.WaitGroup
	epoch             string
	ready             bool
	jobs              map[string]*auditJob
	persistenceFailed bool
}

type auditJob struct {
	orgID, attemptID, id string
	cancel               context.CancelFunc
	done                 chan struct{}
}

const maximumAuditJobs = 4

func (m *Manager) InitializeAuditVerifications(ctx context.Context) error {
	m.audit.admission.Lock()
	defer m.audit.admission.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return policy.ErrConflict
	}
	if m.audit.ready {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	if m.policies == nil {
		return policy.ErrIneligible
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := m.policies.InterruptAuditVerifications(ctx, m.audit.epoch); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return policy.ErrConflict
	}
	m.audit.ready = true
	return nil
}

func (m *Manager) EnqueueAuditVerification(ctx context.Context, sessionID, attemptID string, request policy.AuditVerificationRequest) (*policy.AuditVerification, error) {
	m.audit.admission.Lock()
	defer m.audit.admission.Unlock()
	m.mu.Lock()
	ready := !m.closed && m.audit.ready && !m.audit.persistenceFailed
	m.mu.Unlock()
	if !ready {
		return nil, policy.ErrIneligible
	}
	job, err := m.policies.BeginAuditVerification(ctx, sessionID, attemptID, request, m.audit.epoch)
	if err != nil {
		return nil, err
	}
	identity, _ := authn.IdentityFromContext(ctx)
	return m.admitAuditJob(identity.OrgID, job)
}

func (m *Manager) admitAuditJob(org string, job *policy.AuditVerification) (*policy.AuditVerification, error) {
	m.mu.Lock()
	if job.State != "pending" || m.audit.jobs[job.ID] != nil {
		m.mu.Unlock()
		return job, nil
	}
	failure := ""
	if m.closed || m.audit.persistenceFailed {
		failure = "cancelled"
	} else if len(m.audit.jobs) >= maximumAuditJobs {
		failure = "limit_exceeded"
	}
	if failure != "" {
		m.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		failed, err := m.policies.FailOwnedAuditVerification(ctx, org, job.ID, m.audit.epoch, failure)
		if err != nil {
			m.markAuditPersistenceFailed()
		}
		return failed, err
	}
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	owned := &auditJob{orgID: org, attemptID: job.AttemptID, id: job.ID, cancel: cancel, done: make(chan struct{})}
	m.audit.jobs[job.ID] = owned
	m.audit.wait.Add(1)
	m.mu.Unlock()
	go m.runAuditJob(ctx, owned)
	return job, nil
}

func (m *Manager) markAuditPersistenceFailed() {
	m.mu.Lock()
	m.audit.persistenceFailed = true
	m.mu.Unlock()
}
