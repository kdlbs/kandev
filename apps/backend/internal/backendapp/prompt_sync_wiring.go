package backendapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/promptsync"
	"github.com/kandev/kandev/internal/reposync"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
)

// initPromptSyncServiceRequired wires the install-wide prompt sync service.
// Either provider may be nil; a configuration for it fails at sync time.
func initPromptSyncServiceRequired(
	dbPool *db.Pool, githubSvc *github.Service, gitlabSvc *gitlab.Service,
	promptSvc *promptservice.Service, repos *Repositories, taskSvc *taskservice.Service, log *logger.Logger,
) (*promptsync.Service, func() error, error) {
	var githubClients reposync.GitHubClientProvider
	if githubSvc != nil {
		githubClients = githubSvc
	}
	var gitlabClients reposync.GitLabClientProvider
	if gitlabSvc != nil {
		gitlabClients = gitlabSvc
	}
	svc, cleanup, err := promptsync.Provide(
		dbPool.Writer(), dbPool.Reader(), githubClients, gitlabClients,
		promptSvc, promptReferenceSource{repo: repos.Workflow}, log,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("prompt-sync store: %w", err)
	}
	svc.SetWorkspaceAuthorizer(taskSvc.AuthorizeWorkspaceAccess)
	svc.SetWorkspaceExists(func(ctx context.Context, workspaceID string) (bool, error) {
		_, err := repos.Task.GetWorkspace(ctx, workspaceID)
		if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			return false, nil
		}
		return err == nil, err
	})
	return svc, cleanup, nil
}

// promptReferenceSource adapts workflow and step prompts for prompt sync's
// deletion guard.
type promptReferenceSource struct {
	repo *workflowrepository.Repository
}

func (s promptReferenceSource) ListPromptReferenceTexts(ctx context.Context) ([]promptservice.PromptReferenceText, error) {
	if s.repo == nil {
		return nil, nil
	}
	texts, err := s.repo.ListPromptReferenceTexts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]promptservice.PromptReferenceText, len(texts))
	for i, text := range texts {
		out[i] = promptservice.PromptReferenceText{Owner: text.WorkflowID, Text: text.Text}
	}
	return out, nil
}
