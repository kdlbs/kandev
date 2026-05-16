package projects

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/shared"

	"go.uber.org/zap"
)

// Repository is the persistence interface required by ProjectService.
type Repository interface {
	CreateProject(ctx context.Context, project *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	ListProjects(ctx context.Context, workspaceID string) ([]*Project, error)
	ListProjectsWithCounts(ctx context.Context, workspaceID string) ([]*ProjectWithCounts, error)
	UpdateProject(ctx context.Context, project *Project) error
	DeleteProject(ctx context.Context, id string) error
}

// ProjectService provides project CRUD business logic.
type ProjectService struct {
	repo     Repository
	logger   *logger.Logger
	activity shared.ActivityLogger
}

// NewProjectService creates a new ProjectService.
func NewProjectService(repo Repository, log *logger.Logger, activity shared.ActivityLogger) *ProjectService {
	return &ProjectService{
		repo:     repo,
		logger:   log.WithFields(zap.String("component", "projects-service")),
		activity: activity,
	}
}

// CreateProject validates and creates a new project in the DB.
func (s *ProjectService) CreateProject(ctx context.Context, project *Project) error {
	if err := s.validateProject(project); err != nil {
		return err
	}
	if project.Status == "" {
		project.Status = ProjectStatusActive
	}
	if project.Repositories == "" {
		project.Repositories = "[]"
	}
	if project.ExecutorConfig == "" {
		project.ExecutorConfig = "{}"
	}
	if err := s.repo.CreateProject(ctx, project); err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	s.logger.Info("project created",
		zap.String("project_id", project.ID),
		zap.String("name", project.Name))
	return nil
}

// GetProject returns a project by ID or name.
func (s *ProjectService) GetProject(ctx context.Context, idOrName string) (*Project, error) {
	return s.GetProjectFromConfig(ctx, idOrName)
}

// ListProjects returns all projects for a workspace.
func (s *ProjectService) ListProjects(ctx context.Context, wsID string) ([]*Project, error) {
	return s.ListProjectsFromConfig(ctx, wsID)
}

// ListProjectsWithCounts returns all projects with aggregated task counts.
func (s *ProjectService) ListProjectsWithCounts(ctx context.Context, wsID string) ([]*ProjectWithCounts, error) {
	return s.ListProjectsWithCountsFromConfig(ctx, wsID)
}

// UpdateProject validates and updates a project in the DB.
func (s *ProjectService) UpdateProject(ctx context.Context, project *Project) error {
	if err := s.validateProject(project); err != nil {
		return err
	}
	if err := s.repo.UpdateProject(ctx, project); err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	s.logger.Info("project updated",
		zap.String("project_id", project.ID),
		zap.String("name", project.Name))
	return nil
}

// DeleteProject deletes a project from the DB.
func (s *ProjectService) DeleteProject(ctx context.Context, id string) error {
	project, err := s.GetProjectFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteProject(ctx, project.ID); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

// GetProjectFromConfig looks up a project by ID or name.
func (s *ProjectService) GetProjectFromConfig(ctx context.Context, idOrName string) (*Project, error) {
	if project, err := s.repo.GetProject(ctx, idOrName); err == nil {
		return project, nil
	}
	projects, err := s.repo.ListProjects(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.Name == idOrName {
			return p, nil
		}
	}
	return nil, fmt.Errorf("project not found: %s", idOrName)
}

// ListProjectsFromConfig returns all projects for a workspace.
// An empty workspaceID returns rows across all workspaces.
func (s *ProjectService) ListProjectsFromConfig(ctx context.Context, workspaceID string) ([]*Project, error) {
	return s.repo.ListProjects(ctx, workspaceID)
}

// ListProjectsWithCountsFromConfig returns projects with task counts.
func (s *ProjectService) ListProjectsWithCountsFromConfig(
	ctx context.Context, workspaceID string,
) ([]*ProjectWithCounts, error) {
	return s.repo.ListProjectsWithCounts(ctx, workspaceID)
}

func (s *ProjectService) validateProject(project *Project) error {
	if project.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if project.WorkspaceID == "" {
		return fmt.Errorf("workspace ID is required")
	}
	if project.Status != "" && !ValidProjectStatuses[project.Status] {
		return fmt.Errorf("invalid project status: %s", project.Status)
	}
	return validateRepositories(project.Repositories)
}

func validateRepositories(reposJSON string) error {
	if reposJSON == "" || reposJSON == "[]" {
		return nil
	}
	var repos []string
	if err := json.Unmarshal([]byte(reposJSON), &repos); err != nil {
		return fmt.Errorf("repositories must be a JSON array of strings: %w", err)
	}
	for _, repo := range repos {
		if strings.TrimSpace(repo) == "" {
			return fmt.Errorf("repository entry must not be empty")
		}
	}
	return nil
}
