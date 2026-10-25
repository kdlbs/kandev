package models

import "time"

// InteractionKind names the two agent-initiated request kinds that block a
// turn on a human decision. It is the durable vocabulary behind the compact
// TaskPendingAction projection, kept as its own type because the interaction
// contract carries the full request (options, questions, resolution) rather
// than the one-word "what is this session waiting on" answer.
type InteractionKind string

const (
	// InteractionKindPermission is a tool-permission request (ACP
	// session/request_permission), persisted as a permission_request message.
	InteractionKindPermission InteractionKind = "permission"
	// InteractionKindClarification is a structured question bundle
	// (ask_user_question), persisted as one clarification_request message per
	// question, all sharing metadata.pending_id.
	InteractionKindClarification InteractionKind = "clarification"
)

// InteractionStatus is the normalized resolution state of an interaction. It
// unifies PermissionStatus (approved/rejected/expired) with the clarification
// vocabulary (answered/rejected/cancelled/expired) so a consumer can branch on
// "still owed a response" without knowing which kind it is holding.
//
// Both kinds record the unresolved state differently on disk — a permission
// message omits metadata.status entirely, a clarification writes "pending" —
// so readers must normalize through InteractionStatusFromMetadata rather than
// comparing the raw metadata value.
type InteractionStatus string

const (
	// InteractionStatusPending means the interaction is still owed a response.
	InteractionStatusPending InteractionStatus = "pending"
	// InteractionStatusApproved is a permission the user accepted.
	InteractionStatusApproved InteractionStatus = "approved"
	// InteractionStatusRejected is a permission the user denied or a
	// clarification bundle the user declined.
	InteractionStatusRejected InteractionStatus = "rejected"
	// InteractionStatusAnswered is a clarification bundle the user answered.
	InteractionStatusAnswered InteractionStatus = "answered"
	// InteractionStatusCancelled is a clarification withdrawn by the operator.
	InteractionStatusCancelled InteractionStatus = "cancelled"
	// InteractionStatusExpired means the request became unanswerable before a
	// response arrived (the agent withdrew it, or its pending entry was gone).
	InteractionStatusExpired InteractionStatus = "expired"
)

// metadataStatusPending is the literal a clarification message writes for the
// unresolved state. A permission message writes nothing at all (see
// PermissionStatusPending), so both the empty string and this literal
// normalize to InteractionStatusPending.
const metadataStatusPending = "pending"

// IsTerminal reports whether the interaction can no longer accept a response.
func (s InteractionStatus) IsTerminal() bool {
	return s != "" && s != InteractionStatusPending
}

// InteractionStatusFromMetadata normalizes a message's metadata.status into an
// InteractionStatus. An absent or empty value is pending, matching
// PermissionStatusPending's absence sentinel. An unrecognized value is
// preserved verbatim rather than coerced to pending: an unknown terminal state
// must never read as "still owed a response".
func InteractionStatusFromMetadata(metadata map[string]interface{}) InteractionStatus {
	raw, _ := metadata["status"].(string)
	if raw == "" || raw == metadataStatusPending {
		return InteractionStatusPending
	}
	return InteractionStatus(raw)
}

// PendingInteractionFilter narrows the durable pending-interaction read.
// SessionIDs and TaskIDs are ORed within each axis and ANDed across the two
// (matching PluginMessageFilter); an empty axis is unconstrained. Kinds
// narrows by InteractionKind; an empty slice means both kinds.
type PendingInteractionFilter struct {
	SessionIDs []string
	TaskIDs    []string
	Kinds      []string
}

// InteractionOption is one selectable response on a permission request or a
// clarification question. Kind carries the ACP permission-option kind
// (allow_once / allow_always / reject_once / reject_always) for permissions and
// is empty for clarification options; Description is the reverse.
type InteractionOption struct {
	ID          string `json:"option_id"`
	Label       string `json:"label"`
	Kind        string `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`
}

// InteractionQuestion is one question of a clarification bundle. Every question
// in a bundle must be answered before the agent unblocks.
type InteractionQuestion struct {
	ID      string              `json:"id"`
	Title   string              `json:"title,omitempty"`
	Prompt  string              `json:"prompt"`
	Options []InteractionOption `json:"options,omitempty"`
}

// Interaction is the assembled durable projection of one agent interaction: a
// permission request, or a whole clarification bundle collapsed from its
// per-question messages. ID is the pending_id the response paths key on.
//
// AgentDisconnected marks a still-pending clarification whose original waiter
// has gone away (the agent moved on or the backend restarted). It stays
// pending and answerable — the answer resumes the session in a new turn — so a
// consumer must not treat disconnection as resolution.
type Interaction struct {
	ID        string            `json:"id"`
	Kind      InteractionKind   `json:"kind"`
	TaskID    string            `json:"task_id"`
	SessionID string            `json:"session_id"`
	TurnID    string            `json:"turn_id"`
	Status    InteractionStatus `json:"status"`
	// RequestID is the provider-side permission request identity. Kandev's
	// resolution path is keyed on it alongside the pending id; empty for a
	// clarification bundle and for permission rows written before it existed.
	RequestID         string                `json:"request_id,omitempty"`
	Title             string                `json:"title,omitempty"`
	Context           string                `json:"context,omitempty"`
	ToolCallID        string                `json:"tool_call_id,omitempty"`
	ActionType        string                `json:"action_type,omitempty"`
	Options           []InteractionOption   `json:"options,omitempty"`
	Questions         []InteractionQuestion `json:"questions,omitempty"`
	AgentDisconnected bool                  `json:"agent_disconnected,omitempty"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
}
