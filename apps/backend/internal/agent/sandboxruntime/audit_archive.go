package sandboxruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (p *Prepared) retainFinalAudit(ctx context.Context, attempt *policy.Attempt, archive *contract.AuditArchive) (*policy.FinalAuditReceipt, error) {
	binding, err := policy.AuditBindingDigest(attempt)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(p.manager.base, "audit-archives")
	if err = os.Mkdir(base, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	if _, err = contract.StoreAuditArchive(ctx, base, binding, archive); err != nil {
		return nil, err
	}
	return &policy.FinalAuditReceipt{BindingDigest: binding, ArtifactSetDigest: archive.Digest, CapturedAt: time.Now().UTC()}, nil
}

func (m *Manager) finalAuditReceipt(ctx context.Context, attempt *policy.Attempt) (*policy.FinalAuditReceipt, error) {
	report, err := m.policies.ReadCapturedReport(ctx, attempt.Snapshot.OrgID, attempt.ID)
	if errors.Is(err, policy.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return report.Data.FinalAudit, nil
}
