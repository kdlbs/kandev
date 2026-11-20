package models

import "time"

// TaskSessionSummaryObservation contains the persisted fields required by
// navigation, status summaries, and compact session rows. It intentionally
// excludes the full session metadata and configuration snapshots.
type TaskSessionSummaryObservation struct {
	ID                          string
	TaskID                      string
	QueueIncarnationID          string
	Name                        string
	AgentExecutionID            string
	ContainerID                 string
	ExecutionRuntime            string
	AgentProfileID              string
	AgentProfileName            string
	AgentProfileAgentID         string
	ExecutionProfileID          string
	RouteGeneration             int64
	RouteState                  string
	RouteReason                 string
	ExecutorID                  string
	ExecutorProfileID           string
	ExecutorType                string
	ExecutorName                string
	EnvironmentID               string
	RepositoryID                string
	RepositoryPath              string
	BaseBranch                  string
	BaseCommitSHA               string
	WorkspacePath               string
	AgentWorkspacePath          string
	State                       TaskSessionState
	ErrorMessage                string
	Metadata                    map[string]interface{}
	StartedAt                   time.Time
	CompletedAt                 *time.Time
	UpdatedAt                   time.Time
	IsPrimary                   bool
	IsPassthrough               bool
	ReviewStatus                ReviewStatus
	TaskEnvironmentID           string
	LastReadMessageID           string
	Worktrees                   []*TaskEnvironmentRepo
	AdmittedExecutionGrade      string
	AdmittedExecutionGradeKnown bool
}

// ToTaskSession returns the bounded model fields represented by this
// observation. Callers must treat omitted rich fields as unknown.
func (o *TaskSessionSummaryObservation) ToTaskSession() *TaskSession {
	if o == nil {
		return nil
	}
	session := &TaskSession{
		AdmittedExecutionGrade:      o.AdmittedExecutionGrade,
		AdmittedExecutionGradeKnown: o.AdmittedExecutionGradeKnown,
		ID:                          o.ID, TaskID: o.TaskID, QueueIncarnationID: o.QueueIncarnationID,
		Name: o.Name, AgentExecutionID: o.AgentExecutionID, ContainerID: o.ContainerID,
		ExecutionRuntime: o.ExecutionRuntime,
		AgentProfileID:   o.AgentProfileID, ExecutionProfileID: o.ExecutionProfileID,
		RouteGeneration: o.RouteGeneration, RouteState: o.RouteState, RouteReason: o.RouteReason,
		ExecutorID: o.ExecutorID, ExecutorProfileID: o.ExecutorProfileID,
		EnvironmentID: o.EnvironmentID, RepositoryID: o.RepositoryID,
		BaseBranch: o.BaseBranch, BaseCommitSHA: o.BaseCommitSHA, WorkspacePath: o.WorkspacePath,
		AgentWorkspacePath: o.AgentWorkspacePath,
		State:              o.State, ErrorMessage: o.ErrorMessage, Metadata: o.Metadata,
		StartedAt: o.StartedAt, CompletedAt: o.CompletedAt, UpdatedAt: o.UpdatedAt,
		IsPrimary: o.IsPrimary, IsPassthrough: o.IsPassthrough, ReviewStatus: o.ReviewStatus,
		TaskEnvironmentID: o.TaskEnvironmentID, LastReadMessageID: o.LastReadMessageID,
		Worktrees: o.Worktrees,
	}
	if o.AgentProfileName != "" || o.AgentProfileAgentID != "" {
		session.AgentProfileSnapshot = map[string]interface{}{}
		if o.AgentProfileName != "" {
			session.AgentProfileSnapshot["name"] = o.AgentProfileName
		}
		if o.AgentProfileAgentID != "" {
			session.AgentProfileSnapshot["agent_id"] = o.AgentProfileAgentID
		}
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
