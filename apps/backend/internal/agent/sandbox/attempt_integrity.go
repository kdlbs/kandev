package sandbox

import (
	"context"
	"slices"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func validStoredAttempt(attempt *Attempt, orgID, executionID string, generation uint64) bool {
	snapshot := attempt.Snapshot
	if snapshot.SchemaVersion != 1 || snapshot.OrgID != orgID || snapshot.ExecutionID != executionID || attempt.Version < 1 || attempt.UpdatedAt.IsZero() {
		return false
	}
	if !slices.Contains([]string{AttemptPrepared, AttemptStarting, AttemptRunning, AttemptStopped, AttemptFailed, AttemptRecoveryRequired}, attempt.State) {
		return false
	}
	launch, resolved := snapshot.Launch, snapshot.Resolved
	if !validStoredLaunch(launch, attempt.ID, generation) {
		return false
	}
	if !validStoredResolution(resolved, orgID, launch.PolicyDigest) {
		return false
	}
	if !validAttemptProvider(snapshot.Provider, resolved, launch) {
		return false
	}
	if attempt.Owner != nil && !completeAttemptOwner(attempt.Owner) {
		return false
	}
	return attempt.State != AttemptRunning || attempt.Owner != nil
}

func validStoredLaunch(launch *contract.Launch, attemptID string, generation uint64) bool {
	return launch != nil && launch.Version == 1 && launch.AttemptID == attemptID &&
		launch.Generation == generation && generation != 0 && validApprovalDigest(launch.PolicyDigest) && validApprovalDigest(launch.PackageDigest)
}

func validStoredResolution(resolved *Resolved, orgID, digest string) bool {
	return resolved != nil && resolved.Revision != nil && resolved.Ceiling != nil &&
		resolved.Revision.OrgID == orgID && resolved.Ceiling.OrgID == orgID && resolved.Revision.Digest == digest
}

// ValidateAttemptAuthority rechecks revocation without replacing captured policy.
func (s *Service) ValidateAttemptAuthority(ctx context.Context, attempt *Attempt) error {
	if !s.enabled {
		return ErrDisabled
	}
	if attempt == nil || attempt.Snapshot.Resolved == nil {
		return ErrConflict
	}
	for _, captured := range []*Revision{attempt.Snapshot.Resolved.Revision, attempt.Snapshot.Resolved.Ceiling} {
		if captured == nil {
			return ErrConflict
		}
		current, err := s.readRevision(ctx, attempt.Snapshot.OrgID, captured.ID)
		if err != nil {
			return err
		}
		if current.Revoked || current.Digest != captured.Digest {
			return ErrIneligible
		}
	}
	return nil
}
