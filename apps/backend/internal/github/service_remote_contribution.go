package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

var ErrInvalidRemoteContributionURL = errors.New("invalid GitHub pull request URL")

// ResolveRemoteContributionForWorkspace resolves an existing pull request
// using the workspace's provider credentials and returns only the immutable
// repository/ref identity needed by task creation.
func (s *Service) ResolveRemoteContributionForWorkspace(
	ctx context.Context, workspaceID, userID, rawURL string,
) (*taskmodels.RemoteContributionResolution, error) {
	owner, repo, number, err := parseGitHubContributionURL(rawURL)
	if err != nil {
		return nil, err
	}
	pr, err := s.GetPRForWorkspace(ctx, workspaceID, userID, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("resolve GitHub pull request: %w", err)
	}
	return buildGitHubRemoteContribution(rawURL, owner, repo, number, pr)
}

func parseGitHubContributionURL(rawURL string) (string, string, int, error) {
	if strings.TrimSpace(rawURL) != rawURL || rawURL == "" {
		return "", "", 0, ErrInvalidRemoteContributionURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", 0, ErrInvalidRemoteContributionURL
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" || !validGitHubPathPart(parts[0]) || !validGitHubPathPart(parts[1]) {
		return "", "", 0, ErrInvalidRemoteContributionURL
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number <= 0 {
		return "", "", 0, ErrInvalidRemoteContributionURL
	}
	return parts[0], parts[1], number, nil
}

func validGitHubPathPart(value string) bool {
	if value == "" || strings.ContainsAny(value, "/\\:@?#") {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return !strings.Contains(value, "..")
}

func buildGitHubRemoteContribution(rawURL, owner, repo string, number int, pr *PR) (*taskmodels.RemoteContributionResolution, error) {
	if err := validateGitHubContributionPR(owner, repo, number, pr); err != nil {
		return nil, err
	}
	sourceOwner, sourceName, sameRepository, err := githubContributionSource(pr, owner, repo)
	if err != nil {
		return nil, err
	}
	canonicalURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number)
	sourceRepository := taskmodels.RemoteContributionRepository{
		Host:       "github.com",
		Path:       sourceOwner + "/" + sourceName,
		RemoteURL:  fmt.Sprintf("https://github.com/%s/%s.git", sourceOwner, sourceName),
		ProviderID: positiveID(pr.HeadRepoID),
	}
	binding := taskmodels.RemoteContribution{
		Version:              taskmodels.RemoteContributionVersion,
		Provider:             taskmodels.RemoteContributionProviderGitHub,
		Kind:                 taskmodels.RemoteContributionKindPullRequest,
		CanonicalURL:         canonicalURL,
		Number:               number,
		State:                taskmodels.RemoteContributionStateOpen,
		BaseBranch:           pr.BaseBranch,
		HeadBranch:           pr.HeadBranch,
		HeadSHA:              strings.ToLower(pr.HeadSHA),
		SourceRepository:     sourceRepository,
		CollaborationAllowed: sameRepository || pr.MaintainerCanModify,
	}
	if err := binding.Validate(); err != nil {
		return nil, err
	}
	targetDefault := pr.BaseDefaultBranch
	if targetDefault == "" {
		targetDefault = pr.BaseBranch
	}
	return &taskmodels.RemoteContributionResolution{
		Binding:             binding,
		TargetProvider:      taskmodels.RemoteContributionProviderGitHub,
		TargetHost:          "https://github.com",
		TargetPath:          owner + "/" + repo,
		TargetProviderID:    positiveID(pr.BaseRepoID),
		TargetRemoteURL:     fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
		TargetDefaultBranch: targetDefault,
	}, nil
}

func validateGitHubContributionPR(owner, repo string, number int, pr *PR) error {
	if pr == nil {
		return errors.New("GitHub pull request response is empty")
	}
	if pr.Number != number || (pr.RepoOwner != "" && !strings.EqualFold(pr.RepoOwner, owner)) ||
		(pr.RepoName != "" && !strings.EqualFold(pr.RepoName, repo)) ||
		!strings.EqualFold(pr.BaseRepoOwner, owner) || !strings.EqualFold(pr.BaseRepoName, repo) {
		return errors.New("GitHub pull request identity changed during resolution")
	}
	if strings.ToLower(strings.TrimSpace(pr.State)) != taskmodels.RemoteContributionStateOpen {
		return fmt.Errorf("GitHub pull request #%d is not open", number)
	}
	if !securityutil.IsValidBranchName(pr.HeadBranch) || !securityutil.IsValidBaseBranchRef(pr.BaseBranch) ||
		!securityutil.LooksLikeCommitSHA(pr.HeadSHA) {
		return errors.New("GitHub pull request has unsafe or incomplete refs")
	}
	return nil
}

func githubContributionSource(pr *PR, owner, repo string) (string, string, bool, error) {
	sourceOwner := pr.HeadRepoOwner
	sourceName := pr.HeadRepoName
	if !validGitHubPathPart(sourceOwner) || !validGitHubPathPart(sourceName) {
		return "", "", false, errors.New("GitHub pull request source repository identity is invalid")
	}
	sameRepository := strings.EqualFold(sourceOwner, owner) && strings.EqualFold(sourceName, repo)
	if !sameRepository && !pr.MaintainerCanModify {
		return "", "", false, errors.New("GitHub pull request does not allow maintainers to modify the source branch")
	}
	return sourceOwner, sourceName, sameRepository, nil
}

func positiveID(id int64) string {
	if id <= 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}
