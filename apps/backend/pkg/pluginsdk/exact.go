package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

// CapabilityContext describes the exact Host operations available to this
// installation in one workspace and the approval revision used to evaluate
// them.
type CapabilityContext struct {
	InstallationID     string
	WorkspaceID        string
	ManifestDigest     string
	ApprovalRevision   uint64
	ApprovalState      string
	Operations         []CapabilityOperation
	Limits             map[string]uint64
	SupportedProviders []string
}

// CapabilityOperation reports one exact Host method, whether the host can
// serve it, and whether this workspace currently authorizes it.
type CapabilityOperation struct {
	Method            string
	CapabilityID      string
	Description       string
	Supported         bool
	Authorized        bool
	UnavailableReason string
}

// CommandStatus is the stable result vocabulary for exact Host commands.
type CommandStatus string

const (
	CommandApplied        CommandStatus = "APPLIED"
	CommandAlreadyApplied CommandStatus = "ALREADY_APPLIED"
	CommandNoChange       CommandStatus = "NO_CHANGE"
	CommandConflict       CommandStatus = "CONFLICT"
	CommandDenied         CommandStatus = "DENIED"
	CommandNotFound       CommandStatus = "NOT_FOUND"
	CommandInvalid        CommandStatus = "INVALID"
	CommandUnsupported    CommandStatus = "UNSUPPORTED"
	CommandRateLimited    CommandStatus = "RATE_LIMITED"
	CommandUnavailable    CommandStatus = "UNAVAILABLE"
	CommandPartial        CommandStatus = "PARTIAL"
	CommandUncertain      CommandStatus = "UNCERTAIN"
)

// CommandReceipt identifies the durable Host command intent and its outcome.
type CommandReceipt struct {
	ID               string
	OperationID      string
	Status           CommandStatus
	TargetID         string
	ResourceVersion  string
	ApprovalRevision uint64
	CreatedAt        string
	UpdatedAt        string
}

// CommandResult is the typed outcome of an exact Host command.
type CommandResult struct {
	Status  CommandStatus
	Reason  string
	Receipt *CommandReceipt
}

// ExactTaskUpdate contains one version-checked, idempotent task update.
type ExactTaskUpdate struct {
	RequestID               string
	WorkspaceID             string
	TaskID                  string
	IdempotencyKey          string
	ExpectedResourceVersion string
	ApprovalRevision        uint64
	ManifestDigest          string
	ManagementInstanceKey   string
	ExpectedClaimGeneration int64
	Title                   *string
	Description             *string
	State                   *string
	Priority                *string
}

// ManagedAgentConversationSpec creates a retained, installation-owned
// conversation or updates its launch settings at the supplied revision.
type ManagedAgentConversationSpec struct {
	RequestID          string
	IdempotencyKey     string
	WorkspaceID        string
	InstanceKey        string
	ExpectedRevision   uint64
	ApprovalRevision   uint64
	ManifestDigest     string
	AgentProfileID     string
	ExecutorID         string
	ExecutorProfileID  string
	BasePrompt         string
	InstructionVersion string
	AgentToolNames     []string
}

// ManagedAgentConversationQuery identifies one retained conversation read.
type ManagedAgentConversationQuery struct {
	WorkspaceID      string
	InstanceKey      string
	ApprovalRevision uint64
	ManifestDigest   string
}

// ManagedAgentConversationListQuery scopes retained conversation discovery.
type ManagedAgentConversationListQuery struct {
	WorkspaceID      string
	ApprovalRevision uint64
	ManifestDigest   string
}

// ManagedAgentConversationPause changes desired pause state at one revision.
type ManagedAgentConversationPause struct {
	RequestID        string
	IdempotencyKey   string
	WorkspaceID      string
	InstanceKey      string
	ExpectedRevision uint64
	ApprovalRevision uint64
	ManifestDigest   string
	Paused           bool
}

// ManagedAgentConversationDelete removes one retained conversation at one revision.
type ManagedAgentConversationDelete struct {
	RequestID        string
	IdempotencyKey   string
	WorkspaceID      string
	InstanceKey      string
	ExpectedRevision uint64
	ApprovalRevision uint64
	ManifestDigest   string
}

// ManagedAgentConversationDescriptor is a host-owned projection of a
// retained conversation. Internal task and session IDs are opaque handles.
type ManagedAgentConversationDescriptor struct {
	InstallationID     string
	TaskID             string
	SessionID          string
	WorkspaceID        string
	InstanceKey        string
	Revision           uint64
	AgentProfileID     string
	ExecutorID         string
	ExecutorProfileID  string
	BasePrompt         string
	InstructionVersion string
	DesiredPaused      bool
	RetentionMode      string
	Detached           bool
	AgentToolNames     []string
}

// ManagedAgentInputOrigin identifies the source of durable conversation
// input. Only periodic inputs can request replacement through a coalesce key.
type ManagedAgentInputOrigin string

const (
	ManagedAgentInputHuman       ManagedAgentInputOrigin = "human"
	ManagedAgentInputAutomation  ManagedAgentInputOrigin = "automation"
	ManagedAgentInputPeriodic    ManagedAgentInputOrigin = "periodic"
	ManagedAgentInputInteraction ManagedAgentInputOrigin = "interaction"
)

// ManagedAgentInputState is the durable lifecycle state of one accepted input.
type ManagedAgentInputState string

const (
	ManagedAgentInputAccepted  ManagedAgentInputState = "accepted"
	ManagedAgentInputRunning   ManagedAgentInputState = "running"
	ManagedAgentInputCompleted ManagedAgentInputState = "completed"
	ManagedAgentInputFailed    ManagedAgentInputState = "failed"
	ManagedAgentInputCancelled ManagedAgentInputState = "cancelled"
	ManagedAgentInputUncertain ManagedAgentInputState = "uncertain"
)

// ManagedAgentDispatchStatus describes an immediate attempt to start a
// conversation turn. Busy is an explicit result and never means queued.
type ManagedAgentDispatchStatus string

const (
	ManagedAgentDispatchBusy    ManagedAgentDispatchStatus = "busy"
	ManagedAgentDispatchStarted ManagedAgentDispatchStatus = "started"
	ManagedAgentDispatchSent    ManagedAgentDispatchStatus = "sent"
)

// ManagedAgentInputReceipt is the host-owned receipt for one durable input.
// Payload is canonical text; queue and execution identifiers remain opaque.
type ManagedAgentInputReceipt struct {
	HostInputID          string
	OccurrenceKey        string
	Sequence             uint64
	Origin               ManagedAgentInputOrigin
	Payload              string
	CoalesceKey          string
	ConversationRevision uint64
	State                ManagedAgentInputState
	CreatedAt            string
	UpdatedAt            string
	QueueEntryID         string
	ExecutionID          string
	TurnID               string
	SupersededBy         string
}

// ManagedAgentInputEnqueue requests an idempotent durable input admission.
// CoalesceKey is allowed only for periodic origin and must be non-empty when
// periodic replacement is requested.
type ManagedAgentInputEnqueue struct {
	RequestID                    string
	IdempotencyKey               string
	WorkspaceID                  string
	InstanceKey                  string
	ExpectedConversationRevision uint64
	ApprovalRevision             uint64
	ManifestDigest               string
	OccurrenceKey                string
	Origin                       ManagedAgentInputOrigin
	Payload                      string
	CoalesceKey                  string
}

// ManagedAgentInputQuery identifies one input under the current approved
// conversation authority.
type ManagedAgentInputQuery struct {
	WorkspaceID      string
	InstanceKey      string
	HostInputID      string
	ApprovalRevision uint64
	ManifestDigest   string
}

// ManagedAgentInputListQuery scopes a bounded sequence-cursor read.
type ManagedAgentInputListQuery struct {
	WorkspaceID      string
	InstanceKey      string
	SequenceCursor   uint64
	Limit            uint32
	ApprovalRevision uint64
	ManifestDigest   string
}

// ManagedAgentInputPage is one bounded page of receipts after the supplied
// sequence cursor.
type ManagedAgentInputPage struct {
	Inputs             []ManagedAgentInputReceipt
	NextSequenceCursor uint64
	HasMore            bool
}

// ManagedAgentInputCancel cancels one accepted input or stops one exact
// running execution. ExpectedExecutionID is empty only for accepted inputs.
type ManagedAgentInputCancel struct {
	RequestID                    string
	IdempotencyKey               string
	WorkspaceID                  string
	InstanceKey                  string
	HostInputID                  string
	ExpectedConversationRevision uint64
	ExpectedExecutionID          string
	ApprovalRevision             uint64
	ManifestDigest               string
}

// ManagedAgentConversationDispatch requests an immediate turn. If the
// conversation is busy, Dispatch returns ManagedAgentDispatchBusy and does
// not enqueue the payload.
type ManagedAgentConversationDispatch struct {
	RequestID                    string
	IdempotencyKey               string
	WorkspaceID                  string
	InstanceKey                  string
	ExpectedConversationRevision uint64
	ApprovalRevision             uint64
	ManifestDigest               string
	OccurrenceKey                string
	Origin                       ManagedAgentInputOrigin
	Payload                      string
	CoalesceKey                  string
}

// ManagedAgentConversationManager is the exact, retained conversation
// surface. It remains separate from v1 AgentConversationManager, whose
// delete-on-disable lifetime stays unchanged.
type ManagedAgentConversationManager interface {
	Ensure(ctx context.Context, spec ManagedAgentConversationSpec) (*CommandResult, ManagedAgentConversationDescriptor, error)
	Get(ctx context.Context, query ManagedAgentConversationQuery) (ManagedAgentConversationDescriptor, error)
	List(ctx context.Context, query ManagedAgentConversationListQuery) ([]ManagedAgentConversationDescriptor, error)
	SetPaused(ctx context.Context, input ManagedAgentConversationPause) (*CommandResult, ManagedAgentConversationDescriptor, error)
	Delete(ctx context.Context, input ManagedAgentConversationDelete) (*CommandResult, error)
	EnqueueInput(ctx context.Context, input ManagedAgentInputEnqueue) (*CommandResult, ManagedAgentInputReceipt, error)
	GetInput(ctx context.Context, query ManagedAgentInputQuery) (ManagedAgentInputReceipt, error)
	ListInputs(ctx context.Context, query ManagedAgentInputListQuery) (ManagedAgentInputPage, error)
	CancelInput(ctx context.Context, input ManagedAgentInputCancel) (*CommandResult, ManagedAgentInputReceipt, error)
	Dispatch(ctx context.Context, input ManagedAgentConversationDispatch) (*CommandResult, ManagedAgentDispatchStatus, ManagedAgentConversationDescriptor, error)
}

// ExactHost is the additive v2 extension to Host. A type assertion preserves
// source compatibility for plugins and host implementations that only use v1.
type ExactHost interface {
	Host
	GetCapabilityContext(ctx context.Context, workspaceID string) (*CapabilityContext, error)
	UpdateTaskExact(ctx context.Context, input ExactTaskUpdate) (*CommandResult, *Task, error)
	ManagedAgentConversations() ManagedAgentConversationManager
}

// HostV2 reports whether host implements the additive exact-operation
// contract. Plugins can keep using the unchanged Host v1 methods when an
// older Kandev host does not advertise this extension.
func HostV2(host Host) (ExactHost, bool) {
	exact, ok := host.(ExactHost)
	return exact, ok
}

func capabilityContextToProto(value *CapabilityContext) *pluginv1.HostCapabilityContext {
	if value == nil {
		return nil
	}
	operations := make([]*pluginv1.HostOperationSupport, len(value.Operations))
	for i, operation := range value.Operations {
		operations[i] = &pluginv1.HostOperationSupport{
			Method: operation.Method, CapabilityId: operation.CapabilityID,
			Description: operation.Description, Supported: operation.Supported,
			Authorized: operation.Authorized, UnavailableReason: operation.UnavailableReason,
		}
	}
	return &pluginv1.HostCapabilityContext{
		InstallationId: value.InstallationID, WorkspaceId: value.WorkspaceID,
		ManifestDigest: value.ManifestDigest, ApprovalRevision: value.ApprovalRevision,
		ApprovalState: value.ApprovalState, Operations: operations,
		Limits: value.Limits, SupportedProviders: value.SupportedProviders,
	}
}

func capabilityContextFromProto(value *pluginv1.HostCapabilityContext) *CapabilityContext {
	if value == nil {
		return nil
	}
	operations := make([]CapabilityOperation, len(value.GetOperations()))
	for i, operation := range value.GetOperations() {
		operations[i] = CapabilityOperation{
			Method: operation.GetMethod(), CapabilityID: operation.GetCapabilityId(),
			Description: operation.GetDescription(), Supported: operation.GetSupported(),
			Authorized: operation.GetAuthorized(), UnavailableReason: operation.GetUnavailableReason(),
		}
	}
	limits := make(map[string]uint64, len(value.GetLimits()))
	for key, limit := range value.GetLimits() {
		limits[key] = limit
	}
	return &CapabilityContext{
		InstallationID: value.GetInstallationId(), WorkspaceID: value.GetWorkspaceId(),
		ManifestDigest: value.GetManifestDigest(), ApprovalRevision: value.GetApprovalRevision(),
		ApprovalState: value.GetApprovalState(), Operations: operations, Limits: limits,
		SupportedProviders: append([]string(nil), value.GetSupportedProviders()...),
	}
}

func exactTaskUpdateToProto(value ExactTaskUpdate) *pluginv1.UpdateTaskExactRequest {
	return &pluginv1.UpdateTaskExactRequest{
		RequestId: value.RequestID, WorkspaceId: value.WorkspaceID, TaskId: value.TaskID,
		IdempotencyKey: value.IdempotencyKey, ExpectedResourceVersion: value.ExpectedResourceVersion,
		Title: value.Title, Description: value.Description, State: value.State, Priority: value.Priority,
		ManagementInstanceKey: value.ManagementInstanceKey, ExpectedClaimGeneration: value.ExpectedClaimGeneration,
		ApprovalRevision: value.ApprovalRevision, ManifestDigest: value.ManifestDigest,
	}
}

func exactTaskUpdateFromProto(value *pluginv1.UpdateTaskExactRequest) ExactTaskUpdate {
	if value == nil {
		return ExactTaskUpdate{}
	}
	return ExactTaskUpdate{
		RequestID: value.GetRequestId(), WorkspaceID: value.GetWorkspaceId(), TaskID: value.GetTaskId(),
		IdempotencyKey: value.GetIdempotencyKey(), ExpectedResourceVersion: value.GetExpectedResourceVersion(),
		ApprovalRevision: value.GetApprovalRevision(), ManifestDigest: value.GetManifestDigest(),
		ManagementInstanceKey: value.GetManagementInstanceKey(), ExpectedClaimGeneration: value.GetExpectedClaimGeneration(),
		Title: value.Title, Description: value.Description, State: value.State, Priority: value.Priority,
	}
}

func managedConversationSpecToProto(value ManagedAgentConversationSpec) *pluginv1.ManagedAgentConversationSpec {
	return &pluginv1.ManagedAgentConversationSpec{
		RequestId: value.RequestID, IdempotencyKey: value.IdempotencyKey,
		WorkspaceId: value.WorkspaceID, InstanceKey: value.InstanceKey,
		ExpectedRevision: value.ExpectedRevision, ApprovalRevision: value.ApprovalRevision,
		ManifestDigest: value.ManifestDigest, AgentProfileId: value.AgentProfileID,
		ExecutorId: value.ExecutorID, ExecutorProfileId: value.ExecutorProfileID,
		BasePrompt: value.BasePrompt, InstructionVersion: value.InstructionVersion,
		AgentToolNames: append([]string(nil), value.AgentToolNames...),
	}
}

func managedConversationSpecFromProto(value *pluginv1.ManagedAgentConversationSpec) ManagedAgentConversationSpec {
	if value == nil {
		return ManagedAgentConversationSpec{}
	}
	return ManagedAgentConversationSpec{
		RequestID: value.GetRequestId(), IdempotencyKey: value.GetIdempotencyKey(),
		WorkspaceID: value.GetWorkspaceId(), InstanceKey: value.GetInstanceKey(),
		ExpectedRevision: value.GetExpectedRevision(), ApprovalRevision: value.GetApprovalRevision(),
		ManifestDigest: value.GetManifestDigest(), AgentProfileID: value.GetAgentProfileId(),
		ExecutorID: value.GetExecutorId(), ExecutorProfileID: value.GetExecutorProfileId(),
		BasePrompt: value.GetBasePrompt(), InstructionVersion: value.GetInstructionVersion(),
		AgentToolNames: append([]string(nil), value.GetAgentToolNames()...),
	}
}

func managedConversationToProto(value ManagedAgentConversationDescriptor) *pluginv1.ManagedAgentConversationDescriptor {
	return &pluginv1.ManagedAgentConversationDescriptor{
		InstallationId: value.InstallationID, TaskId: value.TaskID, SessionId: value.SessionID,
		WorkspaceId: value.WorkspaceID, InstanceKey: value.InstanceKey, Revision: value.Revision,
		AgentProfileId: value.AgentProfileID, ExecutorId: value.ExecutorID,
		ExecutorProfileId: value.ExecutorProfileID, BasePrompt: value.BasePrompt,
		InstructionVersion: value.InstructionVersion, DesiredPaused: value.DesiredPaused,
		RetentionMode: value.RetentionMode, Detached: value.Detached,
		AgentToolNames: append([]string(nil), value.AgentToolNames...),
	}
}

func managedConversationFromProto(value *pluginv1.ManagedAgentConversationDescriptor) ManagedAgentConversationDescriptor {
	if value == nil {
		return ManagedAgentConversationDescriptor{}
	}
	return ManagedAgentConversationDescriptor{
		InstallationID: value.GetInstallationId(), TaskID: value.GetTaskId(), SessionID: value.GetSessionId(),
		WorkspaceID: value.GetWorkspaceId(), InstanceKey: value.GetInstanceKey(), Revision: value.GetRevision(),
		AgentProfileID: value.GetAgentProfileId(), ExecutorID: value.GetExecutorId(),
		ExecutorProfileID: value.GetExecutorProfileId(), BasePrompt: value.GetBasePrompt(),
		InstructionVersion: value.GetInstructionVersion(), DesiredPaused: value.GetDesiredPaused(),
		RetentionMode: value.GetRetentionMode(), Detached: value.GetDetached(),
		AgentToolNames: append([]string(nil), value.GetAgentToolNames()...),
	}
}

func managedAgentInputOriginToProto(value ManagedAgentInputOrigin) pluginv1.ManagedAgentInputOrigin {
	switch value {
	case ManagedAgentInputHuman:
		return pluginv1.ManagedAgentInputOrigin_MANAGED_AGENT_INPUT_ORIGIN_HUMAN
	case ManagedAgentInputAutomation:
		return pluginv1.ManagedAgentInputOrigin_MANAGED_AGENT_INPUT_ORIGIN_AUTOMATION
	case ManagedAgentInputPeriodic:
		return pluginv1.ManagedAgentInputOrigin_MANAGED_AGENT_INPUT_ORIGIN_PERIODIC
	case ManagedAgentInputInteraction:
		return pluginv1.ManagedAgentInputOrigin_MANAGED_AGENT_INPUT_ORIGIN_INTERACTION
	default:
		return pluginv1.ManagedAgentInputOrigin_MANAGED_AGENT_INPUT_ORIGIN_UNSPECIFIED
	}
}

func managedAgentInputOriginFromProto(value pluginv1.ManagedAgentInputOrigin) ManagedAgentInputOrigin {
	switch value {
	case pluginv1.ManagedAgentInputOrigin_MANAGED_AGENT_INPUT_ORIGIN_HUMAN:
		return ManagedAgentInputHuman
	case pluginv1.ManagedAgentInputOrigin_MANAGED_AGENT_INPUT_ORIGIN_AUTOMATION:
		return ManagedAgentInputAutomation
	case pluginv1.ManagedAgentInputOrigin_MANAGED_AGENT_INPUT_ORIGIN_PERIODIC:
		return ManagedAgentInputPeriodic
	case pluginv1.ManagedAgentInputOrigin_MANAGED_AGENT_INPUT_ORIGIN_INTERACTION:
		return ManagedAgentInputInteraction
	default:
		return ""
	}
}

func managedAgentInputStateToProto(value ManagedAgentInputState) pluginv1.ManagedAgentInputState {
	switch value {
	case ManagedAgentInputAccepted:
		return pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_ACCEPTED
	case ManagedAgentInputRunning:
		return pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_RUNNING
	case ManagedAgentInputCompleted:
		return pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_COMPLETED
	case ManagedAgentInputFailed:
		return pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_FAILED
	case ManagedAgentInputCancelled:
		return pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_CANCELLED
	case ManagedAgentInputUncertain:
		return pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_UNCERTAIN
	default:
		return pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_UNSPECIFIED
	}
}

func managedAgentInputStateFromProto(value pluginv1.ManagedAgentInputState) ManagedAgentInputState {
	switch value {
	case pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_ACCEPTED:
		return ManagedAgentInputAccepted
	case pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_RUNNING:
		return ManagedAgentInputRunning
	case pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_COMPLETED:
		return ManagedAgentInputCompleted
	case pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_FAILED:
		return ManagedAgentInputFailed
	case pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_CANCELLED:
		return ManagedAgentInputCancelled
	case pluginv1.ManagedAgentInputState_MANAGED_AGENT_INPUT_STATE_UNCERTAIN:
		return ManagedAgentInputUncertain
	default:
		return ""
	}
}

func managedAgentDispatchStatusToProto(value ManagedAgentDispatchStatus) pluginv1.ManagedAgentDispatchStatus {
	switch value {
	case ManagedAgentDispatchBusy:
		return pluginv1.ManagedAgentDispatchStatus_MANAGED_AGENT_DISPATCH_STATUS_BUSY
	case ManagedAgentDispatchStarted:
		return pluginv1.ManagedAgentDispatchStatus_MANAGED_AGENT_DISPATCH_STATUS_STARTED
	case ManagedAgentDispatchSent:
		return pluginv1.ManagedAgentDispatchStatus_MANAGED_AGENT_DISPATCH_STATUS_SENT
	default:
		return pluginv1.ManagedAgentDispatchStatus_MANAGED_AGENT_DISPATCH_STATUS_UNSPECIFIED
	}
}

func managedAgentDispatchStatusFromProto(value pluginv1.ManagedAgentDispatchStatus) ManagedAgentDispatchStatus {
	switch value {
	case pluginv1.ManagedAgentDispatchStatus_MANAGED_AGENT_DISPATCH_STATUS_BUSY:
		return ManagedAgentDispatchBusy
	case pluginv1.ManagedAgentDispatchStatus_MANAGED_AGENT_DISPATCH_STATUS_STARTED:
		return ManagedAgentDispatchStarted
	case pluginv1.ManagedAgentDispatchStatus_MANAGED_AGENT_DISPATCH_STATUS_SENT:
		return ManagedAgentDispatchSent
	default:
		return ""
	}
}

func managedAgentInputToProto(value ManagedAgentInputReceipt) *pluginv1.ManagedAgentInputReceipt {
	return &pluginv1.ManagedAgentInputReceipt{
		HostInputId: value.HostInputID, OccurrenceKey: value.OccurrenceKey,
		Sequence: value.Sequence, Origin: managedAgentInputOriginToProto(value.Origin),
		Payload: value.Payload, CoalesceKey: value.CoalesceKey,
		ConversationRevision: value.ConversationRevision, State: managedAgentInputStateToProto(value.State),
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, QueueEntryId: value.QueueEntryID,
		ExecutionId: value.ExecutionID, TurnId: value.TurnID, SupersededById: value.SupersededBy,
	}
}

func managedAgentInputFromProto(value *pluginv1.ManagedAgentInputReceipt) ManagedAgentInputReceipt {
	if value == nil {
		return ManagedAgentInputReceipt{}
	}
	return ManagedAgentInputReceipt{
		HostInputID: value.GetHostInputId(), OccurrenceKey: value.GetOccurrenceKey(),
		Sequence: value.GetSequence(), Origin: managedAgentInputOriginFromProto(value.GetOrigin()),
		Payload: value.GetPayload(), CoalesceKey: value.GetCoalesceKey(),
		ConversationRevision: value.GetConversationRevision(), State: managedAgentInputStateFromProto(value.GetState()),
		CreatedAt: value.GetCreatedAt(), UpdatedAt: value.GetUpdatedAt(), QueueEntryID: value.GetQueueEntryId(),
		ExecutionID: value.GetExecutionId(), TurnID: value.GetTurnId(), SupersededBy: value.GetSupersededById(),
	}
}

func managedAgentInputEnqueueToProto(value ManagedAgentInputEnqueue) *pluginv1.EnqueueManagedAgentInputExactRequest {
	return &pluginv1.EnqueueManagedAgentInputExactRequest{
		RequestId: value.RequestID, IdempotencyKey: value.IdempotencyKey,
		WorkspaceId: value.WorkspaceID, InstanceKey: value.InstanceKey,
		ExpectedConversationRevision: value.ExpectedConversationRevision,
		ApprovalRevision:             value.ApprovalRevision, ManifestDigest: value.ManifestDigest,
		OccurrenceKey: value.OccurrenceKey, Origin: managedAgentInputOriginToProto(value.Origin),
		Payload: value.Payload, CoalesceKey: value.CoalesceKey,
	}
}

func managedAgentInputEnqueueFromProto(value *pluginv1.EnqueueManagedAgentInputExactRequest) ManagedAgentInputEnqueue {
	if value == nil {
		return ManagedAgentInputEnqueue{}
	}
	return ManagedAgentInputEnqueue{
		RequestID: value.GetRequestId(), IdempotencyKey: value.GetIdempotencyKey(),
		WorkspaceID: value.GetWorkspaceId(), InstanceKey: value.GetInstanceKey(),
		ExpectedConversationRevision: value.GetExpectedConversationRevision(),
		ApprovalRevision:             value.GetApprovalRevision(), ManifestDigest: value.GetManifestDigest(),
		OccurrenceKey: value.GetOccurrenceKey(), Origin: managedAgentInputOriginFromProto(value.GetOrigin()),
		Payload: value.GetPayload(), CoalesceKey: value.GetCoalesceKey(),
	}
}

func managedAgentInputQueryToProto(value ManagedAgentInputQuery) *pluginv1.GetManagedAgentInputExactRequest {
	return &pluginv1.GetManagedAgentInputExactRequest{
		WorkspaceId: value.WorkspaceID, InstanceKey: value.InstanceKey, HostInputId: value.HostInputID,
		ApprovalRevision: value.ApprovalRevision, ManifestDigest: value.ManifestDigest,
	}
}

func managedAgentInputQueryFromProto(value *pluginv1.GetManagedAgentInputExactRequest) ManagedAgentInputQuery {
	if value == nil {
		return ManagedAgentInputQuery{}
	}
	return ManagedAgentInputQuery{
		WorkspaceID: value.GetWorkspaceId(), InstanceKey: value.GetInstanceKey(),
		HostInputID: value.GetHostInputId(), ApprovalRevision: value.GetApprovalRevision(),
		ManifestDigest: value.GetManifestDigest(),
	}
}

func managedAgentInputListQueryToProto(value ManagedAgentInputListQuery) *pluginv1.ListManagedAgentInputsExactRequest {
	return &pluginv1.ListManagedAgentInputsExactRequest{
		WorkspaceId: value.WorkspaceID, InstanceKey: value.InstanceKey,
		SequenceCursor: value.SequenceCursor, Limit: value.Limit,
		ApprovalRevision: value.ApprovalRevision, ManifestDigest: value.ManifestDigest,
	}
}

func managedAgentInputListQueryFromProto(value *pluginv1.ListManagedAgentInputsExactRequest) ManagedAgentInputListQuery {
	if value == nil {
		return ManagedAgentInputListQuery{}
	}
	return ManagedAgentInputListQuery{
		WorkspaceID: value.GetWorkspaceId(), InstanceKey: value.GetInstanceKey(),
		SequenceCursor: value.GetSequenceCursor(), Limit: value.GetLimit(),
		ApprovalRevision: value.GetApprovalRevision(), ManifestDigest: value.GetManifestDigest(),
	}
}

func managedAgentInputCancelToProto(value ManagedAgentInputCancel) *pluginv1.CancelManagedAgentInputExactRequest {
	return &pluginv1.CancelManagedAgentInputExactRequest{
		RequestId: value.RequestID, IdempotencyKey: value.IdempotencyKey,
		WorkspaceId: value.WorkspaceID, InstanceKey: value.InstanceKey, HostInputId: value.HostInputID,
		ExpectedConversationRevision: value.ExpectedConversationRevision,
		ExpectedExecutionId:          value.ExpectedExecutionID,
		ApprovalRevision:             value.ApprovalRevision, ManifestDigest: value.ManifestDigest,
	}
}

func managedAgentInputCancelFromProto(value *pluginv1.CancelManagedAgentInputExactRequest) ManagedAgentInputCancel {
	if value == nil {
		return ManagedAgentInputCancel{}
	}
	return ManagedAgentInputCancel{
		RequestID: value.GetRequestId(), IdempotencyKey: value.GetIdempotencyKey(),
		WorkspaceID: value.GetWorkspaceId(), InstanceKey: value.GetInstanceKey(), HostInputID: value.GetHostInputId(),
		ExpectedConversationRevision: value.GetExpectedConversationRevision(),
		ExpectedExecutionID:          value.GetExpectedExecutionId(), ApprovalRevision: value.GetApprovalRevision(),
		ManifestDigest: value.GetManifestDigest(),
	}
}

func managedAgentConversationDispatchToProto(value ManagedAgentConversationDispatch) *pluginv1.DispatchManagedAgentConversationExactRequest {
	return &pluginv1.DispatchManagedAgentConversationExactRequest{
		RequestId: value.RequestID, IdempotencyKey: value.IdempotencyKey,
		WorkspaceId: value.WorkspaceID, InstanceKey: value.InstanceKey,
		ExpectedConversationRevision: value.ExpectedConversationRevision,
		ApprovalRevision:             value.ApprovalRevision, ManifestDigest: value.ManifestDigest,
		OccurrenceKey: value.OccurrenceKey, Origin: managedAgentInputOriginToProto(value.Origin),
		Payload: value.Payload, CoalesceKey: value.CoalesceKey,
	}
}

func managedAgentConversationDispatchFromProto(value *pluginv1.DispatchManagedAgentConversationExactRequest) ManagedAgentConversationDispatch {
	if value == nil {
		return ManagedAgentConversationDispatch{}
	}
	return ManagedAgentConversationDispatch{
		RequestID: value.GetRequestId(), IdempotencyKey: value.GetIdempotencyKey(),
		WorkspaceID: value.GetWorkspaceId(), InstanceKey: value.GetInstanceKey(),
		ExpectedConversationRevision: value.GetExpectedConversationRevision(),
		ApprovalRevision:             value.GetApprovalRevision(), ManifestDigest: value.GetManifestDigest(),
		OccurrenceKey: value.GetOccurrenceKey(), Origin: managedAgentInputOriginFromProto(value.GetOrigin()),
		Payload: value.GetPayload(), CoalesceKey: value.GetCoalesceKey(),
	}
}

func commandStatusToProto(value CommandStatus) pluginv1.HostCommandStatus {
	switch value {
	case CommandApplied:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_APPLIED
	case CommandAlreadyApplied:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_ALREADY_APPLIED
	case CommandNoChange:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_NO_CHANGE
	case CommandConflict:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_CONFLICT
	case CommandDenied:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_DENIED
	case CommandNotFound:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_NOT_FOUND
	case CommandInvalid:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_INVALID
	case CommandUnsupported:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_UNSUPPORTED
	case CommandRateLimited:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_RATE_LIMITED
	case CommandUnavailable:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_UNAVAILABLE
	case CommandPartial:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_PARTIAL
	case CommandUncertain:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_UNCERTAIN
	default:
		return pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_UNSPECIFIED
	}
}

func commandStatusFromProto(value pluginv1.HostCommandStatus) CommandStatus {
	switch value {
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_APPLIED:
		return CommandApplied
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_ALREADY_APPLIED:
		return CommandAlreadyApplied
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_NO_CHANGE:
		return CommandNoChange
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_CONFLICT:
		return CommandConflict
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_DENIED:
		return CommandDenied
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_NOT_FOUND:
		return CommandNotFound
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_INVALID:
		return CommandInvalid
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_UNSUPPORTED:
		return CommandUnsupported
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_RATE_LIMITED:
		return CommandRateLimited
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_UNAVAILABLE:
		return CommandUnavailable
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_PARTIAL:
		return CommandPartial
	case pluginv1.HostCommandStatus_HOST_COMMAND_STATUS_UNCERTAIN:
		return CommandUncertain
	default:
		return ""
	}
}

func commandResultToProto(value *CommandResult) *pluginv1.HostCommandResult {
	if value == nil {
		return nil
	}
	result := &pluginv1.HostCommandResult{Status: commandStatusToProto(value.Status), Reason: value.Reason}
	if value.Receipt != nil {
		result.Receipt = &pluginv1.HostCommandReceipt{
			ReceiptId: value.Receipt.ID, OperationId: value.Receipt.OperationID,
			Status: commandStatusToProto(value.Receipt.Status), TargetId: value.Receipt.TargetID,
			ResourceVersion: value.Receipt.ResourceVersion, ApprovalRevision: value.Receipt.ApprovalRevision,
			CreatedAt: value.Receipt.CreatedAt, UpdatedAt: value.Receipt.UpdatedAt,
		}
	}
	return result
}

func commandResultFromProto(value *pluginv1.HostCommandResult) *CommandResult {
	if value == nil {
		return nil
	}
	result := &CommandResult{Status: commandStatusFromProto(value.GetStatus()), Reason: value.GetReason()}
	if receipt := value.GetReceipt(); receipt != nil {
		result.Receipt = &CommandReceipt{
			ID: receipt.GetReceiptId(), OperationID: receipt.GetOperationId(),
			Status: commandStatusFromProto(receipt.GetStatus()), TargetID: receipt.GetTargetId(),
			ResourceVersion: receipt.GetResourceVersion(), ApprovalRevision: receipt.GetApprovalRevision(),
			CreatedAt: receipt.GetCreatedAt(), UpdatedAt: receipt.GetUpdatedAt(),
		}
	}
	return result
}
