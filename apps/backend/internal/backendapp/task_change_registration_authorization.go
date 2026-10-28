package backendapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func (p *taskChangeRegistrationProcessor) authorizedRegistrationTarget(
	ctx context.Context, intent *tasksqlite.TaskChangeRegistrationIntent,
) (context.Context, *models.Task, *models.Repository, error) {
	actorCtx, err := p.registrationActorContext(ctx, intent)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := p.authorizeRegistrationScope(actorCtx, intent); err != nil {
		return nil, nil, nil, err
	}
	task, repository, err := p.loadTarget(actorCtx, intent)
	if err != nil || !registrationTargetMatches(task, repository, intent) {
		return nil, nil, nil, errTaskChangeRegistrationTargetChanged
	}
	return actorCtx, task, repository, nil
}

func (p *taskChangeRegistrationProcessor) registrationActorContext(
	ctx context.Context, intent *tasksqlite.TaskChangeRegistrationIntent,
) (context.Context, error) {
	if p == nil || p.tasks == nil || p.repository == nil || p.resolveActor == nil || intent == nil || intent.ActorID == "" {
		return nil, errTaskChangeRegistrationAuthorizationRevoked
	}
	identity, ok := p.resolveActor(ctx, intent.ActorID)
	if !ok || identity.UserID == "" || (!identity.Synthetic && identity.UserID != intent.ActorID) {
		return nil, errTaskChangeRegistrationAuthorizationRevoked
	}
	return authn.WithIdentity(ctx, identity), nil
}

func (p *taskChangeRegistrationProcessor) authorizeRegistrationScope(
	ctx context.Context, intent *tasksqlite.TaskChangeRegistrationIntent,
) error {
	if err := p.tasks.AuthorizeTaskPromptScope(ctx, intent.TaskID); err != nil {
		return fmt.Errorf("%w: task permission check failed", errTaskChangeRegistrationAuthorizationRevoked)
	}
	if err := p.tasks.AuthorizeRepositoryAccess(ctx, intent.RepositoryID); err != nil {
		return fmt.Errorf("%w: repository permission check failed", errTaskChangeRegistrationAuthorizationRevoked)
	}
	return nil
}

func (p *taskChangeRegistrationProcessor) loadTarget(
	ctx context.Context, intent *tasksqlite.TaskChangeRegistrationIntent,
) (*models.Task, *models.Repository, error) {
	task, err := p.repository.GetTask(ctx, intent.TaskID)
	if err != nil || task == nil || task.WorkspaceID != intent.WorkspaceID {
		return nil, nil, fmt.Errorf("registration task is unavailable")
	}
	links, err := p.repository.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		return nil, nil, err
	}
	if !taskHasRegistrationRepository(links, intent.RepositoryID) {
		return nil, nil, fmt.Errorf("registration repository is no longer attached")
	}
	repository, err := p.repository.GetRepository(ctx, intent.RepositoryID)
	if err != nil || repository == nil || repository.WorkspaceID != task.WorkspaceID {
		return nil, nil, fmt.Errorf("registration repository is unavailable")
	}
	return task, repository, nil
}

func taskHasRegistrationRepository(links []*models.TaskRepository, repositoryID string) bool {
	for _, link := range links {
		if link != nil && link.RepositoryID == repositoryID {
			return true
		}
	}
	return false
}

func registrationTargetMatches(
	task *models.Task, repository *models.Repository, intent *tasksqlite.TaskChangeRegistrationIntent,
) bool {
	return task != nil && repository != nil && task.WorkspaceID == intent.WorkspaceID &&
		strings.TrimSpace(repository.ProviderRepoID) == intent.ProviderRepositoryID &&
		strings.EqualFold(strings.TrimSpace(repository.Provider), intent.Provider)
}

func (p *taskChangeRegistrationProcessor) finishProviderError(
	ctx context.Context, intent *tasksqlite.TaskChangeRegistrationIntent, providerErr error,
) error {
	current, readErr := p.repository.GetTaskChangeRegistrationIntent(ctx, intent.TaskID, intent.ID)
	if readErr == nil && current != nil && current.State == registrationStateSucceeded {
		return p.notifyRegistrationSource(ctx, current)
	}
	state, errorCode := taskChangeRegistrationProviderFailure(providerErr)
	return p.finishIntent(ctx, intent, state, errorCode, nil, "")
}

func taskChangeRegistrationProviderFailure(err error) (string, string) {
	switch {
	case errors.Is(err, errTaskChangeRegistrationCommitUncertain):
		return registrationStateUncertain, "registration_uncertain"
	case errors.Is(err, errTaskChangeRegistrationAuthorizationRevoked):
		return registrationStateFailed, "authorization_revoked"
	case errors.Is(err, errTaskChangeRegistrationTargetChanged):
		return registrationStateSuppressed, "repository_changed"
	case registrationProviderErrorMatches(err,
		github.ErrTaskPRRegistrationRevisionChanged, github.ErrTaskPRRegistrationDetached,
		gitlab.ErrTaskMRRegistrationRevisionChanged, gitlab.ErrTaskMRDetached):
		return registrationStateSuppressed, "association_changed"
	case registrationProviderErrorMatches(err,
		github.ErrTaskPRRegistrationIdentityUnresolved, gitlab.ErrTaskMRRegistrationIdentityUnresolved,
		github.ErrTaskPRRepositoryMismatch, gitlab.ErrTaskMRRepositoryMismatch):
		return registrationStateFailed, "identity_unresolved"
	default:
		return registrationStateFailed, registrationErrorProviderUnavailable
	}
}

func registrationProviderErrorMatches(err error, candidates ...error) bool {
	for _, candidate := range candidates {
		if errors.Is(err, candidate) {
			return true
		}
	}
	return false
}
