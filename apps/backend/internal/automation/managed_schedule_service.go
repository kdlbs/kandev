package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

func (s *Service) ListManagedConversationSchedules(ctx context.Context, installationID, workspaceID string) ([]pluginsdk.ManagedConversationSchedule, error) {
	if strings.TrimSpace(installationID) == "" || strings.TrimSpace(workspaceID) == "" {
		return nil, ErrManagedScheduleInvalid
	}
	items, err := s.store.ListManagedSchedules(ctx, installationID, workspaceID)
	if err != nil {
		return nil, err
	}
	result := make([]pluginsdk.ManagedConversationSchedule, 0, len(items))
	for _, item := range items {
		result = append(result, managedScheduleFromAutomation(item))
	}
	return result, nil
}

func (s *Service) CreateManagedConversationSchedule(ctx context.Context, installationID, pluginID string, schedule pluginsdk.ManagedConversationSchedule, operationID, payloadDigest string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	if strings.TrimSpace(installationID) == "" || strings.TrimSpace(pluginID) == "" || operationID == "" || payloadDigest == "" {
		return pluginsdk.ManagedConversationSchedule{}, false, ErrManagedScheduleInvalid
	}
	if err := validateManagedScheduleInput(schedule, true); err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	if schedule.ID == "" {
		return pluginsdk.ManagedConversationSchedule{}, false, ErrManagedScheduleInvalid
	}
	replayed, err := s.store.ManagedScheduleOperationApplied(ctx, operationID, payloadDigest, schedule.ID)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	if replayed {
		item, err := s.store.GetOwnedManagedSchedule(ctx, installationID, schedule.WorkspaceID, schedule.ID)
		if err != nil {
			return pluginsdk.ManagedConversationSchedule{}, true, err
		}
		return managedScheduleFromAutomation(item), true, nil
	}
	triggers, err := managedScheduleTriggerSpecs(schedule)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	request := managedScheduleCreateRequest(schedule, triggers)
	request.ID = schedule.ID
	created, err := s.createAutomation(ctx, request, installationID, operationID, payloadDigest)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	return managedScheduleFromAutomation(created), false, nil
}

func (s *Service) UpdateManagedConversationSchedule(ctx context.Context, installationID, pluginID, workspaceID, automationID string, expectedRevision uint64, schedule pluginsdk.ManagedConversationSchedule, operationID, payloadDigest string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	if err := validateManagedScheduleUpdateIdentity(installationID, pluginID, workspaceID, automationID, expectedRevision, operationID, payloadDigest); err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	schedule.ID = automationID
	schedule.WorkspaceID = workspaceID
	if err := validateManagedScheduleInput(schedule, false); err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	triggers, err := managedScheduleTriggerSpecs(schedule)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	replayed, err := s.store.ManagedScheduleOperationApplied(ctx, operationID, payloadDigest, automationID)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	if replayed {
		return s.readReplayedManagedSchedule(ctx, installationID, workspaceID, automationID)
	}
	return s.applyManagedScheduleUpdate(ctx, installationID, workspaceID, automationID, expectedRevision, schedule, triggers, operationID, payloadDigest)
}

func validateManagedScheduleUpdateIdentity(installationID, pluginID, workspaceID, automationID string, expectedRevision uint64, operationID, payloadDigest string) error {
	if strings.TrimSpace(installationID) == "" || strings.TrimSpace(pluginID) == "" ||
		strings.TrimSpace(workspaceID) == "" || automationID == "" || expectedRevision == 0 || operationID == "" || payloadDigest == "" {
		return ErrManagedScheduleInvalid
	}
	return nil
}

func (s *Service) readReplayedManagedSchedule(ctx context.Context, installationID, workspaceID, automationID string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	item, err := s.store.GetOwnedManagedSchedule(ctx, installationID, workspaceID, automationID)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, true, err
	}
	return managedScheduleFromAutomation(item), true, nil
}

func (s *Service) applyManagedScheduleUpdate(ctx context.Context, installationID, workspaceID, automationID string, expectedRevision uint64, schedule pluginsdk.ManagedConversationSchedule, triggers []CreateTriggerSpec, operationID, payloadDigest string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	current, err := s.store.GetOwnedManagedSchedule(ctx, installationID, workspaceID, automationID)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	if current.ResourceRevision != expectedRevision {
		return pluginsdk.ManagedConversationSchedule{}, false, ErrManagedScheduleRevisionConflict
	}
	request, err := s.managedScheduleAutomation(ctx, schedule, installationID)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	replayed, revision, err := s.store.UpdateManagedSchedule(ctx, installationID, workspaceID, automationID,
		expectedRevision, request, triggers, operationID, payloadDigest)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	updated, err := s.store.GetOwnedManagedSchedule(ctx, installationID, workspaceID, automationID)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, replayed, err
	}
	if revision != 0 {
		updated.ResourceRevision = revision
	}
	return managedScheduleFromAutomation(updated), replayed, nil
}

func (s *Service) SetManagedConversationScheduleEnabled(ctx context.Context, installationID, workspaceID, automationID string, expectedRevision uint64, enabled bool, operationID, payloadDigest string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	replayed, revision, err := s.store.SetManagedScheduleEnabled(ctx, installationID, workspaceID, automationID,
		expectedRevision, enabled, operationID, payloadDigest)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, false, err
	}
	item, err := s.store.GetOwnedManagedSchedule(ctx, installationID, workspaceID, automationID)
	if err != nil {
		return pluginsdk.ManagedConversationSchedule{}, replayed, err
	}
	if revision != 0 {
		item.ResourceRevision = revision
	}
	return managedScheduleFromAutomation(item), replayed, nil
}

func (s *Service) DeleteManagedConversationSchedule(ctx context.Context, installationID, workspaceID, automationID string, expectedRevision uint64, operationID, payloadDigest string) (bool, error) {
	return s.store.DeleteManagedSchedule(ctx, installationID, workspaceID, automationID,
		expectedRevision, operationID, payloadDigest)
}

func (s *Service) managedScheduleAutomation(ctx context.Context, schedule pluginsdk.ManagedConversationSchedule, ownerInstallationID string) (*Automation, error) {
	if s.managedDestinationResolver == nil {
		return nil, ErrManagedDestinationUnavailable
	}
	destination := &ManagedConversationDestination{PluginID: schedule.PluginID, InstanceKey: schedule.InstanceKey, Revision: schedule.DestinationRevision}
	installationID, conversationID, _, err := s.managedDestinationResolver.ResolveManagedConversationDestination(
		ctx, schedule.WorkspaceID, destination.PluginID, destination.InstanceKey, destination.Revision,
	)
	if err != nil || installationID == "" || conversationID == "" {
		return nil, errors.Join(ErrManagedDestinationUnavailable, err)
	}
	triggers, err := managedScheduleTriggerSpecs(schedule)
	if err != nil {
		return nil, err
	}
	return &Automation{
		ID: schedule.ID, WorkspaceID: schedule.WorkspaceID, Name: schedule.Name,
		Description: schedule.Description, Prompt: schedule.Prompt, TaskMode: TaskModeManagedConversation,
		ManagedOwnerInstallationID:       ownerInstallationID,
		ManagedDestination:               destination,
		ManagedDestinationInstallationID: installationID,
		ManagedDestinationConversationID: conversationID,
		RepositoryMode:                   RepositoryModeNone, Enabled: schedule.Enabled, MaxConcurrentRuns: 1,
		ContinuationPolicy: ContinuationPolicyNewTask, Triggers: managedScheduleModels(triggers),
	}, nil
}

func managedScheduleCreateRequest(schedule pluginsdk.ManagedConversationSchedule, triggers []CreateTriggerSpec) *CreateAutomationRequest {
	enabled := schedule.Enabled
	return &CreateAutomationRequest{
		ID: schedule.ID, WorkspaceID: schedule.WorkspaceID, Name: schedule.Name, Description: schedule.Description,
		Prompt: schedule.Prompt, TaskMode: TaskModeManagedConversation,
		ManagedDestination: &ManagedConversationDestination{PluginID: schedule.PluginID, InstanceKey: schedule.InstanceKey, Revision: schedule.DestinationRevision},
		RepositoryMode:     RepositoryModeNone, MaxConcurrentRuns: 1, Enabled: &enabled, Triggers: triggers,
	}
}

func managedScheduleTriggerSpecs(schedule pluginsdk.ManagedConversationSchedule) ([]CreateTriggerSpec, error) {
	if len(schedule.Triggers) != 1 || schedule.Triggers[0].Type != string(TriggerTypeScheduled) {
		return nil, fmt.Errorf("%w: exactly one scheduled trigger is required", ErrManagedScheduleInvalid)
	}
	trigger := schedule.Triggers[0]
	if len(trigger.Config) == 0 || !json.Valid(trigger.Config) {
		return nil, ErrManagedScheduleInvalid
	}
	if err := validateScheduledConfig(TriggerTypeScheduled, trigger.Config); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManagedScheduleInvalid, err)
	}
	return []CreateTriggerSpec{{Type: TriggerTypeScheduled, Config: append(json.RawMessage(nil), trigger.Config...), Enabled: trigger.Enabled}}, nil
}

func managedScheduleModels(triggers []CreateTriggerSpec) []AutomationTrigger {
	result := make([]AutomationTrigger, 0, len(triggers))
	for _, trigger := range triggers {
		result = append(result, AutomationTrigger{Type: trigger.Type, Config: append(json.RawMessage(nil), trigger.Config...), Enabled: trigger.Enabled})
	}
	return result
}

func validateManagedScheduleInput(schedule pluginsdk.ManagedConversationSchedule, creating bool) error {
	if schedule.WorkspaceID == "" || strings.TrimSpace(schedule.Name) == "" || len(schedule.Name) > 256 ||
		len(schedule.Description) > 4096 || len(schedule.Prompt) > 65536 || strings.TrimSpace(schedule.PluginID) == "" ||
		schedule.InstanceKey == "" || strings.TrimSpace(schedule.InstanceKey) != schedule.InstanceKey || len(schedule.InstanceKey) > 128 ||
		schedule.DestinationRevision == 0 || (!creating && schedule.ResourceRevision == 0) {
		return ErrManagedScheduleInvalid
	}
	return nil
}

func managedScheduleFromAutomation(item *Automation) pluginsdk.ManagedConversationSchedule {
	result := pluginsdk.ManagedConversationSchedule{
		ID: item.ID, WorkspaceID: item.WorkspaceID, Name: item.Name, Description: item.Description,
		Prompt: item.Prompt, Enabled: item.Enabled, ResourceRevision: item.ResourceRevision,
	}
	if item.ManagedDestination != nil {
		result.PluginID = item.ManagedDestination.PluginID
		result.InstanceKey = item.ManagedDestination.InstanceKey
		result.DestinationRevision = item.ManagedDestination.Revision
	}
	for _, trigger := range item.Triggers {
		result.Triggers = append(result.Triggers, pluginsdk.ManagedConversationScheduleTrigger{
			Type: string(trigger.Type), Config: append(json.RawMessage(nil), trigger.Config...), Enabled: trigger.Enabled,
		})
	}
	return result
}

var _ = json.Valid
