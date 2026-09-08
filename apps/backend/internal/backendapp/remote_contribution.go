package backendapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/models"
)

type remoteContributionCoordinator struct {
	github *github.Service
	gitlab *gitlab.Service
}

func newRemoteContributionCoordinator(gh *github.Service, gl *gitlab.Service) *remoteContributionCoordinator {
	return &remoteContributionCoordinator{github: gh, gitlab: gl}
}

func (c *remoteContributionCoordinator) Resolve(
	ctx context.Context, workspaceID, userID, rawURL string,
) (*models.RemoteContributionResolution, bool, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, false, err
	}
	path := strings.Trim(parsed.Path, "/")
	parts := strings.Split(path, "/")
	if strings.EqualFold(parsed.Hostname(), "github.com") && len(parts) == 4 && strings.EqualFold(parts[2], "pull") {
		if c.github == nil {
			return nil, true, errors.New("GitHub integration is not configured")
		}
		resolution, err := c.github.ResolveRemoteContributionForWorkspace(ctx, workspaceID, userID, rawURL)
		return resolution, true, err
	}
	if strings.Contains(path, "/-/merge_requests/") {
		if c.gitlab == nil {
			return nil, true, errors.New("GitLab integration is not configured")
		}
		resolution, err := c.gitlab.ResolveRemoteContributionForWorkspace(ctx, workspaceID, rawURL)
		return resolution, true, err
	}
	return nil, false, nil
}

// Associate is a pure pass-through into the provider-specific association
// call. `repositoryID` is repositories.ID, NOT a task_repositories row id —
// see AssociatePRWithTask's doc comment for why the two must not be
// confused.
func (c *remoteContributionCoordinator) Associate(
	ctx context.Context, workspaceID, userID, taskID, repositoryID string,
	resolution *models.RemoteContributionResolution,
) error {
	if resolution == nil {
		return errors.New("remote contribution resolution is required")
	}
	switch resolution.Binding.Provider {
	case models.RemoteContributionProviderGitHub:
		if c.github == nil {
			return errors.New("GitHub integration is not configured")
		}
		_, err := c.github.AssociateExistingPRByURLForWorkspace(
			ctx, workspaceID, userID, taskID, repositoryID, resolution.Binding.CanonicalURL,
		)
		return err
	case models.RemoteContributionProviderGitLab:
		if c.gitlab == nil {
			return errors.New("GitLab integration is not configured")
		}
		_, err := c.gitlab.AssociateExistingMRByURL(
			ctx, workspaceID, taskID, repositoryID, resolution.Binding.CanonicalURL,
		)
		return err
	default:
		return fmt.Errorf("unsupported remote contribution provider %q", resolution.Binding.Provider)
	}
}
