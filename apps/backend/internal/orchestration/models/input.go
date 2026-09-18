package models

import (
	"fmt"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type InputOption struct {
	ID          string `json:"option_id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"`
}
type InputQuestion struct {
	ID                 string        `json:"id"`
	Title              string        `json:"title,omitempty"`
	Prompt             string        `json:"prompt"`
	Options            []InputOption `json:"options"`
	AssistantDelegable bool          `json:"assistant_delegable,omitempty"`
}
type AttentionInput struct {
	SourceID       string                          `json:"source_id"`
	Permission     *streams.PendingAgentPermission `json:"permission,omitempty"`
	Kind           string                          `json:"kind"`
	State          string                          `json:"state"`
	SourceRevision string                          `json:"source_revision"`
	TaskID         string                          `json:"task_id"`
	SessionID      string                          `json:"session_id"`
	PendingID      string                          `json:"pending_id"`
	RequestID      string                          `json:"request_id,omitempty"`
	ProfileID      string                          `json:"profile_id,omitempty"`
	Summary        string                          `json:"summary"`
	Questions      []InputQuestion                 `json:"questions,omitempty"`
	Options        []InputOption                   `json:"options,omitempty"`
}
type InputAnswer struct {
	QuestionID      string   `json:"question_id"`
	SelectedOptions []string `json:"selected_options,omitempty"`
	CustomText      string   `json:"custom_text,omitempty"`
}
type InputResponse struct {
	Answers         []InputAnswer `json:"answers,omitempty"`
	OptionID        string        `json:"option_id,omitempty"`
	Rejected        bool          `json:"rejected,omitempty"`
	RejectReason    string        `json:"reject_reason,omitempty"`
	ActorType       string        `json:"-"`
	ActorID         string        `json:"-"`
	SourceMemoryIDs []string      `json:"-"`
}
type InputRejection struct {
	Status int
	Reason string
}

func (e *InputRejection) Error() string { return fmt.Sprintf("native input rejected: %s", e.Reason) }
