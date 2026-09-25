package service

import (
	"context"
	"errors"
	"reflect"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (s *Service) validateRepositoryCheckoutInput(ctx context.Context, workspaceID string, input TaskRepositoryInput) error {
	if input.CheckoutOptions == nil {
		return nil
	}
	if _, err := models.NormalizeRepositoryCheckoutOptions(input.CheckoutOptions); err != nil {
		return err
	}
	if input.LocalPath != "" {
		return errors.New("checkout options require a remote repository")
	}
	if input.RepositoryID == "" {
		return nil
	}
	repository, err := s.repoEntities.GetRepository(ctx, input.RepositoryID)
	if err != nil {
		return err
	}
	if repository == nil || repository.WorkspaceID != workspaceID {
		return repoerrors.ErrRepositoryNotFound
	}
	if repository.SourceType == "local" {
		return errors.New("checkout options cannot modify a user-managed local repository")
	}
	return nil
}

func (s *Service) preserveRepositoryCheckoutOptions(ctx context.Context, task *models.Task, inputs []TaskRepositoryInput) error {
	existing, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		return err
	}
	for i := range inputs {
		prior, err := matchingRepositoryCheckoutOptions(inputs[i], existing)
		if err != nil {
			return err
		}
		if inputs[i].CheckoutOptions == nil {
			inputs[i].CheckoutOptions = prior
			continue
		}
		next, err := models.NormalizeRepositoryCheckoutOptions(inputs[i].CheckoutOptions)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(prior, next) {
			continue
		}
		if s.taskEnvironments == nil {
			return errors.New("checkout option mutability is unavailable")
		}
		environment, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, task.ID)
		if err != nil && !errors.Is(err, repoerrors.ErrTaskEnvironmentNotFound) {
			return err
		}
		if environment != nil {
			return errors.New("checkout options cannot change after environment creation")
		}
		if err := s.validateTaskCheckoutCapabilities(ctx, &CreateTaskRequest{WorkspaceID: task.WorkspaceID, Metadata: task.Metadata, Repositories: []TaskRepositoryInput{inputs[i]}}); err != nil {
			return err
		}
	}
	return nil
}

func matchingRepositoryCheckoutOptions(input TaskRepositoryInput, existing []*models.TaskRepository) (*models.RepositoryCheckoutOptions, error) {
	var candidate *models.TaskRepository
	matches := 0
	for _, row := range existing {
		if row == nil || row.RepositoryID != input.RepositoryID {
			continue
		}
		candidate = row
		matches++
		if row.BaseBranch == input.BaseBranch && row.CheckoutBranch == input.CheckoutBranch {
			return models.GetRepositoryCheckoutOptions(row.Metadata)
		}
	}
	if matches > 1 {
		return nil, errors.New("ambiguous repository attachment: specify base and checkout branches")
	}
	if matches == 1 {
		return models.GetRepositoryCheckoutOptions(candidate.Metadata)
	}
	return nil, nil
}
