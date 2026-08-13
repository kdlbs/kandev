package models

import "time"

// SubagentContext is a durable, queryable row for one subagent (Task tool)
// invocation observed by the orchestrator. See
// docs/specs/subagent-context-persistence/spec.md.
//
// Every nullable column is a pointer. This is not style: ToolUseCount in
// particular must distinguish a reported 0 from "not reported", and a
// non-pointer int64 cannot carry that distinction.
type SubagentContext struct {
	ID            string
	TaskSessionID string
	TaskID        string
	ToolCallID    string

	TurnID           *string
	ParentToolCallID *string
	SubagentType     *string
	Description      *string
	AgentID          *string
	ChildSessionID   *string
	Model            *string
	// AgentStatus is the child's own report (open, provider-defined vocabulary,
	// e.g. "async_launched"). Stored verbatim, never consulted for terminality.
	AgentStatus *string
	// ToolStatus is the ACP tool-call status of the launching Task call
	// (closed vocabulary). Terminality is decided from this field only.
	ToolStatus *string

	IsAsync bool

	TotalTokens  *int64
	ToolUseCount *int64
	DurationMs   *int64

	// Source is "live" or "backfill". Provenance, write-once.
	Source string

	ObservedAt time.Time
	SettledAt  *time.Time
	UpdatedAt  time.Time
}
