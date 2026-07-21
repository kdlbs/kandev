package azuredevops

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const (
	defaultMentionLimit    = 5
	maxMentionLimit        = 10
	maxMentionProjects     = 5
	mentionPRFetchLimit    = 100
	activePullRequestState = "active"
)

// MentionProject is the stable workspace-owned project scope attached to an
// Azure entity reference.
type MentionProject struct {
	OrganizationURL string
	ProjectID       string
	ProjectName     string
}

// MentionRepository extends a project scope with one Azure Repos repository.
type MentionRepository struct {
	OrganizationURL string
	ProjectID       string
	ProjectName     string
	RepositoryID    string
	RepositoryName  string
}

// MentionWorkItem is the minimal provider-neutral projection needed by the
// mention adapter.
type MentionWorkItem struct {
	ID              int
	Title           string
	OrganizationURL string
	ProjectID       string
	ProjectName     string
}

// MentionPullRequest is the corresponding Azure pull-request projection.
type MentionPullRequest struct {
	ID              int
	Title           string
	OrganizationURL string
	ProjectID       string
	ProjectName     string
	RepositoryID    string
	RepositoryName  string
}

// SearchMentionWorkItemsForWorkspace translates plain composer text into
// server-owned WIQL and searches a bounded set of workspace projects.
func (s *Service) SearchMentionWorkItemsForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]MentionWorkItem, error) {
	workspaceID, query, limit, err := normalizeMentionRequest(workspaceID, query, limit)
	if err != nil {
		return nil, err
	}
	cfg, client, projects, err := s.mentionSearchContext(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	wiql := buildMentionWIQL(query)
	items := make([]MentionWorkItem, 0, limit)
	for _, project := range projects {
		remaining := limit - len(items)
		if remaining == 0 {
			break
		}
		result, searchErr := client.QueryWIQL(ctx, project.ID, wiql, remaining)
		if searchErr != nil {
			return nil, searchErr
		}
		if result == nil {
			continue
		}
		for _, item := range result.Items {
			if item.ID <= 0 || strings.TrimSpace(item.Title) == "" {
				continue
			}
			items = append(items, MentionWorkItem{
				ID: item.ID, Title: item.Title, OrganizationURL: cfg.OrganizationURL,
				ProjectID: project.ID, ProjectName: project.Name,
			})
			if len(items) == limit {
				break
			}
		}
	}
	return items, nil
}

// SearchMentionPullRequestsForWorkspace uses Azure's project-level PR list,
// then performs bounded case-insensitive title filtering in Kandev.
func (s *Service) SearchMentionPullRequestsForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]MentionPullRequest, error) {
	workspaceID, query, limit, err := normalizeMentionRequest(workspaceID, query, limit)
	if err != nil {
		return nil, err
	}
	cfg, client, projects, err := s.mentionSearchContext(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(query)
	items := make([]MentionPullRequest, 0, limit)
	for _, project := range projects {
		result, searchErr := client.ListPullRequests(ctx, PullRequestFilter{
			ProjectID: project.ID,
			Status:    activePullRequestState,
			Top:       mentionPRFetchLimit,
		})
		if searchErr != nil {
			return nil, searchErr
		}
		if result == nil {
			continue
		}
		for _, item := range result.Items {
			if item.ID <= 0 || strings.TrimSpace(item.RepositoryID) == "" ||
				strings.TrimSpace(item.RepositoryName) == "" ||
				!strings.Contains(strings.ToLower(item.Title), query) {
				continue
			}
			items = append(items, MentionPullRequest{
				ID: item.ID, Title: item.Title, OrganizationURL: cfg.OrganizationURL,
				ProjectID: project.ID, ProjectName: project.Name,
				RepositoryID: item.RepositoryID, RepositoryName: item.RepositoryName,
			})
			if len(items) == limit {
				return items, nil
			}
		}
	}
	return items, nil
}

// ResolveMentionProjectForWorkspace revalidates the project encoded in a
// submitted reference against current workspace configuration.
func (s *Service) ResolveMentionProjectForWorkspace(
	ctx context.Context,
	workspaceID, projectID string,
) (*MentionProject, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("%w: project required", ErrInvalidConfig)
	}
	cfg, client, err := s.configuredMentionClient(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	project, err := resolveMentionProject(ctx, client, cfg, projectID)
	if err != nil {
		return nil, err
	}
	return &MentionProject{
		OrganizationURL: cfg.OrganizationURL,
		ProjectID:       project.ID,
		ProjectName:     project.Name,
	}, nil
}

// ResolveMentionRepositoryForWorkspace revalidates the repository encoded in
// a submitted pull-request reference.
func (s *Service) ResolveMentionRepositoryForWorkspace(
	ctx context.Context,
	workspaceID, projectID, repositoryID string,
) (*MentionRepository, error) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(repositoryID) == "" {
		return nil, fmt.Errorf("%w: project and repository required", ErrInvalidConfig)
	}
	cfg, client, err := s.configuredMentionClient(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	project, err := resolveMentionProject(ctx, client, cfg, projectID)
	if err != nil {
		return nil, err
	}
	repositories, err := client.ListRepositories(ctx, project.ID)
	if err != nil {
		return nil, err
	}
	for _, repository := range repositories {
		if strings.EqualFold(strings.TrimSpace(repository.ID), repositoryID) {
			return &MentionRepository{
				OrganizationURL: cfg.OrganizationURL,
				ProjectID:       project.ID, ProjectName: project.Name,
				RepositoryID: repository.ID, RepositoryName: repository.Name,
			}, nil
		}
	}
	return nil, fmt.Errorf("%w: repository is outside workspace scope", ErrInvalidConfig)
}

func (s *Service) mentionSearchContext(
	ctx context.Context,
	workspaceID string,
) (*Config, Client, []Project, error) {
	cfg, client, err := s.configuredMentionClient(ctx, workspaceID)
	if err != nil {
		return nil, nil, nil, err
	}
	projects, err := mentionProjects(ctx, client, cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	return cfg, client, projects, nil
}

func (s *Service) configuredMentionClient(ctx context.Context, workspaceID string) (*Config, Client, error) {
	cfg, pat, err := s.resolveCredentials(ctx, workspaceID, &SetConfigRequest{})
	if err != nil {
		return nil, nil, err
	}
	client := s.clientFn(cfg, pat)
	if client == nil {
		return nil, nil, ErrNotConfigured
	}
	return cfg, client, nil
}

func mentionProjects(ctx context.Context, client Client, cfg *Config) ([]Project, error) {
	if projectID := strings.TrimSpace(cfg.DefaultProjectID); projectID != "" {
		name := strings.TrimSpace(cfg.DefaultProjectName)
		if name == "" {
			name = projectID
		}
		return []Project{{ID: projectID, Name: name}}, nil
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	projects = append([]Project(nil), projects...)
	sort.SliceStable(projects, func(left, right int) bool {
		leftName := strings.ToLower(strings.TrimSpace(projects[left].Name))
		rightName := strings.ToLower(strings.TrimSpace(projects[right].Name))
		if leftName != rightName {
			return leftName < rightName
		}
		return strings.ToLower(projects[left].ID) < strings.ToLower(projects[right].ID)
	})
	filtered := projects[:0]
	for _, project := range projects {
		if strings.TrimSpace(project.ID) == "" || strings.TrimSpace(project.Name) == "" {
			continue
		}
		filtered = append(filtered, project)
		if len(filtered) == maxMentionProjects {
			break
		}
	}
	return filtered, nil
}

func resolveMentionProject(ctx context.Context, client Client, cfg *Config, projectID string) (Project, error) {
	projects, err := mentionProjects(ctx, client, cfg)
	if err != nil {
		return Project{}, err
	}
	for _, project := range projects {
		if strings.EqualFold(strings.TrimSpace(project.ID), strings.TrimSpace(projectID)) {
			return project, nil
		}
	}
	return Project{}, fmt.Errorf("%w: project is outside workspace scope", ErrInvalidConfig)
}

func normalizeMentionRequest(workspaceID, query string, limit int) (string, string, int, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return "", "", 0, ErrInvalidWorkspaceID
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return "", "", 0, fmt.Errorf("%w: mention query is required", ErrInvalidConfig)
	}
	switch {
	case limit <= 0:
		limit = defaultMentionLimit
	case limit > maxMentionLimit:
		limit = maxMentionLimit
	}
	return workspaceID, query, limit, nil
}

func buildMentionWIQL(query string) string {
	query = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(query)
	query = strings.ReplaceAll(query, "'", "''")
	return "SELECT [System.Id] FROM WorkItems " +
		"WHERE [System.Title] CONTAINS '" + query + "' " +
		"ORDER BY [System.ChangedDate] DESC"
}
