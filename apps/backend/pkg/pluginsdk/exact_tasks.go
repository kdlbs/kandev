package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

// ExactTaskCreate requests one source-identified task creation. ExternalID is
// unique within the workspace and lets the task service recover a create after
// the Host command receipt was not acknowledged.
type ExactTaskCreate struct {
	RequestID        string
	WorkspaceID      string
	IdempotencyKey   string
	ExternalID       string
	ApprovalRevision uint64
	ManifestDigest   string
	Task             CreateTaskInput
}

// ExactTaskLabels replaces all labels on one task at the observed resource
// version. An empty slice clears the labels.
type ExactTaskLabels struct {
	RequestID               string
	WorkspaceID             string
	TaskID                  string
	IdempotencyKey          string
	ExpectedResourceVersion string
	ApprovalRevision        uint64
	ManifestDigest          string
	ManagementInstanceKey   string
	ExpectedClaimGeneration int64
	Labels                  []string
}

// ExactTaskAssignment sets or clears the human assignee. Worker agent
// assignment remains a separate native orchestration action.
type ExactTaskAssignment struct {
	RequestID               string
	WorkspaceID             string
	TaskID                  string
	IdempotencyKey          string
	ExpectedResourceVersion string
	ApprovalRevision        uint64
	ManifestDigest          string
	ManagementInstanceKey   string
	ExpectedClaimGeneration int64
	AssigneeUserID          string
}

// ExactTaskMove moves a task through the shared workflow admission path at the
// resource version observed by the caller.
type ExactTaskMove struct {
	RequestID               string
	WorkspaceID             string
	TaskID                  string
	IdempotencyKey          string
	ExpectedResourceVersion string
	WorkflowID              string
	WorkflowStepID          string
	Position                int32
	ApprovalRevision        uint64
	ManifestDigest          string
	ManagementInstanceKey   string
	ExpectedClaimGeneration int64
}

// ExactTaskArchive archives a task through its native lifecycle when its
// resource version still matches the caller's observation.
type ExactTaskArchive struct {
	RequestID               string
	WorkspaceID             string
	TaskID                  string
	IdempotencyKey          string
	ExpectedResourceVersion string
	ApprovalRevision        uint64
	ManifestDigest          string
	ManagementInstanceKey   string
	ExpectedClaimGeneration int64
}

// ExactTaskRelation adds or removes one dependency edge between tasks in the
// same workspace.
type ExactTaskRelation struct {
	RequestID                      string
	WorkspaceID                    string
	TaskID                         string
	RelatedTaskID                  string
	ExpectedTaskResourceVersion    string
	ExpectedRelatedResourceVersion string
	IdempotencyKey                 string
	ApprovalRevision               uint64
	ManifestDigest                 string
	ManagementInstanceKey          string
	ExpectedClaimGeneration        int64
}

// ExactTaskMessage admits one prompt into the target session's durable FIFO
// queue while the observed task and session versions remain current.
type ExactTaskMessage struct {
	RequestID                      string
	WorkspaceID                    string
	TaskID                         string
	SessionID                      string
	IdempotencyKey                 string
	ExpectedTaskResourceVersion    string
	ExpectedSessionResourceVersion string
	ApprovalRevision               uint64
	ManifestDigest                 string
	ManagementInstanceKey          string
	ExpectedClaimGeneration        int64
	Content                        string
}

// TaskManagementClaim is the Host's owner projection for one task. The
// generation is a fencing token and remains monotonic after release.
type TaskManagementClaim struct {
	TaskID          string
	WorkspaceID     string
	OwnerKind       string
	OwnerActorID    string
	InstallationID  string
	InstanceKey     string
	Generation      int64
	ResourceVersion string
	AcquiredAt      string
	UpdatedAt       string
	UpdatedByActor  string
}

type ExactTaskManagementClaimCommand struct {
	RequestID                    string
	WorkspaceID                  string
	TaskID                       string
	ExpectedTaskResourceVersion  string
	ExpectedClaimResourceVersion string
	IdempotencyKey               string
	Reason                       string
	ApprovalRevision             uint64
	ManifestDigest               string
}

type ExactTaskManagementClaimAcquire struct {
	ExactTaskManagementClaimCommand
	InstanceKey string
}

type ExactTaskManagementClaimRelease struct {
	ExactTaskManagementClaimCommand
	InstanceKey string
}

type ExactTaskManagementClaimTransfer struct {
	ExactTaskManagementClaimCommand
	InstanceKey          string
	TargetInstallationID string
	TargetInstanceKey    string
}

type TaskCompletionEvidenceSubject struct {
	Kind     string
	ID       string
	Revision string
}

type TaskCompletionEvidence struct {
	Subject   TaskCompletionEvidenceSubject
	Summary   string
	Reference string
}

type TaskCompletionCriterionInput struct {
	ID              string
	Description     string
	EvidenceSubject TaskCompletionEvidenceSubject
}

type TaskCompletionCriterion struct {
	ID                string
	Description       string
	EvidenceSubject   TaskCompletionEvidenceSubject
	CriterionRevision int64
	VerifiedRevision  int64
	Evidence          *TaskCompletionEvidence
	VerifierKind      string
	VerifierID        string
	VerifiedAt        string
}

type TaskCompletionBlocker struct {
	CriterionID string
	Reason      string
}

type TaskCompletionGate struct {
	TaskID      string
	WorkspaceID string
	Revision    int64
	Criteria    []TaskCompletionCriterion
	Blockers    []TaskCompletionBlocker
	Blocked     bool
}

type ExactTaskCompletionCriteria struct {
	RequestID                   string
	WorkspaceID                 string
	TaskID                      string
	IdempotencyKey              string
	ExpectedTaskResourceVersion string
	ExpectedRevision            int64
	ApprovalRevision            uint64
	ManifestDigest              string
	ManagementInstanceKey       string
	ExpectedClaimGeneration     int64
	Criteria                    []TaskCompletionCriterionInput
}

type ExactTaskCompletionEvidence struct {
	RequestID                   string
	WorkspaceID                 string
	TaskID                      string
	CriterionID                 string
	IdempotencyKey              string
	ExpectedTaskResourceVersion string
	ExpectedRevision            int64
	Evidence                    TaskCompletionEvidence
	ApprovalRevision            uint64
	ManifestDigest              string
	ManagementInstanceKey       string
	ExpectedClaimGeneration     int64
}

type ExactTaskManagementClaimCommandManager interface {
	Acquire(ctx context.Context, input ExactTaskManagementClaimAcquire) (*CommandResult, *TaskManagementClaim, error)
	Release(ctx context.Context, input ExactTaskManagementClaimRelease) (*CommandResult, *TaskManagementClaim, error)
	Transfer(ctx context.Context, input ExactTaskManagementClaimTransfer) (*CommandResult, *TaskManagementClaim, error)
}

type ExactTaskManagementClaimCommandHost interface {
	TaskManagementClaims() ExactTaskManagementClaimCommandManager
}

type ExactTaskCompletionGateCommandManager interface {
	SetCriteria(ctx context.Context, input ExactTaskCompletionCriteria) (*CommandResult, *TaskCompletionGate, error)
	Verify(ctx context.Context, input ExactTaskCompletionEvidence) (*CommandResult, *TaskCompletionGate, error)
}

type ExactTaskCompletionGateCommandHost interface {
	TaskCompletionGates() ExactTaskCompletionGateCommandManager
}

func HostTaskCompletionGates(host Host) (ExactTaskCompletionGateCommandManager, bool) {
	extended, ok := host.(ExactTaskCompletionGateCommandHost)
	if !ok || extended.TaskCompletionGates() == nil {
		return nil, false
	}
	return extended.TaskCompletionGates(), true
}

func HostTaskManagementClaims(host Host) (ExactTaskManagementClaimCommandManager, bool) {
	extended, ok := host.(ExactTaskManagementClaimCommandHost)
	if !ok || extended.TaskManagementClaims() == nil {
		return nil, false
	}
	return extended.TaskManagementClaims(), true
}

// ExactTaskDirectiveIssue creates a short-lived directive bound to one task,
// session, approved capability class, and observed resource versions.
type ExactTaskDirectiveIssue struct {
	RequestID                      string
	WorkspaceID                    string
	TaskID                         string
	SessionID                      string
	CapabilityClass                string
	InstructionDigest              string
	ExpectedTaskResourceVersion    string
	ExpectedSessionResourceVersion string
	ExpiresAt                      string
	IdempotencyKey                 string
	ApprovalRevision               uint64
	ManifestDigest                 string
}

// ExactTaskDirectiveResolve consumes one pending directive at its current
// resource version. Resolution details are stored as a digest.
type ExactTaskDirectiveResolve struct {
	RequestID               string
	WorkspaceID             string
	DirectiveID             string
	ExpectedResourceVersion string
	IdempotencyKey          string
	Resolution              string
	ResolutionDigest        string
	ApprovalRevision        uint64
	ManifestDigest          string
}

// ExactTaskCommandManager exposes versioned, approved task mutations.
type ExactTaskCommandManager interface {
	CreateTask(ctx context.Context, input ExactTaskCreate) (*CommandResult, *Task, error)
	SetLabels(ctx context.Context, input ExactTaskLabels) (*CommandResult, *Task, error)
	Assign(ctx context.Context, input ExactTaskAssignment) (*CommandResult, *Task, error)
	Move(ctx context.Context, input ExactTaskMove) (*CommandResult, *Task, error)
	Archive(ctx context.Context, input ExactTaskArchive) (*CommandResult, *Task, error)
	AddRelation(ctx context.Context, input ExactTaskRelation) (*CommandResult, error)
	RemoveRelation(ctx context.Context, input ExactTaskRelation) (*CommandResult, error)
	SendMessage(ctx context.Context, input ExactTaskMessage) (*CommandResult, error)
	IssueDirective(ctx context.Context, input ExactTaskDirectiveIssue) (*CommandResult, TaskDirective, error)
	ResolveDirective(ctx context.Context, input ExactTaskDirectiveResolve) (*CommandResult, TaskDirective, error)
}

// ExactTaskCommandHost is an additive Host extension for exact task commands.
type ExactTaskCommandHost interface {
	TaskCommands() ExactTaskCommandManager
}

// HostTaskCommands returns the exact task command extension when available.
func HostTaskCommands(host Host) (ExactTaskCommandManager, bool) {
	extended, ok := host.(ExactTaskCommandHost)
	if !ok || extended.TaskCommands() == nil {
		return nil, false
	}
	return extended.TaskCommands(), true
}

type grpcExactTaskCommandManager struct {
	client pluginv1.HostClient
}

type grpcExactTaskManagementClaimCommandManager struct {
	client pluginv1.HostClient
}

type grpcExactTaskCompletionGateCommandManager struct {
	client pluginv1.HostClient
}

func (m grpcExactTaskCompletionGateCommandManager) SetCriteria(ctx context.Context, input ExactTaskCompletionCriteria) (*CommandResult, *TaskCompletionGate, error) {
	criteria := make([]*pluginv1.TaskCompletionCriterionInput, 0, len(input.Criteria))
	for _, item := range input.Criteria {
		criteria = append(criteria, &pluginv1.TaskCompletionCriterionInput{
			Id: item.ID, Description: item.Description,
			EvidenceSubject: completionEvidenceSubjectToProto(item.EvidenceSubject),
		})
	}
	response, err := m.client.SetTaskCompletionCriteriaExact(ctx, &pluginv1.SetTaskCompletionCriteriaExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		IdempotencyKey: input.IdempotencyKey, ExpectedTaskResourceVersion: input.ExpectedTaskResourceVersion,
		ExpectedRevision: input.ExpectedRevision, Criteria: criteria,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
	})
	if err != nil {
		return nil, nil, err
	}
	return commandResultFromProto(response.GetResult()), completionGateFromProto(response.GetSnapshot()), nil
}

func (m grpcExactTaskCompletionGateCommandManager) Verify(ctx context.Context, input ExactTaskCompletionEvidence) (*CommandResult, *TaskCompletionGate, error) {
	response, err := m.client.VerifyTaskCompletionCriterionExact(ctx, &pluginv1.VerifyTaskCompletionCriterionExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		CriterionId: input.CriterionID, IdempotencyKey: input.IdempotencyKey,
		ExpectedTaskResourceVersion: input.ExpectedTaskResourceVersion, ExpectedRevision: input.ExpectedRevision,
		Evidence: completionEvidenceToProto(input.Evidence), ApprovalRevision: input.ApprovalRevision,
		ManifestDigest: input.ManifestDigest, ManagementInstanceKey: input.ManagementInstanceKey,
		ExpectedClaimGeneration: input.ExpectedClaimGeneration,
	})
	if err != nil {
		return nil, nil, err
	}
	return commandResultFromProto(response.GetResult()), completionGateFromProto(response.GetSnapshot()), nil
}

func completionEvidenceSubjectToProto(value TaskCompletionEvidenceSubject) *pluginv1.TaskCompletionEvidenceSubject {
	return &pluginv1.TaskCompletionEvidenceSubject{Kind: value.Kind, Id: value.ID, Revision: value.Revision}
}

func completionEvidenceToProto(value TaskCompletionEvidence) *pluginv1.TaskCompletionEvidence {
	return &pluginv1.TaskCompletionEvidence{Subject: completionEvidenceSubjectToProto(value.Subject), Summary: value.Summary, Reference: value.Reference}
}

func completionGateFromProto(value *pluginv1.TaskCompletionGateSnapshot) *TaskCompletionGate {
	if value == nil {
		return nil
	}
	gate := &TaskCompletionGate{
		TaskID: value.GetTaskId(), WorkspaceID: value.GetWorkspaceId(), Revision: value.GetRevision(), Blocked: value.GetBlocked(),
		Criteria: make([]TaskCompletionCriterion, 0, len(value.GetCriteria())),
		Blockers: make([]TaskCompletionBlocker, 0, len(value.GetBlockers())),
	}
	for _, item := range value.GetCriteria() {
		criterion := TaskCompletionCriterion{
			ID: item.GetId(), Description: item.GetDescription(), EvidenceSubject: completionEvidenceSubjectFromProto(item.GetEvidenceSubject()),
			CriterionRevision: item.GetCriterionRevision(), VerifiedRevision: item.GetVerifiedRevision(),
			VerifierKind: item.GetVerifierKind(), VerifierID: item.GetVerifierId(), VerifiedAt: item.GetVerifiedAt(),
		}
		if item.GetEvidence() != nil {
			evidence := item.GetEvidence()
			criterion.Evidence = &TaskCompletionEvidence{
				Subject: completionEvidenceSubjectFromProto(evidence.GetSubject()), Summary: evidence.GetSummary(), Reference: evidence.GetReference(),
			}
		}
		gate.Criteria = append(gate.Criteria, criterion)
	}
	for _, item := range value.GetBlockers() {
		gate.Blockers = append(gate.Blockers, TaskCompletionBlocker{CriterionID: item.GetCriterionId(), Reason: item.GetReason()})
	}
	return gate
}

func completionEvidenceSubjectFromProto(value *pluginv1.TaskCompletionEvidenceSubject) TaskCompletionEvidenceSubject {
	if value == nil {
		return TaskCompletionEvidenceSubject{}
	}
	return TaskCompletionEvidenceSubject{Kind: value.GetKind(), ID: value.GetId(), Revision: value.GetRevision()}
}

func completionGateToProto(value *TaskCompletionGate) *pluginv1.TaskCompletionGateSnapshot {
	if value == nil {
		return nil
	}
	gate := &pluginv1.TaskCompletionGateSnapshot{
		TaskId: value.TaskID, WorkspaceId: value.WorkspaceID, Revision: value.Revision, Blocked: value.Blocked,
		Criteria: make([]*pluginv1.TaskCompletionCriterion, 0, len(value.Criteria)),
		Blockers: make([]*pluginv1.TaskCompletionBlocker, 0, len(value.Blockers)),
	}
	for _, item := range value.Criteria {
		criterion := &pluginv1.TaskCompletionCriterion{
			Id: item.ID, Description: item.Description, EvidenceSubject: completionEvidenceSubjectToProto(item.EvidenceSubject),
			CriterionRevision: item.CriterionRevision, VerifiedRevision: item.VerifiedRevision,
			VerifierKind: item.VerifierKind, VerifierId: item.VerifierID, VerifiedAt: item.VerifiedAt,
		}
		if item.Evidence != nil {
			criterion.Evidence = completionEvidenceToProto(*item.Evidence)
		}
		gate.Criteria = append(gate.Criteria, criterion)
	}
	for _, item := range value.Blockers {
		gate.Blockers = append(gate.Blockers, &pluginv1.TaskCompletionBlocker{CriterionId: item.CriterionID, Reason: item.Reason})
	}
	return gate
}

func (m grpcExactTaskManagementClaimCommandManager) Acquire(ctx context.Context, input ExactTaskManagementClaimAcquire) (*CommandResult, *TaskManagementClaim, error) {
	response, err := m.client.AcquireTaskManagementClaimExact(ctx, &pluginv1.AcquireTaskManagementClaimExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		InstanceKey: input.InstanceKey, ExpectedTaskResourceVersion: input.ExpectedTaskResourceVersion,
		ExpectedClaimResourceVersion: input.ExpectedClaimResourceVersion, IdempotencyKey: input.IdempotencyKey,
		Reason: input.Reason, ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	return commandResultFromProto(response.GetResult()), taskManagementClaimFromProto(response.GetClaim()), nil
}

func (m grpcExactTaskManagementClaimCommandManager) Release(ctx context.Context, input ExactTaskManagementClaimRelease) (*CommandResult, *TaskManagementClaim, error) {
	response, err := m.client.ReleaseTaskManagementClaimExact(ctx, &pluginv1.ReleaseTaskManagementClaimExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		InstanceKey: input.InstanceKey, ExpectedTaskResourceVersion: input.ExpectedTaskResourceVersion,
		ExpectedClaimResourceVersion: input.ExpectedClaimResourceVersion, IdempotencyKey: input.IdempotencyKey,
		Reason: input.Reason, ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	return commandResultFromProto(response.GetResult()), taskManagementClaimFromProto(response.GetClaim()), nil
}

func (m grpcExactTaskManagementClaimCommandManager) Transfer(ctx context.Context, input ExactTaskManagementClaimTransfer) (*CommandResult, *TaskManagementClaim, error) {
	response, err := m.client.TransferTaskManagementClaimExact(ctx, &pluginv1.TransferTaskManagementClaimExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		InstanceKey: input.InstanceKey, TargetInstallationId: input.TargetInstallationID, TargetInstanceKey: input.TargetInstanceKey,
		ExpectedTaskResourceVersion: input.ExpectedTaskResourceVersion, ExpectedClaimResourceVersion: input.ExpectedClaimResourceVersion,
		IdempotencyKey: input.IdempotencyKey, Reason: input.Reason,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	return commandResultFromProto(response.GetResult()), taskManagementClaimFromProto(response.GetClaim()), nil
}

func taskManagementClaimFromProto(value *pluginv1.TaskManagementClaim) *TaskManagementClaim {
	if value == nil {
		return nil
	}
	return &TaskManagementClaim{
		TaskID: value.GetTaskId(), WorkspaceID: value.GetWorkspaceId(),
		OwnerKind: value.GetOwnerKind(), OwnerActorID: value.GetOwnerActorId(),
		InstallationID: value.GetInstallationId(), InstanceKey: value.GetInstanceKey(),
		Generation: value.GetGeneration(), ResourceVersion: value.GetResourceVersion(),
		AcquiredAt: value.GetAcquiredAt(), UpdatedAt: value.GetUpdatedAt(), UpdatedByActor: value.GetUpdatedByActor(),
	}
}

func taskManagementClaimToProto(value *TaskManagementClaim) *pluginv1.TaskManagementClaim {
	if value == nil {
		return nil
	}
	return &pluginv1.TaskManagementClaim{
		TaskId: value.TaskID, WorkspaceId: value.WorkspaceID, InstallationId: value.InstallationID,
		OwnerKind: value.OwnerKind, OwnerActorId: value.OwnerActorID,
		InstanceKey: value.InstanceKey, Generation: value.Generation, ResourceVersion: value.ResourceVersion,
		AcquiredAt: value.AcquiredAt, UpdatedAt: value.UpdatedAt, UpdatedByActor: value.UpdatedByActor,
	}
}

func (m grpcExactTaskCommandManager) CreateTask(ctx context.Context, input ExactTaskCreate) (*CommandResult, *Task, error) {
	task, err := input.Task.toProto()
	if err != nil {
		return nil, nil, err
	}
	response, err := m.client.CreateTaskExact(ctx, &pluginv1.CreateTaskExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID,
		IdempotencyKey: input.IdempotencyKey, ExternalId: input.ExternalID,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
		Task: task,
	})
	if err != nil {
		return nil, nil, err
	}
	result := commandResultFromProto(response.GetResult())
	if response.GetTask() == nil {
		return result, nil, nil
	}
	created, err := taskFromProto(response.GetTask())
	if err != nil {
		return nil, nil, err
	}
	return result, &created, nil
}

func (m grpcExactTaskCommandManager) SetLabels(ctx context.Context, input ExactTaskLabels) (*CommandResult, *Task, error) {
	response, err := m.client.SetTaskLabelsExact(ctx, &pluginv1.SetTaskLabelsExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		IdempotencyKey: input.IdempotencyKey, ExpectedResourceVersion: input.ExpectedResourceVersion,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
		Labels: append([]string(nil), input.Labels...), ApprovalRevision: input.ApprovalRevision,
		ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	result := commandResultFromProto(response.GetResult())
	if response.GetTask() == nil {
		return result, nil, nil
	}
	task, err := taskFromProto(response.GetTask())
	if err != nil {
		return nil, nil, err
	}
	return result, &task, nil
}

func (m grpcExactTaskCommandManager) Assign(ctx context.Context, input ExactTaskAssignment) (*CommandResult, *Task, error) {
	response, err := m.client.AssignTaskExact(ctx, &pluginv1.AssignTaskExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		IdempotencyKey: input.IdempotencyKey, ExpectedResourceVersion: input.ExpectedResourceVersion,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
		AssigneeUserId: input.AssigneeUserID, ApprovalRevision: input.ApprovalRevision,
		ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	result := commandResultFromProto(response.GetResult())
	if response.GetTask() == nil {
		return result, nil, nil
	}
	task, err := taskFromProto(response.GetTask())
	if err != nil {
		return nil, nil, err
	}
	return result, &task, nil
}

func (m grpcExactTaskCommandManager) Move(ctx context.Context, input ExactTaskMove) (*CommandResult, *Task, error) {
	response, err := m.client.MoveTaskExact(ctx, &pluginv1.MoveTaskExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		IdempotencyKey: input.IdempotencyKey, ExpectedResourceVersion: input.ExpectedResourceVersion,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
		WorkflowId: input.WorkflowID, WorkflowStepId: input.WorkflowStepID, Position: input.Position,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	result := commandResultFromProto(response.GetResult())
	if response.GetTask() == nil {
		return result, nil, nil
	}
	task, err := taskFromProto(response.GetTask())
	if err != nil {
		return nil, nil, err
	}
	return result, &task, nil
}

func (m grpcExactTaskCommandManager) Archive(ctx context.Context, input ExactTaskArchive) (*CommandResult, *Task, error) {
	response, err := m.client.ArchiveTaskExact(ctx, &pluginv1.ArchiveTaskExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		IdempotencyKey: input.IdempotencyKey, ExpectedResourceVersion: input.ExpectedResourceVersion,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	result := commandResultFromProto(response.GetResult())
	if response.GetTask() == nil {
		return result, nil, nil
	}
	task, err := taskFromProto(response.GetTask())
	if err != nil {
		return nil, nil, err
	}
	return result, &task, nil
}

func (m grpcExactTaskCommandManager) AddRelation(ctx context.Context, input ExactTaskRelation) (*CommandResult, error) {
	response, err := m.client.AddTaskRelationExact(ctx, &pluginv1.AddTaskRelationExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		RelatedTaskId: input.RelatedTaskID, ExpectedTaskResourceVersion: input.ExpectedTaskResourceVersion,
		ExpectedRelatedResourceVersion: input.ExpectedRelatedResourceVersion, IdempotencyKey: input.IdempotencyKey,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, err
	}
	return commandResultFromProto(response.GetResult()), nil
}

func (m grpcExactTaskCommandManager) RemoveRelation(ctx context.Context, input ExactTaskRelation) (*CommandResult, error) {
	response, err := m.client.RemoveTaskRelationExact(ctx, &pluginv1.RemoveTaskRelationExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		RelatedTaskId: input.RelatedTaskID, ExpectedTaskResourceVersion: input.ExpectedTaskResourceVersion,
		ExpectedRelatedResourceVersion: input.ExpectedRelatedResourceVersion, IdempotencyKey: input.IdempotencyKey,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, err
	}
	return commandResultFromProto(response.GetResult()), nil
}

func (m grpcExactTaskCommandManager) SendMessage(ctx context.Context, input ExactTaskMessage) (*CommandResult, error) {
	response, err := m.client.SendTaskMessageExact(ctx, &pluginv1.SendTaskMessageExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		SessionId: input.SessionID, IdempotencyKey: input.IdempotencyKey,
		ExpectedTaskResourceVersion:    input.ExpectedTaskResourceVersion,
		ExpectedSessionResourceVersion: input.ExpectedSessionResourceVersion,
		Content:                        input.Content, ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
		ManagementInstanceKey: input.ManagementInstanceKey, ExpectedClaimGeneration: input.ExpectedClaimGeneration,
	})
	if err != nil {
		return nil, err
	}
	return commandResultFromProto(response.GetResult()), nil
}

func (m grpcExactTaskCommandManager) IssueDirective(ctx context.Context, input ExactTaskDirectiveIssue) (*CommandResult, TaskDirective, error) {
	response, err := m.client.IssueTaskDirectiveExact(ctx, &pluginv1.IssueTaskDirectiveExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		SessionId: input.SessionID, CapabilityClass: input.CapabilityClass,
		InstructionDigest:              input.InstructionDigest,
		ExpectedTaskResourceVersion:    input.ExpectedTaskResourceVersion,
		ExpectedSessionResourceVersion: input.ExpectedSessionResourceVersion,
		ExpiresAt:                      input.ExpiresAt, IdempotencyKey: input.IdempotencyKey,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, TaskDirective{}, err
	}
	return commandResultFromProto(response.GetResult()), taskDirectiveFromProto(response.GetDirective()), nil
}

func (m grpcExactTaskCommandManager) ResolveDirective(ctx context.Context, input ExactTaskDirectiveResolve) (*CommandResult, TaskDirective, error) {
	response, err := m.client.ResolveTaskDirectiveExact(ctx, &pluginv1.ResolveTaskDirectiveExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, DirectiveId: input.DirectiveID,
		ExpectedResourceVersion: input.ExpectedResourceVersion, IdempotencyKey: input.IdempotencyKey,
		Resolution: input.Resolution, ResolutionDigest: input.ResolutionDigest,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, TaskDirective{}, err
	}
	return commandResultFromProto(response.GetResult()), taskDirectiveFromProto(response.GetDirective()), nil
}
