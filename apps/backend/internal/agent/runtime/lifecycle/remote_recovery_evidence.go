package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentruntime"
)

func recordedAgentctlInstanceID(runtime agentruntime.Runtime, backendExecutionID string, metadata map[string]interface{}) (string, error) {
	switch runtime {
	case agentruntime.RuntimeSSH:
		if id := strings.TrimSpace(getMetadataString(metadata, MetadataKeySSHAgentctlInstanceID)); id != "" {
			return id, nil
		}
		return "", errors.New("ssh agentctl instance identity is missing")
	case agentruntime.RuntimeKubernetes:
		if id := strings.TrimSpace(getMetadataString(metadata, MetadataKeyKubernetesAgentctlInstanceID)); id != "" {
			return id, nil
		}
		return "", errors.New("kubernetes agentctl instance identity is missing")
	default:
		if strings.TrimSpace(backendExecutionID) == "" {
			return "", errors.New("backend execution identity is missing")
		}
		return backendExecutionID, nil
	}
}

// captureAuthenticatedAgentSessionEvidence reads authenticated GET surfaces
// to prove the exact live native session and delivery owner. It never
// initializes an ACP session or loads a prompt.
func (m *Manager) captureAuthenticatedAgentSessionEvidence(ctx context.Context, instance *ExecutorInstance) error {
	if instance == nil || instance.Client == nil || instance.SessionID == "" || instance.InstanceID == "" {
		return errors.New("remote recovery client identity is incomplete")
	}
	expectedID, err := recordedAgentctlInstanceID(instance.RuntimeName, instance.InstanceID, instance.Metadata)
	if err != nil {
		return err
	}
	association, err := instance.Client.GetAgentSessionAssociation(ctx)
	if err != nil {
		return fmt.Errorf("read authenticated remote session association: %w", err)
	}
	if err := validateRemoteSessionAssociation(instance, association, expectedID); err != nil {
		return err
	}
	instance.ProviderSessionID = association.NativeSessionID
	instance.agentSessionAssociation = association
	return m.captureRemoteDeliveryEvidence(ctx, instance, association)
}

func validateRemoteSessionAssociation(
	instance *ExecutorInstance,
	association *agentctl.AgentSessionAssociation,
	expectedID string,
) error {
	if association == nil || association.AgentStatus != agentProcessStatusRunning ||
		association.InstanceID != expectedID || association.SessionID != instance.SessionID ||
		association.NativeSessionID == "" || association.IncarnationID == "" || association.HarnessGeneration == 0 {
		return errors.New("authenticated remote session association does not match persisted owner")
	}
	if instance.ProviderSessionID != "" && instance.ProviderSessionID != association.NativeSessionID {
		return errors.New("authenticated remote native session changed")
	}
	return nil
}

func (m *Manager) captureRemoteDeliveryEvidence(
	ctx context.Context,
	instance *ExecutorInstance,
	association *agentctl.AgentSessionAssociation,
) error {
	status, err := instance.Client.GetDeliveryStatus(ctx, "")
	if err != nil {
		if isUnsupportedRemoteDeliveryRoute(err) {
			return m.acceptLegacyRemoteDelivery(ctx, instance, association)
		}
		return fmt.Errorf("read authenticated remote delivery owner: %w", err)
	}
	if err := validateRemoteDeliveryOwner(status, association); err != nil {
		return err
	}
	if instance.expectedDeliveryStreamID != "" && status.StreamID != instance.expectedDeliveryStreamID {
		return errors.New("authenticated remote delivery stream changed")
	}
	if status.Durable {
		if status.Version != journal.CurrentVersion || status.StreamID == "" {
			return errors.New("remote agent advertises an unsupported durable delivery stream")
		}
		instance.DeliveryStatus = status
		return nil
	}
	if status.Version != journal.CurrentVersion || status.Reason != "storage_not_durable" {
		return fmt.Errorf("remote retained delivery is unavailable: %s", status.Reason)
	}
	return m.acceptLegacyRemoteDelivery(ctx, instance, association)
}

func validateRemoteDeliveryOwner(status *agentctl.DeliveryStatus, association *agentctl.AgentSessionAssociation) error {
	if status == nil || status.SessionID != association.SessionID || status.IncarnationID != association.IncarnationID ||
		status.HarnessGeneration != association.HarnessGeneration {
		return errors.New("authenticated remote delivery owner does not match session association")
	}
	return nil
}

func isUnsupportedRemoteDeliveryRoute(err error) bool {
	var httpErr *agentctl.DeliveryHTTPError
	return errors.As(err, &httpErr) &&
		(httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusMethodNotAllowed)
}

func (m *Manager) acceptLegacyRemoteDelivery(
	ctx context.Context,
	instance *ExecutorInstance,
	association *agentctl.AgentSessionAssociation,
) error {
	instance.DeliveryLegacyEvidence = true
	return m.validateLegacyRemoteGeneration(ctx, instance, association)
}

func (m *Manager) validateLegacyRemoteGeneration(
	ctx context.Context,
	instance *ExecutorInstance,
	association *agentctl.AgentSessionAssociation,
) error {
	if m.streamManager == nil {
		return errors.New("legacy remote recovery requires the backend delivery repository")
	}
	generation, err := loadRecoveredHarnessGeneration(
		ctx,
		m.streamManager.deliveryRepository(),
		association.SessionID,
		association.IncarnationID,
	)
	if err != nil {
		return fmt.Errorf("verify legacy remote harness generation: %w", err)
	}
	if generation == nil || generation.Generation <= 0 || uint64(generation.Generation) != association.HarnessGeneration ||
		generation.NativeSessionID == "" || generation.NativeSessionID != association.NativeSessionID {
		return errors.New("legacy remote agent session does not match current SQL generation")
	}
	// Legacy recovery records have no stream descriptor to carry these pins.
	// The association is retained on the recovery instance and applied to the
	// reconstructed execution after this SQL-generation check succeeds.
	instance.ProviderSessionID = association.NativeSessionID
	return nil
}
