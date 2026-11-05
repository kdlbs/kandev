package sandboxruntime

import (
	"context"
	"errors"
	"path/filepath"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (m *Manager) finishRecoveredArtifacts(ctx context.Context, attempt *policy.Attempt, mustCapture bool) error {
	root, err := m.validateRecoveredRoot(attempt.Snapshot.Launch)
	if err != nil {
		return err
	}
	if root != nil {
		if err = canFinalizeRecovered(attempt); err != nil {
			return err
		}
	}
	if root != nil || mustCapture {
		if err = m.captureRecoveredReport(ctx, attempt, root != nil); err != nil {
			return err
		}
	}
	if root == nil {
		return nil
	}
	return m.removeRecoveredFiles(attempt.Snapshot.Launch)
}

func canFinalizeRecovered(attempt *policy.Attempt) error {
	if attempt.Owner != nil && !recoveredSessionTerminated(attempt.Owner) {
		return policy.ErrConflict
	}
	containment := attempt.Snapshot.Launch.Containment
	if containment == nil {
		return policy.ErrConflict
	}
	empty, err := containment.Empty()
	if err != nil || !empty {
		return policy.ErrConflict
	}
	return nil
}

func (m *Manager) captureRecoveredReport(ctx context.Context, attempt *policy.Attempt, hasRoot bool) error {
	previous, err := m.policies.ReadCapturedReport(ctx, attempt.Snapshot.OrgID, attempt.ID)
	if err != nil && !errors.Is(err, policy.ErrNotFound) {
		return err
	}
	if previous != nil && previous.Data.Finalized {
		return nil
	}
	data := unavailableReport(previous)
	data.RefreshError = contract.FactUnavailable
	if hasRoot && len(attempt.Snapshot.AuditSources) > 0 && attempt.Owner != nil && attempt.Owner.ProviderSessionID != "" {
		data.FinalAudit, err = m.freezeRecoveredAudit(ctx, attempt, previous)
		if err != nil {
			data.RefreshError = reportRefreshError(err)
		}
	}
	var expected int64
	if previous != nil {
		expected = previous.Version
	}
	_, err = m.policies.CaptureReport(ctx, attempt.Snapshot.OrgID, attempt.ID, attempt.Version, expected, settleReport(data, true))
	return err
}

func (m *Manager) freezeRecoveredAudit(ctx context.Context, attempt *policy.Attempt, previous *policy.Report) (*policy.FinalAuditReceipt, error) {
	root := attempt.Snapshot.Launch.Root
	archive, err := contract.FreezeAuditSources(ctx, filepath.Join(root, "state"), filepath.Join(root, "tmp"), attempt.Snapshot.AuditSources, previousAuditSources(previous))
	if err != nil {
		return nil, err
	}
	defer func() { _ = archive.Close() }()
	if len(archive.Sources) == 0 {
		return nil, nil
	}
	return m.retainFinalAudit(ctx, attempt, archive)
}
