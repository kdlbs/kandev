package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type managedConversationScheduleAdapter struct{ service *automation.Service }

type managedConversationAutomationDeliveryAdapter struct{ plugins *plugins.Service }

func (a managedConversationAutomationDeliveryAdapter) EnqueueManagedAutomationInput(ctx context.Context, schedule *automation.Automation, occurrenceID, payload string) (automation.ManagedAutomationInputReceipt, error) {
	if schedule == nil || schedule.ManagedDestination == nil || a.plugins == nil {
		return automation.ManagedAutomationInputReceipt{}, automation.ErrManagedDestinationUnavailable
	}
	receipt, paused, err := a.plugins.EnqueueManagedAutomationInput(ctx, schedule.WorkspaceID,
		schedule.ManagedDestination.PluginID, schedule.ManagedDestination.InstanceKey,
		schedule.ManagedDestinationInstallationID, schedule.ManagedDestinationConversationID,
		occurrenceID, payload)
	return automation.ManagedAutomationInputReceipt{InputID: receipt.HostInputID, State: string(receipt.State), Paused: paused}, err
}

func (a managedConversationAutomationDeliveryAdapter) ReadManagedAutomationInput(ctx context.Context, schedule *automation.Automation, inputID string) (automation.ManagedAutomationInputReceipt, error) {
	if schedule == nil || schedule.ManagedDestination == nil || a.plugins == nil {
		return automation.ManagedAutomationInputReceipt{}, automation.ErrManagedDestinationUnavailable
	}
	receipt, paused, err := a.plugins.ReadManagedAutomationInput(ctx, schedule.WorkspaceID,
		schedule.ManagedDestination.PluginID, schedule.ManagedDestination.InstanceKey,
		schedule.ManagedDestinationInstallationID, schedule.ManagedDestinationConversationID, inputID)
	return automation.ManagedAutomationInputReceipt{InputID: receipt.HostInputID, State: string(receipt.State), Paused: paused}, err
}

func (a managedConversationScheduleAdapter) ListManagedConversationSchedules(ctx context.Context, installationID, workspaceID string) ([]pluginsdk.ManagedConversationSchedule, error) {
	return a.service.ListManagedConversationSchedules(ctx, installationID, workspaceID)
}

func (a managedConversationScheduleAdapter) CreateManagedConversationSchedule(ctx context.Context, installationID, pluginID string, schedule pluginsdk.ManagedConversationSchedule, operationID, payloadDigest string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	result, replayed, err := a.service.CreateManagedConversationSchedule(ctx, installationID, pluginID, schedule, operationID, payloadDigest)
	return result, replayed, adaptManagedScheduleError(err)
}

func (a managedConversationScheduleAdapter) UpdateManagedConversationSchedule(ctx context.Context, installationID, pluginID, workspaceID, automationID string, expectedRevision uint64, schedule pluginsdk.ManagedConversationSchedule, operationID, payloadDigest string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	result, replayed, err := a.service.UpdateManagedConversationSchedule(ctx, installationID, pluginID, workspaceID, automationID, expectedRevision, schedule, operationID, payloadDigest)
	return result, replayed, adaptManagedScheduleError(err)
}

func (a managedConversationScheduleAdapter) SetManagedConversationScheduleEnabled(ctx context.Context, installationID, workspaceID, automationID string, expectedRevision uint64, enabled bool, operationID, payloadDigest string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	result, replayed, err := a.service.SetManagedConversationScheduleEnabled(ctx, installationID, workspaceID, automationID, expectedRevision, enabled, operationID, payloadDigest)
	return result, replayed, adaptManagedScheduleError(err)
}

func (a managedConversationScheduleAdapter) DeleteManagedConversationSchedule(ctx context.Context, installationID, workspaceID, automationID string, expectedRevision uint64, operationID, payloadDigest string) (bool, error) {
	replayed, err := a.service.DeleteManagedConversationSchedule(ctx, installationID, workspaceID, automationID, expectedRevision, operationID, payloadDigest)
	return replayed, adaptManagedScheduleError(err)
}

func adaptManagedScheduleError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, automation.ErrManagedScheduleNotFound):
		return plugins.ErrManagedScheduleNotFound
	case errors.Is(err, automation.ErrManagedScheduleRevisionConflict), errors.Is(err, automation.ErrManagedScheduleIdempotencyConflict):
		return plugins.ErrManagedScheduleRevisionConflict
	case errors.Is(err, automation.ErrManagedScheduleInvalid):
		return plugins.ErrManagedScheduleInvalid
	default:
		return err
	}
}
