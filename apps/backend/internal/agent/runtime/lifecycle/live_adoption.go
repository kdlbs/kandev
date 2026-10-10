package lifecycle

import (
	"context"
	"fmt"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

const agentProcessStatusRunning = "running"

func (m *Manager) adoptExistingAgentSession(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
) error {
	if m.streamManager == nil || execution == nil || client == nil {
		return fmt.Errorf("existing agent recovery dependencies are unavailable")
	}
	association, err := client.GetAgentSessionAssociation(ctx)
	if err != nil {
		return fmt.Errorf("read live agent session association: %w", err)
	}
	if err := validateExistingAgentAssociation(execution, association); err != nil {
		return err
	}
	status, err := client.GetDeliveryStatus(ctx, execution.DeliveryStreamID)
	if err != nil {
		return fmt.Errorf("read existing agent delivery state: %w", err)
	}
	if err := validateExistingDeliveryAssociation(execution, association, status); err != nil {
		return err
	}
	recovered := &ExecutorInstance{
		InstanceID:        execution.ID,
		SessionID:         execution.SessionID,
		ProviderSessionID: association.NativeSessionID,
		Client:            client,
	}
	if err := m.restoreExistingAgentDelivery(ctx, execution, association, status, recovered); err != nil {
		return err
	}
	execution.ACPSessionID = association.NativeSessionID
	if err := m.attachExistingAgentStreams(ctx, execution, client); err != nil {
		return err
	}
	execution.setSessionInitialized(true)
	return nil
}

func validateExistingAgentAssociation(
	execution *AgentExecution,
	association *agentctl.AgentSessionAssociation,
) error {
	if association == nil || association.AgentStatus != agentProcessStatusRunning {
		return fmt.Errorf("existing agent is not running")
	}
	if err := validateExistingAgentIdentity(execution, association); err != nil {
		return err
	}
	return validateExistingAgentGeneration(execution, association)
}

func validateExistingAgentIdentity(
	execution *AgentExecution,
	association *agentctl.AgentSessionAssociation,
) error {
	if association.InstanceID == "" || association.InstanceID != execution.ID {
		return fmt.Errorf("existing agent instance identity does not match execution")
	}
	if association.SessionID == "" || association.SessionID != execution.SessionID {
		return fmt.Errorf("existing agent Kandev session does not match execution")
	}
	if association.NativeSessionID == "" {
		return fmt.Errorf("existing agent has no initialized native session")
	}
	if execution.ACPSessionID != "" && execution.ACPSessionID != association.NativeSessionID {
		return fmt.Errorf("existing native session does not match the recorded conversation")
	}
	return nil
}

func validateExistingAgentGeneration(
	execution *AgentExecution,
	association *agentctl.AgentSessionAssociation,
) error {
	if association.IncarnationID == "" || association.HarnessGeneration == 0 {
		return fmt.Errorf("existing agent does not expose a supported session generation")
	}
	if execution.DeliveryIncarnationID != "" && execution.DeliveryIncarnationID != association.IncarnationID {
		return fmt.Errorf("existing agent session incarnation does not match execution")
	}
	if execution.DeliveryHarnessGeneration > 0 && execution.DeliveryHarnessGeneration != association.HarnessGeneration {
		return fmt.Errorf("existing agent harness generation does not match execution")
	}
	return nil
}

func validateExistingDeliveryAssociation(
	execution *AgentExecution,
	association *agentctl.AgentSessionAssociation,
	status *agentctl.DeliveryStatus,
) error {
	if status == nil || status.SessionID != association.SessionID ||
		status.IncarnationID != association.IncarnationID || status.HarnessGeneration != association.HarnessGeneration {
		return fmt.Errorf("existing agent delivery owner does not match live session association")
	}
	if execution.DeliveryStreamID != "" && status.StreamID != execution.DeliveryStreamID {
		return fmt.Errorf("existing agent delivery stream does not match execution")
	}
	return nil
}

func (m *Manager) restoreExistingAgentDelivery(
	ctx context.Context,
	execution *AgentExecution,
	association *agentctl.AgentSessionAssociation,
	status *agentctl.DeliveryStatus,
	recovered *ExecutorInstance,
) error {
	if status.Durable && status.Version == journal.CurrentVersion {
		recovered.DeliveryStatus = status
		if err := m.restoreRecoveredDelivery(ctx, execution, recovered); err != nil {
			return fmt.Errorf("restore existing agent delivery state: %w", err)
		}
		return nil
	}
	return m.restoreLegacyAgentDelivery(ctx, execution, association)
}

func (m *Manager) restoreLegacyAgentDelivery(
	ctx context.Context,
	execution *AgentExecution,
	association *agentctl.AgentSessionAssociation,
) error {
	generation, err := loadRecoveredHarnessGeneration(
		ctx,
		m.streamManager.deliveryRepository(),
		association.SessionID,
		association.IncarnationID,
	)
	if err != nil {
		return fmt.Errorf("verify legacy agent session generation: %w", err)
	}
	if generation == nil || generation.Generation <= 0 || uint64(generation.Generation) != association.HarnessGeneration ||
		generation.NativeSessionID == "" || generation.NativeSessionID != association.NativeSessionID {
		return fmt.Errorf("legacy agent session identity does not match the current SQL generation")
	}
	if err := m.restoreLegacyDelivery(ctx, execution); err != nil {
		return fmt.Errorf("restore legacy agent delivery state: %w", err)
	}
	execution.DeliveryIncarnationID = association.IncarnationID
	execution.DeliveryHarnessGeneration = association.HarnessGeneration
	execution.DeliveryMode = DurableDeliveryLegacy
	return nil
}

func (m *Manager) attachExistingAgentStreams(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
) error {
	if client.HasAgentStream() {
		m.streamManager.ConnectWorkspaceStream(execution, nil)
	} else {
		updatesReady := make(chan struct{})
		m.streamManager.ConnectAll(execution, updatesReady)
		select {
		case <-updatesReady:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timed out attaching to existing agent stream")
		}
	}
	if err := m.streamManager.ReplayRecoveredDelivery(ctx, execution); err != nil {
		return fmt.Errorf("replay existing agent delivery tail: %w", err)
	}
	return nil
}
