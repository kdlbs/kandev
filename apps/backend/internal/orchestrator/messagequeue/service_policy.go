package messagequeue

import "time"
import "context"

import "go.uber.org/zap"

func (s *Service) loadGlobalQueuePolicy(ctx context.Context) (GlobalQueuePolicy, error) {
	if loader := s.globalLoader.Load(); loader != nil {
		policy, err := loader.load(ctx)
		if err != nil {
			return GlobalQueuePolicy{}, err
		}
		s.SetGlobalQueuePolicy(policy)
		policy.AutoMerge.Source = AutoMergeSourceGlobal
		return policy, nil
	}
	return s.globalQueuePolicy(), nil
}

func resolveAutoMergeOverride(override *AutoMergeOverride, global AutoMergePolicy) AutoMergePolicy {
	if override != nil {
		return AutoMergePolicy{Enabled: override.Enabled, Source: AutoMergeSourceSession, Revision: override.Revision}
	}
	return global
}

func (s *Service) setSessionPolicy(ctx context.Context, identity QueueSessionIdentity, enabled bool) (SessionPolicyResult, error) {
	var result SessionPolicyResult
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(ctx context.Context) error {
		global, err := s.loadGlobalQueuePolicy(ctx)
		if err != nil {
			return err
		}
		result, err = s.repo.SetSessionPolicyAndCompact(ctx, identity, SessionPolicyChange{
			Enabled: enabled, Leased: s.sessionEditLeases(identity.SessionID),
		}, global)
		if err == nil && result.compactionRan {
			s.logger.Info("compacted session queue", zap.String("session_id", identity.SessionID), zap.Int("compacted_count", result.CompactedCount), zap.String("trigger", "auto_merge"))
		}
		return err
	})
	return result, err
}

func (s *Service) sessionEditLeases(sessionID string) map[string]struct{} {
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	var leased map[string]struct{}
	now := time.Now().UTC()
	for key := range s.editLeases {
		if key.sessionID != sessionID {
			continue
		}
		s.expireEditLeaseLocked(key, now)
		if s.editLeases[key] == nil {
			continue
		}
		if leased == nil {
			leased = make(map[string]struct{})
		}
		leased[key.entryID] = struct{}{}
	}
	return leased
}
