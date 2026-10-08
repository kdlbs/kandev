package backendapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type repositorySelectionResolver struct {
	github      githubRepositorySelectionService
	gitlab      gitLabRepositorySelectionService
	azureDevOps azureDevOpsRepositorySelectionService
	plugins     repositoryProviderInspector
}

type githubRepositorySelectionService interface {
	InspectRepositoryForWorkspace(context.Context, string, string, string, string) (*github.GitHubRepository, error)
}

type gitLabRepositorySelectionService interface {
	ClientForWorkspaceHost(context.Context, string, string) (gitlab.Client, error)
}

type azureDevOpsRepositorySelectionService interface {
	ListProjectsForWorkspace(context.Context, string) ([]azuredevops.Project, error)
	ListRepositoriesForWorkspace(context.Context, string, string) ([]azuredevops.Repository, error)
}

type gitLabProjectByPathReader interface {
	GetProjectByPath(context.Context, string) (*gitlab.Project, error)
}

const (
	remoteSelectionHTTPScheme = "https"
	remoteSelectionHTTPS      = remoteSelectionHTTPScheme + "://"
	remoteSelectionGitHubHost = remoteSelectionHTTPS + gitCredentialGitHubHost
)

func (r repositorySelectionResolver) ResolveRepositorySelection(
	ctx context.Context, workspaceID string, input taskservice.TaskRepositoryInput,
) (taskservice.TaskRepositoryInput, error) {
	switch strings.ToLower(strings.TrimSpace(input.Provider)) {
	case "github":
		return r.inspectGitHub(ctx, workspaceID, input)
	case "gitlab":
		return r.inspectGitLab(ctx, workspaceID, input)
	case "azure_devops":
		return r.inspectAzureDevOps(ctx, workspaceID, input)
	default:
		return pluginRepositorySelectionResolver{inspector: r.plugins}.ResolveRepositorySelection(ctx, workspaceID, input)
	}
}

func (r repositorySelectionResolver) inspectGitHub(
	ctx context.Context, workspaceID string, input taskservice.TaskRepositoryInput,
) (taskservice.TaskRepositoryInput, error) {
	owner, name, requested, err := githubRepositorySelectionTarget(input.RemoteURL)
	if err != nil {
		return taskservice.TaskRepositoryInput{}, err
	}
	if r.github == nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorUnavailable, nil)
	}
	userID := repositorySelectionUserID(ctx)
	repository, err := r.github.InspectRepositoryForWorkspace(ctx, workspaceID, userID, owner, name)
	if err != nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(githubRepositorySelectionErrorCode(err), err)
	}
	if repository == nil || strings.TrimSpace(repository.CloneURL) == "" || strings.TrimSpace(repository.DefaultBranch) == "" {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorNotFound, nil)
	}
	cloneLocation, err := parseRemoteSelectionURL(repository.CloneURL)
	if err != nil || cloneLocation.host != gitCredentialGitHubHost || !remotePathsEqual(requested.path, cloneLocation.path, true) {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, err)
	}
	return taskservice.TaskRepositoryInput{
		RemoteURL: repository.CloneURL, Provider: "github", ProviderHost: remoteSelectionGitHubHost,
		ProviderRepoID: repository.FullName, ProviderOwner: repository.Owner,
		ProviderName: repository.Name, DefaultBranch: repository.DefaultBranch,
		TrustedProviderDescriptor: true,
	}, nil
}

func githubRepositorySelectionTarget(raw string) (string, string, remoteSelectionURL, error) {
	invalid := taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, nil)
	location, err := parseRemoteSelectionURL(raw)
	if err != nil || location.host != gitCredentialGitHubHost || len(location.path) != 2 {
		return "", "", remoteSelectionURL{}, invalid
	}
	owner, name := location.path[0], location.path[1]
	if owner == "" || name == "" {
		return "", "", remoteSelectionURL{}, invalid
	}
	return owner, name, location, nil
}

func repositorySelectionUserID(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if ok && strings.TrimSpace(identity.UserID) != "" {
		return identity.UserID
	}
	return github.DefaultUserID
}

func githubRepositorySelectionErrorCode(err error) taskservice.RepositorySelectionErrorCode {
	if errors.Is(err, github.ErrRepoNotResolvable) {
		return taskservice.RepositorySelectionErrorNotFound
	}
	return taskservice.RepositorySelectionErrorUnavailable
}

func (r repositorySelectionResolver) inspectGitLab(
	ctx context.Context, workspaceID string, input taskservice.TaskRepositoryInput,
) (taskservice.TaskRepositoryInput, error) {
	client, project, requestedPath, err := r.readGitLabRepositorySelection(ctx, workspaceID, input)
	if err != nil {
		return taskservice.TaskRepositoryInput{}, err
	}
	if project == nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorNotFound, nil)
	}
	if !validGitLabRepositorySelection(input.RemoteURL, requestedPath, project) {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, nil)
	}
	return taskservice.TaskRepositoryInput{
		RemoteURL: project.HTTPURLToRepo, Provider: "gitlab", ProviderHost: client.Host(),
		ProviderRepoID: strconv.FormatInt(project.ID, 10), ProviderOwner: project.Namespace,
		ProviderName: project.Path, DefaultBranch: project.DefaultBranch,
		TrustedProviderDescriptor: true,
	}, nil
}

func (r repositorySelectionResolver) readGitLabRepositorySelection(
	ctx context.Context, workspaceID string, input taskservice.TaskRepositoryInput,
) (gitlab.Client, *gitlab.Project, string, error) {
	location, err := parseRemoteSelectionURL(input.RemoteURL)
	if err != nil || len(location.path) < 2 {
		return nil, nil, "", taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, err)
	}
	if r.gitlab == nil {
		return nil, nil, "", taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorUnavailable, nil)
	}
	host := input.ProviderHost
	if strings.TrimSpace(host) == "" {
		host = location.origin
	}
	client, err := r.gitlab.ClientForWorkspaceHost(ctx, workspaceID, host)
	if err != nil {
		return nil, nil, "", taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorUnavailable, err)
	}
	if !sameOrigin(location.origin, client.Host()) {
		return nil, nil, "", taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, nil)
	}
	reader, ok := client.(gitLabProjectByPathReader)
	if !ok {
		return nil, nil, "", taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorUnavailable, nil)
	}
	requestedPath := strings.Join(location.path, "/")
	project, err := reader.GetProjectByPath(ctx, requestedPath)
	if err != nil {
		return nil, nil, "", taskservice.NewRepositorySelectionError(gitLabRepositorySelectionErrorCode(err), err)
	}
	return client, project, requestedPath, nil
}

func gitLabRepositorySelectionErrorCode(err error) taskservice.RepositorySelectionErrorCode {
	var apiErr *gitlab.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
		return taskservice.RepositorySelectionErrorNotFound
	}
	return taskservice.RepositorySelectionErrorUnavailable
}

func validGitLabRepositorySelection(rawURL, requestedPath string, project *gitlab.Project) bool {
	if project.PathWithNamespace != requestedPath || project.HTTPURLToRepo == "" || project.DefaultBranch == "" {
		return false
	}
	return sameRemoteURLLocation(rawURL, project.WebURL, false) ||
		sameRemoteURLLocation(rawURL, project.HTTPURLToRepo, false)
}

func (r repositorySelectionResolver) inspectAzureDevOps(
	ctx context.Context, workspaceID string, input taskservice.TaskRepositoryInput,
) (taskservice.TaskRepositoryInput, error) {
	location, sshURL, projectName, name, err := azureRepositorySelectionTarget(input.RemoteURL)
	if err != nil {
		return taskservice.TaskRepositoryInput{}, err
	}
	if r.azureDevOps == nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorUnavailable, nil)
	}
	projects, err := r.azureDevOps.ListProjectsForWorkspace(ctx, workspaceID)
	if err != nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorUnavailable, err)
	}
	project, err := findAzureSelectionProject(projects, projectName, input.ProviderScope)
	if err != nil {
		return taskservice.TaskRepositoryInput{}, err
	}
	if project == nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorNotFound, nil)
	}
	repositories, err := r.azureDevOps.ListRepositoriesForWorkspace(ctx, workspaceID, project.ID)
	if err != nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorUnavailable, err)
	}
	match, err := findAzureSelectionRepository(repositories, location, sshURL, project.ID, name, input.ProviderRepoID)
	if err != nil {
		return taskservice.TaskRepositoryInput{}, err
	}
	if match == nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorNotFound, nil)
	}
	if match.WebURL == "" || match.DefaultBranch == "" {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, nil)
	}
	canonicalLocation, err := parseRemoteSelectionURL(match.WebURL)
	if err != nil {
		return taskservice.TaskRepositoryInput{}, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, err)
	}
	return taskservice.TaskRepositoryInput{
		RemoteURL: match.WebURL, Provider: "azure_devops", ProviderHost: canonicalLocation.origin,
		ProviderScope: project.ID, ProviderRepoID: match.ID, ProviderOwner: project.Name,
		ProviderName: match.Name, DefaultBranch: strings.TrimPrefix(match.DefaultBranch, "refs/heads/"),
		TrustedProviderDescriptor: true,
	}, nil
}

func azureRepositorySelectionTarget(
	rawURL string,
) (remoteSelectionURL, bool, string, string, error) {
	location, err := parseRemoteSelectionURL(rawURL)
	sshURL := err == nil && location.host == "ssh.dev.azure.com" &&
		len(location.path) == 4 && strings.EqualFold(location.path[0], "v3")
	if err != nil || (!sshURL &&
		(len(location.path) < 4 || !strings.EqualFold(location.path[len(location.path)-2], "_git"))) {
		return remoteSelectionURL{}, false, "", "", taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, err)
	}
	projectName := location.path[len(location.path)-3]
	if sshURL {
		projectName = location.path[2]
	}
	return location, sshURL, projectName, location.path[len(location.path)-1], nil
}

func findAzureSelectionProject(
	projects []azuredevops.Project, name, requestedScope string,
) (*azuredevops.Project, error) {
	var match *azuredevops.Project
	for i := range projects {
		candidate := &projects[i]
		if !strings.EqualFold(candidate.Name, name) ||
			(requestedScope != "" && candidate.ID != requestedScope) {
			continue
		}
		if match != nil {
			return nil, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, nil)
		}
		match = candidate
	}
	return match, nil
}

func findAzureSelectionRepository(
	repositories []azuredevops.Repository,
	location remoteSelectionURL,
	sshURL bool,
	projectID string,
	name string,
	requestedID string,
) (*azuredevops.Repository, error) {
	var match *azuredevops.Repository
	for i := range repositories {
		candidate := &repositories[i]
		if candidate.ProjectID != projectID || !strings.EqualFold(candidate.Name, name) ||
			(requestedID != "" && candidate.ID != requestedID) ||
			!sameAzureRepositoryLocation(location, candidate.WebURL, sshURL) {
			continue
		}
		if match != nil {
			return nil, taskservice.NewRepositorySelectionError(taskservice.RepositorySelectionErrorInvalid, nil)
		}
		match = candidate
	}
	return match, nil
}

type remoteSelectionURL struct {
	origin string
	host   string
	path   []string
}

func parseRemoteSelectionURL(raw string) (remoteSelectionURL, error) {
	value := strings.TrimSpace(raw)
	if strings.Contains(value, "@") && !strings.Contains(value, "://") && strings.Contains(value, ":") {
		parts := strings.SplitN(value, ":", 2)
		host := strings.TrimPrefix(parts[0], "git@")
		if host == parts[0] || host == "" {
			return remoteSelectionURL{}, fmt.Errorf("invalid remote selection URL")
		}
		return remoteSelectionURL{origin: remoteSelectionHTTPS + strings.ToLower(host), host: strings.ToLower(host), path: cleanRemotePath(parts[1])}, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != remoteSelectionHTTPScheme && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return remoteSelectionURL{}, fmt.Errorf("invalid remote selection URL")
	}
	host := strings.ToLower(parsed.Host)
	origin := strings.ToLower(parsed.Scheme) + "://" + host
	return remoteSelectionURL{origin: origin, host: host, path: cleanRemotePath(parsed.Path)}, nil
}

func cleanRemotePath(path string) []string {
	path = strings.Trim(path, "/")
	segments := strings.Split(path, "/")
	if len(segments) > 0 {
		segments[len(segments)-1] = strings.TrimSuffix(segments[len(segments)-1], ".git")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return nil
		}
	}
	return segments
}

func sameOrigin(left, right string) bool {
	leftURL, leftErr := url.Parse(strings.TrimRight(left, "/"))
	rightURL, rightErr := url.Parse(strings.TrimRight(right, "/"))
	return leftErr == nil && rightErr == nil && strings.EqualFold(leftURL.Host, rightURL.Host)
}

func sameRemoteURLLocation(left, right string, foldPath bool) bool {
	leftURL, leftErr := parseRemoteSelectionURL(left)
	rightURL, rightErr := parseRemoteSelectionURL(right)
	if leftErr != nil || rightErr != nil || !strings.EqualFold(leftURL.host, rightURL.host) || len(leftURL.path) != len(rightURL.path) {
		return false
	}
	return remotePathsEqual(leftURL.path, rightURL.path, foldPath)
}

func sameAzureRepositoryLocation(requested remoteSelectionURL, canonicalURL string, sshURL bool) bool {
	canonical, err := parseRemoteSelectionURL(canonicalURL)
	if err != nil {
		return false
	}
	if !sshURL {
		return strings.EqualFold(canonical.host, requested.host) && remotePathsEqual(canonical.path, requested.path, true)
	}
	if len(requested.path) != 4 || len(canonical.path) != 4 || !strings.EqualFold(canonical.path[2], "_git") {
		return false
	}
	return strings.EqualFold(canonical.path[0], requested.path[1]) &&
		strings.EqualFold(canonical.path[1], requested.path[2]) &&
		strings.EqualFold(canonical.path[3], requested.path[3])
}

func remotePathsEqual(left, right []string, fold bool) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if fold {
			if !strings.EqualFold(left[i], right[i]) {
				return false
			}
		} else if left[i] != right[i] {
			return false
		}
	}
	return true
}
