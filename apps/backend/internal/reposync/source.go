package reposync

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/githubhost"
	"github.com/kandev/kandev/internal/common/securityutil"
)

// Supported repository providers.
const (
	ProviderGitHub = "github"
	ProviderGitLab = "gitlab"
)

// Defaults and bounds shared by repository-backed sync configurations.
const (
	DefaultBranch          = "main"
	DefaultIntervalSeconds = 300
	MinIntervalSeconds     = 60
	// MaxIntervalSeconds caps the poll interval at 30 days.
	MaxIntervalSeconds = 30 * 24 * 60 * 60
)

// Target is the user-supplied repository location of a sync source.
type Target struct {
	GitHubHost      string
	Provider        string
	RepoOwner       string
	RepoName        string
	ProjectPath     string
	Branch          string
	Path            string
	IntervalSeconds int
}

// Normalize validates the target and fills defaults. Validation failures wrap
// invalid, the calling feature's sentinel, so handlers can map them to 400.
func (t *Target) Normalize(defaultPath string, invalid error) error {
	t.Branch = strings.TrimSpace(t.Branch)
	t.Path = strings.Trim(strings.TrimSpace(t.Path), "/")
	if err := t.normalizeProvider(invalid); err != nil {
		return err
	}
	if t.Branch == "" {
		t.Branch = DefaultBranch
	}
	if !securityutil.IsValidBranchName(t.Branch) {
		return fmt.Errorf("%w: branch is not a valid git branch name", invalid)
	}
	if t.Path == "" {
		t.Path = defaultPath
	}
	for _, segment := range strings.Split(t.Path, "/") {
		if segment == ".." {
			return fmt.Errorf("%w: path cannot contain \"..\"", invalid)
		}
	}
	if t.IntervalSeconds == 0 {
		t.IntervalSeconds = DefaultIntervalSeconds
	}
	if t.IntervalSeconds < MinIntervalSeconds {
		return fmt.Errorf("%w: interval_seconds must be at least %d", invalid, MinIntervalSeconds)
	}
	if t.IntervalSeconds > MaxIntervalSeconds {
		return fmt.Errorf("%w: interval_seconds must be at most %d", invalid, MaxIntervalSeconds)
	}
	return nil
}

// normalizeProvider validates the coordinates for the selected provider and
// clears the other provider's fields, so a switch leaves no stale values.
func (t *Target) normalizeProvider(invalid error) error {
	t.Provider = strings.TrimSpace(t.Provider)
	if t.Provider == "" {
		t.Provider = ProviderGitHub
	}
	t.RepoOwner = strings.TrimSpace(t.RepoOwner)
	t.RepoName = strings.TrimSpace(t.RepoName)
	t.ProjectPath = strings.TrimSpace(t.ProjectPath)

	switch t.Provider {
	case ProviderGitHub:
		return t.normalizeGitHub(invalid)
	case ProviderGitLab:
		return t.normalizeGitLab(invalid)
	default:
		return fmt.Errorf("%w: provider must be %q or %q", invalid, ProviderGitHub, ProviderGitLab)
	}
}

func (t *Target) normalizeGitHub(invalid error) error {
	if t.GitHubHost == "" {
		t.GitHubHost = githubhost.Public().String()
	}
	host, err := githubhost.Parse(t.GitHubHost)
	if err != nil {
		return fmt.Errorf("%w: invalid github_host", invalid)
	}
	t.GitHubHost = host.String()
	if t.ProjectPath != "" {
		return fmt.Errorf("%w: project_path is only valid for the %q provider", invalid, ProviderGitLab)
	}
	if t.RepoOwner == "" || strings.ContainsAny(t.RepoOwner, "/ ") {
		return fmt.Errorf("%w: repo_owner is required and cannot contain slashes or spaces", invalid)
	}
	if t.RepoName == "" || strings.ContainsAny(t.RepoName, "/ ") {
		return fmt.Errorf("%w: repo_name is required and cannot contain slashes or spaces", invalid)
	}
	return nil
}

// normalizeGitLab requires a namespace path. GitLab projects live at
// arbitrarily nested paths, so the value is validated as a whole.
func (t *Target) normalizeGitLab(invalid error) error {
	t.GitHubHost = ""
	if t.RepoOwner != "" || t.RepoName != "" {
		return fmt.Errorf(
			"%w: repo_owner and repo_name are only valid for the %q provider", invalid, ProviderGitHub)
	}
	if t.ProjectPath == "" {
		return fmt.Errorf("%w: project_path is required", invalid)
	}
	if strings.Contains(t.ProjectPath, " ") {
		return fmt.Errorf("%w: project_path cannot contain spaces", invalid)
	}
	segments := strings.Split(t.ProjectPath, "/")
	if len(segments) < 2 {
		return fmt.Errorf(
			"%w: project_path must include the namespace, e.g. \"group/project\"", invalid)
	}
	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf(
				"%w: project_path cannot have empty segments or leading/trailing slashes", invalid)
		}
		if segment == "." || segment == ".." {
			return fmt.Errorf("%w: project_path cannot contain \".\" or \"..\" segments", invalid)
		}
	}
	return nil
}
