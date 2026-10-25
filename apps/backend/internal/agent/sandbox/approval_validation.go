package sandbox

import (
	"context"
	"encoding/hex"
	"slices"
	"strings"
	"time"
	"unicode"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (s *Service) validateApprovalCapture(attempt *Attempt, operation ApprovalOperation, digest string, deadline time.Time) error {
	if !s.enabled {
		return ErrDisabled
	}
	if attempt.State != AttemptRunning || attempt.Owner == nil || attempt.Owner.ProviderSessionID == "" || operation.ProviderSessionID != attempt.Owner.ProviderSessionID || operation.ChildPID != attempt.Owner.ChildPID {
		return ErrConflict
	}
	if !validApprovalOperation(operation) || !validApprovalDigest(digest) || !deadline.After(time.Now()) || deadline.After(time.Now().Add(time.Minute)) {
		return contract.ErrInvalidPolicy
	}
	return s.validateApprovalTarget(attempt, operation)
}

func validApprovalOperation(operation ApprovalOperation) bool {
	return operation.RequestID != "" && len(operation.RequestID) <= 256 && !strings.ContainsFunc(operation.RequestID, unicode.IsControl) &&
		(operation.Access == filesystemRead || operation.Access == filesystemWrite)
}

func validApprovalDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func (s *Service) validateApprovalTarget(attempt *Attempt, operation ApprovalOperation) error {
	roots := append(slices.Clone(s.protectedRoots), attempt.Snapshot.Launch.Root)
	return contract.ValidateFilesystemOperation(operation.Target, operation.Access == filesystemWrite, attempt.Snapshot.Ceiling, roots)
}

func (s *Service) admitApprovalGrant(ctx context.Context, approval *Approval) error {
	if !s.enabled {
		return ErrDisabled
	}
	if s.approvalGuard == nil {
		return ErrConflict
	}
	attempt, err := s.ReadAttempt(ctx, approval.Snapshot.OrgID, approval.Snapshot.AttemptID)
	if err != nil {
		return err
	}
	if attempt.State != AttemptRunning || attempt.Owner == nil || *attempt.Owner != approval.Snapshot.Owner {
		return ErrConflict
	}
	if err = s.ValidateAttemptAuthority(ctx, attempt); err != nil {
		return err
	}
	if err = s.validateApprovalTarget(attempt, approval.Snapshot.Operation); err != nil {
		return err
	}
	return s.approvalGuard(ctx, attempt, approval)
}
