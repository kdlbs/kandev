package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/automation"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func applyScheduledRoutineWakeMetadata(
	ctx context.Context,
	prompt string,
	senderTask *models.Task,
	senderSessionID string,
	targetTask *models.Task,
	targetSession *models.TaskSession,
	explicitTargetSession bool,
	metadata map[string]interface{},
) error {
	principal, automationID, ok := scheduledRoutinePrincipal(ctx, senderTask, senderSessionID)
	if !ok {
		return nil
	}
	if err := validateRoutineWakeTarget(principal, senderTask, targetTask, targetSession, explicitTargetSession); err != nil {
		return err
	}

	payloadDigest := sha256.Sum256([]byte(prompt))
	routineType := metadataValueOr(senderTask.Metadata, "routine_type", string(automation.TriggerTypeScheduled))
	routineName := metadataValueOr(senderTask.Metadata, "routine_name",
		metadataValueOr(senderTask.Metadata, "automation_name", automationID))
	policyGeneration := metadataValueOr(senderTask.Metadata, "routine_policy_generation", hex.EncodeToString(payloadDigest[:]))
	scopeGeneration := metadataValueOr(senderTask.Metadata, "routine_scope_generation", targetTask.ID)
	identityPayload, _ := json.Marshal([]string{
		principal.WorkspaceID, routineType, routineName, policyGeneration, scopeGeneration,
	})
	identityDigest := sha256.Sum256(identityPayload)
	identity := "routine:" + hex.EncodeToString(identityDigest[:])
	keyDigest := sha256.Sum256([]byte(identity + "\x00" + hex.EncodeToString(payloadDigest[:])))
	metadata[messagequeue.MetadataRoutineWake] = true
	metadata[messagequeue.MetadataRoutineIdentity] = identity
	metadata[messagequeue.MetadataRoutineWorkspaceID] = principal.WorkspaceID
	metadata[messagequeue.MetadataRoutineType] = routineType
	metadata[messagequeue.MetadataRoutineName] = routineName
	metadata[messagequeue.MetadataRoutinePolicyGeneration] = policyGeneration
	metadata[messagequeue.MetadataRoutineScopeGeneration] = scopeGeneration
	metadata[messagequeue.MetadataRoutineLeaderFencingToken] = metadataValueOr(senderTask.Metadata, "routine_leader_fencing_token", automationID)
	metadata[messagequeue.MetadataRoutineDirtyGeneration] = metadataValueOr(senderTask.Metadata, "routine_dirty_generation", senderTask.ID)
	metadata[messagequeue.MetadataCoalesceKey] = "routine-wake:" + hex.EncodeToString(keyDigest[:])
	return nil
}

func scheduledRoutinePrincipal(
	ctx context.Context,
	senderTask *models.Task,
	senderSessionID string,
) (mcpscope.Principal, string, bool) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsAutomation() || senderTask == nil ||
		principal.CallerTaskID != senderTask.ID || principal.CallerSessionID != senderSessionID ||
		senderTask.Origin != models.TaskOriginAutomationRun ||
		models.StringFromAny(senderTask.Metadata["trigger_type"]) != string(automation.TriggerTypeScheduled) {
		return mcpscope.Principal{}, "", false
	}
	automationID := models.StringFromAny(senderTask.Metadata["automation_id"])
	triggerID := models.StringFromAny(senderTask.Metadata["trigger_id"])
	return principal, automationID, automationID != "" && triggerID != "" && automationID == principal.AutomationID
}

func validateRoutineWakeTarget(
	principal mcpscope.Principal,
	senderTask, targetTask *models.Task,
	targetSession *models.TaskSession,
	explicitTargetSession bool,
) error {
	if targetTask == nil || targetSession == nil || principal.WorkspaceID == "" ||
		senderTask.WorkspaceID != principal.WorkspaceID || targetTask.WorkspaceID != principal.WorkspaceID {
		return fmt.Errorf("routine wake scope denied: sender and target must belong to authenticated workspace %s", principal.WorkspaceID)
	}
	if targetSession.TaskID != targetTask.ID || (explicitTargetSession && !targetSession.IsPrimary) {
		return fmt.Errorf("routine wake scope denied: target session is not the current primary for task %s", targetTask.ID)
	}
	return nil
}

func metadataValueOr(metadata map[string]interface{}, key, fallback string) string {
	if value := models.StringFromAny(metadata[key]); value != "" {
		return value
	}
	return fallback
}
