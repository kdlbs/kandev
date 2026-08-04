package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

var ErrInvalidRemoteContributionURL = errors.New("invalid GitLab merge request URL")

var gitLabProjectPathPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*$`)

func (s *Service) ResolveRemoteContributionForWorkspace(
	ctx context.Context, workspaceID, rawURL string,
) (*taskmodels.RemoteContributionResolution, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	projectPath, iid, err := parseGitLabContributionURL(rawURL, client.Host())
	if err != nil {
		return nil, err
	}
	mr, err := client.GetMR(ctx, projectPath, iid)
	if err != nil {
		return nil, fmt.Errorf("resolve GitLab merge request: %w", err)
	}
	return buildGitLabRemoteContribution(rawURL, client.Host(), projectPath, iid, mr)
}

func parseGitLabContributionURL(rawURL, configuredHost string) (string, int, error) {
	if strings.TrimSpace(rawURL) != rawURL || rawURL == "" {
		return "", 0, ErrInvalidRemoteContributionURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", 0, ErrInvalidRemoteContributionURL
	}
	projectPath, iid, err := parseMRURLForHost(rawURL, configuredHost)
	if err != nil {
		return "", 0, ErrInvalidRemoteContributionURL
	}
	return projectPath, iid, nil
}

func buildGitLabRemoteContribution(rawURL, configuredHost, projectPath string, iid int, mr *MR) (*taskmodels.RemoteContributionResolution, error) {
	if err := validateGitLabContributionMR(projectPath, iid, mr); err != nil {
		return nil, err
	}
	sourcePath, sameProject, err := gitLabContributionSource(projectPath, mr)
	if err != nil {
		return nil, err
	}
	hostOrigin, host, err := gitLabContributionHost(configuredHost)
	if err != nil {
		return nil, err
	}
	canonicalURL := fmt.Sprintf("%s/%s/-/merge_requests/%d", strings.TrimRight(hostOrigin, "/"), projectPath, iid)
	sourceRepository := taskmodels.RemoteContributionRepository{
		Host:       host,
		Path:       sourcePath,
		ProviderID: positiveGitLabID(mr.SourceProjectID),
		RemoteURL:  fmt.Sprintf("%s/%s.git", strings.TrimRight(hostOrigin, "/"), sourcePath),
	}
	binding := taskmodels.RemoteContribution{
		Version:              taskmodels.RemoteContributionVersion,
		Provider:             taskmodels.RemoteContributionProviderGitLab,
		Kind:                 taskmodels.RemoteContributionKindMergeRequest,
		CanonicalURL:         canonicalURL,
		Number:               iid,
		State:                taskmodels.RemoteContributionStateOpen,
		BaseBranch:           mr.BaseBranch,
		HeadBranch:           mr.HeadBranch,
		HeadSHA:              strings.ToLower(mr.HeadSHA),
		SourceRepository:     sourceRepository,
		CollaborationAllowed: sameProject || mr.AllowCollaboration,
	}
	if err := binding.Validate(); err != nil {
		return nil, err
	}
	targetDefault := mr.TargetDefaultBranch
	if targetDefault == "" {
		targetDefault = mr.BaseBranch
	}
	return &taskmodels.RemoteContributionResolution{
		Binding:             binding,
		TargetProvider:      taskmodels.RemoteContributionProviderGitLab,
		TargetHost:          hostOrigin,
		TargetPath:          projectPath,
		TargetProviderID:    positiveGitLabID(firstGitLabID(mr.TargetProjectID, mr.ProjectID)),
		TargetRemoteURL:     fmt.Sprintf("%s/%s.git", strings.TrimRight(hostOrigin, "/"), projectPath),
		TargetDefaultBranch: targetDefault,
	}, nil
}

func validateGitLabContributionMR(projectPath string, iid int, mr *MR) error {
	if mr == nil {
		return errors.New("GitLab merge request response is empty")
	}
	if mr.IID != iid {
		return errors.New("GitLab merge request identity changed during resolution")
	}
	targetPath := mr.TargetProjectPath
	if targetPath == "" {
		targetPath = mr.ProjectPath
	}
	if targetPath != "" && !strings.EqualFold(targetPath, projectPath) {
		return errors.New("GitLab merge request target project changed during resolution")
	}
	if strings.ToLower(strings.TrimSpace(mr.State)) != taskmodels.RemoteContributionStateOpen {
		return fmt.Errorf("GitLab merge request !%d is not open", iid)
	}
	if !securityutil.IsValidBranchName(mr.HeadBranch) || !securityutil.IsValidBaseBranchRef(mr.BaseBranch) ||
		!securityutil.LooksLikeCommitSHA(mr.HeadSHA) {
		return errors.New("GitLab merge request has unsafe or incomplete refs")
	}
	return nil
}

func gitLabContributionSource(projectPath string, mr *MR) (string, bool, error) {
	sourcePath := mr.SourceProjectPath
	if sourcePath == "" {
		sourcePath = projectPath
	}
	if !validGitLabProjectPath(sourcePath) || !validGitLabProjectPath(projectPath) {
		return "", false, errors.New("GitLab merge request repository identity is invalid")
	}
	sameProject := strings.EqualFold(sourcePath, projectPath)
	if !sameProject && !mr.AllowCollaboration {
		return "", false, errors.New("GitLab merge request does not allow maintainers to modify the source branch")
	}
	return sourcePath, sameProject, nil
}

func gitLabContributionHost(configuredHost string) (string, string, error) {
	hostOrigin, err := normalizeHostOrigin(configuredHost)
	if err != nil {
		return "", "", ErrInvalidRemoteContributionURL
	}
	parsedHost, err := url.Parse(hostOrigin)
	if err != nil || parsedHost.Host == "" {
		return "", "", ErrInvalidRemoteContributionURL
	}
	return hostOrigin, parsedHost.Host, nil
}

func validGitLabProjectPath(path string) bool {
	if path == "" || strings.ContainsAny(path, "\\:@?#") || strings.Contains(path, "..") ||
		strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || strings.Contains(path, "//") {
		return false
	}
	return gitLabProjectPathPattern.MatchString(path)
}

func firstGitLabID(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func positiveGitLabID(id int64) string {
	if id <= 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}
