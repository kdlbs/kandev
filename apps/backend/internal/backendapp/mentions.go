package backendapp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/mentions"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	api "github.com/kandev/kandev/pkg/api/v1"
)

// MentionComponents owns the shared descriptor registry used by search and
// message-submission authorization.
type MentionComponents struct {
	Registry   *mentions.Registry
	Search     mentions.Searcher
	Submission entityrefs.SubmissionValidator
}

type mentionWorkspaceResolver interface {
	GetWorkspace(context.Context, string) (*models.Workspace, error)
}

type mentionConversationResolver interface {
	ResolveWorkspace(context.Context, string, string) (string, error)
}

func newMentionComponents(
	workspaces mentionWorkspaceResolver,
	conversations mentionConversationResolver,
	providers ...mentions.MentionProvider,
) (*MentionComponents, error) {
	registry := mentions.NewRegistry()
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			return nil, fmt.Errorf("register mention provider: %w", err)
		}
	}
	search := &workspaceValidatingMentionSearcher{
		workspaces: workspaces,
		searcher:   mentions.NewService(registry),
	}
	return &MentionComponents{
		Registry:   registry,
		Search:     search,
		Submission: entityrefs.NewSubmissionService(conversations, registry),
	}, nil
}

type workspaceValidatingMentionSearcher struct {
	workspaces mentionWorkspaceResolver
	searcher   mentions.Searcher
}

func (s *workspaceValidatingMentionSearcher) Search(
	ctx context.Context,
	request mentions.SearchRequest,
) (*api.MentionSearchResponse, error) {
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" {
		return nil, mentions.ErrInvalidRequest
	}
	if s == nil || s.workspaces == nil || s.searcher == nil {
		return nil, errors.New("mention search is unavailable")
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		if errors.Is(err, repository.ErrWorkspaceNotFound) {
			return nil, mentions.ErrWorkspaceNotFound
		}
		return nil, fmt.Errorf("validate mention workspace: %w", err)
	}
	if workspace == nil || workspace.ID != request.WorkspaceID {
		return nil, mentions.ErrWorkspaceNotFound
	}
	return s.searcher.Search(ctx, request)
}

func builtinMentionProviders(
	services *Services,
	gitLabRepositories gitLabMentionRepositoryResolver,
) []mentions.MentionProvider {
	if services == nil {
		services = &Services{}
	}

	var jiraService mentions.JiraMentionService
	if services.Jira != nil {
		jiraService = services.Jira
	}
	providers := []mentions.MentionProvider{mentions.NewJiraProvider(jiraService)}

	var linearService mentions.LinearMentionService
	if services.Linear != nil {
		linearService = services.Linear
	}
	providers = append(providers, mentions.NewLinearProvider(linearService))

	var githubService mentions.GitHubMentionService
	if services.GitHub != nil {
		githubService = services.GitHub
	}
	providers = append(providers,
		mentions.NewGitHubIssueProvider(githubService),
		mentions.NewGitHubPullRequestProvider(githubService),
	)

	var gitlabService mentions.GitLabMentionService
	if services.GitLab != nil {
		gitlabService = newWorkspaceScopedGitLabMentionService(services.GitLab, gitLabRepositories)
	}
	providers = append(providers,
		mentions.NewGitLabIssueProvider(gitlabService),
		mentions.NewGitLabMergeRequestProvider(gitlabService),
	)

	var azureService mentions.AzureMentionService
	if services.AzureDevOps != nil {
		azureService = services.AzureDevOps
	}
	providers = append(providers,
		mentions.NewAzureWorkItemProvider(azureService),
		mentions.NewAzurePullRequestProvider(azureService),
	)

	var sentryService mentions.SentryMentionService
	if services.Sentry != nil {
		sentryService = services.Sentry
	}
	providers = append(providers, mentions.NewSentryIssueProvider(sentryService))
	return providers
}

type gitLabMentionRepositoryResolver interface {
	ListRepositories(context.Context, string) ([]*models.Repository, error)
}

type gitLabMentionScopeService interface {
	mentions.GitLabMentionService
	ConfigureMentionScopeForWorkspace(
		context.Context,
		string,
		string,
		[]gitlab.MentionProjectScope,
	) error
	Host() string
}

type workspaceScopedGitLabMentionService struct {
	service      gitLabMentionScopeService
	repositories gitLabMentionRepositoryResolver
}

func newWorkspaceScopedGitLabMentionService(
	service gitLabMentionScopeService,
	repositories gitLabMentionRepositoryResolver,
) mentions.GitLabMentionService {
	if service == nil {
		return nil
	}
	return &workspaceScopedGitLabMentionService{service: service, repositories: repositories}
}

func (s *workspaceScopedGitLabMentionService) SearchMentionIssuesForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]gitlab.MentionItem, error) {
	if err := s.ensureScope(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.service.SearchMentionIssuesForWorkspace(ctx, workspaceID, query, limit)
}

func (s *workspaceScopedGitLabMentionService) SearchMentionMRsForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]gitlab.MentionItem, error) {
	if err := s.ensureScope(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.service.SearchMentionMRsForWorkspace(ctx, workspaceID, query, limit)
}

func (s *workspaceScopedGitLabMentionService) MentionScopeForWorkspace(
	ctx context.Context,
	workspaceID string,
) (*gitlab.MentionScope, error) {
	if err := s.ensureScope(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.service.MentionScopeForWorkspace(ctx, workspaceID)
}

func (s *workspaceScopedGitLabMentionService) ensureScope(ctx context.Context, workspaceID string) error {
	if s == nil || s.service == nil || s.repositories == nil {
		return gitlab.ErrMentionUnsupportedScope
	}
	repositories, err := s.repositories.ListRepositories(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list workspace GitLab mention repositories: %w", err)
	}
	projects, err := gitLabMentionProjects(workspaceID, repositories)
	if err != nil {
		return err
	}
	host := s.service.Host()
	current, currentErr := s.service.MentionScopeForWorkspace(ctx, workspaceID)
	if currentErr == nil && sameGitLabMentionScope(current, workspaceID, host, projects) {
		return nil
	}
	if currentErr != nil && !errors.Is(currentErr, gitlab.ErrMentionUnsupportedScope) {
		return currentErr
	}
	return s.service.ConfigureMentionScopeForWorkspace(ctx, workspaceID, host, projects)
}

func gitLabMentionProjects(
	workspaceID string,
	repositories []*models.Repository,
) ([]gitlab.MentionProjectScope, error) {
	byID := make(map[int64]string)
	byPath := make(map[string]int64)
	for _, repository := range repositories {
		if repository == nil || !strings.EqualFold(strings.TrimSpace(repository.Provider), "gitlab") {
			continue
		}
		if repository.WorkspaceID != workspaceID {
			return nil, gitlab.ErrMentionInvalidScope
		}
		projectIDText := strings.TrimSpace(repository.ProviderRepoID)
		projectID, err := strconv.ParseInt(projectIDText, 10, 64)
		projectPath := strings.Trim(strings.TrimSpace(repository.ProviderOwner), "/") + "/" +
			strings.Trim(strings.TrimSpace(repository.ProviderName), "/")
		if err != nil || projectID <= 0 || strconv.FormatInt(projectID, 10) != projectIDText {
			return nil, gitlab.ErrMentionInvalidScope
		}
		if existingPath, exists := byID[projectID]; exists && existingPath != projectPath {
			return nil, gitlab.ErrMentionInvalidScope
		}
		if existingID, exists := byPath[projectPath]; exists && existingID != projectID {
			return nil, gitlab.ErrMentionInvalidScope
		}
		byID[projectID] = projectPath
		byPath[projectPath] = projectID
	}
	if len(byID) == 0 {
		return nil, gitlab.ErrMentionUnsupportedScope
	}
	projects := make([]gitlab.MentionProjectScope, 0, len(byID))
	for projectID, projectPath := range byID {
		projects = append(projects, gitlab.MentionProjectScope{ID: projectID, Path: projectPath})
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].ID == projects[j].ID {
			return projects[i].Path < projects[j].Path
		}
		return projects[i].ID < projects[j].ID
	})
	return projects, nil
}

func sameGitLabMentionScope(
	current *gitlab.MentionScope,
	workspaceID, host string,
	projects []gitlab.MentionProjectScope,
) bool {
	if current == nil || current.WorkspaceID != workspaceID || current.Host != strings.TrimRight(host, "/") ||
		len(current.Projects) != len(projects) {
		return false
	}
	for index := range projects {
		if current.Projects[index] != projects[index] {
			return false
		}
	}
	return true
}

func registerMentionRoutes(router gin.IRoutes, components *MentionComponents) {
	if router == nil || components == nil || components.Search == nil {
		return
	}
	mentions.NewHandler(components.Search).RegisterRoutes(router)
}
