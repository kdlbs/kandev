package coordinator

import (
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/coordinator/mcpcontract"
)

// Error codes for the response bodies Build decision 16 defines. error_code
// duplicates error on these bodies so the web client's ApiError.errorCode
// getter works unchanged.
const (
	ErrorCodeProposalConflict              = "proposal_conflict"
	ErrorCodeConversationConflict          = "conversation_conflict"
	ErrorCodeCoordinatorProfileUnavailable = "coordinator_profile_unavailable"
)

// ErrorResponse is the body of a plain coordinator-route error response
// (Build decision 4): {"error": "<message>"} for 403/404/500, or
// {"error": "<message>", "field": "<field>"} for 400. Field is omitted
// unless the error names a specific JSON field.
type ErrorResponse struct {
	Error string `json:"error"`
	Field string `json:"field,omitempty"`
}

// NewErrorResponse builds a plain error response, without a field.
func NewErrorResponse(message string) *ErrorResponse {
	return &ErrorResponse{Error: message}
}

// NewFieldErrorResponse builds a 400 body naming err's field.
func NewFieldErrorResponse(err *FieldError) *ErrorResponse {
	return &ErrorResponse{Error: err.Message, Field: err.Field}
}

// CoordinatorDTO is a coordinator's JSON shape, common to every coordinator
// route (Build decision 9). OpenProposals is set only by the list route (via
// WithOpenProposals); AgentProfileStatus and ExecutorProfileStatus are set
// only by GET (via WithProfileStatuses). Both use pointer types so a
// legitimately zero open_proposals count, or a legitimately "ok" status,
// still serializes rather than being dropped by omitempty.
type CoordinatorDTO struct {
	ID                    string         `json:"id"`
	WorkspaceID           string         `json:"workspace_id"`
	Name                  string         `json:"name"`
	AgentProfileID        string         `json:"agent_profile_id"`
	ExecutorProfileID     string         `json:"executor_profile_id"`
	Context               string         `json:"context"`
	ConversationTaskID    *string        `json:"conversation_task_id"`
	ConfigRevision        int64          `json:"config_revision"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	OpenProposals         *int           `json:"open_proposals,omitempty"`
	AgentProfileStatus    *ProfileStatus `json:"agent_profile_status,omitempty"`
	ExecutorProfileStatus *ProfileStatus `json:"executor_profile_status,omitempty"`
}

// NewCoordinatorDTO builds the base DTO shape shared by every coordinator
// route, from the domain type.
func NewCoordinatorDTO(c *Coordinator) *CoordinatorDTO {
	return &CoordinatorDTO{
		ID:                 c.ID,
		WorkspaceID:        c.WorkspaceID,
		Name:               c.Name,
		AgentProfileID:     c.AgentProfileID,
		ExecutorProfileID:  c.ExecutorProfileID,
		Context:            c.Context,
		ConversationTaskID: c.ConversationTaskID,
		ConfigRevision:     c.ConfigRevision,
		CreatedAt:          c.CreatedAt,
		UpdatedAt:          c.UpdatedAt,
	}
}

// WithOpenProposals sets the list route's open_proposals count and returns
// the receiver.
func (d *CoordinatorDTO) WithOpenProposals(count int) *CoordinatorDTO {
	d.OpenProposals = &count
	return d
}

// WithProfileStatuses sets the GET route's agent_profile_status and
// executor_profile_status and returns the receiver.
func (d *CoordinatorDTO) WithProfileStatuses(agent, executor ProfileStatus) *CoordinatorDTO {
	d.AgentProfileStatus = &agent
	d.ExecutorProfileStatus = &executor
	return d
}

// CoordinatorListResponse is the list route's body.
type CoordinatorListResponse struct {
	Coordinators []*CoordinatorDTO `json:"coordinators"`
}

// NewCoordinatorListResponse wraps items, substituting an empty slice for
// nil so the list route always serializes "coordinators": [] rather than
// null (Build decision 9).
func NewCoordinatorListResponse(items []*CoordinatorDTO) *CoordinatorListResponse {
	if items == nil {
		items = []*CoordinatorDTO{}
	}
	return &CoordinatorListResponse{Coordinators: items}
}

// CreateCoordinatorRequest is the POST .../coordinators request body. An
// absent context defaults to the zero value "" (Build decision 5); create
// has no partial-update semantics, so unlike PatchCoordinatorRequest a plain
// string field is enough.
type CreateCoordinatorRequest struct {
	Name              string `json:"name"`
	AgentProfileID    string `json:"agent_profile_id"`
	ExecutorProfileID string `json:"executor_profile_id"`
	Context           string `json:"context"`
}

// PatchCoordinatorRequest is the raw PATCH .../coordinators/:cid request
// body. It decodes into a map of the fields the caller actually sent so
// name, agent_profile_id, executor_profile_id and context can each be told
// apart as absent, sent as JSON null, or sent with a value (Build decision
// 7); encoding/json's usual pointer-based null handling collapses "absent"
// and "null" for a plain *string field, so this type keeps every field as
// json.RawMessage instead and StringField interprets it. Unknown fields are
// ignored (decision 7): callers only ever look up the four known keys.
type PatchCoordinatorRequest map[string]json.RawMessage

// PatchCoordinatorRequest field names, matching their JSON keys.
const (
	PatchFieldName              = "name"
	PatchFieldAgentProfileID    = "agent_profile_id"
	PatchFieldExecutorProfileID = "executor_profile_id"
	PatchFieldContext           = "context"
)

// StringField reports field's presence and value: (nil, false, nil) when
// field was absent from the body (unchanged), or (value, true, nil) when
// field was sent with a string value. A field sent as JSON null, or as any
// other JSON type, returns a *FieldError naming field, per Build decision
// 7's 400.
func (r PatchCoordinatorRequest) StringField(field string) (*string, bool, error) {
	return rawStringField(r, field)
}

// rawStringField implements the absent/null/string decoding shared by
// PatchCoordinatorRequest (Build decision 7) and ApproveProposalRequest
// (Build decision 16): both tell an absent field apart from an explicit
// JSON null using the same json.RawMessage map technique.
func rawStringField(values map[string]json.RawMessage, field string) (*string, bool, error) {
	raw, present := values[field]
	if !present {
		return nil, false, nil
	}
	if string(raw) == "null" {
		return nil, true, &FieldError{Field: field, Message: field + " must not be null"}
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, true, &FieldError{Field: field, Message: field + " must be a string"}
	}
	return &value, true, nil
}

// ProposalDTO is a proposal's JSON shape (Build decision 10). ClaimToken is
// deliberately never a field here: it is never serialized.
type ProposalDTO struct {
	ID            string         `json:"id"`
	CoordinatorID string         `json:"coordinator_id"`
	WorkspaceID   string         `json:"workspace_id"`
	Status        ProposalStatus `json:"status"`
	Spec          ProposalSpec   `json:"spec"`
	FinalSpec     *ProposalSpec  `json:"final_spec"`
	ClaimedAt     *time.Time     `json:"claimed_at"`
	TaskID        *string        `json:"task_id"`
	Error         *string        `json:"error"`
	RejectReason  *string        `json:"reject_reason"`
	DecidedBy     *string        `json:"decided_by"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// NewProposalDTO builds a ProposalDTO from the domain type.
func NewProposalDTO(p *Proposal) *ProposalDTO {
	return &ProposalDTO{
		ID:            p.ID,
		CoordinatorID: p.CoordinatorID,
		WorkspaceID:   p.WorkspaceID,
		Status:        p.Status,
		Spec:          p.Spec,
		FinalSpec:     p.FinalSpec,
		ClaimedAt:     p.ClaimedAt,
		TaskID:        p.TaskID,
		Error:         p.Error,
		RejectReason:  p.RejectReason,
		DecidedBy:     p.DecidedBy,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

// ProposalListResponse is the proposals list route's body.
type ProposalListResponse struct {
	Proposals []*ProposalDTO `json:"proposals"`
}

// NewProposalListResponse wraps items, substituting an empty slice for nil
// so the list route always serializes "proposals": [] rather than null.
func NewProposalListResponse(items []*ProposalDTO) *ProposalListResponse {
	if items == nil {
		items = []*ProposalDTO{}
	}
	return &ProposalListResponse{Proposals: items}
}

// ProposalConflictResponse is the 409 body every approve or reject route
// returns for a settled or non-stale approving proposal (Build decision 16).
// All three keys are always present.
type ProposalConflictResponse struct {
	Error     string      `json:"error"`
	ErrorCode string      `json:"error_code"`
	Proposal  ProposalDTO `json:"proposal"`
}

// NewProposalConflictResponse builds the 409 body from the proposal row, as
// re-read after the conflict.
func NewProposalConflictResponse(p *Proposal) *ProposalConflictResponse {
	return &ProposalConflictResponse{
		Error:     ErrorCodeProposalConflict,
		ErrorCode: ErrorCodeProposalConflict,
		Proposal:  *NewProposalDTO(p),
	}
}

// ConversationConflictResponse is the 409 body the conversation route
// returns for both race outcomes of copilot.md's conversation-route steps 4
// and 7 (Build decision 16). Both keys are always present.
type ConversationConflictResponse struct {
	Error     string `json:"error"`
	ErrorCode string `json:"error_code"`
}

// NewConversationConflictResponse builds the conversation route's 409 body.
func NewConversationConflictResponse() *ConversationConflictResponse {
	return &ConversationConflictResponse{
		Error:     ErrorCodeConversationConflict,
		ErrorCode: ErrorCodeConversationConflict,
	}
}

// CoordinatorProfileUnavailableResponse is the 409 body returned when either
// coordinator profile is not ok
// (docs/specs/coordinator/system-design/coordinators.md#validation).
type CoordinatorProfileUnavailableResponse struct {
	Error                 string        `json:"error"`
	AgentProfileStatus    ProfileStatus `json:"agent_profile_status"`
	ExecutorProfileStatus ProfileStatus `json:"executor_profile_status"`
}

// NewCoordinatorProfileUnavailableResponse builds the 409 body from the two
// profile statuses.
func NewCoordinatorProfileUnavailableResponse(agent, executor ProfileStatus) *CoordinatorProfileUnavailableResponse {
	return &CoordinatorProfileUnavailableResponse{
		Error:                 ErrorCodeCoordinatorProfileUnavailable,
		AgentProfileStatus:    agent,
		ExecutorProfileStatus: executor,
	}
}

// ConversationResponse is the conversation route's 200 body
// (docs/specs/coordinator/system-design/copilot.md#conversation-task).
// ArchiveState is always false from this route: a task the route would
// otherwise return as archived is returned as a ConversationConflictResponse
// 409 instead.
type ConversationResponse struct {
	TaskID       string `json:"task_id"`
	SessionID    string `json:"session_id"`
	ArchiveState bool   `json:"archive_state"`
}

// StallDTO is a stall record's JSON shape
// (docs/specs/coordinator/system-design/needs-you.md#inputs). WorkspaceID is
// deliberately not a field: the route is scoped to one workspace by its URL.
type StallDTO struct {
	TaskID       string    `json:"task_id"`
	StalledForMs int64     `json:"stalled_for_ms"`
	LastEventAt  time.Time `json:"last_event_at"`
	DetectedAt   time.Time `json:"detected_at"`
}

// NewStallDTO builds a StallDTO from the domain type.
func NewStallDTO(s *Stall) *StallDTO {
	return &StallDTO{
		TaskID:       s.TaskID,
		StalledForMs: s.StalledForMs,
		LastEventAt:  s.LastEventAt,
		DetectedAt:   s.DetectedAt,
	}
}

// StallListResponse is the stalls route's body.
type StallListResponse struct {
	Stalls []*StallDTO `json:"stalls"`
}

// NewStallListResponse wraps items, substituting an empty slice for nil so
// the stalls route always serializes "stalls": [] rather than null.
func NewStallListResponse(items []*StallDTO) *StallListResponse {
	if items == nil {
		items = []*StallDTO{}
	}
	return &StallListResponse{Stalls: items}
}

// ApproveProposalRequest is the raw POST .../proposals/:pid/approve request
// body: an optional set of edits to the proposal's spec, applied before
// approving (Build decision 16, proposals.md#edits). Like
// PatchCoordinatorRequest, it decodes into a map of json.RawMessage so an
// absent field (unchanged) can be told apart from an explicit JSON null
// (400 naming the field); rationale and source_task_id are not editable and,
// like any unknown field, are ignored. The handler that applies these edits
// lands in task 07.
type ApproveProposalRequest map[string]json.RawMessage

// ApproveProposalRequest field names, matching their JSON keys
// (proposals.md#edits).
const (
	ApproveFieldTitle        = "title"
	ApproveFieldDescription  = "description"
	ApproveFieldWorkflowID   = mcpcontract.FieldWorkflowID
	ApproveFieldStepID       = mcpcontract.FieldStepID
	ApproveFieldRepositoryID = "repository_id"
)

// fieldSourceTaskID names a proposal spec's source task in field errors.
const fieldSourceTaskID = "source_task_id"

// StringField reports field's presence and value, identically to
// PatchCoordinatorRequest.StringField.
func (r ApproveProposalRequest) StringField(field string) (*string, bool, error) {
	return rawStringField(r, field)
}

// RejectProposalRequest is the POST .../proposals/:pid/reject request body
// (Build decision 16, proposals.md#reject). Unlike the approve edits,
// absent, JSON null, an empty string and a whitespace-only string are all
// handled identically by the caller (as "no reason"), so a plain optional
// string field is enough: encoding/json already treats an absent key and an
// explicit null the same way for a pointer field. The handler that applies
// this lands in task 07.
type RejectProposalRequest struct {
	Reason *string `json:"reason"`
}
