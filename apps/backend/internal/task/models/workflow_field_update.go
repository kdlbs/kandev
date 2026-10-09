package models

// WorkflowFieldUpdate identifies supplied fields. Nil preserves the stored
// value; a non-nil pointer also supplies an empty string or false.
type WorkflowFieldUpdate struct {
	Name           *string
	Description    *string
	Prompt         *string
	AgentProfileID *string
	Hidden         *bool
	Source         *string
	SourcePath     *string
}
