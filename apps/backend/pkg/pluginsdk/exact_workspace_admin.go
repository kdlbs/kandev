package pluginsdk

import (
	"context"
	"fmt"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

type WorkspaceAdminOperationKind string

const (
	WorkspaceAdminUpdateDefaults      WorkspaceAdminOperationKind = "UpdateWorkspaceDefaultsExact"
	WorkspaceAdminCreateWorkflow      WorkspaceAdminOperationKind = "CreateWorkflowExact"
	WorkspaceAdminUpdateWorkflow      WorkspaceAdminOperationKind = "UpdateWorkflowExact"
	WorkspaceAdminReorderWorkflows    WorkspaceAdminOperationKind = "ReorderWorkflowsExact"
	WorkspaceAdminCreateWorkflowStep  WorkspaceAdminOperationKind = "CreateWorkflowStepExact"
	WorkspaceAdminUpdateWorkflowStep  WorkspaceAdminOperationKind = "UpdateWorkflowStepExact"
	WorkspaceAdminReorderWorkflowStep WorkspaceAdminOperationKind = "ReorderWorkflowStepsExact"
	WorkspaceAdminRegisterRepository  WorkspaceAdminOperationKind = "RegisterRepositoryExact"
	WorkspaceAdminUpdateRepository    WorkspaceAdminOperationKind = "UpdateRepositoryExact"
)

type WorkspaceAdminCommand struct {
	RequestID        string
	WorkspaceID      string
	IdempotencyKey   string
	ApprovalRevision uint64
	ManifestDigest   string
	UpdateDefaults   *WorkspaceDefaultsUpdate
	CreateWorkflow   *WorkspaceWorkflowCreate
	UpdateWorkflow   *WorkspaceWorkflowUpdate
	ReorderWorkflows *WorkspaceWorkflowReorder
	CreateStep       *WorkspaceWorkflowStepCreate
	UpdateStep       *WorkspaceWorkflowStepUpdate
	ReorderSteps     *WorkspaceWorkflowStepReorder
	RegisterRepo     *WorkspaceRepositoryRegister
	UpdateRepo       *WorkspaceRepositoryUpdate
}

type WorkspaceDefaultsUpdate struct {
	ExpectedResourceVersion   string
	Name                      *string
	Description               *string
	DefaultExecutorID         *string
	DefaultEnvironmentID      *string
	DefaultAgentProfileID     *string
	DefaultConfigAgentProfile *string
}

type WorkspaceWorkflowCreate struct {
	ExpectedWorkspaceResourceVersion string
	Name                             string
	Description                      string
	Prompt                           string
}

type WorkspaceWorkflowUpdate struct {
	WorkflowID              string
	ExpectedResourceVersion string
	Name                    *string
	Description             *string
	Prompt                  *string
	AgentProfileID          *string
}

type WorkspaceWorkflowReorder struct {
	ExpectedWorkspaceResourceVersion string
	WorkflowIDs                      []string
	WorkflowResourceVersions         []string
}

type WorkspaceWorkflowStepCreate struct {
	WorkflowID                      string
	ExpectedWorkflowResourceVersion string
	Configuration                   WorkflowStepConfiguration
}

type WorkspaceWorkflowStepUpdate struct {
	WorkflowID                      string
	StepID                          string
	ExpectedWorkflowResourceVersion string
	ExpectedStepResourceVersion     string
	Configuration                   WorkflowStepConfiguration
}

type WorkspaceWorkflowStepReorder struct {
	WorkflowID                      string
	ExpectedWorkflowResourceVersion string
	StepIDs                         []string
	StepResourceVersions            []string
}

type WorkflowStepConfiguration struct {
	ID                         string
	WorkflowID                 string
	Name                       string
	Position                   int32
	StageType                  string
	Color                      string
	Prompt                     string
	AllowManualMove            bool
	IsStartStep                bool
	ShowInCommandPanel         bool
	AutoArchiveAfterHours      int32
	AgentProfileID             string
	ProfileSessionStartPolicy  string
	ProfileSessionEndPolicy    string
	WIPLimit                   int32
	PullFromStepID             string
	AutoAdvanceRequiresSignal  bool
	CancelTriggersTurnComplete bool
	CompleteTaskOnEnter        bool
	OnEnterActionTypes         []string
	OnTurnStartActions         []WorkflowTransitionAction
	OnTurnCompleteActions      []WorkflowTransitionAction
}

type WorkflowTransitionAction struct {
	Type         string
	TargetStepID string
}

type WorkspaceRepositoryRegister struct {
	ExpectedWorkspaceResourceVersion string
	Name                             string
	SourceType                       string
	LocalPath                        string
	Provider                         string
	ProviderRepoID                   string
	ProviderHost                     string
	ProviderScope                    string
	ProviderOwner                    string
	ProviderName                     string
	RemoteURL                        string
	DefaultBranch                    string
	WorktreeBranchPrefix             string
	WorktreeBranchTemplate           string
	PullBeforeWorktree               *bool
}

type WorkspaceRepositoryUpdate struct {
	RepositoryID            string
	ExpectedResourceVersion string
	Name                    *string
	DefaultBranch           *string
	WorktreeBranchPrefix    *string
	WorktreeBranchTemplate  *string
	PullBeforeWorktree      *bool
}

type ExactWorkspaceAdministrationManager interface {
	Apply(ctx context.Context, command WorkspaceAdminCommand) (*CommandResult, error)
}

type ExactWorkspaceAdministrationHost interface {
	WorkspaceAdministration() ExactWorkspaceAdministrationManager
}

func HostWorkspaceAdministration(host Host) (ExactWorkspaceAdministrationManager, bool) {
	extended, ok := host.(ExactWorkspaceAdministrationHost)
	if !ok || extended.WorkspaceAdministration() == nil {
		return nil, false
	}
	return extended.WorkspaceAdministration(), true
}

type grpcExactWorkspaceAdministrationManager struct{ client pluginv1.HostClient }

func (m grpcExactWorkspaceAdministrationManager) Apply(ctx context.Context, command WorkspaceAdminCommand) (*CommandResult, error) {
	request, err := workspaceAdminCommandToProto(command)
	if err != nil {
		return nil, err
	}
	response, err := m.client.ApplyWorkspaceAdministrationExact(ctx, request)
	if err != nil {
		return nil, err
	}
	return commandResultFromProto(response.GetResult()), nil
}

//nolint:funlen // The encoder keeps every supported operation adjacent to its generated oneof type.
func workspaceAdminCommandToProto(command WorkspaceAdminCommand) (*pluginv1.ApplyWorkspaceAdministrationExactRequest, error) {
	request := &pluginv1.ApplyWorkspaceAdministrationExactRequest{
		RequestId: command.RequestID, WorkspaceId: command.WorkspaceID, IdempotencyKey: command.IdempotencyKey,
		ApprovalRevision: command.ApprovalRevision, ManifestDigest: command.ManifestDigest,
	}
	operations := 0
	if value := command.UpdateDefaults; value != nil {
		operations++
		request.Operation = &pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkspaceDefaults{WorkspaceDefaults: &pluginv1.WorkspaceDefaultsUpdateExact{
			ExpectedResourceVersion: value.ExpectedResourceVersion, Name: value.Name, Description: value.Description,
			DefaultExecutorId: value.DefaultExecutorID, DefaultEnvironmentId: value.DefaultEnvironmentID,
			DefaultAgentProfileId: value.DefaultAgentProfileID, DefaultConfigAgentProfileId: value.DefaultConfigAgentProfile,
		}}
	}
	if value := command.CreateWorkflow; value != nil {
		operations++
		request.Operation = &pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowCreate{WorkflowCreate: &pluginv1.WorkspaceWorkflowCreateExact{
			ExpectedWorkspaceResourceVersion: value.ExpectedWorkspaceResourceVersion,
			Configuration:                    &pluginv1.WorkspaceWorkflowConfig{Name: value.Name, Description: value.Description, Prompt: value.Prompt},
		}}
	}
	if value := command.UpdateWorkflow; value != nil {
		operations++
		request.Operation = &pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowUpdate{WorkflowUpdate: &pluginv1.WorkspaceWorkflowUpdateExact{
			WorkflowId: value.WorkflowID, ExpectedResourceVersion: value.ExpectedResourceVersion,
			Name: value.Name, Description: value.Description, Prompt: value.Prompt, AgentProfileId: value.AgentProfileID,
		}}
	}
	if value := command.ReorderWorkflows; value != nil {
		operations++
		request.Operation = &pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowReorder{WorkflowReorder: &pluginv1.WorkspaceWorkflowReorderExact{
			ExpectedWorkspaceResourceVersion: value.ExpectedWorkspaceResourceVersion,
			WorkflowIds:                      value.WorkflowIDs, WorkflowResourceVersions: value.WorkflowResourceVersions,
		}}
	}
	if value := command.CreateStep; value != nil {
		operations++
		request.Operation = &pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowStepCreate{WorkflowStepCreate: &pluginv1.WorkspaceWorkflowStepCreateExact{
			WorkflowId: value.WorkflowID, ExpectedWorkflowResourceVersion: value.ExpectedWorkflowResourceVersion,
			Configuration: workflowStepConfigurationToProto(value.Configuration),
		}}
	}
	if value := command.UpdateStep; value != nil {
		operations++
		request.Operation = &pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowStepUpdate{WorkflowStepUpdate: &pluginv1.WorkspaceWorkflowStepUpdateExact{
			WorkflowId: value.WorkflowID, StepId: value.StepID,
			ExpectedWorkflowResourceVersion: value.ExpectedWorkflowResourceVersion,
			ExpectedStepResourceVersion:     value.ExpectedStepResourceVersion,
			Configuration:                   workflowStepConfigurationToProto(value.Configuration),
		}}
	}
	if value := command.ReorderSteps; value != nil {
		operations++
		request.Operation = &pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowStepReorder{WorkflowStepReorder: &pluginv1.WorkspaceWorkflowStepReorderExact{
			WorkflowId: value.WorkflowID, ExpectedWorkflowResourceVersion: value.ExpectedWorkflowResourceVersion,
			StepIds: value.StepIDs, StepResourceVersions: value.StepResourceVersions,
		}}
	}
	if value := command.RegisterRepo; value != nil {
		operations++
		request.Operation = &pluginv1.ApplyWorkspaceAdministrationExactRequest_RepositoryRegister{RepositoryRegister: &pluginv1.WorkspaceRepositoryRegisterExact{
			ExpectedWorkspaceResourceVersion: value.ExpectedWorkspaceResourceVersion,
			Name:                             value.Name, SourceType: value.SourceType, LocalPath: value.LocalPath,
			Provider: value.Provider, ProviderRepositoryId: value.ProviderRepoID, ProviderHost: value.ProviderHost,
			ProviderScope: value.ProviderScope, ProviderOwner: value.ProviderOwner, ProviderName: value.ProviderName,
			RemoteUrl: value.RemoteURL, DefaultBranch: value.DefaultBranch,
			WorktreeBranchPrefix: value.WorktreeBranchPrefix, WorktreeBranchTemplate: value.WorktreeBranchTemplate,
			PullBeforeWorktree: value.PullBeforeWorktree,
		}}
	}
	if value := command.UpdateRepo; value != nil {
		operations++
		request.Operation = &pluginv1.ApplyWorkspaceAdministrationExactRequest_RepositoryUpdate{RepositoryUpdate: &pluginv1.WorkspaceRepositoryUpdateExact{
			RepositoryId: value.RepositoryID, ExpectedResourceVersion: value.ExpectedResourceVersion,
			Name: value.Name, DefaultBranch: value.DefaultBranch, WorktreeBranchPrefix: value.WorktreeBranchPrefix,
			WorktreeBranchTemplate: value.WorktreeBranchTemplate, PullBeforeWorktree: value.PullBeforeWorktree,
		}}
	}
	if operations != 1 {
		return nil, fmt.Errorf("workspace administration command must contain exactly one operation")
	}
	return request, nil
}

func workflowStepConfigurationToProto(value WorkflowStepConfiguration) *pluginv1.WorkflowStepConfiguration {
	result := &pluginv1.WorkflowStepConfiguration{
		Id: value.ID, WorkflowId: value.WorkflowID, Name: value.Name, Position: value.Position,
		StageType: value.StageType, Color: value.Color, Prompt: value.Prompt,
		AllowManualMove: value.AllowManualMove, IsStartStep: value.IsStartStep,
		ShowInCommandPanel: value.ShowInCommandPanel, AutoArchiveAfterHours: value.AutoArchiveAfterHours,
		AgentProfileId: value.AgentProfileID, ProfileSessionStartPolicy: value.ProfileSessionStartPolicy,
		ProfileSessionEndPolicy: value.ProfileSessionEndPolicy, WipLimit: value.WIPLimit,
		PullFromStepId: value.PullFromStepID, AutoAdvanceRequiresSignal: value.AutoAdvanceRequiresSignal,
		CancelTriggersTurnComplete: value.CancelTriggersTurnComplete, CompleteTaskOnEnter: value.CompleteTaskOnEnter,
		OnEnterActionTypes: append([]string(nil), value.OnEnterActionTypes...),
	}
	for _, action := range value.OnTurnStartActions {
		result.OnTurnStartActions = append(result.OnTurnStartActions, &pluginv1.WorkflowTransitionActionConfig{Type: action.Type, TargetStepId: action.TargetStepID})
	}
	for _, action := range value.OnTurnCompleteActions {
		result.OnTurnCompleteActions = append(result.OnTurnCompleteActions, &pluginv1.WorkflowTransitionActionConfig{Type: action.Type, TargetStepId: action.TargetStepID})
	}
	return result
}

func workflowStepConfigurationFromProto(value *pluginv1.WorkflowStepConfiguration) WorkflowStepConfiguration {
	if value == nil {
		return WorkflowStepConfiguration{}
	}
	result := WorkflowStepConfiguration{
		ID: value.GetId(), WorkflowID: value.GetWorkflowId(), Name: value.GetName(), Position: value.GetPosition(),
		StageType: value.GetStageType(), Color: value.GetColor(), Prompt: value.GetPrompt(),
		AllowManualMove: value.GetAllowManualMove(), IsStartStep: value.GetIsStartStep(),
		ShowInCommandPanel: value.GetShowInCommandPanel(), AutoArchiveAfterHours: value.GetAutoArchiveAfterHours(),
		AgentProfileID: value.GetAgentProfileId(), ProfileSessionStartPolicy: value.GetProfileSessionStartPolicy(),
		ProfileSessionEndPolicy: value.GetProfileSessionEndPolicy(), WIPLimit: value.GetWipLimit(),
		PullFromStepID: value.GetPullFromStepId(), AutoAdvanceRequiresSignal: value.GetAutoAdvanceRequiresSignal(),
		CancelTriggersTurnComplete: value.GetCancelTriggersTurnComplete(), CompleteTaskOnEnter: value.GetCompleteTaskOnEnter(),
		OnEnterActionTypes: append([]string(nil), value.GetOnEnterActionTypes()...),
	}
	for _, action := range value.GetOnTurnStartActions() {
		if action != nil {
			result.OnTurnStartActions = append(result.OnTurnStartActions, WorkflowTransitionAction{Type: action.GetType(), TargetStepID: action.GetTargetStepId()})
		}
	}
	for _, action := range value.GetOnTurnCompleteActions() {
		if action != nil {
			result.OnTurnCompleteActions = append(result.OnTurnCompleteActions, WorkflowTransitionAction{Type: action.GetType(), TargetStepID: action.GetTargetStepId()})
		}
	}
	return result
}

//nolint:cyclop,funlen // The decoder keeps every supported oneof operation adjacent to its field validation.
func workspaceAdminCommandFromProto(request *pluginv1.ApplyWorkspaceAdministrationExactRequest) (WorkspaceAdminCommand, error) {
	if request == nil {
		return WorkspaceAdminCommand{}, fmt.Errorf("workspace administration request is required")
	}
	command := WorkspaceAdminCommand{
		RequestID: request.GetRequestId(), WorkspaceID: request.GetWorkspaceId(), IdempotencyKey: request.GetIdempotencyKey(),
		ApprovalRevision: request.GetApprovalRevision(), ManifestDigest: request.GetManifestDigest(),
	}
	switch operation := request.GetOperation().(type) {
	case *pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkspaceDefaults:
		value := operation.WorkspaceDefaults
		if value == nil {
			break
		}
		command.UpdateDefaults = &WorkspaceDefaultsUpdate{
			ExpectedResourceVersion: value.GetExpectedResourceVersion(), Name: value.Name, Description: value.Description,
			DefaultExecutorID: value.DefaultExecutorId, DefaultEnvironmentID: value.DefaultEnvironmentId,
			DefaultAgentProfileID: value.DefaultAgentProfileId, DefaultConfigAgentProfile: value.DefaultConfigAgentProfileId,
		}
	case *pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowCreate:
		value := operation.WorkflowCreate
		if value == nil {
			break
		}
		config := value.GetConfiguration()
		command.CreateWorkflow = &WorkspaceWorkflowCreate{ExpectedWorkspaceResourceVersion: value.GetExpectedWorkspaceResourceVersion()}
		if config != nil {
			command.CreateWorkflow.Name, command.CreateWorkflow.Description, command.CreateWorkflow.Prompt = config.GetName(), config.GetDescription(), config.GetPrompt()
		}
	case *pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowUpdate:
		value := operation.WorkflowUpdate
		if value == nil {
			break
		}
		command.UpdateWorkflow = &WorkspaceWorkflowUpdate{WorkflowID: value.GetWorkflowId(), ExpectedResourceVersion: value.GetExpectedResourceVersion(), Name: value.Name, Description: value.Description, Prompt: value.Prompt, AgentProfileID: value.AgentProfileId}
	case *pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowReorder:
		value := operation.WorkflowReorder
		if value == nil {
			break
		}
		command.ReorderWorkflows = &WorkspaceWorkflowReorder{ExpectedWorkspaceResourceVersion: value.GetExpectedWorkspaceResourceVersion(), WorkflowIDs: append([]string(nil), value.GetWorkflowIds()...), WorkflowResourceVersions: append([]string(nil), value.GetWorkflowResourceVersions()...)}
	case *pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowStepCreate:
		value := operation.WorkflowStepCreate
		if value == nil {
			break
		}
		command.CreateStep = &WorkspaceWorkflowStepCreate{WorkflowID: value.GetWorkflowId(), ExpectedWorkflowResourceVersion: value.GetExpectedWorkflowResourceVersion(), Configuration: workflowStepConfigurationFromProto(value.GetConfiguration())}
	case *pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowStepUpdate:
		value := operation.WorkflowStepUpdate
		if value == nil {
			break
		}
		command.UpdateStep = &WorkspaceWorkflowStepUpdate{WorkflowID: value.GetWorkflowId(), StepID: value.GetStepId(), ExpectedWorkflowResourceVersion: value.GetExpectedWorkflowResourceVersion(), ExpectedStepResourceVersion: value.GetExpectedStepResourceVersion(), Configuration: workflowStepConfigurationFromProto(value.GetConfiguration())}
	case *pluginv1.ApplyWorkspaceAdministrationExactRequest_WorkflowStepReorder:
		value := operation.WorkflowStepReorder
		if value == nil {
			break
		}
		command.ReorderSteps = &WorkspaceWorkflowStepReorder{WorkflowID: value.GetWorkflowId(), ExpectedWorkflowResourceVersion: value.GetExpectedWorkflowResourceVersion(), StepIDs: append([]string(nil), value.GetStepIds()...), StepResourceVersions: append([]string(nil), value.GetStepResourceVersions()...)}
	case *pluginv1.ApplyWorkspaceAdministrationExactRequest_RepositoryRegister:
		value := operation.RepositoryRegister
		if value == nil {
			break
		}
		command.RegisterRepo = &WorkspaceRepositoryRegister{ExpectedWorkspaceResourceVersion: value.GetExpectedWorkspaceResourceVersion(), Name: value.GetName(), SourceType: value.GetSourceType(), LocalPath: value.GetLocalPath(), Provider: value.GetProvider(), ProviderRepoID: value.GetProviderRepositoryId(), ProviderHost: value.GetProviderHost(), ProviderScope: value.GetProviderScope(), ProviderOwner: value.GetProviderOwner(), ProviderName: value.GetProviderName(), RemoteURL: value.GetRemoteUrl(), DefaultBranch: value.GetDefaultBranch(), WorktreeBranchPrefix: value.GetWorktreeBranchPrefix(), WorktreeBranchTemplate: value.GetWorktreeBranchTemplate(), PullBeforeWorktree: value.PullBeforeWorktree}
	case *pluginv1.ApplyWorkspaceAdministrationExactRequest_RepositoryUpdate:
		value := operation.RepositoryUpdate
		if value == nil {
			break
		}
		command.UpdateRepo = &WorkspaceRepositoryUpdate{RepositoryID: value.GetRepositoryId(), ExpectedResourceVersion: value.GetExpectedResourceVersion(), Name: value.Name, DefaultBranch: value.DefaultBranch, WorktreeBranchPrefix: value.WorktreeBranchPrefix, WorktreeBranchTemplate: value.WorktreeBranchTemplate, PullBeforeWorktree: value.PullBeforeWorktree}
	default:
		return WorkspaceAdminCommand{}, fmt.Errorf("workspace administration operation is required")
	}
	if workspaceAdminOperationCount(command) != 1 {
		return WorkspaceAdminCommand{}, fmt.Errorf("workspace administration operation is invalid")
	}
	return command, nil
}

func workspaceAdminOperationCount(command WorkspaceAdminCommand) int {
	count := 0
	for _, exists := range []bool{
		command.UpdateDefaults != nil, command.CreateWorkflow != nil, command.UpdateWorkflow != nil,
		command.ReorderWorkflows != nil, command.CreateStep != nil, command.UpdateStep != nil,
		command.ReorderSteps != nil, command.RegisterRepo != nil, command.UpdateRepo != nil,
	} {
		if exists {
			count++
		}
	}
	return count
}
