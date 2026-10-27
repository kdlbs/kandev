package sandbox

import "context"

// ResolveRestart retains a stopped session's policy. Mutable defaults never
// replace the captured revision during a restart.
func (s *Service) ResolveRestart(ctx context.Context, selection SelectionContext, sessionID, workDir, attemptID string) (*Attempt, error) {
	if !s.enabled {
		return nil, ErrDisabled
	}
	previous, err := s.ReadAttempt(ctx, selection.OrgID, attemptID)
	if err != nil {
		return nil, err
	}
	if !restartScopeMatches(previous, selection, sessionID, workDir) {
		return nil, ErrConflict
	}
	if err = s.ValidateAttemptAuthority(ctx, previous); err != nil {
		return nil, err
	}
	if err = s.validateResolvedGrants(previous.Snapshot.Resolved.Revision, previous.Snapshot.Resolved.Ceiling); err != nil {
		return nil, err
	}
	if selection.TaskID != "" {
		guard, _, readErr := s.readTaskGuard(ctx, selection.OrgID, selection.TaskID, selection.WorkspaceID)
		if readErr != nil {
			return nil, readErr
		}
		if guard == nil || !noBroader(previous.Snapshot.Resolved, guard) {
			return nil, ErrIneligible
		}
	}
	return previous, nil
}

func restartScopeMatches(previous *Attempt, selection SelectionContext, sessionID, workDir string) bool {
	snapshot := previous.Snapshot
	return previous.State == AttemptStopped && sessionID != "" && snapshot.SessionID == sessionID &&
		snapshot.WorkspaceID == selection.WorkspaceID && snapshot.TaskID == selection.TaskID &&
		snapshot.AgentProfileID == selection.AgentProfileID && snapshot.Resolved.ExecutorID == selection.ExecutorID && snapshot.WorkDir == workDir
}

func (s *Service) resolveCapture(ctx context.Context, request CaptureRequest) (*Resolved, *Attempt, error) {
	if request.RestartOf == "" {
		resolved, err := s.Resolve(ctx, request.Selection)
		return resolved, nil, err
	}
	previous, err := s.ResolveRestart(ctx, request.Selection, request.SessionID, request.WorkDir, request.RestartOf)
	if err != nil {
		return nil, nil, err
	}
	if previous.Snapshot.Provider != request.Provider {
		return nil, nil, ErrConflict
	}
	return previous.Snapshot.Resolved, previous, nil
}
