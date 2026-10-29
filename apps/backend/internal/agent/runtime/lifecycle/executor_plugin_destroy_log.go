package lifecycle

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
)

// The conditions that leave a destroy unconfirmed, in the order they are tested.
const (
	pluginDestroyCallError              = "call_error"
	pluginDestroyErrorResponse          = "error_response"
	pluginDestroyNotConfirmedAbsent     = "not_confirmed_absent"
	pluginDestroyClaimCheckpointMissing = "claim_checkpoint_missing"
)

// pluginDestroyUnconfirmedCondition names the first condition that makes a
// destroy unconfirmed, or "" when the destroy is confirmed.
func pluginDestroyUnconfirmedCondition(response *pluginsdk.DestroyExecutorEnvironmentResponse, callErr error, claimCheckpointPersisted bool) string {
	switch {
	case callErr != nil || response == nil:
		return pluginDestroyCallError
	case response.GetError() != nil:
		return pluginDestroyErrorResponse
	case !response.GetConfirmedAbsent():
		return pluginDestroyNotConfirmedAbsent
	case !claimCheckpointPersisted:
		return pluginDestroyClaimCheckpointMissing
	default:
		return ""
	}
}

// logPluginExecutorDestroy writes the one structured line per destroy: the
// evidence sent, the provider's answer and the condition that left it
// unconfirmed. It carries identifiers and codes only, never a secret value.
func (r *PluginRemoteExecutor) logPluginExecutorDestroy(
	record *models.ExecutorRunning,
	claim *models.TaskEnvironmentRecoveryClaim,
	request *pluginsdk.DestroyExecutorEnvironmentRequest,
	response *pluginsdk.DestroyExecutorEnvironmentResponse,
	callErr error,
	condition string,
) {
	fields := []zap.Field{
		zap.String("execution_id", record.AgentExecutionID),
		zap.String("cleanup_reason", request.GetCleanupReason()),
		zap.String("drain_outcome", request.GetDrainOutcome()),
		zap.Bool("drain_receipt_attached", request.GetDrainReceipt() != nil),
		zap.String("cleanup_claim", claim.OperationID),
		zap.Bool("confirmed_absent", response.GetConfirmedAbsent()),
		zap.String("error_code", response.GetError().GetCode()),
		zap.String("error_message_id", response.GetError().GetMessageId()),
		zap.String("unconfirmed_condition", condition),
	}
	if callErr != nil {
		fields = append(fields, zap.Error(callErr))
	}
	if condition != "" {
		r.logger.Warn("plugin executor destroy", fields...)
		return
	}
	r.logger.Info("plugin executor destroy", fields...)
}
