package lifecycle

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const pluginExecutorExpiryCheckInterval = time.Minute

// pluginExecutorEarlierExpiry preserves the earliest deadline observed for a
// resource. Reattachment and inspection cannot renew provider retention.
func pluginExecutorEarlierExpiry(current, reported string) (string, error) {
	if reported == "" {
		return current, nil
	}
	reportedAt, err := time.Parse(time.RFC3339Nano, reported)
	if err != nil {
		return "", errors.New("provider returned an invalid executor expiry")
	}
	if current == "" {
		return reported, nil
	}
	currentAt, err := time.Parse(time.RFC3339Nano, current)
	if err != nil {
		return "", errors.New("recorded executor expiry is invalid")
	}
	if reportedAt.Before(currentAt) {
		return reported, nil
	}
	return current, nil
}

func (r *PluginRemoteExecutor) GetEnvironmentStatus(ctx context.Context, record *models.ExecutorRunning) (*models.PluginExecutorEnvironmentStatus, error) {
	if record == nil {
		return nil, nil
	}
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		return nil, err
	}
	status := pluginExecutorStatusFromInventory(inventory, time.Now())
	if !pluginExecutorExpiryInspectionNeeded(status, inventory) {
		return status, nil
	}
	return r.inspectPluginExecutorExpiry(ctx, record, inventory)
}

func pluginExecutorExpiryInspectionNeeded(status *models.PluginExecutorEnvironmentStatus, inventory pluginExecutorInventory) bool {
	if status.State == pluginExecutorPhaseExpired || status.State == pluginExecutorPhaseCleanupPending ||
		status.ExpiresAt == "" || !status.DeadlinePassed {
		return false
	}
	if inventory.Phase != pluginExecutorPhaseReady && inventory.Phase != pluginExecutorPhaseProvisioned {
		return false
	}
	return pluginExecutorExpiryCheckDue(inventory.ExpiryCheckAt, time.Now())
}

func (r *PluginRemoteExecutor) inspectPluginExecutorExpiry(ctx context.Context, record *models.ExecutorRunning, inventory pluginExecutorInventory) (*models.PluginExecutorEnvironmentStatus, error) {
	if r.operations == nil || r.profileLoader == nil || inventory.Resource == nil {
		return r.pluginExecutorUnavailableStatus(ctx, record, inventory)
	}
	profile, err := r.profileLoader.ExecutorProviderProfileForRecovery(
		ctx, inventory.ProfileID, record.TaskID, inventory.EnvironmentID, inventory.ProviderIdentity,
		inventory.EnvironmentGeneration, inventory.ProfileConfig, inventory.SecretReferences,
	)
	if err != nil || profile == nil || profile.Provider.Identity != inventory.ProviderIdentity || profile.Provider.InstallationID != inventory.InstallationID {
		return r.pluginExecutorUnavailableStatus(ctx, record, inventory)
	}
	operationContext := pluginExecutorRequestContextForRecord(record, inventory, *profile)
	for attempt := 0; attempt < 2; attempt++ {
		inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		inspection, inspectErr := r.operations.InspectExecutorEnvironment(inspectCtx, &pluginsdk.InspectExecutorEnvironmentRequest{
			Context: operationContext, Resource: inventory.Resource,
		})
		cancel()
		if !pluginExecutorInspectionUsable(inspection, inspectErr) {
			continue
		}
		if err := r.applyPluginExecutorInspection(ctx, record, &inventory, inspection); err != nil {
			return nil, err
		}
		return pluginExecutorStatusFromInventory(inventory, time.Now()), nil
	}
	inventory.LastInspectionState = pluginExecutorStateUnknown
	inventory.ExpiryCheckAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := r.checkpointPluginExecutorRecord(ctx, record, inventory); err != nil {
		return nil, err
	}
	status := pluginExecutorStatusFromInventory(inventory, time.Now())
	status.State = pluginExecutorStateUnknown
	status.Reason = "inspection_unknown"
	return status, nil
}

func (r *PluginRemoteExecutor) pluginExecutorUnavailableStatus(ctx context.Context, record *models.ExecutorRunning, inventory pluginExecutorInventory) (*models.PluginExecutorEnvironmentStatus, error) {
	inventory.LastInspectionState = pluginExecutorStateUnknown
	inventory.ExpiryCheckAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := r.checkpointPluginExecutorRecord(ctx, record, inventory); err != nil {
		return nil, err
	}
	status := pluginExecutorStatusFromInventory(inventory, time.Now())
	status.State = pluginExecutorStateUnavailable
	status.Reason = "provider_unavailable"
	return status, nil
}

func pluginExecutorInspectionUsable(inspection *pluginsdk.InspectExecutorEnvironmentResponse, err error) bool {
	return err == nil && inspection != nil && inspection.GetError() == nil &&
		!strings.EqualFold(strings.TrimSpace(inspection.GetState()), pluginExecutorStateUnknown)
}

func pluginExecutorExpiryCheckDue(lastCheck string, now time.Time) bool {
	if lastCheck == "" {
		return true
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, lastCheck)
	return err != nil || !now.Before(checkedAt.Add(pluginExecutorExpiryCheckInterval))
}

func pluginExecutorStatusFromInventory(inventory pluginExecutorInventory, now time.Time) *models.PluginExecutorEnvironmentStatus {
	status := &models.PluginExecutorEnvironmentStatus{
		State: pluginExecutorStateUnknown, Retention: inventory.Capabilities.Retention, ExpiresAt: inventory.ExpiresAt,
	}
	if status.Retention == "" {
		status.Retention = pluginExecutorStateUnknown
	}
	if inventory.ExpiresAt != "" {
		if expiry, err := time.Parse(time.RFC3339Nano, inventory.ExpiresAt); err == nil {
			status.DeadlinePassed = !now.Before(expiry)
		}
	}
	switch inventory.Phase {
	case pluginExecutorPhaseExpired:
		status.State = pluginExecutorPhaseExpired
		status.Reason = pluginExecutorPhaseExpired
	case pluginExecutorPhaseCleanupPending:
		status.State = pluginExecutorPhaseCleanupPending
		status.Reason = pluginExecutorPhaseCleanupPending
	case pluginExecutorPhaseReady, pluginExecutorPhaseProvisioned:
		switch inventory.LastInspectionState {
		case pluginExecutorStateRunning, pluginExecutorStateSuspended:
			status.State = pluginExecutorStateRunning
		case pluginExecutorStateTerminated, pluginExecutorPhaseAbsent:
			status.State = pluginExecutorStateUnavailable
		case pluginExecutorStateUnknown:
			status.State = pluginExecutorStateUnknown
		default:
			status.State = pluginExecutorStateRunning
		}
	case pluginExecutorPhaseAllocating:
		status.State = "starting"
	case pluginExecutorPhaseAbsent:
		status.State = pluginExecutorStateUnavailable
	}
	return status
}

func pluginExecutorInspectionConfirmsExpiry(inspection *pluginsdk.InspectExecutorEnvironmentResponse, deadlinePassed bool) bool {
	state := strings.ToLower(strings.TrimSpace(inspection.GetState()))
	return pluginExecutorSafeReason(inspection.GetReason()) == pluginExecutorPhaseExpired ||
		(deadlinePassed && (state == pluginExecutorPhaseAbsent || state == pluginExecutorStateTerminated))
}

func pluginExecutorSafeReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case pluginExecutorPhaseExpired, "maximum_lifetime_exceeded", "retention_expired":
		return pluginExecutorPhaseExpired
	default:
		return ""
	}
}
