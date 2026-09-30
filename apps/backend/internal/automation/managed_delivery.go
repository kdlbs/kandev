package automation

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

const managedAutomationDeliveryAttemptLimit = 3

// DispatchManagedAutomationRun delivers one admitted occurrence. The run ID
// is the stable input occurrence key, so recovery cannot enqueue a duplicate.
func (s *Service) DispatchManagedAutomationRun(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil || (run.Status != RunStatusTriggered && run.Status != RunStatusTaskCreated) {
		return nil
	}
	if run.ManagedInputID != "" {
		return s.observeManagedAutomationRun(ctx, run)
	}
	a, err := s.store.GetAutomation(ctx, run.AutomationID)
	if err != nil {
		return err
	}
	if a == nil || a.TaskMode != TaskModeManagedConversation {
		return nil
	}
	return s.dispatchManagedAutomationRun(ctx, run, a)
}

func (s *Service) dispatchManagedAutomationRun(ctx context.Context, run *AutomationRun, a *Automation) error {
	if !a.Enabled {
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, "", ManagedDeliveryFailed,
			run.DeliveryAttempts, "schedule was disabled before delivery")
	}
	target, ok := managedAutomationForRun(a, run)
	if !ok {
		if run.ManagedInputID != "" {
			return s.store.UpdateManagedRunObservation(ctx, run.ID, run.ManagedInputID, ManagedDeliveryUnavailable,
				"admitted managed destination snapshot is unavailable")
		}
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, "", ManagedDeliveryFailed,
			run.DeliveryAttempts, "admitted managed destination snapshot is unavailable; input was not sent")
	}
	if run.DeliveryAttempts >= managedAutomationDeliveryAttemptLimit {
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, "", ManagedDeliveryUnavailable,
			run.DeliveryAttempts, "managed destination delivery retry limit reached")
	}
	if s.managedAutomationDelivery == nil {
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, "", ManagedDeliveryUnavailable,
			run.DeliveryAttempts+1, "managed conversation delivery is unavailable")
	}
	prompt := InterpolateAgentPrompt(a.Prompt, run.TriggerType, run.TriggerData)
	if prompt == "" {
		prompt = fmt.Sprintf("Automation '%s' triggered by %s", a.Name, run.TriggerType)
	}
	receipt, deliveryErr := s.managedAutomationDelivery.EnqueueManagedAutomationInput(ctx, target, run.ID, prompt)
	if deliveryErr != nil {
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, "", ManagedDeliveryUnavailable,
			run.DeliveryAttempts+1, safeManagedDeliveryError(deliveryErr))
	}
	if receipt.InputID == "" {
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, "", ManagedDeliveryUnavailable,
			run.DeliveryAttempts+1, "managed conversation returned an empty input receipt")
	}
	state := managedDeliveryStatus(receipt.State, receipt.Paused)
	return s.store.UpdateManagedRunDelivery(ctx, run.ID, receipt.InputID, state,
		run.DeliveryAttempts+1, "")
}

// ReconcileManagedConversationDeliveries observes durable input receipts and
// retries only occurrences without a receipt, preserving the run-derived key.
func (s *Service) ReconcileManagedConversationDeliveries(ctx context.Context) error {
	runs, err := s.store.ListAllOpenRuns(ctx)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run == nil {
			continue
		}
		isManaged, err := s.store.IsManagedConversationAutomation(ctx, run.AutomationID)
		if err != nil {
			return err
		}
		if !isManaged {
			continue
		}
		if err := s.DispatchManagedAutomationRun(ctx, run.ID); err != nil {
			s.logger.Warn("managed automation delivery reconciliation failed", zap.Error(err), zap.String("run_id", run.ID))
		}
	}
	return nil
}

func (s *Service) observeManagedAutomationRun(ctx context.Context, run *AutomationRun) error {
	a, err := s.store.GetAutomation(ctx, run.AutomationID)
	if err != nil {
		return err
	}
	if a == nil || a.TaskMode != TaskModeManagedConversation || s.managedAutomationDelivery == nil {
		return s.store.UpdateManagedRunObservation(ctx, run.ID, run.ManagedInputID, ManagedDeliveryUnavailable,
			"managed conversation receipt is unavailable")
	}
	target, ok := managedAutomationForRun(a, run)
	if !ok {
		return s.store.UpdateManagedRunObservation(ctx, run.ID, run.ManagedInputID, ManagedDeliveryUnavailable,
			"admitted managed destination snapshot is unavailable")
	}
	receipt, readErr := s.managedAutomationDelivery.ReadManagedAutomationInput(ctx, target, run.ManagedInputID)
	if readErr != nil {
		return s.store.UpdateManagedRunObservation(ctx, run.ID, run.ManagedInputID, ManagedDeliveryUnavailable,
			safeManagedDeliveryError(readErr))
	}
	if receipt.InputID != run.ManagedInputID {
		return s.store.UpdateManagedRunObservation(ctx, run.ID, run.ManagedInputID, ManagedDeliveryUnavailable,
			"managed conversation receipt identity changed")
	}
	state := managedDeliveryStatus(receipt.State, receipt.Paused)
	return s.store.UpdateManagedRunObservation(ctx, run.ID, run.ManagedInputID, state, "")
}

func snapshotManagedAutomationDestination(a *Automation, run *AutomationRun) {
	if a == nil || run == nil || a.TaskMode != TaskModeManagedConversation || a.ManagedDestination == nil {
		return
	}
	run.DeliveryStatus = ManagedDeliveryPending
	run.ManagedDestinationInstallationID = a.ManagedDestinationInstallationID
	run.ManagedDestinationPluginID = a.ManagedDestination.PluginID
	run.ManagedDestinationInstanceKey = a.ManagedDestination.InstanceKey
	run.ManagedDestinationRevision = a.ManagedDestination.Revision
	run.ManagedConversationID = a.ManagedDestinationConversationID
}

func managedAutomationForRun(a *Automation, run *AutomationRun) (*Automation, bool) {
	if a == nil || run == nil || run.ManagedDestinationInstallationID == "" ||
		run.ManagedDestinationPluginID == "" || run.ManagedDestinationInstanceKey == "" || run.ManagedConversationID == "" {
		return nil, false
	}
	target := *a
	target.ManagedDestinationInstallationID = run.ManagedDestinationInstallationID
	target.ManagedDestinationPluginID = run.ManagedDestinationPluginID
	target.ManagedDestinationInstanceKey = run.ManagedDestinationInstanceKey
	target.ManagedDestinationConversationID = run.ManagedConversationID
	target.ManagedDestinationRevision = run.ManagedDestinationRevision
	target.ManagedDestination = &ManagedConversationDestination{
		PluginID: run.ManagedDestinationPluginID, InstanceKey: run.ManagedDestinationInstanceKey,
		Revision: run.ManagedDestinationRevision,
	}
	return &target, true
}

func managedDeliveryStatus(state string, paused bool) ManagedDeliveryStatus {
	if paused && state == string(pluginsdk.ManagedAgentInputAccepted) {
		return ManagedDeliveryPaused
	}
	switch pluginsdk.ManagedAgentInputState(state) {
	case pluginsdk.ManagedAgentInputAccepted:
		return ManagedDeliveryAccepted
	case pluginsdk.ManagedAgentInputRunning:
		return ManagedDeliveryRunning
	case pluginsdk.ManagedAgentInputCompleted:
		return ManagedDeliveryCompleted
	case pluginsdk.ManagedAgentInputFailed, pluginsdk.ManagedAgentInputCancelled:
		return ManagedDeliveryFailed
	case pluginsdk.ManagedAgentInputUncertain:
		return ManagedDeliveryUncertain
	default:
		return ManagedDeliveryUnavailable
	}
}

func safeManagedDeliveryError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "managed conversation delivery failed"
	}
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}
