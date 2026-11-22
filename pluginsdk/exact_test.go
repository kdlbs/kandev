package pluginsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type exactHostFixture struct {
	recordingHost
	managed         *exactManagedConversationFixture
	tasks           *exactTaskCommandFixture
	completionGates *exactTaskCompletionGateCommandFixture
}

func (exactHostFixture) GetCapabilityContext(context.Context, string) (*CapabilityContext, error) {
	return &CapabilityContext{
		InstallationID:   "installation-1",
		WorkspaceID:      "workspace-1",
		ManifestDigest:   "manifest-digest",
		ApprovalRevision: 7,
		ApprovalState:    "active",
		Operations: []CapabilityOperation{{
			Method:       "UpdateTaskExact",
			CapabilityID: "host.v2.write:tasks",
			Description:  "Update one task at its current version",
			Supported:    true,
			Authorized:   true,
		}},
		Limits: map[string]uint64{"page_size": 200},
	}, nil
}

func (exactHostFixture) UpdateTaskExact(_ context.Context, request ExactTaskUpdate) (*CommandResult, *Task, error) {
	return &CommandResult{Status: CommandApplied, Receipt: &CommandReceipt{
		ID: "receipt-1", OperationID: "operation-1", Status: CommandApplied,
		TargetID: request.TaskID, ResourceVersion: "2026-09-25T12:00:00Z",
		ApprovalRevision: request.ApprovalRevision,
	}}, &Task{ID: request.TaskID, ResourceVersion: "2026-09-25T12:00:00Z"}, nil
}

func (h *exactHostFixture) ManagedAgentConversations() ManagedAgentConversationManager {
	if h.managed == nil {
		h.managed = &exactManagedConversationFixture{}
	}
	return h.managed
}

func (h *exactHostFixture) TaskCommands() ExactTaskCommandManager {
	if h.tasks == nil {
		h.tasks = &exactTaskCommandFixture{}
	}
	return h.tasks
}

func (h *exactHostFixture) TaskCompletionGates() ExactTaskCompletionGateCommandManager {
	if h.completionGates == nil {
		h.completionGates = &exactTaskCompletionGateCommandFixture{}
	}
	return h.completionGates
}

type exactTaskCompletionGateCommandFixture struct {
	lastCriteria ExactTaskCompletionCriteria
	lastEvidence ExactTaskCompletionEvidence
}

func (f *exactTaskCompletionGateCommandFixture) SetCriteria(_ context.Context, input ExactTaskCompletionCriteria) (*CommandResult, *TaskCompletionGate, error) {
	f.lastCriteria = input
	return &CommandResult{Status: CommandApplied}, &TaskCompletionGate{
		TaskID: input.TaskID, WorkspaceID: input.WorkspaceID, Revision: input.ExpectedRevision + 1, Blocked: true,
		Criteria: []TaskCompletionCriterion{{
			ID: input.Criteria[0].ID, Description: input.Criteria[0].Description,
			EvidenceSubject: input.Criteria[0].EvidenceSubject, CriterionRevision: 1,
		}},
		Blockers: []TaskCompletionBlocker{{CriterionID: input.Criteria[0].ID, Reason: "unverified"}},
	}, nil
}

func (f *exactTaskCompletionGateCommandFixture) Verify(_ context.Context, input ExactTaskCompletionEvidence) (*CommandResult, *TaskCompletionGate, error) {
	f.lastEvidence = input
	return &CommandResult{Status: CommandApplied}, &TaskCompletionGate{
		TaskID: input.TaskID, WorkspaceID: input.WorkspaceID, Revision: input.ExpectedRevision, Blocked: false,
		Criteria: []TaskCompletionCriterion{{ID: input.CriterionID, Evidence: &input.Evidence}},
	}, nil
}

type exactTaskCommandFixture struct {
	lastLabels     ExactTaskLabels
	lastAssignment ExactTaskAssignment
	lastMove       ExactTaskMove
	lastArchive    ExactTaskArchive
	lastRelation   ExactTaskRelation
	lastMessage    ExactTaskMessage
	lastDirective  ExactTaskDirectiveIssue
	lastResolution ExactTaskDirectiveResolve
}

func (*exactTaskCommandFixture) CreateTask(_ context.Context, input ExactTaskCreate) (*CommandResult, *Task, error) {
	return &CommandResult{Status: CommandApplied}, &Task{ID: input.ExternalID}, nil
}

func (f *exactTaskCommandFixture) SetLabels(_ context.Context, input ExactTaskLabels) (*CommandResult, *Task, error) {
	f.lastLabels = input
	return &CommandResult{Status: CommandApplied}, &Task{ID: input.TaskID}, nil
}

func (f *exactTaskCommandFixture) Assign(_ context.Context, input ExactTaskAssignment) (*CommandResult, *Task, error) {
	f.lastAssignment = input
	return &CommandResult{Status: CommandApplied}, &Task{ID: input.TaskID}, nil
}

func (f *exactTaskCommandFixture) Move(_ context.Context, input ExactTaskMove) (*CommandResult, *Task, error) {
	f.lastMove = input
	return &CommandResult{Status: CommandApplied}, &Task{ID: input.TaskID}, nil
}

func (f *exactTaskCommandFixture) Archive(_ context.Context, input ExactTaskArchive) (*CommandResult, *Task, error) {
	f.lastArchive = input
	return &CommandResult{Status: CommandApplied}, &Task{ID: input.TaskID}, nil
}

func (f *exactTaskCommandFixture) AddRelation(_ context.Context, input ExactTaskRelation) (*CommandResult, error) {
	f.lastRelation = input
	return &CommandResult{Status: CommandApplied}, nil
}

func (f *exactTaskCommandFixture) RemoveRelation(_ context.Context, input ExactTaskRelation) (*CommandResult, error) {
	f.lastRelation = input
	return &CommandResult{Status: CommandApplied}, nil
}

func (f *exactTaskCommandFixture) SendMessage(_ context.Context, input ExactTaskMessage) (*CommandResult, error) {
	f.lastMessage = input
	return &CommandResult{Status: CommandApplied}, nil
}

func (f *exactTaskCommandFixture) IssueDirective(_ context.Context, input ExactTaskDirectiveIssue) (*CommandResult, TaskDirective, error) {
	f.lastDirective = input
	expiresAt := input.ExpiresAt
	return &CommandResult{Status: CommandApplied}, TaskDirective{
		ID: "directive-1", WorkspaceID: input.WorkspaceID, TaskID: input.TaskID,
		CapabilityClass: input.CapabilityClass, State: "pending", InstructionDigest: input.InstructionDigest,
		ExpiresAt: &expiresAt, ResourceVersion: "directive-version-1",
	}, nil
}

func (f *exactTaskCommandFixture) ResolveDirective(_ context.Context, input ExactTaskDirectiveResolve) (*CommandResult, TaskDirective, error) {
	f.lastResolution = input
	return &CommandResult{Status: CommandApplied}, TaskDirective{
		ID: input.DirectiveID, WorkspaceID: input.WorkspaceID, State: "resolved", ResourceVersion: "directive-version-2",
	}, nil
}

type exactManagedConversationFixture struct {
	lastEnqueue  ManagedAgentInputEnqueue
	lastGet      ManagedAgentInputQuery
	lastList     ManagedAgentInputListQuery
	lastCancel   ManagedAgentInputCancel
	lastDispatch ManagedAgentConversationDispatch
}

func (*exactManagedConversationFixture) Ensure(_ context.Context, spec ManagedAgentConversationSpec) (*CommandResult, ManagedAgentConversationDescriptor, error) {
	return &CommandResult{Status: CommandApplied}, ManagedAgentConversationDescriptor{
		InstallationID: "installation-1", TaskID: "task-managed-1", SessionID: "session-managed-1",
		WorkspaceID: spec.WorkspaceID, InstanceKey: spec.InstanceKey, Revision: spec.ExpectedRevision + 1,
		AgentProfileID: spec.AgentProfileID, ExecutorID: spec.ExecutorID, ExecutorProfileID: spec.ExecutorProfileID,
		BasePrompt: spec.BasePrompt, InstructionVersion: spec.InstructionVersion, RetentionMode: "retain_on_uninstall",
	}, nil
}

func (*exactManagedConversationFixture) Get(_ context.Context, query ManagedAgentConversationQuery) (ManagedAgentConversationDescriptor, error) {
	return ManagedAgentConversationDescriptor{
		InstallationID: "installation-1", TaskID: "task-managed-1", WorkspaceID: query.WorkspaceID,
		InstanceKey: query.InstanceKey, Revision: 2,
	}, nil
}

func (*exactManagedConversationFixture) List(_ context.Context, query ManagedAgentConversationListQuery) ([]ManagedAgentConversationDescriptor, error) {
	return []ManagedAgentConversationDescriptor{{
		InstallationID: "installation-1", WorkspaceID: query.WorkspaceID, InstanceKey: "lead", Revision: 2,
	}}, nil
}

func (*exactManagedConversationFixture) SetPaused(_ context.Context, input ManagedAgentConversationPause) (*CommandResult, ManagedAgentConversationDescriptor, error) {
	return &CommandResult{Status: CommandApplied}, ManagedAgentConversationDescriptor{
		InstallationID: "installation-1", WorkspaceID: input.WorkspaceID, InstanceKey: input.InstanceKey,
		Revision: input.ExpectedRevision + 1, DesiredPaused: input.Paused,
	}, nil
}

func (*exactManagedConversationFixture) Delete(context.Context, ManagedAgentConversationDelete) (*CommandResult, error) {
	return &CommandResult{Status: CommandApplied}, nil
}

func (m *exactManagedConversationFixture) EnqueueInput(_ context.Context, input ManagedAgentInputEnqueue) (*CommandResult, ManagedAgentInputReceipt, error) {
	m.lastEnqueue = input
	return &CommandResult{Status: CommandApplied}, ManagedAgentInputReceipt{
		HostInputID: "input-1", OccurrenceKey: input.OccurrenceKey, Sequence: 11,
		Origin: input.Origin, Payload: input.Payload, CoalesceKey: input.CoalesceKey,
		ConversationRevision: input.ExpectedConversationRevision, State: ManagedAgentInputAccepted,
		CreatedAt: "2026-09-25T12:00:00Z", UpdatedAt: "2026-09-25T12:00:00Z", QueueEntryID: "queue-1",
	}, nil
}

func (m *exactManagedConversationFixture) GetInput(_ context.Context, query ManagedAgentInputQuery) (ManagedAgentInputReceipt, error) {
	m.lastGet = query
	return ManagedAgentInputReceipt{
		HostInputID: query.HostInputID, OccurrenceKey: "occurrence-1", Sequence: 11,
		Origin: ManagedAgentInputHuman, Payload: "queued text", ConversationRevision: 4,
		State: ManagedAgentInputRunning, CreatedAt: "2026-09-25T12:00:00Z", UpdatedAt: "2026-09-25T12:01:00Z",
		QueueEntryID: "queue-1", ExecutionID: "execution-1", TurnID: "turn-1",
	}, nil
}

func (m *exactManagedConversationFixture) ListInputs(_ context.Context, query ManagedAgentInputListQuery) (ManagedAgentInputPage, error) {
	m.lastList = query
	return ManagedAgentInputPage{
		Inputs: []ManagedAgentInputReceipt{{
			HostInputID: "input-2", OccurrenceKey: "occurrence-2", Sequence: query.SequenceCursor + 1,
			Origin: ManagedAgentInputPeriodic, Payload: "tick", CoalesceKey: "schedule-1",
			ConversationRevision: 6, State: ManagedAgentInputCompleted,
			CreatedAt: "2026-09-25T12:02:00Z", UpdatedAt: "2026-09-25T12:03:00Z",
			QueueEntryID: "queue-2", ExecutionID: "execution-2", TurnID: "turn-2", SupersededBy: "input-3",
		}},
		NextSequenceCursor: query.SequenceCursor + 1, HasMore: true,
	}, nil
}

func (m *exactManagedConversationFixture) CancelInput(_ context.Context, input ManagedAgentInputCancel) (*CommandResult, ManagedAgentInputReceipt, error) {
	m.lastCancel = input
	return &CommandResult{Status: CommandApplied}, ManagedAgentInputReceipt{
		HostInputID: input.HostInputID, OccurrenceKey: "occurrence-1", Sequence: 11,
		Origin: ManagedAgentInputHuman, Payload: "queued text", ConversationRevision: input.ExpectedConversationRevision,
		State: ManagedAgentInputCancelled, CreatedAt: "2026-09-25T12:00:00Z", UpdatedAt: "2026-09-25T12:04:00Z",
		QueueEntryID: "queue-1", ExecutionID: input.ExpectedExecutionID, TurnID: "turn-1",
	}, nil
}

func (m *exactManagedConversationFixture) Dispatch(_ context.Context, input ManagedAgentConversationDispatch) (*CommandResult, ManagedAgentDispatchStatus, ManagedAgentConversationDescriptor, error) {
	m.lastDispatch = input
	return &CommandResult{Status: CommandConflict, Reason: "conversation is busy"}, ManagedAgentDispatchBusy, ManagedAgentConversationDescriptor{
		InstallationID: "installation-1", TaskID: "task-managed-1", SessionID: "session-managed-1",
		WorkspaceID: input.WorkspaceID, InstanceKey: input.InstanceKey, Revision: input.ExpectedConversationRevision,
	}, nil
}

func TestExactHost_GetCapabilityContextRoundTrips(t *testing.T) {
	host := dialHostOverBufconn(t, &exactHostFixture{})
	exact, ok := HostV2(host)
	require.True(t, ok, "the generated Host client should expose the additive ExactHost contract")

	got, err := exact.GetCapabilityContext(context.Background(), "workspace-1")
	require.NoError(t, err)
	require.Equal(t, "installation-1", got.InstallationID)
	require.Equal(t, uint64(7), got.ApprovalRevision)
	require.Equal(t, "UpdateTaskExact", got.Operations[0].Method)
	require.True(t, got.Operations[0].Authorized)
	require.Equal(t, uint64(200), got.Limits["page_size"])

	title := "approved change"
	result, task, err := exact.UpdateTaskExact(context.Background(), ExactTaskUpdate{
		RequestID: "request-1", WorkspaceID: "workspace-1", TaskID: "task-1",
		IdempotencyKey: "key-1", ExpectedResourceVersion: "2026-09-25T11:00:00Z",
		ApprovalRevision: 7, ManifestDigest: "manifest-digest", Title: &title,
	})
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, "operation-1", result.Receipt.OperationID)
	require.Equal(t, "2026-09-25T12:00:00Z", task.ResourceVersion)
}

func TestExactTaskCompletionGateCommandsRoundTrip(t *testing.T) {
	fixture := &exactHostFixture{}
	host := dialHostOverBufconn(t, fixture)
	manager, ok := HostTaskCompletionGates(host)
	require.True(t, ok)
	criteria := ExactTaskCompletionCriteria{
		RequestID: "request-gate", WorkspaceID: "workspace-1", TaskID: "task-1", IdempotencyKey: "criteria-1",
		ExpectedTaskResourceVersion: "2026-09-26T12:00:00Z", ExpectedRevision: 2,
		ApprovalRevision: 7, ManifestDigest: "manifest-digest", ManagementInstanceKey: "coordinator", ExpectedClaimGeneration: 3,
		Criteria: []TaskCompletionCriterionInput{{
			ID: "checks", Description: "All checks pass",
			EvidenceSubject: TaskCompletionEvidenceSubject{Kind: "artifact_revision", ID: "artifact-1"},
		}},
	}
	result, gate, err := manager.SetCriteria(context.Background(), criteria)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, int64(3), gate.Revision)
	require.Equal(t, "artifact-1", gate.Criteria[0].EvidenceSubject.ID)
	require.True(t, gate.Blocked)
	require.Equal(t, criteria.ManagementInstanceKey, fixture.completionGates.lastCriteria.ManagementInstanceKey)
	require.Equal(t, criteria.Criteria, fixture.completionGates.lastCriteria.Criteria)

	evidence := ExactTaskCompletionEvidence{
		RequestID: "request-evidence", WorkspaceID: "workspace-1", TaskID: "task-1", CriterionID: "checks",
		IdempotencyKey: "evidence-1", ExpectedTaskResourceVersion: criteria.ExpectedTaskResourceVersion,
		ExpectedRevision: 3, ApprovalRevision: 7, ManifestDigest: "manifest-digest",
		ManagementInstanceKey: "coordinator", ExpectedClaimGeneration: 3,
		Evidence: TaskCompletionEvidence{
			Subject: TaskCompletionEvidenceSubject{Kind: "artifact_revision", ID: "artifact-1", Revision: "artifact-v1"},
			Summary: "The test suite passed.", Reference: "run-42",
		},
	}
	verifiedResult, verified, err := manager.Verify(context.Background(), evidence)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, verifiedResult.Status)
	require.False(t, verified.Blocked)
	require.Equal(t, "artifact-v1", verified.Criteria[0].Evidence.Subject.Revision)
	require.Equal(t, evidence, fixture.completionGates.lastEvidence)
}

func TestExactTaskCommandsSetLabelsRoundTrip(t *testing.T) {
	fixture := &exactHostFixture{}
	host := dialHostOverBufconn(t, fixture)
	manager, ok := HostTaskCommands(host)
	require.True(t, ok)

	result, task, err := manager.SetLabels(context.Background(), ExactTaskLabels{
		RequestID: "request-1", WorkspaceID: "workspace-1", TaskID: "task-1",
		IdempotencyKey: "labels-1", ExpectedResourceVersion: "2026-09-25T11:00:00Z",
		ApprovalRevision: 7, ManifestDigest: "manifest-digest", Labels: []string{"urgent", "coordination"},
	})
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, "task-1", task.ID)
	require.Equal(t, []string{"urgent", "coordination"}, fixture.tasks.lastLabels.Labels)
}

func TestExactTaskCommandsAssignRoundTrip(t *testing.T) {
	fixture := &exactHostFixture{}
	host := dialHostOverBufconn(t, fixture)
	manager, ok := HostTaskCommands(host)
	require.True(t, ok)

	result, task, err := manager.Assign(context.Background(), ExactTaskAssignment{
		RequestID: "request-assign", WorkspaceID: "workspace-1", TaskID: "task-1",
		IdempotencyKey: "assign-1", ExpectedResourceVersion: "2026-09-25T11:00:00Z",
		ApprovalRevision: 7, ManifestDigest: "manifest-digest", AssigneeUserID: "user-42",
	})
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, "task-1", task.ID)
	require.Equal(t, "user-42", fixture.tasks.lastAssignment.AssigneeUserID)
}

func TestExactTaskCommandsMoveRoundTrip(t *testing.T) {
	fixture := &exactHostFixture{}
	host := dialHostOverBufconn(t, fixture)
	manager, ok := HostTaskCommands(host)
	require.True(t, ok)

	result, task, err := manager.Move(context.Background(), ExactTaskMove{
		RequestID: "request-move", WorkspaceID: "workspace-1", TaskID: "task-1",
		IdempotencyKey: "move-1", ExpectedResourceVersion: "2026-09-25T11:00:00Z",
		WorkflowID: "workflow-1", WorkflowStepID: "step-2", Position: 3,
		ApprovalRevision: 7, ManifestDigest: "manifest-digest",
	})
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, "task-1", task.ID)
	require.Equal(t, "step-2", fixture.tasks.lastMove.WorkflowStepID)
	require.Equal(t, int32(3), fixture.tasks.lastMove.Position)
}

func TestExactTaskCommandsArchiveRoundTrip(t *testing.T) {
	fixture := &exactHostFixture{}
	host := dialHostOverBufconn(t, fixture)
	manager, ok := HostTaskCommands(host)
	require.True(t, ok)

	result, task, err := manager.Archive(context.Background(), ExactTaskArchive{
		RequestID: "request-archive", WorkspaceID: "workspace-1", TaskID: "task-1",
		IdempotencyKey: "archive-1", ExpectedResourceVersion: "2026-09-25T11:00:00Z",
		ApprovalRevision: 7, ManifestDigest: "manifest-digest",
	})
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, "task-1", task.ID)
	require.Equal(t, "archive-1", fixture.tasks.lastArchive.IdempotencyKey)
}

func TestExactTaskRelationCommandsRoundTripVersions(t *testing.T) {
	fixture := &exactHostFixture{}
	host := dialHostOverBufconn(t, fixture)
	manager, ok := HostTaskCommands(host)
	require.True(t, ok)
	input := ExactTaskRelation{
		RequestID: "relation-request", WorkspaceID: "workspace-1", TaskID: "task-1", RelatedTaskID: "task-2",
		ExpectedTaskResourceVersion:    "2026-09-25T11:00:00Z",
		ExpectedRelatedResourceVersion: "2026-09-25T11:30:00Z",
		IdempotencyKey:                 "relation-1", ApprovalRevision: 7, ManifestDigest: "manifest-digest",
	}
	result, err := manager.AddRelation(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, input.ExpectedTaskResourceVersion, fixture.tasks.lastRelation.ExpectedTaskResourceVersion)
	require.Equal(t, input.ExpectedRelatedResourceVersion, fixture.tasks.lastRelation.ExpectedRelatedResourceVersion)
}

func TestExactTaskMessageCommandRoundTripsVersionsAndContent(t *testing.T) {
	fixture := &exactHostFixture{}
	host := dialHostOverBufconn(t, fixture)
	manager, ok := HostTaskCommands(host)
	require.True(t, ok)
	input := ExactTaskMessage{
		RequestID: "message-request", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		IdempotencyKey: "message-1", ExpectedTaskResourceVersion: "2026-09-25T11:00:00Z",
		ExpectedSessionResourceVersion: "2026-09-25T11:30:00Z", ApprovalRevision: 7,
		ManifestDigest: "manifest-digest", Content: "Continue after the accepted receipt.",
	}
	result, err := manager.SendMessage(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, input, fixture.tasks.lastMessage)
}

func TestExactTaskDirectiveCommandsRoundTripBoundFields(t *testing.T) {
	fixture := &exactHostFixture{}
	host := dialHostOverBufconn(t, fixture)
	manager, ok := HostTaskCommands(host)
	require.True(t, ok)
	issue := ExactTaskDirectiveIssue{
		RequestID: "directive-request", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		CapabilityClass: "host.v2.write:tasks", InstructionDigest: "sha256:instruction",
		ExpectedTaskResourceVersion: "2026-09-25T11:00:00Z", ExpectedSessionResourceVersion: "2026-09-25T11:30:00Z",
		ExpiresAt: "2026-09-26T11:00:00Z", IdempotencyKey: "directive-1", ApprovalRevision: 7, ManifestDigest: "manifest-digest",
	}
	result, directive, err := manager.IssueDirective(context.Background(), issue)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, issue, fixture.tasks.lastDirective)
	require.Equal(t, "directive-1", directive.ID)
	require.Equal(t, issue.ExpiresAt, *directive.ExpiresAt)

	resolution := ExactTaskDirectiveResolve{
		RequestID: "resolve-request", WorkspaceID: "workspace-1", DirectiveID: directive.ID,
		ExpectedResourceVersion: directive.ResourceVersion, IdempotencyKey: "resolve-1", Resolution: "completed",
		ResolutionDigest: "sha256:resolution", ApprovalRevision: 7, ManifestDigest: "manifest-digest",
	}
	result, resolved, err := manager.ResolveDirective(context.Background(), resolution)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, resolution, fixture.tasks.lastResolution)
	require.Equal(t, "resolved", resolved.State)
}

func TestManagedAgentConversationHostMethodsRoundTrip(t *testing.T) {
	host := dialHostOverBufconn(t, &exactHostFixture{})
	exact, ok := HostV2(host)
	require.True(t, ok)
	manager := exact.ManagedAgentConversations()

	result, conversation, err := manager.Ensure(context.Background(), ManagedAgentConversationSpec{
		WorkspaceID: "workspace-1", InstanceKey: "lead", ExpectedRevision: 3,
		AgentProfileID: "profile-1", ExecutorID: "executor-1", ExecutorProfileID: "executor-profile-1",
		BasePrompt: "Coordinate only assigned work", InstructionVersion: "v4",
	})
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, "task-managed-1", conversation.TaskID)
	require.Equal(t, uint64(4), conversation.Revision)
	require.Equal(t, "v4", conversation.InstructionVersion)
	require.Equal(t, "retain_on_uninstall", conversation.RetentionMode)

	got, err := manager.Get(context.Background(), ManagedAgentConversationQuery{
		WorkspaceID: "workspace-1", InstanceKey: "lead", ApprovalRevision: 7, ManifestDigest: "digest",
	})
	require.NoError(t, err)
	require.Equal(t, "task-managed-1", got.TaskID)

	listed, err := manager.List(context.Background(), ManagedAgentConversationListQuery{
		WorkspaceID: "workspace-1", ApprovalRevision: 7, ManifestDigest: "digest",
	})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "lead", listed[0].InstanceKey)

	paused, descriptor, err := manager.SetPaused(context.Background(), ManagedAgentConversationPause{
		RequestID: "request-1", IdempotencyKey: "pause-1", WorkspaceID: "workspace-1", InstanceKey: "lead",
		ExpectedRevision: 4, ApprovalRevision: 7, ManifestDigest: "digest", Paused: true,
	})
	require.NoError(t, err)
	require.Equal(t, CommandApplied, paused.Status)
	require.True(t, descriptor.DesiredPaused)

	deleted, err := manager.Delete(context.Background(), ManagedAgentConversationDelete{
		RequestID: "request-2", IdempotencyKey: "delete-1", WorkspaceID: "workspace-1", InstanceKey: "lead",
		ExpectedRevision: 5, ApprovalRevision: 7, ManifestDigest: "digest",
	})
	require.NoError(t, err)
	require.Equal(t, CommandApplied, deleted.Status)
}

func TestHostV2FeatureDetectionPreservesLegacyHost(t *testing.T) {
	var legacy Host = &recordingHost{}
	if _, ok := HostV2(legacy); ok {
		t.Fatal("legacy Host unexpectedly advertises exact Host v2 operations")
	}
	var extended Host = &exactHostFixture{recordingHost: recordingHost{}}
	if _, ok := HostV2(extended); !ok {
		t.Fatal("exact Host implementation was not detected")
	}
}
