package messagequeue

import (
	"context"
	"errors"
)

type recoveryQueuePause struct {
	identity QueueSessionIdentity
	ownerID  string
}

// PauseAutoRunForRecovery remembers only a pause made by this recovery owner.
// The policy read and pause share admission with explicit policy writes.
func (s *Service) PauseAutoRunForRecovery(ctx context.Context, identity QueueSessionIdentity, ownerID string) (bool, error) {
	if ownerID == "" {
		return false, errors.New("queue recovery pause owner is required")
	}
	var paused bool
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		snapshot, err := s.repo.Snapshot(admittedCtx, identity)
		if err != nil {
			return err
		}
		if !snapshot.AutoRun {
			owner, exists := s.recoveryPauses.Load(identity.SessionID)
			paused = exists && owner == (recoveryQueuePause{identity, ownerID})
			return nil
		}
		paused, err = s.repo.PauseAutoRunIfPendingForSession(admittedCtx, identity)
		if err == nil && paused {
			s.recoveryPauses.Store(identity.SessionID, recoveryQueuePause{identity, ownerID})
		}
		return err
	})
	return paused, err
}

// ResumeAutoRunAfterRecovery restores a pause only while its exact owner still
// owns it. Explicit policy writes revoke ownership even when they write OFF again.
func (s *Service) ResumeAutoRunAfterRecovery(ctx context.Context, identity QueueSessionIdentity, ownerID string) (bool, error) {
	var resumed bool
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		owner, exists := s.recoveryPauses.Load(identity.SessionID)
		if !exists || owner != (recoveryQueuePause{identity, ownerID}) {
			return nil
		}
		snapshot, err := s.repo.Snapshot(admittedCtx, identity)
		if err != nil {
			return err
		}
		if snapshot.AutoRun {
			s.recoveryPauses.Delete(identity.SessionID)
			return nil
		}
		if err := s.repo.SetAutoRunForSession(admittedCtx, identity, true); err != nil {
			return err
		}
		s.recoveryPauses.Delete(identity.SessionID)
		resumed = true
		return nil
	})
	return resumed, err
}

func (s *Service) revokeRecoveryPause(sessionID string) {
	s.recoveryPauses.Delete(sessionID)
}
