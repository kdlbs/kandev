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
	if run.DeliveryAttempts >= managedAutomationDeliveryAttemptLimit {
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, "", ManagedDeliveryUnavailable,
			run.DeliveryAttempts, "managed destination delivery retry limit reached")
	}
	if s.managedAutomationDelivery == nil || a.ManagedDestination == nil || a.ManagedDestinationInstallationID == "" || a.ManagedDestinationConversationID == "" {
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, "", ManagedDeliveryUnavailable,
			run.DeliveryAttempts+1, "managed conversation delivery is unavailable")
	}
	prompt := InterpolateAgentPrompt(a.Prompt, run.TriggerType, run.TriggerData)
	if prompt == "" {
		prompt = fmt.Sprintf("Automation '%s' triggered by %s", a.Name, run.TriggerType)
	}
	receipt, deliveryErr := s.managedAutomationDelivery.EnqueueManagedAutomationInput(ctx, a, run.ID, prompt)
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
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, run.ManagedInputID, ManagedDeliveryUnavailable,
			run.DeliveryAttempts+1, "managed conversation receipt is unavailable")
	}
	receipt, readErr := s.managedAutomationDelivery.ReadManagedAutomationInput(ctx, a, run.ManagedInputID)
	if readErr != nil {
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, run.ManagedInputID, ManagedDeliveryUnavailable,
			run.DeliveryAttempts+1, safeManagedDeliveryError(readErr))
	}
	if receipt.InputID != run.ManagedInputID {
		return s.store.UpdateManagedRunDelivery(ctx, run.ID, run.ManagedInputID, ManagedDeliveryUnavailable,
			run.DeliveryAttempts+1, "managed conversation receipt identity changed")
	}
	state := managedDeliveryStatus(receipt.State, receipt.Paused)
	return s.store.UpdateManagedRunDelivery(ctx, run.ID, run.ManagedInputID, state,
		run.DeliveryAttempts+1, "")
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
