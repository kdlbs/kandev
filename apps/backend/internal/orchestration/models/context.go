package models

import "strings"

const ContextBudgetBytes = 12 * 1024

type ContextScope struct {
	ProfileID     string `json:"profile_id"`
	ProjectID     string `json:"project_id,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
}

type ContextMemory struct {
	ID              string `json:"id"`
	Revision        int64  `json:"revision"`
	Scope           string `json:"scope"`
	ScopeID         string `json:"scope_id"`
	SourceCommentID string `json:"source_comment_id,omitempty"`
	Confirmed       bool   `json:"confirmed"`
	Content         string `json:"content"`
	Truncated       bool   `json:"truncated,omitempty"`
}

type ContextPacket struct {
	ID                 string `json:"id"`
	BindingID          string `json:"binding_id"`
	BindingVersion     int64  `json:"binding_version"`
	ObjectiveID        string `json:"objective_id"`
	ObjectiveRevision  int64  `json:"objective_revision"`
	AcceptanceRevision int64  `json:"acceptance_revision"`
	IntentRevision     int64  `json:"intent_revision"`
	WorkspaceID        string `json:"workspace_id"`
	ProfileRevision    string `json:"profile_revision"`
	ContextScope
	Mode            string              `json:"mode"`
	Objective       string              `json:"objective"`
	Acceptance      []Criterion         `json:"acceptance"`
	SourceCommentID string              `json:"source_comment_id"`
	UserInstruction string              `json:"user_instruction"`
	Policy          string              `json:"policy"`
	Memory          []ContextMemory     `json:"memory"`
	Credentials     []ContextCredential `json:"credentials"`
	OmittedMemory   int                 `json:"omitted_memory"`
	MemoryReference string              `json:"memory_reference"`
}

func (m AgentMemory) MatchesContext(b *AssistantBinding, s ContextScope) bool {
	if m.ForgottenAt != nil {
		return false
	}
	if m.OwnerUserID != "" && m.OwnerUserID != b.OwnerUserID {
		return false
	}
	switch m.Scope {
	case "user":
		return m.OwnerUserID == b.OwnerUserID && m.ScopeID == b.OwnerUserID
	case "workspace":
		return m.ScopeID == "" || m.ScopeID == b.WorkspaceID
	case "project":
		return s.ProjectID != "" && m.ScopeID == s.ProjectID
	case "environment":
		return s.EnvironmentID != "" && m.ScopeID == s.EnvironmentID
	case "task":
		return s.TaskID != "" && m.ScopeID == s.TaskID
	default:
		return false
	}
}

func (s ContextScope) Valid() bool {
	for _, v := range []string{s.ProfileID, s.ProjectID, s.EnvironmentID, s.TaskID} {
		if len(v) > 200 || strings.ContainsAny(v, "\r\n") {
			return false
		}
	}
	return s.ProfileID != ""
}
