package sandbox

import (
	"context"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

const auditHistoryPageSize = 20

type AuditVerificationPage struct {
	Verifications []*AuditVerificationView `json:"verifications"`
	NextCursor    string                   `json:"next_cursor,omitempty"`
}

func (s *Service) ListAuditVerifications(ctx context.Context, session, attempt, cursor string) (*AuditVerificationPage, error) {
	org, err := s.authorizeReport(ctx, session, attempt)
	if err != nil {
		return nil, err
	}
	query := `SELECT id FROM sandbox_audit_verifications WHERE org_id=? AND attempt_id=?`
	args := []any{org, attempt}
	if cursor != "" {
		if !validAuditUUID(cursor) {
			return nil, contract.ErrInvalidPolicy
		}
		view, readErr := s.ReadAuditVerification(ctx, session, attempt, cursor)
		if readErr != nil {
			return nil, readErr
		}
		stamp := view.Verification.RequestedAt.UnixMilli()
		query += ` AND (requested_at_ms<? OR (requested_at_ms=? AND id<?))`
		args = append(args, stamp, stamp, cursor)
	}
	query += ` ORDER BY requested_at_ms DESC,id DESC LIMIT 21`
	ids := []string{}
	if err = s.db.SelectContext(ctx, &ids, s.db.Rebind(query), args...); err != nil {
		return nil, err
	}
	page := &AuditVerificationPage{Verifications: []*AuditVerificationView{}}
	if len(ids) > auditHistoryPageSize {
		ids = ids[:auditHistoryPageSize]
		page.NextCursor = ids[len(ids)-1]
	}
	for _, id := range ids {
		view, readErr := s.ReadAuditVerification(ctx, session, attempt, id)
		if readErr != nil {
			return nil, readErr
		}
		page.Verifications = append(page.Verifications, view)
	}
	if _, err = s.authorizeReport(ctx, session, attempt); err != nil {
		return nil, err
	}
	return page, nil
}
