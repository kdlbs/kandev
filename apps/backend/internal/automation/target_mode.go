package automation

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidTaskMode              = errors.New("automation: invalid task mode")
	ErrInvalidRepositoryMode        = errors.New("automation: invalid repository mode")
	ErrWorkflowRequired             = errors.New("automation: normal task mode requires a workflow")
	ErrManagedDestinationRequired   = errors.New("automation: managed conversation destination is required")
	ErrUnexpectedManagedDestination = errors.New("automation: managed destination requires managed conversation mode")
)

func validateManagedDestination(taskMode TaskMode, destination *ManagedConversationDestination) error {
	if taskMode != TaskModeManagedConversation {
		if destination != nil {
			return ErrUnexpectedManagedDestination
		}
		return nil
	}
	if destination == nil || strings.TrimSpace(destination.PluginID) == "" ||
		strings.TrimSpace(destination.InstanceKey) == "" || destination.Revision == 0 {
		return ErrManagedDestinationRequired
	}
	return nil
}

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
	case TaskModeManagedConversation:
		if strings.TrimSpace(workflowID) != "" || len(repositoryIDs) != 0 || repositoryMode == RepositoryModeSelected || repositoryMode == RepositoryModeWorkspaceDefault {
			return fmt.Errorf("%w: managed conversations cannot include task resources", ErrInvalidTaskMode)
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
