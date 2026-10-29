package sandbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (s *Service) initReports() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS sandbox_reports (
	org_id TEXT NOT NULL, attempt_id TEXT NOT NULL, session_id TEXT NOT NULL,
	version BIGINT NOT NULL, finalized INTEGER NOT NULL, content_json TEXT NOT NULL,
	PRIMARY KEY (org_id,attempt_id),
	FOREIGN KEY (org_id,attempt_id) REFERENCES sandbox_attempts(org_id,attempt_id) ON DELETE CASCADE
	)`)
	return err
}

// CaptureReport is an internal runtime write. The expected versions fence both
// process-state changes and later reads of the same audit source.
func (s *Service) CaptureReport(ctx context.Context, orgID, attemptID string, attemptVersion, expected int64, data ReportData) (*Report, error) {
	attempt, err := s.ReadAttempt(ctx, orgID, attemptID)
	if err != nil {
		return nil, err
	}
	if attempt.Version != attemptVersion || expected < 0 {
		return nil, ErrConflict
	}
	if data.Finalized && attempt.State != AttemptStopped && attempt.State != AttemptFailed {
		return nil, ErrConflict
	}
	resolved := attempt.Snapshot.Resolved
	report := Report{SchemaVersion: 1, AttemptID: attemptID, SessionID: attempt.Snapshot.SessionID,
		RevisionID: resolved.Revision.ID, PolicyDigest: resolved.Revision.Digest, CeilingDigest: resolved.Ceiling.Digest,
		Provider: attempt.Snapshot.Provider, Version: expected + 1, CapturedAt: time.Now().UTC(), Data: data}
	if !validReportData(data, report.CapturedAt) {
		return nil, contract.ErrInvalidPolicy
	}
	report.Digest, err = reportDigest(report)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(report)
	if err != nil || len(raw) > 256<<10 {
		return nil, contract.ErrInvalidPolicy
	}
	if err = s.insertReport(ctx, orgID, attemptVersion, expected, &report, raw); err != nil {
		return nil, err
	}
	return s.ReadCapturedReport(ctx, orgID, attemptID)
}

func (s *Service) insertReport(ctx context.Context, orgID string, attemptVersion, expected int64, report *Report, raw []byte) error {
	finalized := 0
	if report.Data.Finalized {
		finalized = 1
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO sandbox_reports (org_id,attempt_id,session_id,version,finalized,content_json)
	SELECT ?,?,?,?,?,? WHERE EXISTS (SELECT 1 FROM sandbox_attempts WHERE org_id=? AND attempt_id=? AND version=?)
	AND (?=0 OR EXISTS (SELECT 1 FROM sandbox_reports WHERE org_id=? AND attempt_id=? AND version=?))
	ON CONFLICT (org_id,attempt_id) DO UPDATE SET version=excluded.version,finalized=excluded.finalized,content_json=excluded.content_json
	WHERE sandbox_reports.version=? AND sandbox_reports.finalized=0`),
		orgID, report.AttemptID, report.SessionID, report.Version, finalized, string(raw),
		orgID, report.AttemptID, attemptVersion, expected, orgID, report.AttemptID, expected, expected)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	return nil
}

// ReadCapturedReport is for trusted runtime callers. Public reads use ReadReport.
func (s *Service) ReadCapturedReport(ctx context.Context, orgID, attemptID string) (*Report, error) {
	var raw string
	var version int64
	var finalized int
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT version,finalized,content_json FROM sandbox_reports WHERE org_id=? AND attempt_id=?`), orgID, attemptID).Scan(&version, &finalized, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var report Report
	if len(raw) > 256<<10 || json.Unmarshal([]byte(raw), &report) != nil || !validReportData(report.Data, report.CapturedAt) {
		return nil, ErrConflict
	}
	attempt, err := s.ReadAttempt(ctx, orgID, attemptID)
	if err != nil {
		return nil, err
	}
	if !validStoredReport(&report, version, finalized) || !reportMatchesAttempt(&report, attempt) {
		return nil, ErrConflict
	}
	return &report, nil
}

func (s *Service) ReadReport(ctx context.Context, sessionID, attemptID string) (*Report, error) {
	orgID, err := s.authorizeReport(ctx, sessionID, attemptID)
	if err != nil {
		return nil, err
	}
	return s.ReadCapturedReport(ctx, orgID, attemptID)
}

func (s *Service) authorizeReport(ctx context.Context, sessionID, attemptID string) (string, error) {
	identity, err := s.approvalReader(ctx)
	if err != nil {
		return "", err
	}
	if err = s.sessionAccess(ctx, sessionID); err != nil {
		return "", err
	}
	attempt, err := s.ReadAttempt(ctx, identity.OrgID, attemptID)
	if err != nil {
		return "", err
	}
	if attempt.Snapshot.SessionID != sessionID {
		return "", ErrNotFound
	}
	if err = s.workspaceAccess(ctx, attempt.Snapshot.WorkspaceID); err != nil {
		return "", err
	}
	return identity.OrgID, nil
}
