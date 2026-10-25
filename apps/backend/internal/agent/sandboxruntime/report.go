package sandboxruntime

import (
	"context"
	"errors"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
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
	data, readErr := p.readReportData(ctx)
	var expected int64
	if previous != nil {
		expected = previous.Version
	}
	if readErr != nil {
		data = unavailableReport(previous)
	}
	data.Finalized = attempt.State == policy.AttemptStopped || attempt.State == policy.AttemptFailed
	return p.manager.policies.CaptureReport(ctx, p.orgID, attempt.ID, attempt.Version, expected, data)
}

func unavailableReport(previous *policy.Report) policy.ReportData {
	data := policy.ReportData{Integrity: "unverified"}
	if previous != nil {
		data = previous.Data
	}
	data.Status = "unavailable"
	data.Complete = false
	return data
}
