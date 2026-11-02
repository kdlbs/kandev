package sandboxruntime

import (
	"context"
	"errors"
	"github.com/kdlbs/kandev/pluginsdk"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (m *Manager) RefreshReport(ctx context.Context, orgID, attemptID string) (*policy.Report, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	prepared, err := m.retained(orgID, attemptID)
	if err != nil {
		return m.policies.ReadCapturedReport(ctx, orgID, attemptID)
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.closed {
		return m.policies.ReadCapturedReport(ctx, orgID, attemptID)
	}
	return prepared.captureReport(ctx)
}

func (p *Prepared) captureReport(ctx context.Context) (*policy.Report, error) {
	previous, err := p.manager.policies.ReadCapturedReport(ctx, p.orgID, p.launch.AttemptID)
	if err != nil && !errors.Is(err, policy.ErrNotFound) {
		return nil, err
	}
	if previous != nil && previous.Data.Finalized {
		return previous, nil
	}
	attempt, err := p.manager.policies.ReadAttempt(ctx, p.orgID, p.launch.AttemptID)
	if err != nil {
		return nil, err
	}
	var data policy.ReportData
	var readErr error
	if p.typedReports() {
		data, readErr = p.readTypedReport(ctx, attempt, previous)
	} else {
		data, readErr = p.readReportData(ctx)
	}
	var expected int64
	if previous != nil {
		expected = previous.Version
	}
	if readErr != nil {
		data = unavailableReport(previous)
		data.RefreshError = reportRefreshError(readErr)
	} else if data.Status == reportStatusAvailable {
		now := time.Now().UTC()
		data.LastSuccessfulReadAt = &now
	}
	data = settleReport(data, attempt.State == policy.AttemptStopped || attempt.State == policy.AttemptFailed)
	return p.manager.policies.CaptureReport(ctx, p.orgID, attempt.ID, attempt.Version, expected, data)
}

func unavailableReport(previous *policy.Report) policy.ReportData {
	data := policy.ReportData{Integrity: pluginsdk.SandboxReportUnverified}
	if previous != nil {
		data = previous.Data
		if data.LastSuccessfulReadAt == nil && previous.Data.Status == reportStatusAvailable {
			captured := previous.CapturedAt
			data.LastSuccessfulReadAt = &captured
		}
	}
	data.Status = contract.FactUnavailable
	data.Complete = false
	return data
}

func settleReport(data policy.ReportData, terminal bool) policy.ReportData {
	data.Finalized = terminal
	if terminal && data.V2 != nil && data.V2.Finalization == "live" {
		settled := *data.V2
		settled.Finalization = "interrupted"
		data.V2 = &settled
	}
	return data
}
