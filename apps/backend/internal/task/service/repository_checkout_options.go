package service

import (
	"context"
	"errors"

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
