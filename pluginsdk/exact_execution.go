package pluginsdk

import "context"

// ExactTaskRunCommand requests one task-scoped run action at an observed task version.
// The Host derives the caller installation from the connected plugin. The
// management key and generation are assertions copied from the observed task
// claim: pass an empty key and generation zero for a task that has never had a
// claim, or an empty key and the current positive generation after release.
type ExactTaskRunCommand struct {
	RequestID, WorkspaceID, TaskID, IdempotencyKey string
	ExpectedTaskResourceVersion                    string
	ManagementInstanceKey                          string
	ExpectedClaimGeneration                        int64
	ApprovalRevision                               uint64
	ManifestDigest                                 string
}

// ExactTaskRunResult reports the canonical session and execution created or found.
type ExactTaskRunResult struct {
	Result       *CommandResult
	SessionID    string
	SessionState string
	ExecutionID  string
}

// ExactTaskExecutionCommand targets one observed session execution and carries
// the task management claim fence described by ExactTaskRunCommand.
type ExactTaskExecutionCommand struct {
	RequestID, WorkspaceID, TaskID, SessionID string
	ExpectedSessionResourceVersion            string
	ExpectedExecutionID, IdempotencyKey       string
	ManagementInstanceKey                     string
	ExpectedClaimGeneration                   int64
	ApprovalRevision                          uint64
	ManifestDigest                            string
}

// ExactSessionRecoveryCommand requests only the safe resume recovery action.
type ExactSessionRecoveryCommand struct {
	ExactTaskExecutionCommand
	Action string
}

// ExactPendingTransitionCommand cancels one versioned pending task transition
// and carries the task management claim fence described by ExactTaskRunCommand.
type ExactPendingTransitionCommand struct {
	RequestID, WorkspaceID, TaskID, TransitionID string
	ExpectedResourceVersion, IdempotencyKey      string
	ManagementInstanceKey                        string
	ExpectedClaimGeneration                      int64
	ApprovalRevision                             uint64
	ManifestDigest                               string
}

// SessionModeOption describes a mode advertised by the active provider.
type SessionModeOption struct {
	ID, Name, Description string
}

// SessionModeContext is a provider snapshot for one observed execution.
type SessionModeContext struct {
	SessionID, ExecutionID, CurrentModeID string
	AvailableModes                        []SessionModeOption
	ResourceVersion, ObservedAt           string
}

// ExactSessionModeCommand changes mode only on the observed execution generation.
type ExactSessionModeCommand struct {
	ExactTaskExecutionCommand
	ModeID string
}

// ExactPermissionResponse relays a human-approved response to one exact request.
type ExactPermissionResponse struct {
	RequestID, WorkspaceID, InteractionID, ExpectedResourceVersion string
	OptionID, HumanResponseReceiptID                               string
	Cancelled                                                      bool
	ApprovalRevision                                               uint64
	ManifestDigest                                                 string
}

// ExactClarificationResponse relays a human-submitted answer bundle.
type ExactClarificationResponse struct {
	RequestID, WorkspaceID, InteractionID, ExpectedResourceVersion string
	Answers                                                        []ClarificationAnswer
	HumanResponseReceiptID                                         string
	ApprovalRevision                                               uint64
	ManifestDigest                                                 string
}

// ExactExecutionCommandManager exposes exact task and session lifecycle controls.
type ExactExecutionCommandManager interface {
	EnsureTaskRun(context.Context, ExactTaskRunCommand) (*CommandResult, ExactTaskRunResult, error)
	StopTaskRun(context.Context, ExactTaskExecutionCommand) (*CommandResult, bool, error)
	RecoverSession(context.Context, ExactSessionRecoveryCommand) (*CommandResult, ExactTaskRunResult, error)
	CancelPendingTaskTransition(context.Context, ExactPendingTransitionCommand) (*CommandResult, error)
	GetSessionModeContext(context.Context, ExactTaskExecutionCommand) (SessionModeContext, error)
	SetSessionMode(context.Context, ExactSessionModeCommand) (*CommandResult, string, error)
}

// ExactInteractionCommandManager exposes responses that require a Host-issued human receipt.
type ExactInteractionCommandManager interface {
	RespondPermission(context.Context, ExactPermissionResponse) (*CommandResult, *Interaction, error)
	AnswerClarification(context.Context, ExactClarificationResponse) (*CommandResult, *Interaction, error)
}

type ExactExecutionCommandHost interface {
	ExecutionCommands() ExactExecutionCommandManager
}

type ExactInteractionCommandHost interface {
	ExactInteractionCommands() ExactInteractionCommandManager
}

// HostExecutionCommands returns the optional exact execution-control extension.
func HostExecutionCommands(host Host) (ExactExecutionCommandManager, bool) {
	provider, ok := host.(ExactExecutionCommandHost)
	if !ok {
		return nil, false
	}
	return provider.ExecutionCommands(), true
}

// HostExactInteractionCommands returns the optional receipt-gated interaction extension.
func HostExactInteractionCommands(host Host) (ExactInteractionCommandManager, bool) {
	provider, ok := host.(ExactInteractionCommandHost)
	if !ok {
		return nil, false
	}
	return provider.ExactInteractionCommands(), true
}
