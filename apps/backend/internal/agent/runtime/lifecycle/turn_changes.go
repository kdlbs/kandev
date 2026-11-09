package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

// TurnChangeSummaryProcessor handles comparison and content export after the
// terminal endpoints have been accepted and the prompt-generation fence ends.
type TurnChangeSummaryProcessor interface {
	ProcessTurnChanges(context.Context, TurnChangeTerminal, TurnChangeCheckpointClient) error
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

// TurnChangeCaptureOperation reports whether the exact execution generation
// still holds a live prompt or capture fence. A missing execution has no live
// writer and is safe to settle.
func (m *Manager) TurnChangeCaptureOperation(executionID string, promptGeneration int64) (bool, bool) {
	if m == nil || executionID == "" || promptGeneration <= 0 {
		return false, false
	}
	execution, found := m.executionStore.Get(executionID)
	if !found || execution == nil {
		return false, false
	}
	generation := uint64(promptGeneration)
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	active := execution.turnChangeCaptureGeneration == generation ||
		(execution.promptGeneration == generation && execution.promptCompletionGeneration != generation)
	return true, active
}

// TurnChangeRestoreInProgress reports whether a session execution is starting
// without an attached agentctl client.
func (m *Manager) TurnChangeRestoreInProgress(sessionID string) bool {
	if m == nil || sessionID == "" {
		return false
	}
	execution, found := m.executionStore.GetBySessionID(sessionID)
	if !found || execution == nil {
		return false
	}
	client, release := execution.AcquireAgentCtlClient()
	release()
	return client == nil && execution.Status == v1.AgentStatusStarting
}

// AcquireTurnChangeExporter returns the retained executor client for a session
// without creating, relaunching, or capturing anything.
func (m *Manager) AcquireTurnChangeExporter(sessionID string) (TurnChangeCheckpointClient, func(), bool) {
	if m == nil || sessionID == "" {
		return nil, nil, false
	}
	execution, found := m.executionStore.GetBySessionID(sessionID)
	if !found || execution == nil {
		return nil, nil, false
	}
	client, release := execution.AcquireAgentCtlClient()
	if client == nil {
		release()
		return nil, nil, false
	}
	return client, release, true
}
