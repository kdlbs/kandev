package pluginsdk

import (
	"context"
	"encoding/json"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ManagedConversationScheduleTrigger struct {
	Type    string
	Config  json.RawMessage
	Enabled bool
}

type ManagedConversationSchedule struct {
	ID                  string
	WorkspaceID         string
	Name                string
	Description         string
	Prompt              string
	PluginID            string
	InstanceKey         string
	DestinationRevision uint64
	ResourceRevision    uint64
	Enabled             bool
	Triggers            []ManagedConversationScheduleTrigger
}

type ManagedConversationScheduleQuery struct {
	WorkspaceID      string
	ApprovalRevision uint64
	ManifestDigest   string
}

type ManagedConversationScheduleCreate struct {
	RequestID        string
	IdempotencyKey   string
	ApprovalRevision uint64
	ManifestDigest   string
	Schedule         ManagedConversationSchedule
}

type ManagedConversationScheduleUpdate struct {
	RequestID                string
	IdempotencyKey           string
	AutomationID             string
	ExpectedResourceRevision uint64
	ApprovalRevision         uint64
	ManifestDigest           string
	Schedule                 ManagedConversationSchedule
}

type ManagedConversationScheduleEnabled struct {
	RequestID                string
	IdempotencyKey           string
	WorkspaceID              string
	AutomationID             string
	ExpectedResourceRevision uint64
	Enabled                  bool
	ApprovalRevision         uint64
	ManifestDigest           string
}

type ManagedConversationScheduleDelete struct {
	RequestID                string
	IdempotencyKey           string
	WorkspaceID              string
	AutomationID             string
	ExpectedResourceRevision uint64
	ApprovalRevision         uint64
	ManifestDigest           string
}

type ManagedConversationScheduleManager interface {
	List(context.Context, ManagedConversationScheduleQuery) ([]ManagedConversationSchedule, error)
	Create(context.Context, ManagedConversationScheduleCreate) (*CommandResult, ManagedConversationSchedule, error)
	Update(context.Context, ManagedConversationScheduleUpdate) (*CommandResult, ManagedConversationSchedule, error)
	SetEnabled(context.Context, ManagedConversationScheduleEnabled) (*CommandResult, ManagedConversationSchedule, error)
	Delete(context.Context, ManagedConversationScheduleDelete) (*CommandResult, error)
}

type ManagedConversationScheduleHost interface {
	ManagedConversationSchedules() ManagedConversationScheduleManager
}

func HostManagedConversationSchedules(host Host) (ManagedConversationScheduleManager, bool) {
	extended, ok := host.(ManagedConversationScheduleHost)
	if !ok || extended.ManagedConversationSchedules() == nil {
		return nil, false
	}
	return extended.ManagedConversationSchedules(), true
}

type grpcManagedConversationScheduleManager struct{ client pluginv1.HostClient }

func (h *grpcHostClient) ManagedConversationSchedules() ManagedConversationScheduleManager {
	return grpcManagedConversationScheduleManager{client: h.client}
}

func (m grpcManagedConversationScheduleManager) List(ctx context.Context, query ManagedConversationScheduleQuery) ([]ManagedConversationSchedule, error) {
	response, err := m.client.ListManagedConversationSchedulesExact(ctx, &pluginv1.ListManagedConversationSchedulesExactRequest{
		WorkspaceId: query.WorkspaceID, ApprovalRevision: query.ApprovalRevision, ManifestDigest: query.ManifestDigest,
	})
	if err != nil {
		return nil, err
	}
	result := make([]ManagedConversationSchedule, 0, len(response.GetSchedules()))
	for _, schedule := range response.GetSchedules() {
		result = append(result, managedConversationScheduleFromProto(schedule))
	}
	return result, nil
}

func (m grpcManagedConversationScheduleManager) Create(ctx context.Context, input ManagedConversationScheduleCreate) (*CommandResult, ManagedConversationSchedule, error) {
	response, err := m.client.CreateManagedConversationScheduleExact(ctx, &pluginv1.CreateManagedConversationScheduleExactRequest{
		RequestId: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
		Schedule: managedConversationScheduleToProto(input.Schedule),
	})
	if err != nil {
		return nil, ManagedConversationSchedule{}, err
	}
	return commandResultFromProto(response.GetResult()), managedConversationScheduleFromProto(response.GetSchedule()), nil
}

func (m grpcManagedConversationScheduleManager) Update(ctx context.Context, input ManagedConversationScheduleUpdate) (*CommandResult, ManagedConversationSchedule, error) {
	response, err := m.client.UpdateManagedConversationScheduleExact(ctx, &pluginv1.UpdateManagedConversationScheduleExactRequest{
		RequestId: input.RequestID, IdempotencyKey: input.IdempotencyKey, AutomationId: input.AutomationID,
		ExpectedResourceRevision: input.ExpectedResourceRevision, ApprovalRevision: input.ApprovalRevision,
		ManifestDigest: input.ManifestDigest, Schedule: managedConversationScheduleToProto(input.Schedule),
	})
	if err != nil {
		return nil, ManagedConversationSchedule{}, err
	}
	return commandResultFromProto(response.GetResult()), managedConversationScheduleFromProto(response.GetSchedule()), nil
}

func (m grpcManagedConversationScheduleManager) SetEnabled(ctx context.Context, input ManagedConversationScheduleEnabled) (*CommandResult, ManagedConversationSchedule, error) {
	response, err := m.client.SetManagedConversationScheduleEnabledExact(ctx, &pluginv1.SetManagedConversationScheduleEnabledExactRequest{
		RequestId: input.RequestID, IdempotencyKey: input.IdempotencyKey, WorkspaceId: input.WorkspaceID,
		AutomationId: input.AutomationID, ExpectedResourceRevision: input.ExpectedResourceRevision,
		Enabled: input.Enabled, ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, ManagedConversationSchedule{}, err
	}
	return commandResultFromProto(response.GetResult()), managedConversationScheduleFromProto(response.GetSchedule()), nil
}

func (m grpcManagedConversationScheduleManager) Delete(ctx context.Context, input ManagedConversationScheduleDelete) (*CommandResult, error) {
	response, err := m.client.DeleteManagedConversationScheduleExact(ctx, &pluginv1.DeleteManagedConversationScheduleExactRequest{
		RequestId: input.RequestID, IdempotencyKey: input.IdempotencyKey, WorkspaceId: input.WorkspaceID,
		AutomationId: input.AutomationID, ExpectedResourceRevision: input.ExpectedResourceRevision,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, err
	}
	return commandResultFromProto(response), nil
}

func managedConversationScheduleToProto(value ManagedConversationSchedule) *pluginv1.ManagedConversationSchedule {
	result := &pluginv1.ManagedConversationSchedule{
		Id: value.ID, WorkspaceId: value.WorkspaceID, Name: value.Name, Description: value.Description,
		Prompt: value.Prompt, PluginId: value.PluginID, InstanceKey: value.InstanceKey,
		DestinationRevision: value.DestinationRevision, ResourceRevision: value.ResourceRevision, Enabled: value.Enabled,
	}
	for _, trigger := range value.Triggers {
		result.Triggers = append(result.Triggers, &pluginv1.ManagedConversationScheduleTrigger{
			Type: trigger.Type, ConfigJson: string(trigger.Config), Enabled: trigger.Enabled,
		})
	}
	return result
}

func managedConversationScheduleFromProto(value *pluginv1.ManagedConversationSchedule) ManagedConversationSchedule {
	if value == nil {
		return ManagedConversationSchedule{}
	}
	result := ManagedConversationSchedule{
		ID: value.GetId(), WorkspaceID: value.GetWorkspaceId(), Name: value.GetName(), Description: value.GetDescription(),
		Prompt: value.GetPrompt(), PluginID: value.GetPluginId(), InstanceKey: value.GetInstanceKey(),
		DestinationRevision: value.GetDestinationRevision(), ResourceRevision: value.GetResourceRevision(), Enabled: value.GetEnabled(),
	}
	for _, trigger := range value.GetTriggers() {
		result.Triggers = append(result.Triggers, ManagedConversationScheduleTrigger{
			Type: trigger.GetType(), Config: json.RawMessage(trigger.GetConfigJson()), Enabled: trigger.GetEnabled(),
		})
	}
	return result
}

func (s *grpcHostServer) managedConversationScheduleManager() (ManagedConversationScheduleManager, error) {
	manager, ok := HostManagedConversationSchedules(s.exact)
	if !ok {
		return nil, status.Error(codes.Unavailable, "managed conversation schedules are unavailable")
	}
	return manager, nil
}

func (s *grpcHostServer) ListManagedConversationSchedulesExact(ctx context.Context, request *pluginv1.ListManagedConversationSchedulesExactRequest) (*pluginv1.ListManagedConversationSchedulesExactResponse, error) {
	manager, err := s.managedConversationScheduleManager()
	if err != nil {
		return nil, err
	}
	schedules, err := manager.List(ctx, ManagedConversationScheduleQuery{
		WorkspaceID: request.GetWorkspaceId(), ApprovalRevision: request.GetApprovalRevision(), ManifestDigest: request.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	response := &pluginv1.ListManagedConversationSchedulesExactResponse{Schedules: make([]*pluginv1.ManagedConversationSchedule, 0, len(schedules))}
	for _, schedule := range schedules {
		response.Schedules = append(response.Schedules, managedConversationScheduleToProto(schedule))
	}
	return response, nil
}

func (s *grpcHostServer) CreateManagedConversationScheduleExact(ctx context.Context, request *pluginv1.CreateManagedConversationScheduleExactRequest) (*pluginv1.ManagedConversationScheduleExactResponse, error) {
	manager, err := s.managedConversationScheduleManager()
	if err != nil {
		return nil, err
	}
	result, schedule, err := manager.Create(ctx, ManagedConversationScheduleCreate{
		RequestID: request.GetRequestId(), IdempotencyKey: request.GetIdempotencyKey(),
		ApprovalRevision: request.GetApprovalRevision(), ManifestDigest: request.GetManifestDigest(),
		Schedule: managedConversationScheduleFromProto(request.GetSchedule()),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.ManagedConversationScheduleExactResponse{Result: commandResultToProto(result), Schedule: managedConversationScheduleToProto(schedule)}, nil
}

func (s *grpcHostServer) UpdateManagedConversationScheduleExact(ctx context.Context, request *pluginv1.UpdateManagedConversationScheduleExactRequest) (*pluginv1.ManagedConversationScheduleExactResponse, error) {
	manager, err := s.managedConversationScheduleManager()
	if err != nil {
		return nil, err
	}
	result, schedule, err := manager.Update(ctx, ManagedConversationScheduleUpdate{
		RequestID: request.GetRequestId(), IdempotencyKey: request.GetIdempotencyKey(),
		AutomationID: request.GetAutomationId(), ExpectedResourceRevision: request.GetExpectedResourceRevision(),
		ApprovalRevision: request.GetApprovalRevision(), ManifestDigest: request.GetManifestDigest(),
		Schedule: managedConversationScheduleFromProto(request.GetSchedule()),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.ManagedConversationScheduleExactResponse{Result: commandResultToProto(result), Schedule: managedConversationScheduleToProto(schedule)}, nil
}

func (s *grpcHostServer) SetManagedConversationScheduleEnabledExact(ctx context.Context, request *pluginv1.SetManagedConversationScheduleEnabledExactRequest) (*pluginv1.ManagedConversationScheduleExactResponse, error) {
	manager, err := s.managedConversationScheduleManager()
	if err != nil {
		return nil, err
	}
	result, schedule, err := manager.SetEnabled(ctx, ManagedConversationScheduleEnabled{
		RequestID: request.GetRequestId(), IdempotencyKey: request.GetIdempotencyKey(),
		WorkspaceID: request.GetWorkspaceId(), AutomationID: request.GetAutomationId(),
		ExpectedResourceRevision: request.GetExpectedResourceRevision(), Enabled: request.GetEnabled(),
		ApprovalRevision: request.GetApprovalRevision(), ManifestDigest: request.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.ManagedConversationScheduleExactResponse{Result: commandResultToProto(result), Schedule: managedConversationScheduleToProto(schedule)}, nil
}

func (s *grpcHostServer) DeleteManagedConversationScheduleExact(ctx context.Context, request *pluginv1.DeleteManagedConversationScheduleExactRequest) (*pluginv1.HostCommandResult, error) {
	manager, err := s.managedConversationScheduleManager()
	if err != nil {
		return nil, err
	}
	result, err := manager.Delete(ctx, ManagedConversationScheduleDelete{
		RequestID: request.GetRequestId(), IdempotencyKey: request.GetIdempotencyKey(),
		WorkspaceID: request.GetWorkspaceId(), AutomationID: request.GetAutomationId(),
		ExpectedResourceRevision: request.GetExpectedResourceRevision(),
		ApprovalRevision:         request.GetApprovalRevision(), ManifestDigest: request.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return commandResultToProto(result), nil
}
