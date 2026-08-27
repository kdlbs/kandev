package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/worktree"
)

const (
	repositoryBranchPolicyNameMaxLength        = 100
	repositoryBranchPolicyDescriptionMaxLength = 500
	defaultGitflowDevelopmentBranch            = "develop"
	BranchPolicyStaleErrorCode                 = "branch_policy_stale"
)

var (
	ErrInvalidRepositoryBranchPolicy       = errors.New("invalid repository branch policy")
	ErrRepositoryBranchPolicyNameConflict  = errors.New("repository branch policy name already used")
	ErrRepositoryBranchPolicyAlreadySeeded = errors.New("repository branch policies already seeded")
	ErrRepositoryBranchPolicyReadOnly      = errors.New("this workspace is managed by Improve Kandev and is read-only")
	ErrRepositoryBranchPolicyStoreMissing  = errors.New("repository branch policy store is unavailable")
	ErrRepositoryBranchPolicyStale         = errors.New("repository branch policy is no longer available")
)

type CreateRepositoryBranchPolicyRequest struct {
	RepositoryID      string `json:"repository_id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	BaseBranch        string `json:"base_branch"`
	BranchTemplate    string `json:"branch_template"`
	PullRequestTarget string `json:"pull_request_target"`
}

type UpdateRepositoryBranchPolicyRequest struct {
	Name              *string `json:"name"`
	Description       *string `json:"description"`
	BaseBranch        *string `json:"base_branch"`
	BranchTemplate    *string `json:"branch_template"`
	PullRequestTarget *string `json:"pull_request_target"`
}

type CreateGitflowRepositoryBranchPoliciesRequest struct {
	RepositoryID      string `json:"repository_id"`
	ProductionBranch  string `json:"production_branch"`
	DevelopmentBranch string `json:"development_branch"`
}

func (s *Service) CreateRepositoryBranchPolicy(ctx context.Context, req *CreateRepositoryBranchPolicyRequest) (*models.RepositoryBranchPolicy, error) {
	if err := s.ensureBranchPolicyStore(); err != nil {
		return nil, err
	}
	repository, err := s.authorizeWritableBranchPolicyRepository(ctx, req.RepositoryID)
	if err != nil {
		return nil, err
	}
	policy, err := normalizeRepositoryBranchPolicy(&models.RepositoryBranchPolicy{
		RepositoryID: req.RepositoryID, Name: req.Name, Description: req.Description,
		BaseBranch: req.BaseBranch, BranchTemplate: req.BranchTemplate,
		PullRequestTarget: req.PullRequestTarget,
	})
	if err != nil {
		return nil, err
	}
	if err := s.assertBranchPolicyNameFree(ctx, repository.ID, policy.Name, ""); err != nil {
		return nil, err
	}
	if err := s.branchPolicies.CreateRepositoryBranchPolicy(ctx, policy); err != nil {
		return nil, err
	}
	s.publishRepositoryBranchPolicyEvent(ctx, events.RepositoryBranchPolicyCreated, repository.WorkspaceID, policy)
	return policy, nil
}

func (s *Service) GetRepositoryBranchPolicy(ctx context.Context, id string) (*models.RepositoryBranchPolicy, error) {
	if err := s.ensureBranchPolicyStore(); err != nil {
		return nil, repoerrors.ErrRepositoryBranchPolicyNotFound
	}
	policy, err := s.branchPolicies.GetRepositoryBranchPolicy(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, err := s.authorizeBranchPolicyRepository(ctx, policy.RepositoryID); err != nil {
		return nil, repoerrors.ErrRepositoryBranchPolicyNotFound
	}
	return policy, nil
}

func (s *Service) ListRepositoryBranchPolicies(ctx context.Context, repositoryID string) ([]*models.RepositoryBranchPolicy, error) {
	if err := s.ensureBranchPolicyStore(); err != nil {
		return nil, err
	}
	if _, err := s.authorizeBranchPolicyRepository(ctx, repositoryID); err != nil {
		return nil, err
	}
	return s.branchPolicies.ListRepositoryBranchPolicies(ctx, repositoryID)
}

func (s *Service) ListRepositoryBranchPoliciesForWorkspace(ctx context.Context, workspaceID string) ([]*models.RepositoryBranchPolicy, error) {
	if err := s.ensureBranchPolicyStore(); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.branchPolicies.ListRepositoryBranchPoliciesByWorkspace(ctx, workspaceID)
}

func (s *Service) UpdateRepositoryBranchPolicy(ctx context.Context, id string, req *UpdateRepositoryBranchPolicyRequest) (*models.RepositoryBranchPolicy, error) {
	if err := s.ensureBranchPolicyStore(); err != nil {
		return nil, err
	}
	policy, err := s.GetRepositoryBranchPolicy(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		policy.Name = *req.Name
	}
	if req.Description != nil {
		policy.Description = *req.Description
	}
	if req.BaseBranch != nil {
		policy.BaseBranch = *req.BaseBranch
	}
	if req.BranchTemplate != nil {
		policy.BranchTemplate = *req.BranchTemplate
	}
	if req.PullRequestTarget != nil {
		policy.PullRequestTarget = *req.PullRequestTarget
	}
	repository, err := s.authorizeWritableBranchPolicyRepository(ctx, policy.RepositoryID)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeRepositoryBranchPolicy(policy)
	if err != nil {
		return nil, err
	}
	if err := s.assertBranchPolicyNameFree(ctx, policy.RepositoryID, normalized.Name, policy.ID); err != nil {
		return nil, err
	}
	if err := s.branchPolicies.UpdateRepositoryBranchPolicy(ctx, normalized); err != nil {
		return nil, err
	}
	s.publishRepositoryBranchPolicyEvent(ctx, events.RepositoryBranchPolicyUpdated, repository.WorkspaceID, normalized)
	return normalized, nil
}

func (s *Service) DeleteRepositoryBranchPolicy(ctx context.Context, id string) error {
	if err := s.ensureBranchPolicyStore(); err != nil {
		return err
	}
	policy, err := s.GetRepositoryBranchPolicy(ctx, id)
	if err != nil {
		return err
	}
	repository, err := s.authorizeWritableBranchPolicyRepository(ctx, policy.RepositoryID)
	if err != nil {
		return err
	}
	deleted, err := s.branchPolicies.DeleteRepositoryBranchPolicy(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return repoerrors.ErrRepositoryBranchPolicyNotFound
	}
	s.publishRepositoryBranchPolicyEvent(ctx, events.RepositoryBranchPolicyDeleted, repository.WorkspaceID, policy)
	return nil
}

func (s *Service) CreateGitflowRepositoryBranchPolicies(ctx context.Context, req *CreateGitflowRepositoryBranchPoliciesRequest) ([]*models.RepositoryBranchPolicy, error) {
	if err := s.ensureBranchPolicyStore(); err != nil {
		return nil, err
	}
	repository, err := s.authorizeWritableBranchPolicyRepository(ctx, req.RepositoryID)
	if err != nil {
		return nil, err
	}
	production := strings.TrimSpace(req.ProductionBranch)
	if production == "" {
		production = strings.TrimSpace(repository.DefaultBranch)
	}
	development := strings.TrimSpace(req.DevelopmentBranch)
	if development == "" {
		development = defaultGitflowDevelopmentBranch
	}
	if err := validatePolicyBranchRef(production, "production branch"); err != nil {
		return nil, err
	}
	if err := validatePolicyBranchRef(development, "development branch"); err != nil {
		return nil, err
	}
	if production == development {
		return nil, fmt.Errorf("%w: production and development branches must differ", ErrInvalidRepositoryBranchPolicy)
	}
	if err := s.validateGitflowBranches(ctx, repository.ID, production, development); err != nil {
		return nil, err
	}
	policies := gitflowPolicies(req.RepositoryID, production, development)
	if err := s.branchPolicies.CreateRepositoryBranchPoliciesIfEmpty(ctx, repository.ID, policies); err != nil {
		if errors.Is(err, repoerrors.ErrRepositoryBranchPoliciesExist) {
			return nil, fmt.Errorf("%w: repository already has branch policies", ErrRepositoryBranchPolicyAlreadySeeded)
		}
		return nil, err
	}
	for _, policy := range policies {
		s.publishRepositoryBranchPolicyEvent(ctx, events.RepositoryBranchPolicyCreated, repository.WorkspaceID, policy)
	}
	return policies, nil
}

func (s *Service) validateGitflowBranches(ctx context.Context, repositoryID, production, development string) error {
	branches, err := s.ListBranches(ctx, repositoryID, "")
	if err != nil {
		return fmt.Errorf("%w: cannot list branches: %v", ErrInvalidRepositoryBranchPolicy, err)
	}
	available := make(map[string]struct{}, len(branches)*2)
	for _, branch := range branches {
		available[branch.Name] = struct{}{}
		if branch.Type == "remote" && branch.Remote != "" {
			available[branch.Remote+"/"+branch.Name] = struct{}{}
		}
	}
	for label, branch := range map[string]string{
		"production branch":  production,
		"development branch": development,
	} {
		if _, ok := available[branch]; !ok {
			return fmt.Errorf("%w: %s %q does not exist", ErrInvalidRepositoryBranchPolicy, label, branch)
		}
	}
	return nil
}

func (s *Service) ensureBranchPolicyStore() error {
	if s.branchPolicies == nil {
		return ErrRepositoryBranchPolicyStoreMissing
	}
	return nil
}

func (s *Service) authorizeBranchPolicyRepository(ctx context.Context, repositoryID string) (*models.Repository, error) {
	repository, err := s.repoEntities.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	if repository == nil {
		return nil, repoerrors.ErrRepositoryNotFound
	}
	if err := s.authorizeWorkspaceID(ctx, repository.WorkspaceID); err != nil {
		return nil, repoerrors.ErrRepositoryNotFound
	}
	return repository, nil
}

func (s *Service) authorizeWritableBranchPolicyRepository(ctx context.Context, repositoryID string) (*models.Repository, error) {
	repository, err := s.authorizeBranchPolicyRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, repository.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if workspace != nil && workspace.IsImproveKandev() {
		return nil, ErrRepositoryBranchPolicyReadOnly
	}
	return repository, nil
}

func (s *Service) assertBranchPolicyNameFree(ctx context.Context, repositoryID, name, exceptID string) error {
	existing, err := s.branchPolicies.GetRepositoryBranchPolicyByName(ctx, repositoryID, name)
	if err != nil {
		return err
	}
	if existing != nil && existing.ID != exceptID {
		return fmt.Errorf("%w: %q", ErrRepositoryBranchPolicyNameConflict, existing.Name)
	}
	return nil
}

func normalizeRepositoryBranchPolicy(policy *models.RepositoryBranchPolicy) (*models.RepositoryBranchPolicy, error) {
	policy.Name = strings.TrimSpace(policy.Name)
	policy.Description = strings.TrimSpace(policy.Description)
	policy.BaseBranch = strings.TrimSpace(policy.BaseBranch)
	policy.BranchTemplate = strings.TrimSpace(policy.BranchTemplate)
	policy.PullRequestTarget = strings.TrimSpace(policy.PullRequestTarget)
	if policy.PullRequestTarget == "" {
		policy.PullRequestTarget = policy.BaseBranch
	}
	if policy.Name == "" || len([]rune(policy.Name)) > repositoryBranchPolicyNameMaxLength {
		return nil, fmt.Errorf("%w: name must be between 1 and %d characters", ErrInvalidRepositoryBranchPolicy, repositoryBranchPolicyNameMaxLength)
	}
	if len([]rune(policy.Description)) > repositoryBranchPolicyDescriptionMaxLength {
		return nil, fmt.Errorf("%w: description must be at most %d characters", ErrInvalidRepositoryBranchPolicy, repositoryBranchPolicyDescriptionMaxLength)
	}
	if err := validatePolicyBranchRef(policy.BaseBranch, "base branch"); err != nil {
		return nil, err
	}
	if err := worktree.ValidateBranchNameTemplate(policy.BranchTemplate); err != nil {
		return nil, fmt.Errorf("%w: branch template: %v", ErrInvalidRepositoryBranchPolicy, err)
	}
	if err := validatePolicyBranchRef(policy.PullRequestTarget, "pull request target"); err != nil {
		return nil, err
	}
	return policy, nil
}

func validatePolicyBranchRef(value, label string) error {
	if _, err := sanitizeGitRef(strings.TrimSpace(value), label); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRepositoryBranchPolicy, err)
	}
	return nil
}

func gitflowPolicies(repositoryID, production, development string) []*models.RepositoryBranchPolicy {
	return []*models.RepositoryBranchPolicy{
		{RepositoryID: repositoryID, Name: "Feature", Description: "Feature branches", BaseBranch: development, BranchTemplate: "feature/{title}-{suffix}", PullRequestTarget: development},
		{RepositoryID: repositoryID, Name: "Bugfix", Description: "Bug-fix branches", BaseBranch: development, BranchTemplate: "bugfix/{title}-{suffix}", PullRequestTarget: development},
		{RepositoryID: repositoryID, Name: "Hotfix", Description: "Production hotfix branches", BaseBranch: production, BranchTemplate: "hotfix/{title}-{suffix}", PullRequestTarget: production},
		{RepositoryID: repositoryID, Name: "Release", Description: "Release branches", BaseBranch: development, BranchTemplate: "release/{title}-{suffix}", PullRequestTarget: production},
	}
}

func (s *Service) publishRepositoryBranchPolicyEvent(ctx context.Context, eventType, workspaceID string, policy *models.RepositoryBranchPolicy) {
	if s.eventBus == nil || policy == nil {
		return
	}
	event := map[string]interface{}{
		"id": policy.ID, "repository_id": policy.RepositoryID, "workspace_id": workspaceID, "name": policy.Name,
		"description": policy.Description, "base_branch": policy.BaseBranch,
		"branch_template": policy.BranchTemplate, "pull_request_target": policy.PullRequestTarget,
		"created_at": policy.CreatedAt.Format(time.RFC3339), "updated_at": policy.UpdatedAt.Format(time.RFC3339),
	}
	if err := s.eventBus.Publish(ctx, eventType, bus.NewEvent(eventType, "task-service", event)); err != nil {
		s.logger.Error("failed to publish repository branch policy event", zap.String("event_type", eventType), zap.String("repository_branch_policy_id", policy.ID), zap.Error(err))
	}
}
