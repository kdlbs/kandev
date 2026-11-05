package sandboxruntime

import (
	"context"
	"errors"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Manager) runAuditJob(ctx context.Context, owned *auditJob) {
	defer m.audit.wait.Done()
	defer close(owned.done)
	defer func() { m.mu.Lock(); delete(m.audit.jobs, owned.id); m.mu.Unlock() }()
	defer owned.cancel()
	job, err := m.policies.ClaimAuditVerification(ctx, owned.orgID, owned.id, m.audit.epoch, 1)
	var result *policy.AuditVerificationResult
	if err == nil {
		result, err = m.verifyCapturedAudit(ctx, owned.orgID, job)
	}
	if ctx.Err() != nil {
		result, err = nil, ctx.Err()
	}
	write, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err == nil {
		_, err = m.policies.FinishAuditVerification(write, owned.orgID, owned.id, m.audit.epoch, job.Version, result, "")
		if err == nil {
			return
		}
	}
	_, saveErr := m.policies.FailOwnedAuditVerification(write, owned.orgID, owned.id, m.audit.epoch, auditFailureCode(err))
	if saveErr != nil && !errors.Is(saveErr, policy.ErrNotFound) {
		m.markAuditPersistenceFailed()
	}
}

func auditFailureCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded), status.Code(err) == codes.DeadlineExceeded:
		return "timeout"
	case errors.Is(err, plugins.ErrSandboxReportUnsupported), status.Code(err) == codes.Unimplemented:
		return "unsupported"
	case errors.Is(err, contract.ErrInvalidPolicy), errors.Is(err, policy.ErrConflict):
		return "invalid_evidence"
	default:
		return "unavailable"
	}
}
