package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

type TurnChangeCheckout struct {
	ID                     string
	EnvironmentRepoID      string
	TaskRepositoryID       string
	RepositoryID           string
	WorktreeID             string
	DisplayName            string
	RepositorySubpath      string
	RepositorySubpathKnown bool
}

type TurnChangeAdmission struct {
	TaskID             string
	SessionID          string
	TaskEnvironmentID  string
	TurnID             string
	ExecutionID        string
	StartupAttemptID   string
	PromptGeneration   uint64
	ExecutionProfileID string
	RouteGeneration    int64
	Checkouts          []TurnChangeCheckout
}

type TurnChangeTerminal struct {
	TurnChangeAdmission
	At                      time.Time
	Outcome                 string
	FinalAssistantMessageID string
}

type TurnChangeCheckpointClient interface {
	TurnCheckpointRepositoryScopes(context.Context) ([]string, error)
	CaptureTurnCheckpoint(context.Context, turnchanges.CheckpointRequest) (*turnchanges.CheckpointResult, error)
	DeleteTurnCheckpoint(context.Context, turnchanges.CheckpointDeleteRequest) error
	CompareTurnCheckpoints(context.Context, turnchanges.CompareRequest) (*turnchanges.CheckpointComparison, error)
	ExportTurnCheckpoint(context.Context, turnchanges.ExportRequest) (*turnchanges.CheckpointExport, error)
}

type TurnChangeCaptureHandler interface {
	AdmitTurnChanges(context.Context, TurnChangeAdmission, TurnChangeCheckpointClient) error
	FinishTurnChanges(context.Context, TurnChangeTerminal, TurnChangeCheckpointClient) error
}

func turnChangeCheckoutsFromWorkspaceRepositories(repositories []WorkspaceRepositorySpec) []TurnChangeCheckout {
	if len(repositories) == 0 {
		return nil
	}
	checkouts := make([]TurnChangeCheckout, 0, len(repositories))
	for _, repository := range repositories {
		checkouts = append(checkouts, TurnChangeCheckout{
			ID: repository.TaskEnvironmentRepoID, EnvironmentRepoID: repository.TaskEnvironmentRepoID,
			TaskRepositoryID: repository.TaskRepositoryID, RepositoryID: repository.RepositoryID,
			WorktreeID: repository.WorktreeID, DisplayName: repository.RepoName,
			RepositorySubpath: repository.RepositorySubpath, RepositorySubpathKnown: repository.RepositorySubpathKnown,
		})
	}
	return checkouts
}
