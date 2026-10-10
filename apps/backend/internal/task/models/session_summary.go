package models

import "time"

// TaskSessionSummaryObservation contains the persisted fields required by
// navigation, status summaries, and compact session rows. It intentionally
// excludes the full session metadata and configuration snapshots.
type TaskSessionSummaryObservation struct {
	ID                 string
	TaskID             string
	QueueIncarnationID string
	Name               string
	AgentExecutionID   string
	ContainerID        string
	AgentProfileID     string
	AgentProfileName   string
	ExecutionProfileID string
	RouteGeneration    int64
	RouteState         string
	RouteReason        string
	ExecutorID         string
	ExecutorProfileID  string
	ExecutorType       string
	ExecutorName       string
	EnvironmentID      string
	RepositoryID       string
	RepositoryPath     string
	BaseBranch         string
	BaseCommitSHA      string
	WorkspacePath      string
	State              TaskSessionState
	ErrorMessage       string
	Metadata           map[string]interface{}
	StartedAt          time.Time
	CompletedAt        *time.Time
	UpdatedAt          time.Time
	IsPrimary          bool
	IsPassthrough      bool
	ReviewStatus       ReviewStatus
	TaskEnvironmentID  string
	LastReadMessageID  string
	Worktrees          []*TaskEnvironmentRepo
}

// ToTaskSession returns the bounded model fields represented by this
// observation. Callers must treat omitted rich fields as unknown.
func (o *TaskSessionSummaryObservation) ToTaskSession() *TaskSession {
	if o == nil {
		return nil
	}
	session := &TaskSession{
		ID: o.ID, TaskID: o.TaskID, QueueIncarnationID: o.QueueIncarnationID,
		Name: o.Name, AgentExecutionID: o.AgentExecutionID, ContainerID: o.ContainerID,
		AgentProfileID: o.AgentProfileID, ExecutionProfileID: o.ExecutionProfileID,
		RouteGeneration: o.RouteGeneration, RouteState: o.RouteState, RouteReason: o.RouteReason,
		ExecutorID: o.ExecutorID, ExecutorProfileID: o.ExecutorProfileID,
		EnvironmentID: o.EnvironmentID, RepositoryID: o.RepositoryID,
		BaseBranch: o.BaseBranch, BaseCommitSHA: o.BaseCommitSHA, WorkspacePath: o.WorkspacePath,
		State: o.State, ErrorMessage: o.ErrorMessage, Metadata: o.Metadata,
		StartedAt: o.StartedAt, CompletedAt: o.CompletedAt, UpdatedAt: o.UpdatedAt,
		IsPrimary: o.IsPrimary, IsPassthrough: o.IsPassthrough, ReviewStatus: o.ReviewStatus,
		TaskEnvironmentID: o.TaskEnvironmentID, LastReadMessageID: o.LastReadMessageID,
		Worktrees: o.Worktrees,
	}
	if o.AgentProfileName != "" {
		session.AgentProfileSnapshot = map[string]interface{}{"name": o.AgentProfileName}
	}
	if o.ExecutorType != "" || o.ExecutorName != "" {
		session.ExecutorSnapshot = map[string]interface{}{}
		if o.ExecutorType != "" {
			session.ExecutorSnapshot["executor_type"] = o.ExecutorType
		}
		if o.ExecutorName != "" {
			session.ExecutorSnapshot["executor_name"] = o.ExecutorName
		}
	}
	if o.RepositoryPath != "" {
		session.RepositorySnapshot = map[string]interface{}{"path": o.RepositoryPath}
	}
	return session
}
