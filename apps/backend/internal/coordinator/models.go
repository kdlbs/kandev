// Package coordinator implements workspace coordinators: a per-workspace
// configuration (agent profile, executor profile, standing context) that a
// copilot conversation uses to propose ordinary, unstarted tasks for a human
// to approve. See docs/specs/coordinator/system-design/{coordinators,
// proposals,needs-you,copilot}.md.
package coordinator

import (
	"time"

	"github.com/kandev/kandev/internal/coordinator/mcpcontract"
)

// ActionProposeTask is the MCP action name the copilot's propose_task_kandev
// tool dispatches (docs/specs/coordinator/system-design/proposals.md#propose).
// The dispatch site is owned by a later work package.
const ActionProposeTask = mcpcontract.ActionProposeTask

// ActionGetItem is mcpcontract.ActionGetItem, re-exported
// (docs/specs/coordinator/system-design/copilot-tools.md#item-read).
const ActionGetItem = mcpcontract.ActionGetItem

// ActionListActivity is mcpcontract.ActionListActivity, re-exported.
const ActionListActivity = mcpcontract.ActionListActivity

// ActionApproveProposal, ActionRejectProposal and DecisionActions re-export
// mcpcontract's reserved decision action names
// (docs/specs/coordinator/system-design/proposals.md#security) for callers
// that already import this package.
const (
	ActionApproveProposal = mcpcontract.ActionApproveProposal
	ActionRejectProposal  = mcpcontract.ActionRejectProposal
)

// DecisionActions is mcpcontract.DecisionActions, re-exported.
var DecisionActions = mcpcontract.DecisionActions

// Coordinator is a workspace's coordinator configuration.
type Coordinator struct {
	ID                 string
	WorkspaceID        string
	Name               string
	AgentProfileID     string
	ExecutorProfileID  string
	Context            string
	ConversationTaskID *string
	ConfigRevision     int64
	CreatedAt          time.Time
	UpdatedAt          time.Time

	// PolicyJSON is the stored permission map, nil for a phase-1 coordinator.
	PolicyJSON     *string
	PolicyRevision int
	// WatchScope is the stored scope, "all" by default.
	WatchScope string
}

// ProposalStatus is the coordinator_proposals.status enum.
type ProposalStatus string

// Proposal status values, per
// docs/specs/coordinator/system-design/proposals.md#store.
const (
	ProposalStatusPending   ProposalStatus = "pending"
	ProposalStatusApproving ProposalStatus = "approving"
	ProposalStatusApproved  ProposalStatus = "approved"
	ProposalStatusRejected  ProposalStatus = "rejected"
	ProposalStatusFailed    ProposalStatus = "failed"
)

// ProposalSpec is the proposed task's shape, serialized into spec_json /
// final_spec_json. Rationale and SourceTaskID are not editable through
// approve edits.
type ProposalSpec struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Rationale    string `json:"rationale"`
	WorkflowID   string `json:"workflow_id"`
	StepID       string `json:"step_id"`
	RepositoryID string `json:"repository_id"`
	SourceTaskID string `json:"source_task_id"`
}

// Proposal is a coordinator's proposed task, awaiting or past a human
// decision.
type Proposal struct {
	ID            string
	CoordinatorID string
	WorkspaceID   string
	Status        ProposalStatus
	Spec          ProposalSpec
	FinalSpec     *ProposalSpec
	ClaimedAt     *time.Time
	ClaimToken    *string
	TaskID        *string
	Error         *string
	RejectReason  *string
	DecidedBy     *string
	CreatedAt     time.Time
	UpdatedAt     time.Time

	// Kind is the stored proposal kind. Phase-1 rows read as create_task.
	Kind             string
	TargetTaskID     *string
	StandingOrderIDs []string
	StartsAgent      bool
	OutcomeJSON      *string
	// RawSpec is the stored spec JSON of a non-create_task kind, whose shape
	// this package does not parse.
	RawSpec string
	// RawFinalSpec is the frozen spec of a non-create_task kind, set once claimed.
	RawFinalSpec string
}

// Proposal kinds.
const (
	ProposalKindCreateTask = "create_task"
	ProposalKindMessage    = "message"
	ProposalKindMove       = "move"
	ProposalKindResume     = "resume"
)

// Stall is a coordinator_stalls row: the most recent task.stalled episode
// observed for a task, in a workspace with at least one coordinator.
type Stall struct {
	TaskID       string
	WorkspaceID  string
	StalledForMs int64
	LastEventAt  time.Time
	DetectedAt   time.Time
}

// StepNode is the subset of a workflow step's shape the step-eligibility
// check needs: identity, start/manual-move flags, whether it auto-starts an
// agent on enter, and its feeder link.
type StepNode struct {
	ID               string
	IsStart          bool
	AllowManualMove  bool
	AutoStartOnEnter bool
	CompletesOnEnter bool
	PullFromStepID   string
}
