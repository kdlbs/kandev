package pluginsdk

import (
	"testing"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestWorkspaceAdminCommandsRoundTripEveryTypedOperation(t *testing.T) {
	version := "2026-09-26T11:00:00Z"
	name := "Coordinator workspace"
	branch := "main"
	pull := false
	step := WorkflowStepConfiguration{
		ID: "step-1", WorkflowID: "workflow-1", Name: "Review", Position: 2,
		StageType: "review", Prompt: "Review changes", IsStartStep: true,
		OnEnterActionTypes: []string{"clear_decisions"},
		OnTurnStartActions: []WorkflowTransitionAction{{Type: "move_to_step", TargetStepID: "step-2"}},
		OnTurnCompleteActions: []WorkflowTransitionAction{
			{Type: "move_to_next"}, {Type: "disable_plan_mode"},
		},
	}
	base := WorkspaceAdminCommand{
		RequestID: "request-1", WorkspaceID: "workspace-1", IdempotencyKey: "operation-1",
		ApprovalRevision: 7, ManifestDigest: "manifest-digest",
	}
	cases := []WorkspaceAdminCommand{
		{UpdateDefaults: &WorkspaceDefaultsUpdate{ExpectedResourceVersion: version, Name: &name, DefaultConfigAgentProfile: &name}},
		{CreateWorkflow: &WorkspaceWorkflowCreate{ExpectedWorkspaceResourceVersion: version, Name: "Workflow", Prompt: "Prompt"}},
		{UpdateWorkflow: &WorkspaceWorkflowUpdate{WorkflowID: "workflow-1", ExpectedResourceVersion: version, Name: &name}},
		{ReorderWorkflows: &WorkspaceWorkflowReorder{ExpectedWorkspaceResourceVersion: version, WorkflowIDs: []string{"workflow-1", "workflow-2"}, WorkflowResourceVersions: []string{version, version}}},
		{CreateStep: &WorkspaceWorkflowStepCreate{WorkflowID: "workflow-1", ExpectedWorkflowResourceVersion: version, Configuration: step}},
		{UpdateStep: &WorkspaceWorkflowStepUpdate{WorkflowID: "workflow-1", StepID: "step-1", ExpectedWorkflowResourceVersion: version, ExpectedStepResourceVersion: version, Configuration: step}},
		{ReorderSteps: &WorkspaceWorkflowStepReorder{WorkflowID: "workflow-1", ExpectedWorkflowResourceVersion: version, StepIDs: []string{"step-1", "step-2"}, StepResourceVersions: []string{version, version}}},
		{RegisterRepo: &WorkspaceRepositoryRegister{ExpectedWorkspaceResourceVersion: version, Name: "repo", SourceType: "local", LocalPath: "/work/repo", DefaultBranch: branch, PullBeforeWorktree: &pull}},
		{UpdateRepo: &WorkspaceRepositoryUpdate{RepositoryID: "repo-1", ExpectedResourceVersion: version, DefaultBranch: &branch, PullBeforeWorktree: &pull}},
	}
	for index, operation := range cases {
		command := base
		command.RequestID += string(rune('a' + index))
		command.IdempotencyKey += string(rune('a' + index))
		operation.RequestID = command.RequestID
		operation.WorkspaceID = command.WorkspaceID
		operation.IdempotencyKey = command.IdempotencyKey
		operation.ApprovalRevision = command.ApprovalRevision
		operation.ManifestDigest = command.ManifestDigest
		request, err := workspaceAdminCommandToProto(operation)
		require.NoError(t, err, "operation %d", index)
		decoded, err := workspaceAdminCommandFromProto(request)
		require.NoError(t, err, "operation %d", index)
		require.Equal(t, operation, decoded, "operation %d", index)
	}
}

func TestWorkspaceAdminSchemaOmitsHumanOnlyAndDestructiveFields(t *testing.T) {
	registerFields := (&pluginv1.WorkspaceRepositoryRegisterExact{}).ProtoReflect().Descriptor().Fields()
	updateFields := (&pluginv1.WorkspaceRepositoryUpdateExact{}).ProtoReflect().Descriptor().Fields()
	for _, forbidden := range []string{"setup_script", "cleanup_script", "dev_script", "copy_files", "secret_bindings"} {
		require.Nil(t, registerFields.ByName(protoreflect.Name(forbidden)), "register field %s is human-only", forbidden)
		require.Nil(t, updateFields.ByName(protoreflect.Name(forbidden)), "update field %s is human-only", forbidden)
	}
}

func TestWorkspaceAdminCommandRejectsMissingOrMultipleOperations(t *testing.T) {
	_, err := workspaceAdminCommandToProto(WorkspaceAdminCommand{RequestID: "request-1"})
	require.Error(t, err)

	_, err = workspaceAdminCommandToProto(WorkspaceAdminCommand{
		UpdateDefaults: &WorkspaceDefaultsUpdate{},
		UpdateRepo:     &WorkspaceRepositoryUpdate{},
	})
	require.Error(t, err)
}
