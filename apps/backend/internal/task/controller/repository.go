package controller

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

type RepositoryController struct {
	service *service.Service
}

func NewRepositoryController(svc *service.Service) *RepositoryController {
	return &RepositoryController{service: svc}
}

func (c *RepositoryController) ListRepositories(ctx context.Context, req dto.ListRepositoriesRequest) (dto.ListRepositoriesResponse, error) {
	repositories, err := c.service.ListRepositories(ctx, req.WorkspaceID)
	if err != nil {
		return dto.ListRepositoriesResponse{}, err
	}
	resp := dto.ListRepositoriesResponse{
		Repositories: make([]dto.RepositoryDTO, 0, len(repositories)),
		Total:        len(repositories),
	}
	for _, repository := range repositories {
		resp.Repositories = append(resp.Repositories, dto.FromRepository(repository))
	}
	return resp, nil
}

func (c *RepositoryController) GetRepository(ctx context.Context, req dto.GetRepositoryRequest) (dto.RepositoryDTO, error) {
	repository, err := c.service.GetRepository(ctx, req.ID)
	if err != nil {
		return dto.RepositoryDTO{}, err
	}
	return dto.FromRepository(repository), nil
}

func (c *RepositoryController) CreateRepository(ctx context.Context, req dto.CreateRepositoryRequest) (dto.RepositoryDTO, error) {
	repository, err := c.service.CreateRepository(ctx, &service.CreateRepositoryRequest{
		WorkspaceID:    req.WorkspaceID,
		Name:           req.Name,
		SourceType:     req.SourceType,
		LocalPath:      req.LocalPath,
		Provider:       req.Provider,
		ProviderRepoID: req.ProviderRepoID,
		ProviderOwner:  req.ProviderOwner,
		ProviderName:   req.ProviderName,
		DefaultBranch:  req.DefaultBranch,
		SetupScript:    req.SetupScript,
		CleanupScript:  req.CleanupScript,
	})
	if err != nil {
		return dto.RepositoryDTO{}, err
	}
	return dto.FromRepository(repository), nil
}

func (c *RepositoryController) UpdateRepository(ctx context.Context, req dto.UpdateRepositoryRequest) (dto.RepositoryDTO, error) {
	repository, err := c.service.UpdateRepository(ctx, req.ID, &service.UpdateRepositoryRequest{
		Name:           req.Name,
		SourceType:     req.SourceType,
		LocalPath:      req.LocalPath,
		Provider:       req.Provider,
		ProviderRepoID: req.ProviderRepoID,
		ProviderOwner:  req.ProviderOwner,
		ProviderName:   req.ProviderName,
		DefaultBranch:  req.DefaultBranch,
		SetupScript:    req.SetupScript,
		CleanupScript:  req.CleanupScript,
	})
	if err != nil {
		return dto.RepositoryDTO{}, err
	}
	return dto.FromRepository(repository), nil
}

func (c *RepositoryController) DeleteRepository(ctx context.Context, req dto.DeleteRepositoryRequest) (dto.SuccessResponse, error) {
	if err := c.service.DeleteRepository(ctx, req.ID); err != nil {
		if errors.Is(err, service.ErrActiveTaskSessions) {
			return dto.SuccessResponse{}, ErrActiveTaskSessions
		}
		return dto.SuccessResponse{}, err
	}
	return dto.SuccessResponse{Success: true}, nil
}

func (c *RepositoryController) ListRepositoryScripts(ctx context.Context, req dto.ListRepositoryScriptsRequest) (dto.ListRepositoryScriptsResponse, error) {
	scripts, err := c.service.ListRepositoryScripts(ctx, req.RepositoryID)
	if err != nil {
		return dto.ListRepositoryScriptsResponse{}, err
	}
	resp := dto.ListRepositoryScriptsResponse{
		Scripts: make([]dto.RepositoryScriptDTO, 0, len(scripts)),
		Total:   len(scripts),
	}
	for _, script := range scripts {
		resp.Scripts = append(resp.Scripts, dto.FromRepositoryScript(script))
	}
	return resp, nil
}

func (c *RepositoryController) GetRepositoryScript(ctx context.Context, req dto.GetRepositoryScriptRequest) (dto.RepositoryScriptDTO, error) {
	script, err := c.service.GetRepositoryScript(ctx, req.ID)
	if err != nil {
		return dto.RepositoryScriptDTO{}, err
	}
	return dto.FromRepositoryScript(script), nil
}

func (c *RepositoryController) CreateRepositoryScript(ctx context.Context, req dto.CreateRepositoryScriptRequest) (dto.RepositoryScriptDTO, error) {
	script, err := c.service.CreateRepositoryScript(ctx, &service.CreateRepositoryScriptRequest{
		RepositoryID: req.RepositoryID,
		Name:         req.Name,
		Command:      req.Command,
		Position:     req.Position,
	})
	if err != nil {
		return dto.RepositoryScriptDTO{}, err
	}
	return dto.FromRepositoryScript(script), nil
}

func (c *RepositoryController) UpdateRepositoryScript(ctx context.Context, req dto.UpdateRepositoryScriptRequest) (dto.RepositoryScriptDTO, error) {
	script, err := c.service.UpdateRepositoryScript(ctx, req.ID, &service.UpdateRepositoryScriptRequest{
		Name:     req.Name,
		Command:  req.Command,
		Position: req.Position,
	})
	if err != nil {
		return dto.RepositoryScriptDTO{}, err
	}
	return dto.FromRepositoryScript(script), nil
}

func (c *RepositoryController) DeleteRepositoryScript(ctx context.Context, req dto.DeleteRepositoryScriptRequest) (dto.SuccessResponse, error) {
	if err := c.service.DeleteRepositoryScript(ctx, req.ID); err != nil {
		return dto.SuccessResponse{}, err
	}
	return dto.SuccessResponse{Success: true}, nil
}

func (c *RepositoryController) ListRepositoryBranches(ctx context.Context, req dto.ListRepositoryBranchesRequest) (dto.RepositoryBranchesResponse, error) {
	branches, err := c.service.ListRepositoryBranches(ctx, req.ID)
	if err != nil {
		return dto.RepositoryBranchesResponse{}, err
	}
	// Convert service.Branch to dto.BranchDTO
	dtoBranches := make([]dto.BranchDTO, len(branches))
	for i, branch := range branches {
		dtoBranches[i] = dto.BranchDTO{
			Name:   branch.Name,
			Type:   branch.Type,
			Remote: branch.Remote,
		}
	}
	return dto.RepositoryBranchesResponse{
		Branches: dtoBranches,
		Total:    len(dtoBranches),
	}, nil
}

func (c *RepositoryController) DiscoverRepositories(ctx context.Context, req dto.DiscoverRepositoriesRequest) (dto.RepositoryDiscoveryResponse, error) {
	result, err := c.service.DiscoverLocalRepositories(ctx, req.Root)
	if err != nil {
		return dto.RepositoryDiscoveryResponse{}, err
	}
	resp := dto.RepositoryDiscoveryResponse{
		Roots:        result.Roots,
		Repositories: make([]dto.LocalRepositoryDTO, 0, len(result.Repositories)),
		Total:        len(result.Repositories),
	}
	for _, repo := range result.Repositories {
		resp.Repositories = append(resp.Repositories, dto.FromLocalRepository(repo))
	}
	return resp, nil
}

func (c *RepositoryController) ValidateRepositoryPath(ctx context.Context, req dto.ValidateRepositoryPathRequest) (dto.RepositoryPathValidationResponse, error) {
	result, err := c.service.ValidateLocalRepositoryPath(ctx, req.Path)
	if err != nil {
		return dto.RepositoryPathValidationResponse{}, err
	}
	return dto.RepositoryPathValidationResponse{
		Path:          result.Path,
		Exists:        result.Exists,
		IsGitRepo:     result.IsGitRepo,
		Allowed:       result.Allowed,
		DefaultBranch: result.DefaultBranch,
		Message:       result.Message,
	}, nil
}
