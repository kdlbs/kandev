package backendapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// turnServiceAdapter adapts the task service to the orchestrator.TurnService interface.
type turnServiceAdapter struct {
	svc *taskservice.Service
}

func (a *turnServiceAdapter) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return a.svc.StartTurn(ctx, sessionID)
}

func (a *turnServiceAdapter) CompleteTurn(ctx context.Context, turnID string) error {
	return a.svc.CompleteTurn(ctx, turnID)
}

func (a *turnServiceAdapter) GetTurn(ctx context.Context, turnID string) (*models.Turn, error) {
	return a.svc.GetTurn(ctx, turnID)
}

func (a *turnServiceAdapter) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return a.svc.GetActiveTurn(ctx, sessionID)
}

func (a *turnServiceAdapter) UpdateTurn(ctx context.Context, turn *models.Turn) error {
	return a.svc.UpdateTurn(ctx, turn)
}

func (a *turnServiceAdapter) AbandonOpenTurns(ctx context.Context, sessionID string) error {
	return a.svc.AbandonOpenTurns(ctx, sessionID)
}

func newTurnServiceAdapter(svc *taskservice.Service) *turnServiceAdapter {
	return &turnServiceAdapter{svc: svc}
}

// taskSessionCheckerAdapter adapts the task repository for github.TaskSessionChecker.
type taskSessionCheckerAdapter struct {
	repo interface {
		ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
		ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error)
	}
}

// HasUserAuthoredMessage reports whether the user has authored any message
// on this task that wasn't created by an automated trigger (workflow
// auto-start, PR/issue watch, Jira/Linear integration). Auto-start messages
// are tagged with metadata.auto_start = true; the check ignores them so a
// task whose only "user" message is the agent's auto-injected prompt counts
// as untouched and is eligible for cleanup when its PR/issue merges.
func (a *taskSessionCheckerAdapter) HasUserAuthoredMessage(ctx context.Context, taskID string) (bool, error) {
	sessions, err := a.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, sess := range sessions {
		messages, err := a.repo.ListMessages(ctx, sess.ID)
		if err != nil {
			return false, err
		}
		for _, m := range messages {
			if m.AuthorType != models.MessageAuthorUser {
				continue
			}
			// New code paths tag both auto_start and workflow_auto_start.
			// Legacy rows (pre-cleanup-policy upgrade) carry only the
			// workflow_auto_start tag from the old recordAutoStartMessage
			// implementation — recognize it too so the install-wide
			// cleanup button actually drains piled-up tasks after upgrade.
			if metaFlag(m.Metadata, "auto_start") || metaFlag(m.Metadata, "workflow_auto_start") {
				continue
			}
			return true, nil
		}
	}
	return false, nil
}

// metaFlag returns true when meta[key] is a bool with value true. Returns
// false for missing keys, nil maps, non-bool values, and false values.
func metaFlag(meta map[string]interface{}, key string) bool {
	v, ok := meta[key].(bool)
	return ok && v
}

// taskDeleterAdapter satisfies github.TaskDeleter and translates the task
// repository's ErrTaskNotFound sentinel to github.ErrTaskNotFound so the
// github cleanup paths can classify the "already gone" case via errors.Is
// without importing the task repository's package.
type taskDeleterAdapter struct {
	svc *taskservice.Service
}

func (a *taskDeleterAdapter) DeleteTask(ctx context.Context, taskID string) error {
	return a.translateDeleteErr(a.svc.DeleteTask(ctx, taskID))
}

// DeleteTaskWithReason satisfies github.TaskDeleterWithReason so the review/issue
// cleanup paths can attach a deletion reason to the task.deleted event.
func (a *taskDeleterAdapter) DeleteTaskWithReason(ctx context.Context, taskID, reason string) error {
	return a.translateDeleteErr(a.svc.DeleteTaskWithReason(ctx, taskID, reason))
}

// translateDeleteErr maps the task repository's ErrTaskNotFound sentinel to
// github.ErrTaskNotFound so cleanup can classify the "already gone" case.
func (a *taskDeleterAdapter) translateDeleteErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return fmt.Errorf("%w: %w", github.ErrTaskNotFound, err)
	}
	return err
}

// automationTaskDeleterAdapter satisfies automation.TaskDeleter and
// translates the task repository's ErrTaskNotFound sentinel to
// automation.ErrTaskNotFound so the automation run-cleanup paths can
// classify the "already gone" case via errors.Is without importing the task
// repository's package.
type automationTaskDeleterAdapter struct {
	svc *taskservice.Service
}

func (a *automationTaskDeleterAdapter) DeleteTask(ctx context.Context, taskID string) error {
	err := a.svc.DeleteTask(ctx, taskID)
	if err == nil {
		return nil
	}
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return fmt.Errorf("%w: %w", automation.ErrTaskNotFound, err)
	}
	return err
}

// repositoryLookupAdapter satisfies the linear/jira/sentry RepositoryLookup
// interface over the task service. It is the validation seam for a watcher's
// optional repository binding. The task service's GetRepository filters
// soft-deleted rows and errors on a miss, so a missing or deleted repository
// maps to ok=false and watcher create/update rejects the binding.
type repositoryLookupAdapter struct {
	svc *taskservice.Service
}

func (a *repositoryLookupAdapter) GetRepository(ctx context.Context, id string) (string, string, bool) {
	repo, err := a.svc.GetRepository(ctx, id)
	if err != nil || repo == nil {
		return "", "", false
	}
	return repo.WorkspaceID, repo.DefaultBranch, true
}

// LookupTaskRepository resolves provider metadata only when repositoryID is
// linked to taskID. Azure association validation fails closed on a nil result.
func (a *repositoryLookupAdapter) LookupTaskRepository(
	ctx context.Context,
	taskID, repositoryID string,
) (*azuredevops.RepositoryBinding, error) {
	task, err := a.svc.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	linked := false
	for _, taskRepository := range task.Repositories {
		if taskRepository != nil && taskRepository.RepositoryID == repositoryID {
			linked = true
			break
		}
	}
	if !linked {
		return nil, nil
	}
	repository, err := a.svc.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	return &azuredevops.RepositoryBinding{
		WorkspaceID: repository.WorkspaceID, Provider: repository.Provider,
		ProviderOwner: repository.ProviderOwner, ProviderRepoID: repository.ProviderRepoID,
	}, nil
}

// RepositoryExists satisfies orchestrator.RepositoryChecker. It uses the
// workspace listing (which excludes soft-deleted repos) so a definitive
// "absent" is distinguishable from a transient error: a non-nil err lets the
// dispatch pre-flight fail open, while (false, nil) means the bound repository
// was removed and the watcher should self-heal.
func (a *repositoryLookupAdapter) RepositoryExists(ctx context.Context, workspaceID, repositoryID string) (bool, error) {
	repos, err := a.svc.ListRepositories(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	for _, repo := range repos {
		if repo.ID == repositoryID {
			return true, nil
		}
	}
	return false, nil
}
