package automation

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidTaskMode       = errors.New("automation: invalid task mode")
	ErrInvalidRepositoryMode = errors.New("automation: invalid repository mode")
	ErrWorkflowRequired      = errors.New("automation: normal task mode requires a workflow")
)

// validateAutomationTarget enforces the persisted target contract shared by
// create, update, and direct store callers. Repository selection is kept
// separate from task visibility so a normal task can still use the local
// scratch executor when it has no repository.
func validateAutomationTarget(taskMode TaskMode, repositoryMode RepositoryMode, workflowID string, repositoryIDs []string) error {
	if taskMode == "" {
		taskMode = TaskModeAutomationRun
	}
	switch taskMode {
	case TaskModeAutomationRun:
	case TaskModeNormalTask:
		if strings.TrimSpace(workflowID) == "" {
			return ErrWorkflowRequired
		}
	default:
		return fmt.Errorf("%w: %q", ErrInvalidTaskMode, taskMode)
	}

	if repositoryMode == "" {
		repositoryMode = RepositoryModeNone
	}
	switch repositoryMode {
	case RepositoryModeSelected:
		if len(repositoryIDs) == 0 {
			return fmt.Errorf("%w: selected requires repository_ids", ErrInvalidRepositoryMode)
		}
	case RepositoryModeNone:
		if len(repositoryIDs) > 0 {
			return fmt.Errorf("%w: none cannot include repository_ids", ErrInvalidRepositoryMode)
		}
	default:
		return fmt.Errorf("%w: %q", ErrInvalidRepositoryMode, repositoryMode)
	}

	return nil
}
