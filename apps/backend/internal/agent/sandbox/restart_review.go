package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// RestartReviewReceipt is server-owned evidence, captured before process dispatch.
type RestartReviewReceipt struct {
	OperatorID      string    `json:"operator_id"`
	SelectionDigest string    `json:"selection_digest"`
	ReviewedAt      time.Time `json:"reviewed_at"`
}

func resolvedReviewDigest(resolved *Resolved) (string, error) {
	raw, err := json.Marshal(resolved)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ResolveReviewedRestart admits only a current selection explicitly chosen by a
// browser operator. Automatic replacement uses ResolveRestart instead.
func (s *Service) ResolveReviewedRestart(ctx context.Context, selection SelectionContext, sessionID, workDir, attemptID string) (*Resolved, *Attempt, error) {
	identity, err := s.operator(ctx)
	if err != nil {
		return nil, nil, err
	}
	if identity.OrgID != selection.OrgID {
		return nil, nil, ErrAuthority
	}
	review := selection.RestartReview
	if review == nil || review.AttemptID != attemptID || !validApprovalDigest(review.SelectionDigest) {
		return nil, nil, ErrConflict
	}
	if err = s.sessionAccess(ctx, sessionID); err != nil {
		return nil, nil, err
	}
	if err = s.workspaceAccess(ctx, selection.WorkspaceID); err != nil {
		return nil, nil, err
	}
	previous, err := s.ReadAttempt(ctx, selection.OrgID, attemptID)
	if err != nil {
		return nil, nil, err
	}
	if !restartScopeMatches(previous, selection, sessionID, workDir) {
		return nil, nil, ErrConflict
	}
	selection.Prior = previous.Snapshot.Resolved
	resolved, err := s.Resolve(ctx, selection)
	if err != nil {
		return nil, nil, err
	}
	if resolved == nil || resolved.Revision.Policy.InstallationID != previous.Snapshot.Provider.InstallationID || resolved.Revision.Policy.ProviderKey != previous.Snapshot.Provider.Key {
		return nil, nil, ErrIneligible
	}
	if err = s.validateReviewedSelection(ctx, selection, resolved); err != nil {
		return nil, nil, err
	}
	return resolved, previous, nil
}

func (s *Service) restartReviewReceipt(ctx context.Context, selection SelectionContext) (*RestartReviewReceipt, error) {
	if selection.RestartReview == nil {
		return nil, nil
	}
	identity, err := s.operator(ctx)
	if err != nil || identity.OrgID != selection.OrgID {
		return nil, ErrAuthority
	}
	return &RestartReviewReceipt{OperatorID: identity.UserID, SelectionDigest: selection.RestartReview.SelectionDigest, ReviewedAt: time.Now().UTC()}, nil
}

func (s *Service) validateReviewedSelection(ctx context.Context, selection SelectionContext, resolved *Resolved) error {
	if selection.RestartReview == nil {
		return nil
	}
	identity, err := s.operator(ctx)
	if err != nil {
		return err
	}
	if identity.OrgID != selection.OrgID {
		return ErrAuthority
	}
	digest, err := resolvedReviewDigest(resolved)
	if err != nil {
		return err
	}
	if resolved == nil || digest != selection.RestartReview.SelectionDigest {
		return ErrConflict
	}
	return nil
}
