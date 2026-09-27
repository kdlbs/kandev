package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

// ExactReadPage is a bounded page within one immutable Host read snapshot.
// Plugins must treat both cursor fields as opaque.
type ExactReadPage struct {
	Limit           int32
	Cursor          string
	SnapshotVersion string
}

// HostReadReceipt identifies the installation and approval generation used
// for an authoritative snapshot read.
type HostReadReceipt struct {
	InstallationID     string
	WorkspaceID        string
	CapabilityRevision uint64
	RequestID          string
	SnapshotVersion    string
	ObservedAt         string
}

type ExactReadPageInfo struct {
	NextCursor      string
	HasMore         bool
	SnapshotVersion string
	ObservedAt      string
	Receipt         HostReadReceipt
}

type ExactWorkspaceQuery struct {
	RequestID   string
	WorkspaceID string
	Page        ExactReadPage
}

type ExactWorkflowQuery = ExactWorkspaceQuery

type ExactWorkflowStepQuery struct {
	RequestID   string
	WorkspaceID string
	WorkflowID  string
	Page        ExactReadPage
}

type ExactTaskQuery struct {
	RequestID   string
	WorkspaceID string
	Filter      TaskFilter
	Page        ExactReadPage
}

type ExactTaskGetQuery struct {
	RequestID   string
	WorkspaceID string
	TaskID      string
}

type ExactSessionQuery struct {
	RequestID   string
	WorkspaceID string
	Filter      SessionFilter
	Page        ExactReadPage
}

type ExactInteractionQuery struct {
	RequestID   string
	WorkspaceID string
	Filter      InteractionFilter
	Page        ExactReadPage
}

type ExactInteractionGetQuery struct {
	RequestID     string
	WorkspaceID   string
	InteractionID string
}

type ExactMessageQuery struct {
	RequestID   string
	WorkspaceID string
	Filter      MessageFilter
	Page        ExactReadPage
}

type ExactTaskRelationsQuery struct {
	RequestID   string
	WorkspaceID string
	TaskID      string
	Page        ExactReadPage
}

type ExactChangeRequestEvidenceQuery struct {
	RequestID   string
	WorkspaceID string
	TaskIDs     []string
	Page        ExactReadPage
}

type ExactTaskUsageQuery struct {
	RequestID   string
	WorkspaceID string
	TaskIDs     []string
	Page        ExactReadPage
}

type ExactWorkspaceObservation struct {
	Workspace       Workspace
	ResourceVersion string
	ObservedAt      string
}

type ExactWorkflowObservation struct {
	Workflow        Workflow
	ResourceVersion string
	ObservedAt      string
}

type ExactWorkflowStepObservation struct {
	Step            WorkflowStep
	ResourceVersion string
	ObservedAt      string
}

type ExactTaskObservation struct {
	Task                     Task
	CanonicalStatus          string
	SemanticActivity         string
	LastMeaningfulActivityAt *string
	ExecutionState           string
	BlockingReasons          []string
	StatusKnown              bool
	ResourceVersion          string
	ObservedAt               string
}

type ExactSessionObservation struct {
	Session         Session
	ExecutionState  string
	ResourceVersion string
	ObservedAt      string
	// ExecutionID is the opaque agent generation used to fence exact lifecycle commands.
	ExecutionID string
}

type ExactInteractionObservation struct {
	Interaction     Interaction
	ResourceVersion string
	ObservedAt      string
}

type ExactMessageObservation struct {
	Message         Message
	ResourceVersion string
	ObservedAt      string
	Sanitization    string
}

type TaskRelation struct {
	WorkspaceID           string
	SourceTaskID          string
	TargetTaskID          string
	Kind                  string
	SourceResourceVersion string
	TargetResourceVersion string
	ResourceVersion       string
}

type TaskInboxItem struct {
	ID              string
	WorkspaceID     string
	TaskID          string
	SessionID       string
	Kind            string
	State           string
	CreatedAt       string
	ResourceVersion string
}

type TaskDirective struct {
	ID                string
	WorkspaceID       string
	TaskID            string
	CapabilityClass   string
	State             string
	InstructionDigest string
	ExpiresAt         *string
	ResourceVersion   string
}

type PendingTaskTransition struct {
	ID              string
	WorkspaceID     string
	TaskID          string
	FromWorkflowID  string
	FromStepID      string
	ToWorkflowID    string
	ToStepID        string
	State           string
	ResourceVersion string
	CreatedAt       string
}

type ChangeRequestEvidence struct {
	TaskID                  string
	Provider                string
	RepositoryID            string
	RepositoryOwner         string
	RepositoryName          string
	Number                  int64
	URL                     string
	HeadRevision            string
	ReviewState             string
	ChecksState             string
	MergeableState          string
	UnresolvedReviewThreads int32
	ChecksTotal             int32
	ChecksPassing           int32
	ProviderState           string
	ProviderError           *string
	FetchedAt               *string
	ResourceVersion         string
	ObservedAt              string
}

type TaskUsageObservation struct {
	TaskID               string
	SessionID            *string
	TokensIn             int64
	TokensCachedRead     int64
	TokensCachedWrite    int64
	TokensOut            int64
	TokensThought        int64
	TokensTotal          int64
	CostSubcents         *int64
	Currency             string
	EventCount           int64
	EstimatedEventCount  int64
	UnpricedEventCount   int64
	OutputTokensComplete bool
	ObservationState     string
	CostUnknownReason    *string
	CostComplete         bool
	FirstEventAt         *string
	LastEventAt          *string
	ElapsedMilliseconds  *int64
	ResourceVersion      string
	ObservedAt           string
}

type ExactWorkspacePage struct {
	Items    []ExactWorkspaceObservation
	PageInfo ExactReadPageInfo
}
type ExactWorkflowPage struct {
	Items    []ExactWorkflowObservation
	PageInfo ExactReadPageInfo
}
type ExactWorkflowStepPage struct {
	Items    []ExactWorkflowStepObservation
	PageInfo ExactReadPageInfo
}
type ExactTaskPage struct {
	Items    []ExactTaskObservation
	PageInfo ExactReadPageInfo
}
type ExactSessionPage struct {
	Items    []ExactSessionObservation
	PageInfo ExactReadPageInfo
}
type ExactInteractionPage struct {
	Items    []ExactInteractionObservation
	PageInfo ExactReadPageInfo
}
type ExactMessagePage struct {
	Items    []ExactMessageObservation
	PageInfo ExactReadPageInfo
}
type ExactTaskRelationsPage struct {
	Items    []TaskRelation
	PageInfo ExactReadPageInfo
}
type ExactTaskInboxPage struct {
	Items    []TaskInboxItem
	PageInfo ExactReadPageInfo
}
type ExactTaskDirectivesPage struct {
	Items    []TaskDirective
	PageInfo ExactReadPageInfo
}
type ExactPendingTransitionsPage struct {
	Items    []PendingTaskTransition
	PageInfo ExactReadPageInfo
}
type ExactChangeRequestEvidencePage struct {
	Items    []ChangeRequestEvidence
	PageInfo ExactReadPageInfo
}
type ExactTaskUsagePage struct {
	Items    []TaskUsageObservation
	PageInfo ExactReadPageInfo
}

type ExactQueryManager interface {
	ListWorkspaces(context.Context, ExactWorkspaceQuery) (ExactWorkspacePage, error)
	ListWorkflows(context.Context, ExactWorkflowQuery) (ExactWorkflowPage, error)
	ListWorkflowSteps(context.Context, ExactWorkflowStepQuery) (ExactWorkflowStepPage, error)
	ListTasks(context.Context, ExactTaskQuery) (ExactTaskPage, error)
	GetTask(context.Context, ExactTaskGetQuery) (ExactTaskObservation, HostReadReceipt, error)
	ListSessions(context.Context, ExactSessionQuery) (ExactSessionPage, error)
	ListPendingInteractions(context.Context, ExactInteractionQuery) (ExactInteractionPage, error)
	GetInteraction(context.Context, ExactInteractionGetQuery) (ExactInteractionObservation, HostReadReceipt, error)
	ListSanitizedMessages(context.Context, ExactMessageQuery) (ExactMessagePage, error)
	ListTaskInbox(context.Context, ExactTaskInboxQuery) (ExactTaskInboxPage, error)
	GetTaskDirective(context.Context, ExactTaskDirectiveGetQuery) (TaskDirective, HostReadReceipt, error)
	ListTaskDirectives(context.Context, ExactTaskDirectiveQuery) (ExactTaskDirectivesPage, error)
	GetTaskRelations(context.Context, ExactTaskRelationsQuery) (ExactTaskRelationsPage, error)
	ListTaskRelations(context.Context, ExactTaskRelationsQuery) (ExactTaskRelationsPage, error)
	ListPendingTaskTransitions(context.Context, ExactPendingTaskTransitionsQuery) (ExactPendingTransitionsPage, error)
	ListChangeRequestEvidence(context.Context, ExactChangeRequestEvidenceQuery) (ExactChangeRequestEvidencePage, error)
	ListTaskUsage(context.Context, ExactTaskUsageQuery) (ExactTaskUsagePage, error)
}

type ExactTaskInboxQuery struct {
	RequestID, WorkspaceID, TaskID, SessionID string
	Page                                      ExactReadPage
}
type ExactTaskDirectiveGetQuery struct{ RequestID, WorkspaceID, DirectiveID string }
type ExactTaskDirectiveQuery struct {
	RequestID, WorkspaceID, TaskID string
	Page                           ExactReadPage
}
type ExactPendingTaskTransitionsQuery struct {
	RequestID, WorkspaceID, TaskID string
	Page                           ExactReadPage
}

// ExactQueryHost is implemented by Host v2 transports that expose canonical,
// snapshot-bound workspace observations. It stays separate from Host so v1
// plugins and implementations retain source compatibility.
type ExactQueryHost interface{ ExactQueries() ExactQueryManager }

// HostExactQueries returns the optional exact query manager when supported.
func HostExactQueries(host Host) (ExactQueryManager, bool) {
	provider, ok := host.(ExactQueryHost)
	if !ok || provider.ExactQueries() == nil {
		return nil, false
	}
	return provider.ExactQueries(), true
}

func exactReadPageToProto(page ExactReadPage) *pluginv1.ExactReadPage {
	return &pluginv1.ExactReadPage{Limit: page.Limit, Cursor: page.Cursor, SnapshotVersion: page.SnapshotVersion}
}

func exactReadPageFromProto(page *pluginv1.ExactReadPage) ExactReadPage {
	if page == nil {
		return ExactReadPage{}
	}
	return ExactReadPage{Limit: page.GetLimit(), Cursor: page.GetCursor(), SnapshotVersion: page.GetSnapshotVersion()}
}

func hostReadReceiptFromProto(value *pluginv1.HostReadReceipt) HostReadReceipt {
	if value == nil {
		return HostReadReceipt{}
	}
	return HostReadReceipt{InstallationID: value.GetInstallationId(), WorkspaceID: value.GetWorkspaceId(), CapabilityRevision: value.GetCapabilityRevision(), RequestID: value.GetRequestId(), SnapshotVersion: value.GetSnapshotVersion(), ObservedAt: value.GetObservedAt()}
}

func hostReadReceiptToProto(value HostReadReceipt) *pluginv1.HostReadReceipt {
	return &pluginv1.HostReadReceipt{InstallationId: value.InstallationID, WorkspaceId: value.WorkspaceID, CapabilityRevision: value.CapabilityRevision, RequestId: value.RequestID, SnapshotVersion: value.SnapshotVersion, ObservedAt: value.ObservedAt}
}

func exactReadPageInfoFromProto(value *pluginv1.ExactReadPageInfo) ExactReadPageInfo {
	if value == nil {
		return ExactReadPageInfo{}
	}
	return ExactReadPageInfo{NextCursor: value.GetNextCursor(), HasMore: value.GetHasMore(), SnapshotVersion: value.GetSnapshotVersion(), ObservedAt: value.GetObservedAt(), Receipt: hostReadReceiptFromProto(value.GetReceipt())}
}

func exactReadPageInfoToProto(value ExactReadPageInfo) *pluginv1.ExactReadPageInfo {
	return &pluginv1.ExactReadPageInfo{NextCursor: value.NextCursor, HasMore: value.HasMore, SnapshotVersion: value.SnapshotVersion, ObservedAt: value.ObservedAt, Receipt: hostReadReceiptToProto(value.Receipt)}
}

func exactTaskObservationFromProto(value *pluginv1.ExactTaskObservation) (ExactTaskObservation, error) {
	if value == nil {
		return ExactTaskObservation{}, nil
	}
	task, err := taskFromProto(value.GetTask())
	if err != nil {
		return ExactTaskObservation{}, err
	}
	return ExactTaskObservation{Task: task, CanonicalStatus: value.GetCanonicalStatus(), SemanticActivity: value.GetSemanticActivity(), LastMeaningfulActivityAt: value.LastMeaningfulActivityAt, ExecutionState: value.GetExecutionState(), BlockingReasons: value.GetBlockingReasons(), StatusKnown: value.GetStatusKnown(), ResourceVersion: value.GetResourceVersion(), ObservedAt: value.GetObservedAt()}, nil
}

func exactTaskObservationToProto(value ExactTaskObservation) (*pluginv1.ExactTaskObservation, error) {
	task, err := value.Task.toProto()
	if err != nil {
		return nil, err
	}
	return &pluginv1.ExactTaskObservation{Task: task, CanonicalStatus: value.CanonicalStatus, SemanticActivity: value.SemanticActivity, LastMeaningfulActivityAt: value.LastMeaningfulActivityAt, ExecutionState: value.ExecutionState, BlockingReasons: value.BlockingReasons, StatusKnown: value.StatusKnown, ResourceVersion: value.ResourceVersion, ObservedAt: value.ObservedAt}, nil
}

func exactTaskUsageToProto(value TaskUsageObservation) *pluginv1.TaskUsageObservation {
	out := &pluginv1.TaskUsageObservation{TaskId: value.TaskID, SessionId: value.SessionID, TokensIn: value.TokensIn, TokensCachedRead: value.TokensCachedRead, TokensCachedWrite: value.TokensCachedWrite, TokensOut: value.TokensOut, TokensThought: value.TokensThought, TokensTotal: value.TokensTotal, CostSubcents: value.CostSubcents, Currency: value.Currency, EventCount: value.EventCount, EstimatedEventCount: value.EstimatedEventCount, UnpricedEventCount: value.UnpricedEventCount, OutputTokensComplete: value.OutputTokensComplete, ObservationState: value.ObservationState, CostUnknownReason: value.CostUnknownReason, FirstEventAt: value.FirstEventAt, LastEventAt: value.LastEventAt, ElapsedMilliseconds: value.ElapsedMilliseconds, ResourceVersion: value.ResourceVersion, ObservedAt: value.ObservedAt, CostComplete: value.CostComplete}
	return out
}

func exactTaskUsageFromProto(value *pluginv1.TaskUsageObservation) TaskUsageObservation {
	if value == nil {
		return TaskUsageObservation{}
	}
	return TaskUsageObservation{TaskID: value.GetTaskId(), SessionID: value.SessionId, TokensIn: value.GetTokensIn(), TokensCachedRead: value.GetTokensCachedRead(), TokensCachedWrite: value.GetTokensCachedWrite(), TokensOut: value.GetTokensOut(), TokensThought: value.GetTokensThought(), TokensTotal: value.GetTokensTotal(), CostSubcents: value.CostSubcents, Currency: value.GetCurrency(), EventCount: value.GetEventCount(), EstimatedEventCount: value.GetEstimatedEventCount(), UnpricedEventCount: value.GetUnpricedEventCount(), OutputTokensComplete: value.GetOutputTokensComplete(), ObservationState: value.GetObservationState(), CostUnknownReason: value.CostUnknownReason, FirstEventAt: value.FirstEventAt, LastEventAt: value.LastEventAt, ElapsedMilliseconds: value.ElapsedMilliseconds, ResourceVersion: value.GetResourceVersion(), ObservedAt: value.GetObservedAt(), CostComplete: value.GetCostComplete()}
}
