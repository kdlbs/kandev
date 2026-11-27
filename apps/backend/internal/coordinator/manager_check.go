package coordinator

import "context"

// IsWorkspaceManager reports whether userID is an active user holding
// workspace.manage in workspaceID, the check the automatic class applies to its
// raiser. An error means the check could not decide.
func (s *Service) IsWorkspaceManager(ctx context.Context, workspaceID, userID string) (bool, error) {
	_, verdict, err := s.raiserContext(ctx, workspaceID, userID)
	if err != nil {
		return false, err
	}
	return verdict == raiserValid, nil
}
