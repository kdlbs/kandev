package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

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
	metadata map[string]interface{},
) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsAutomation() || senderTask == nil ||
		principal.CallerTaskID != senderTask.ID || principal.CallerSessionID != senderSessionID ||
		senderTask.Origin != models.TaskOriginAutomationRun ||
		models.StringFromAny(senderTask.Metadata["trigger_type"]) != string(automation.TriggerTypeScheduled) {
		return
	}
	automationID := models.StringFromAny(senderTask.Metadata["automation_id"])
	triggerID := models.StringFromAny(senderTask.Metadata["trigger_id"])
	if automationID == "" || triggerID == "" || automationID != principal.AutomationID {
		return
	}
	payloadDigest := sha256.Sum256([]byte(prompt))
	identity := automationID + ":" + triggerID
	keyDigest := sha256.Sum256([]byte(identity + "\x00" + hex.EncodeToString(payloadDigest[:])))
	metadata[messagequeue.MetadataRoutineWake] = true
	metadata[messagequeue.MetadataRoutineIdentity] = identity
	metadata[messagequeue.MetadataCoalesceKey] = "routine-wake:" + hex.EncodeToString(keyDigest[:])
}
