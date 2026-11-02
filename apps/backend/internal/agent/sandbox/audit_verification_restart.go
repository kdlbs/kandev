package sandbox

import (
	"context"
	"errors"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

type activeAuditVerification struct {
	OrgID, ID string
}

// InterruptAuditVerifications settles abandoned work without starting a verifier.
func (s *Service) InterruptAuditVerifications(ctx context.Context, epoch string) error {
	if !validAuditUUID(epoch) {
		return contract.ErrInvalidPolicy
	}
	for {
		page, err := s.activeAuditVerificationPage(ctx, epoch)
		if err != nil {
			return err
		}
		if len(page) == 0 {
			return nil
		}
		for _, record := range page {
			if err = s.interruptAuditVerification(ctx, record, epoch); err != nil {
				return err
			}
		}
	}
}

func (s *Service) interruptAuditVerification(ctx context.Context, item activeAuditVerification, epoch string) error {
	record, err := s.readAuditVerification(ctx, item.OrgID, item.ID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	job := record.Verification
	if record.OwnerEpoch == epoch || job.State != auditVerificationPending && job.State != auditVerificationRunning {
		return nil
	}
	_, err = s.FinishAuditVerification(ctx, item.OrgID, item.ID, record.OwnerEpoch, job.Version, nil, auditVerificationRestarted)
	return err
}

func (s *Service) activeAuditVerificationPage(ctx context.Context, epoch string) ([]activeAuditVerification, error) {
	rows, err := s.db.QueryxContext(ctx, s.db.Rebind(`SELECT org_id,id FROM sandbox_audit_verifications WHERE owner_epoch<>? AND state IN ('pending','running') ORDER BY org_id,id LIMIT 100`), epoch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []activeAuditVerification
	for rows.Next() {
		var item activeAuditVerification
		if err = rows.Scan(&item.OrgID, &item.ID); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
