package github

import "context"

func (s *Service) validateIssueWatchHost(ctx context.Context, watch *IssueWatch) error {
	expected := watch.GitHubHost
	if expected != "" {
		return s.validateExpectedGitHubHost(withExpectedGitHubHost(ctx, expected), watch.WorkspaceID)
	}
	if watch.ID != "" && s.store != nil {
		stored, err := s.store.GetIssueWatch(ctx, watch.ID)
		if err != nil {
			return err
		}
		if stored != nil && stored.GitHubHost == "" {
			return ErrGitHubHostMismatch
		}
	}
	return s.validateExpectedGitHubHost(withExpectedGitHubHost(ctx, defaultGitHubHost), watch.WorkspaceID)
}

func (s *Service) validateReviewWatchHost(ctx context.Context, watch *ReviewWatch) error {
	expected := watch.GitHubHost
	if expected != "" {
		return s.validateExpectedGitHubHost(withExpectedGitHubHost(ctx, expected), watch.WorkspaceID)
	}
	if watch.ID != "" && s.store != nil {
		stored, err := s.store.GetReviewWatch(ctx, watch.ID)
		if err != nil {
			return err
		}
		if stored != nil && stored.GitHubHost == "" {
			return ErrGitHubHostMismatch
		}
	}
	return s.validateExpectedGitHubHost(withExpectedGitHubHost(ctx, defaultGitHubHost), watch.WorkspaceID)
}

func (s *Service) watchEventIdentity(ctx context.Context, workspaceID, watchHost string, fence *githubOperationFence) (string, int64, error) {
	if err := s.validateExpectedGitHubHost(withExpectedGitHubHost(ctx, defaultAuthFlowHost(watchHost)), workspaceID); err != nil {
		return "", 0, err
	}
	if fence != nil {
		ctx = context.WithValue(ctx, githubOperationFenceKey{}, *fence)
		if err := requireGitHubOperationCurrent(ctx); err != nil {
			return "", 0, err
		}
		generation := int64(0)
		if fence.resolved.credential != nil {
			generation = fence.resolved.credential.CredentialGeneration
		}
		return fence.resolved.host().String(), generation, nil
	}
	if defaultAuthFlowHost(watchHost) != defaultGitHubHost {
		return "", 0, ErrGitHubHostMismatch
	}
	if s.store != nil {
		connection, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
		if err != nil {
			return "", 0, err
		}
		if connection != nil {
			return defaultGitHubHost, connection.CredentialGeneration, nil
		}
	}
	return defaultGitHubHost, 0, nil
}

type githubWatchConnectionKey struct{}
type githubWatchConnection struct {
	host       string
	generation int64
}

// WithGitHubWatchConnection carries the serialized provider authority of a watch event.
func WithGitHubWatchConnection(ctx context.Context, host string, generation int64) context.Context {
	return context.WithValue(ctx, githubWatchConnectionKey{}, githubWatchConnection{host: host, generation: generation})
}

func (s *Service) validateWatchReservation(ctx context.Context, workspaceID, watchHost string) error {
	identity, _ := ctx.Value(githubWatchConnectionKey{}).(githubWatchConnection)
	if identity.host == "" {
		identity.host = defaultGitHubHost
	}
	if defaultAuthFlowHost(watchHost) != identity.host {
		return ErrGitHubHostMismatch
	}
	if err := s.validateExpectedGitHubHost(withExpectedGitHubHost(ctx, identity.host), workspaceID); err != nil {
		return err
	}
	if identity.generation > 0 {
		connection, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
		if err != nil {
			return err
		}
		if connection == nil || connection.CredentialGeneration != identity.generation {
			return ErrGitHubHostMismatch
		}
	}
	return nil
}

func requireCleanupResourceHost(ctx context.Context, host, recordID string) error {
	fence, ok := ctx.Value(githubOperationFenceKey{}).(githubOperationFence)
	if !ok {
		return nil
	}
	if host == "" && recordID != "" {
		return ErrGitHubHostMismatch
	}
	if defaultAuthFlowHost(host) != fence.resolved.host().String() {
		return ErrGitHubHostMismatch
	}
	return requireGitHubOperationCurrent(ctx)
}

func (s *Service) prepareWatchCreation(ctx context.Context, workspaceID string) (context.Context, string, func(), error) {
	ctx, unlock, err := s.lockResourceMutation(ctx, workspaceID)
	if err != nil {
		return ctx, "", nil, err
	}
	if err := s.validateExpectedGitHubHost(ctx, workspaceID); err != nil {
		unlock()
		return ctx, "", nil, err
	}
	host, err := s.selectedGitHubHost(ctx, workspaceID)
	if err != nil {
		unlock()
		return ctx, "", nil, err
	}
	return ctx, host.String(), unlock, nil
}
